package provider

import (
	"context"
	"testing"
)

type stubClient struct {
	name string
}

func (s stubClient) Name() string { return s.name }
func (s stubClient) ListModels(context.Context) ([]Model, error) {
	return []Model{{ID: "m1"}}, nil
}
func (s stubClient) ChatStream(context.Context, ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}

func TestRouterGetUnknown(t *testing.T) {
	r := NewRouter()
	if _, err := r.Get("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRouterRegisterAndDefault(t *testing.T) {
	r := NewRouter()
	r.Register("tempai", stubClient{name: "tempai"})
	r.Register("byok", stubClient{name: "byok"})
	r.SetDefault("byok")
	c, err := r.Get("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "byok" {
		t.Fatalf("got %q", c.Name())
	}
	c2, err := r.Get("tempai")
	if err != nil || c2.Name() != "tempai" {
		t.Fatalf("tempai: %v %v", c2, err)
	}
}
