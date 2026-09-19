package core

import (
	"strings"
	"testing"
)

func TestNormalizeChatImages_MaxFour(t *testing.T) {
	in := make([]ChatImagePart, 0, 6)
	for i := 0; i < 6; i++ {
		in = append(in, ChatImagePart{
			Name:      "a.png",
			MediaType: "image/png",
			DataURL:   "data:image/png;base64,QUFB" + string(rune('A'+i)),
		})
	}
	out := normalizeChatImages(in)
	if len(out) != MaxChatImages {
		t.Fatalf("got %d images, want %d", len(out), MaxChatImages)
	}
	if MaxChatImages != 4 {
		t.Fatalf("MaxChatImages=%d want 4", MaxChatImages)
	}
}

func TestNormalizeChatImages_RejectsNonImage(t *testing.T) {
	out := normalizeChatImages([]ChatImagePart{
		{DataURL: "file:///etc/passwd"},
		{DataURL: "data:text/plain;base64,AA=="},
		{DataURL: "data:image/jpeg;base64,/9j/4AAQ"},
		{DataURL: "https://example.com/a.png"},
	})
	if len(out) != 2 {
		t.Fatalf("got %d, want 2 (jpeg data + https)", len(out))
	}
	if !strings.HasPrefix(out[0].DataURL, "data:image/jpeg") {
		t.Fatalf("first=%q", out[0].DataURL)
	}
	if out[0].MediaType != "image/jpeg" {
		t.Fatalf("media_type inferred=%q", out[0].MediaType)
	}
}

func TestNormalizeChatImages_Empty(t *testing.T) {
	if got := normalizeChatImages(nil); got != nil {
		t.Fatalf("nil in → %v", got)
	}
	if got := normalizeChatImages([]ChatImagePart{{DataURL: "  "}}); len(got) != 0 {
		t.Fatalf("blank → %v", got)
	}
}
