package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// cdpClient is a minimal Chrome DevTools Protocol client over WebSocket.
// Used by debug + playwright (Chrome for Testing) engines — not for cookies
// serialization into LLM context.
type cdpClient struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	writeMu sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan cdpResponse
	closed  atomic.Bool
	target  string // page target id
	session string // Target.attachToTarget session id
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *cdpError       `json:"error"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpEnvelope struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// discoverWSDebuggerURL probes http://127.0.0.1:port/json/version for the browser WS URL.
func discoverWSDebuggerURL(ctx context.Context, port int) (string, error) {
	u := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	deadline := time.Now().Add(15 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			last = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			last = fmt.Errorf("cdp version status %d", resp.StatusCode)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var payload struct {
			WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			last = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if payload.WebSocketDebuggerURL == "" {
			last = fmt.Errorf("empty webSocketDebuggerUrl")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return payload.WebSocketDebuggerURL, nil
	}
	if last == nil {
		last = fmt.Errorf("timeout waiting for CDP on port %d", port)
	}
	return "", fmt.Errorf("cdp discover port %d: %w", port, last)
}

// connectCDP dials the browser-level debugger websocket and attaches to a page.
func connectCDP(ctx context.Context, wsURL string) (*cdpClient, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp dial: %w", err)
	}
	c := &cdpClient{
		ws:      conn,
		pending: make(map[int64]chan cdpResponse),
	}
	go c.readLoop()

	// Prefer an existing page; else create one.
	list, err := c.call(ctx, "", "Target.getTargets", map[string]any{})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
		} `json:"targetInfos"`
	}
	_ = json.Unmarshal(list, &targets)
	targetID := ""
	for _, t := range targets.TargetInfos {
		if t.Type == "page" {
			targetID = t.TargetID
			break
		}
	}
	if targetID == "" {
		created, err := c.call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank"})
		if err != nil {
			_ = c.Close()
			return nil, err
		}
		var out struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(created, &out); err != nil || out.TargetID == "" {
			_ = c.Close()
			return nil, fmt.Errorf("cdp createTarget: %v", err)
		}
		targetID = out.TargetID
	}
	c.target = targetID

	attached, err := c.call(ctx, "", "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attached, &sess); err != nil || sess.SessionID == "" {
		_ = c.Close()
		return nil, fmt.Errorf("cdp attachToTarget: %v", err)
	}
	c.session = sess.SessionID

	// Enable domains we need.
	if _, err := c.call(ctx, c.session, "Page.enable", map[string]any{}); err != nil {
		_ = c.Close()
		return nil, err
	}
	if _, err := c.call(ctx, c.session, "Runtime.enable", map[string]any{}); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *cdpClient) readLoop() {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.failAll(err)
			return
		}
		var env cdpEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		if env.ID == 0 {
			continue // event
		}
		c.mu.Lock()
		ch, ok := c.pending[env.ID]
		if ok {
			delete(c.pending, env.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- cdpResponse{ID: env.ID, Result: env.Result, Error: env.Error}
		}
	}
}

func (c *cdpClient) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- cdpResponse{ID: id, Error: &cdpError{Message: err.Error()}}
		delete(c.pending, id)
	}
}

func (c *cdpClient) call(ctx context.Context, sessionID, method string, params map[string]any) (json.RawMessage, error) {
	if c == nil || c.closed.Load() {
		return nil, fmt.Errorf("cdp: closed")
	}
	id := c.nextID.Add(1)
	msg := map[string]any{
		"id":     id,
		"method": method,
	}
	if params == nil {
		params = map[string]any{}
	}
	msg["params"] = params
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	ch := make(chan cdpResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	c.writeMu.Lock()
	err = c.ws.WriteMessage(websocket.TextMessage, raw)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp write %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("cdp %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(60 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp %s: timeout", method)
	}
}

func (c *cdpClient) Navigate(ctx context.Context, pageURL string) error {
	// Use Page.navigate and wait for load event via lifecycle.
	_, err := c.call(ctx, c.session, "Page.navigate", map[string]any{"url": pageURL})
	if err != nil {
		return err
	}
	// Best-effort wait for document ready via Runtime.evaluate poll.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res, err := c.Evaluate(ctx, "document.readyState")
		if err == nil {
			if s, ok := res.Value.(string); ok && (s == "interactive" || s == "complete") {
				return nil
			}
			// some CDP values come as raw JSON strings with quotes already unmarshaled
			if res.Raw == `"complete"` || res.Raw == `"interactive"` {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil // navigated; load wait best-effort
}

func (c *cdpClient) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	result, err := c.call(ctx, c.session, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return EvalResult{}, err
	}
	var out struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return EvalResult{}, err
	}
	if len(out.ExceptionDetails) > 0 && string(out.ExceptionDetails) != "null" {
		return EvalResult{}, fmt.Errorf("evaluate exception: %s", string(out.ExceptionDetails))
	}
	raw := string(out.Result.Value)
	var val any
	if len(out.Result.Value) > 0 && string(out.Result.Value) != "null" {
		_ = json.Unmarshal(out.Result.Value, &val)
	}
	return EvalResult{Value: val, Raw: raw}, nil
}

func (c *cdpClient) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	format := opts.Format
	if format == "" {
		format = "png"
	}
	params := map[string]any{
		"format": format,
	}
	if format == "jpeg" && opts.Quality > 0 {
		params["quality"] = opts.Quality
	}
	if opts.FullPage {
		// Capture beyond viewport via metrics + clip when possible.
		metrics, err := c.call(ctx, c.session, "Page.getLayoutMetrics", map[string]any{})
		if err == nil {
			var m struct {
				CSSContentSize struct {
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"cssContentSize"`
				ContentSize struct {
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"contentSize"`
			}
			_ = json.Unmarshal(metrics, &m)
			w, h := m.CSSContentSize.Width, m.CSSContentSize.Height
			if w == 0 {
				w, h = m.ContentSize.Width, m.ContentSize.Height
			}
			if w > 0 && h > 0 {
				params["clip"] = map[string]any{
					"x": 0, "y": 0, "width": w, "height": h, "scale": 1,
				}
				params["captureBeyondViewport"] = true
			}
		}
	}
	result, err := c.call(ctx, c.session, "Page.captureScreenshot", params)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return nil, err
	}
	if out.Data == "" {
		return nil, fmt.Errorf("cdp screenshot: empty data")
	}
	return base64.StdEncoding.DecodeString(out.Data)
}

func (c *cdpClient) Close() error {
	if c == nil || !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	for id, ch := range c.pending {
		ch <- cdpResponse{ID: id, Error: &cdpError{Message: "closed"}}
		delete(c.pending, id)
	}
	c.mu.Unlock()
	return c.ws.Close()
}

// cdpHTTPURL builds the HTTP debug base for a port.
func cdpHTTPURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// cdpBrowserURL is the user-facing cdp_url field (VAL-BRW-001).
func cdpBrowserURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// parsePortFromURL extracts the port from a browser debug URL if present.
func parsePortFromURL(raw string) int {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	p := u.Port()
	if p == "" {
		return 0
	}
	n, _ := strconv.Atoi(p)
	return n
}

// chromeUserDataDir creates a temp profile dir hint string for isolation.
func sanitizeProfileArg(dir string) string {
	return strings.TrimSpace(dir)
}
