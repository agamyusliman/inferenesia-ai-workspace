package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// P9-prov: AddProvider with api_format=anthropic persists the field, and the
// settings view exposes it on the profile row.
func TestAddProviderPersistsAPIFormatAnthropic(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.AddProvider(AddProviderRequest{
		ID:        "my-claude",
		Name:      "My Claude",
		Type:      "anthropic",
		APIFormat: "anthropic",
		BaseURL:   "https://api.anthropic.com",
		APIKey:    "anth-test-key-not-real",
	})
	if err != nil {
		t.Fatal(err)
	}

	var pv *ProfileView
	for i := range view.Profiles {
		if view.Profiles[i].ID == "my-claude" {
			pv = &view.Profiles[i]
			break
		}
	}
	if pv == nil {
		t.Fatalf("my-claude not in profiles")
	}
	if pv.APIFormat != config.APIFormatAnthropic {
		t.Fatalf("api_format=%q want anthropic", pv.APIFormat)
	}
	if pv.Type != "anthropic" {
		t.Fatalf("type=%q want anthropic", pv.Type)
	}

	// Config file must contain api_format: anthropic.
	data, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "api_format: anthropic") {
		t.Fatalf("config missing api_format:\n%s", data)
	}
}

// P9-prov: AddProvider with an openai_compatible type + anthropic wire format
// (LiteLLM Claude host) persists BOTH type and api_format independently.
func TestAddProviderOpenAICompatibleWithAnthropicWire(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.AddProvider(AddProviderRequest{
		ID:        "litellm-claude",
		Name:      "LiteLLM Claude",
		Type:      "openai_compatible",
		APIFormat: "anthropic",
		BaseURL:   "http://127.0.0.1:4198/v1",
		APIKey:    "litellm-key",
	})
	if err != nil {
		t.Fatal(err)
	}

	var pv *ProfileView
	for i := range view.Profiles {
		if view.Profiles[i].ID == "litellm-claude" {
			pv = &view.Profiles[i]
			break
		}
	}
	if pv == nil {
		t.Fatalf("litellm-claude not in profiles")
	}
	if pv.Type != "openai_compatible" {
		t.Fatalf("type=%q want openai_compatible", pv.Type)
	}
	if pv.APIFormat != config.APIFormatAnthropic {
		t.Fatalf("api_format=%q want anthropic (wire override)", pv.APIFormat)
	}

	data, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "type: openai_compatible") {
		t.Fatalf("config missing type:\n%s", data)
	}
	if !strings.Contains(string(data), "api_format: anthropic") {
		t.Fatalf("config missing api_format:\n%s", data)
	}
}

// P9-prov: AddProvider with empty type + api_format=anthropic coerces type to
// anthropic so config stays self-consistent.
func TestAddProviderEmptyTypeWithAnthropicFormatCoercesType(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddProvider(AddProviderRequest{
		ID:        "implicit-claude",
		Name:      "Implicit Claude",
		APIFormat: "anthropic",
		APIKey:    "k",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "type: anthropic") {
		t.Fatalf("expected type coerced to anthropic:\n%s", data)
	}
	if !strings.Contains(string(data), "api_format: anthropic") {
		t.Fatalf("expected api_format persisted:\n%s", data)
	}
}

// P9-prov: AddProvider with empty type + empty api_format defaults to openai.
func TestAddProviderEmptyTypeAndFormatDefaultsOpenAI(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.AddProvider(AddProviderRequest{
		ID:      "plain-byok",
		Name:    "Plain",
		BaseURL: "http://127.0.0.1:4197/v1",
		APIKey:  "k",
	})
	if err != nil {
		t.Fatal(err)
	}

	var pv *ProfileView
	for i := range view.Profiles {
		if view.Profiles[i].ID == "plain-byok" {
			pv = &view.Profiles[i]
			break
		}
	}
	if pv == nil {
		t.Fatalf("plain-byok not in profiles")
	}
	if pv.Type != "openai_compatible" {
		t.Fatalf("type=%q want openai_compatible", pv.Type)
	}
	if pv.APIFormat != config.APIFormatOpenAI {
		t.Fatalf("api_format=%q want openai", pv.APIFormat)
	}
}
