package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapImageRefs_MaxFour(t *testing.T) {
	in := make([]ImageRef, 0, 6)
	for i := 0; i < 6; i++ {
		in = append(in, ImageRef{DataURL: "data:image/png;base64,AA" + string(rune('A'+i))})
	}
	out := CapImageRefs(in)
	if len(out) != MaxImageGenRefs {
		t.Fatalf("got %d want %d", len(out), MaxImageGenRefs)
	}
	// Reject non-image
	bad := CapImageRefs([]ImageRef{{DataURL: "file:///etc/passwd"}, {DataURL: "data:text/plain;base64,AA"}})
	if len(bad) != 0 {
		t.Fatalf("bad refs should drop: %v", bad)
	}
}

// Images API is tried FIRST (Console DPoP path via Cloud gateway). A gateway
// that only implements /responses is reached on fallback.
func TestGenerateImage_ResponsesAPI_TempAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/images/generations" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path=%s want /v1/responses", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing auth")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		tools, _ := body["tools"].([]any)
		if len(tools) == 0 {
			t.Fatalf("missing tools: %v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"type":   "image_generation_call",
					"result": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
				},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      "tempai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key-not-secret",
	})
	res, err := c.GenerateImage(context.Background(), ImageGenRequest{
		Prompt: "a red cube",
		Model:  "grok-4.5",
		N:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || res.Images[0].B64JSON == "" {
		t.Fatalf("res=%+v", res)
	}
	md := FormatGeneratedImagesMarkdown("a red cube", res)
	if !strings.Contains(md, "data:image/png;base64,") {
		t.Fatalf("markdown=%q", md)
	}
}

// Images API unavailable → falls back to /responses + image_generation tool.
func TestGenerateImage_FallbackImagesAPI(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		if r.URL.Path == "/v1/images/generations" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/v1/responses" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]any{
					{"type": "image_generation_call", "result": "QUJD"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      "tempai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key-not-secret",
	})
	res, err := c.GenerateImage(context.Background(), ImageGenRequest{
		Prompt: "a red cube",
		N:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || res.Images[0].B64JSON != "QUJD" {
		t.Fatalf("res=%+v", res)
	}
	if len(hits) != 2 || hits[0] != "/v1/images/generations" || hits[1] != "/v1/responses" {
		t.Fatalf("expected images then responses, hits=%v", hits)
	}
}

func TestGenerateImage_Unsupported404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      "tempai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key-not-secret",
	})
	_, err := c.GenerateImage(context.Background(), ImageGenRequest{Prompt: "cat"})
	if !IsImageGenUnsupported(err) {
		t.Fatalf("want ImageGenUnsupportedError, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Image generation is not supported") {
		t.Fatalf("msg=%q", msg)
	}
}

// Images API answers first, so /responses is never reached.
func TestGenerateImage_ResponsesThenImagesOrder(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		if r.URL.Path == "/v1/images/generations" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"b64_json": "QUJDREVGR0g="}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      "tempai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key-not-secret",
	})
	res, err := c.GenerateImage(context.Background(), ImageGenRequest{Prompt: "dog"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || res.Images[0].B64JSON != "QUJDREVGR0g=" {
		t.Fatalf("res=%+v", res)
	}
	if len(hits) != 1 || hits[0] != "/v1/images/generations" {
		t.Fatalf("hits=%v", hits)
	}
}

func TestImageGenUnsupported_AnthropicMessage(t *testing.T) {
	err := &ImageGenUnsupportedError{Profile: "anthropic", Reason: "anthropic adapter has no image gen path"}
	if !IsImageGenUnsupported(err) {
		t.Fatal("expected unsupported")
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Fatalf("msg=%q", err.Error())
	}
}

func TestGenerateImage_HTMLBodyUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>portal</body></html>"))
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      "tempai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key-not-secret",
	})
	_, err := c.GenerateImage(context.Background(), ImageGenRequest{Prompt: "x"})
	if !IsImageGenUnsupported(err) {
		t.Fatalf("want unsupported, got %v", err)
	}
}
