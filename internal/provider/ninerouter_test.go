package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// VAL-KIT-009: YAML openai_compatible profile can point at 9Router base_url.
func TestNineRouterYAMLProfileRegistration(t *testing.T) {
	var sawHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHost = r.Host
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "m1"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "YAML-NR"}},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(EnvAPIKey, "temp-key-not-used")
	t.Setenv(EnvNineRouterURL, "") // YAML-only path
	t.Setenv(EnvBYOKBaseURL, "")

	file := config.FileConfig{
		DefaultProvider: "my-9r",
		Providers: []config.ProviderProfile{
			{
				ID:      "my-9r",
				Type:    "openai_compatible",
				Name:    "Local 9Router",
				BaseURL: srv.URL + "/v1",
				APIKey:  "yaml-nr-secret",
			},
		},
	}
	r := Bootstrap(BootstrapOptions{File: file, PreferredID: "my-9r"})
	if r.DefaultID() != "my-9r" {
		t.Fatalf("default=%q", r.DefaultID())
	}
	client, err := r.GetOpenAI("my-9r")
	if err != nil {
		t.Fatal(err)
	}
	text, err := client.ChatComplete(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "YAML-NR" {
		t.Fatalf("text=%q", text)
	}
	if sawHost == "" {
		t.Fatal("request never hit mock 9Router")
	}
}

func TestNormalizeNineRouterBaseURL(t *testing.T) {
	if got := normalizeNineRouterBaseURL("http://127.0.0.1:20128"); got != "http://127.0.0.1:20128/v1" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeNineRouterBaseURL("http://127.0.0.1:20128/v1"); got != "http://127.0.0.1:20128/v1" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeNineRouterBaseURL("http://127.0.0.1:20128/v1/"); got != "http://127.0.0.1:20128/v1" {
		// TrimRight removes trailing slash first → ends with /v1.
		if got != "http://127.0.0.1:20128/v1" {
			t.Fatalf("got %q", got)
		}
	}
}

// No NINEROUTER_URL → profile absent (optional pack).
func TestNineRouterAbsentWhenEnvUnset(t *testing.T) {
	t.Setenv(EnvNineRouterURL, "")
	t.Setenv(EnvNineRouterKey, "")
	t.Setenv(EnvBYOKBaseURL, "")
	t.Setenv(EnvAPIKey, "k")
	r := Bootstrap(BootstrapOptions{})
	if _, err := r.Get(ProfileNineRouter); err == nil {
		t.Fatal("ninerouter profile must not register without NINEROUTER_URL")
	}
}
