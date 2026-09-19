// Fixture stdio MCP server for host integration tests (VAL-MCP-001/005).
// Built as a helper binary via `go run` / `go test` exec of this package main.
//
// Usage:
//
//	go run ./internal/mcp/testdata/echo_server.go
//
// Speaks Content-Length framed JSON-RPC on stdio with tools:
//   - echo: returns text payload
//   - fs.read: deliberately collides with a built-in-looking name (namespaced by host)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type req struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type resp struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *errObj `json:"error,omitempty"`
}

type errObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	r := bufio.NewReader(os.Stdin)
	for {
		body, err := readFramed(r)
		if err != nil {
			if err == io.EOF {
				return
			}
			return
		}
		var rq req
		if err := json.Unmarshal(body, &rq); err != nil {
			continue
		}
		switch rq.Method {
		case "initialize":
			writeFramed(os.Stdout, resp{
				JSONRPC: "2.0",
				ID:      rq.ID,
				Result: map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]string{"name": "echo-fixture", "version": "0.0.1"},
				},
			})
		case "notifications/initialized":
			// no response
		case "tools/list":
			writeFramed(os.Stdout, resp{
				JSONRPC: "2.0",
				ID:      rq.ID,
				Result: map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo",
							"description": "Echo back the message field",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"message": map[string]any{"type": "string"},
								},
								"required": []string{"message"},
							},
						},
						{
							"name":        "fs.read",
							"description": "Fixture tool that deliberately shares a built-in-looking name",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"path": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(rq.Params, &p)
			text := ""
			switch p.Name {
			case "echo":
				msg, _ := p.Arguments["message"].(string)
				text = "echo:" + msg
			case "fs.read":
				path, _ := p.Arguments["path"].(string)
				text = "mcp-fs-read:" + path
			default:
				writeFramed(os.Stdout, resp{
					JSONRPC: "2.0",
					ID:      rq.ID,
					Error:   &errObj{Code: -32601, Message: "unknown tool " + p.Name},
				})
				continue
			}
			writeFramed(os.Stdout, resp{
				JSONRPC: "2.0",
				ID:      rq.ID,
				Result: map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": text},
					},
				},
			})
		default:
			if rq.ID != nil {
				writeFramed(os.Stdout, resp{
					JSONRPC: "2.0",
					ID:      rq.ID,
					Error:   &errObj{Code: -32601, Message: "method not found: " + rq.Method},
				})
			}
		}
	}
}

func writeFramed(w io.Writer, v any) {
	body, _ := json.Marshal(v)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	_, _ = w.Write(body)
}

func readFramed(r *bufio.Reader) ([]byte, error) {
	b, err := r.Peek(1)
	if err != nil {
		return nil, err
	}
	if b[0] == '{' {
		line, err := r.ReadBytes('\n')
		return bytes.TrimSpace(line), err
	}
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			parts := strings.SplitN(line, ":", 2)
			n, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
