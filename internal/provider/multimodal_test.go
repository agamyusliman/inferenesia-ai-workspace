package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToWireMessages_MultimodalImageParts(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "sys"},
		{
			Role:    RoleUser,
			Content: "look at this",
			Parts: []ContentPart{
				{Type: "text", Text: "look at this"},
				{
					Type:      "image_url",
					ImageURL:  "data:image/png;base64,QUJD",
					MediaType: "image/png",
				},
			},
		},
	}
	wire := toWireMessages(msgs)
	if len(wire) != 2 {
		t.Fatalf("len=%d", len(wire))
	}
	// User content must be an array of parts, not a bare string.
	raw, err := json.Marshal(wire[1].Content)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"type":"image_url"`) {
		t.Fatalf("missing image_url part: %s", s)
	}
	if !strings.Contains(s, `"url":"data:image/png;base64,QUJD"`) {
		t.Fatalf("missing data url: %s", s)
	}
	if !strings.Contains(s, `"type":"text"`) {
		t.Fatalf("missing text part: %s", s)
	}
	// System stays string content.
	if _, ok := wire[0].Content.(string); !ok {
		t.Fatalf("system content should be string, got %T", wire[0].Content)
	}
}

func TestToWireMessages_TextOnlyUnchanged(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hello"},
	}
	wire := toWireMessages(msgs)
	if s, ok := wire[0].Content.(string); !ok || s != "hello" {
		t.Fatalf("content=%v (%T)", wire[0].Content, wire[0].Content)
	}
}

func TestAnthropicImageBlock_DataURL(t *testing.T) {
	blk := anthropicImageBlock(ContentPart{
		Type:      "image_url",
		ImageURL:  "data:image/png;base64,QUJDRA==",
		MediaType: "image/png",
	})
	if blk == nil || blk.Type != "image" || blk.Source == nil {
		t.Fatalf("blk=%+v", blk)
	}
	if blk.Source.Type != "base64" || blk.Source.Data != "QUJDRA==" {
		t.Fatalf("source=%+v", blk.Source)
	}
	if blk.Source.MediaType != "image/png" {
		t.Fatalf("media=%s", blk.Source.MediaType)
	}
}

func TestToAnthropicMessages_Multimodal(t *testing.T) {
	_, out := toAnthropicMessages([]Message{
		{
			Role:    RoleUser,
			Content: "see",
			Parts: []ContentPart{
				{Type: "text", Text: "see"},
				{Type: "image_url", ImageURL: "data:image/jpeg;base64,/9j/4"},
			},
		},
	})
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	blocks, ok := out[0].Content.([]anthropicContentBlock)
	if !ok || len(blocks) != 2 {
		t.Fatalf("content=%T %#v", out[0].Content, out[0].Content)
	}
	if blocks[0].Type != "text" || blocks[1].Type != "image" {
		t.Fatalf("blocks=%+v", blocks)
	}
}
