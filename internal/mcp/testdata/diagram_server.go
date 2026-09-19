// Fixture stdio MCP server for cross-area diagram save tests (VAL-CROSS-008).
// Built as a helper binary via `go build` from package tests.
//
// Usage:
//
//	go build -o <out> ./internal/mcp/testdata/diagram_server.go
//
// Speaks Content-Length framed JSON-RPC on stdio with tools:
//   - get_diagram: returns a mermaid diagram source string (the kind arg selects
//     among a small set of fixtures; default "flowchart"). The returned text is
//     what the agent then hands to create_diagram to save under docs/diagrams/.
//
// This fixture is intentionally dependency-free and deterministic so the
// cross-area integration test (MCP host -> create_diagram -> WriteGateway) does
// not depend on a live LLM or network.
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
	JSONRPC string  `json:"jsonrpc"`
	ID      any     `json:"id,omitempty"`
	Result  any     `json:"result,omitempty"`
	Error   *errObj `json:"error,omitempty"`
}

type errObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mermaidFixtures holds deterministic diagram sources returned by get_diagram.
// Each is valid Mermaid accepted by internal/tools/diagram.Parse.
var mermaidFixtures = map[string]string{
	"flowchart": "flowchart TD\n    A[Start] --> B[Process]\n    B --> C[End]",
	"er":        "erDiagram\n    USER ||--o{ POST : has\n    POST ||--o{ COMMENT : has",
	"sequence":  "sequenceDiagram\n    participant U as User\n    participant S as Server\n    U->>S: request\n    S-->>U: response",
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
					"serverInfo":      map[string]string{"name": "diagram-fixture", "version": "0.0.1"},
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
							"name":        "get_diagram",
							"description": "Return a mermaid diagram source string for the requested kind (flowchart, er, sequence). The caller saves the result via create_diagram.",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"kind": map[string]any{
										"type":        "string",
										"description": "Diagram kind to return: flowchart (default), er, or sequence",
										"enum":        []string{"flowchart", "er", "sequence"},
									},
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
			switch p.Name {
			case "get_diagram":
				kind, _ := p.Arguments["kind"].(string)
				kind = strings.TrimSpace(kind)
				if kind == "" {
					kind = "flowchart"
				}
				src, ok := mermaidFixtures[kind]
				if !ok {
					writeFramed(os.Stdout, resp{
						JSONRPC: "2.0",
						ID:      rq.ID,
						Error:   &errObj{Code: -32602, Message: "unknown diagram kind " + kind},
					})
					continue
				}
				writeFramed(os.Stdout, resp{
					JSONRPC: "2.0",
					ID:      rq.ID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": src},
						},
					},
				})
			default:
				writeFramed(os.Stdout, resp{
					JSONRPC: "2.0",
					ID:      rq.ID,
					Error:   &errObj{Code: -32601, Message: "unknown tool " + p.Name},
				})
			}
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
