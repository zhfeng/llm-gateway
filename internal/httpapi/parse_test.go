package httpapi

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/zhfeng/llm-gateway/internal/protocol"
)

func TestParseAnthropicContentToolResultString(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"plain string"}]`)
	blocks := parseAnthropicContent(raw)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Type != protocol.ContentToolResult || b.ToolUseID != "call_1" {
		t.Fatalf("unexpected block: %+v", b)
	}
	if b.Text != "plain string" {
		t.Fatalf("text = %q, want %q", b.Text, "plain string")
	}
	if len(b.Content) != 0 {
		t.Fatalf("expected empty Content, got %s", b.Content)
	}
}

func TestParseAnthropicContentToolResultArrayPreserved(t *testing.T) {
	inner := `[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]`
	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_2","content":` + inner + `}]`)
	blocks := parseAnthropicContent(raw)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Type != protocol.ContentToolResult || b.ToolUseID != "call_2" {
		t.Fatalf("unexpected block: %+v", b)
	}
	if b.Text != "" {
		t.Fatalf("expected empty Text for structured payload, got %q", b.Text)
	}
	if len(b.Content) == 0 {
		t.Fatalf("expected raw Content to be preserved")
	}
	// Round-trip: Content should be a JSON array equal to inner.
	var got []json.RawMessage
	if err := json.Unmarshal(b.Content, &got); err != nil {
		t.Fatalf("Content is not valid JSON array: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sub-blocks, got %d", len(got))
	}
	if !bytes.Contains(b.Content, []byte(`"media_type":"image/png"`)) || !bytes.Contains(b.Content, []byte(`"data":"AAAA"`)) {
		t.Fatalf("image source not preserved: %s", b.Content)
	}
	if !bytes.Contains(b.Content, []byte(`"text":"hello"`)) {
		t.Fatalf("text block not preserved: %s", b.Content)
	}
}

func TestParseAnthropicContentToolResultErrorFlag(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_3","content":"boom","is_error":true}]`)
	blocks := parseAnthropicContent(raw)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if !blocks[0].IsError || blocks[0].Text != "boom" {
		t.Fatalf("unexpected block: %+v", blocks[0])
	}
}
