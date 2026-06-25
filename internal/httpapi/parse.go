package httpapi

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/zhfeng/llm-gateway/internal/protocol"
)

func parseStop(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case string:
		if s != "" {
			return []string{s}
		}
		return nil
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func parseOpenAIMessages(raw []json.RawMessage) []protocol.Message {
	messages := make([]protocol.Message, 0, len(raw))
	for _, msg := range raw {
		var base struct {
			Role       string            `json:"role"`
			Name       string            `json:"name"`
			Content    json.RawMessage   `json:"content"`
			ToolCalls  []json.RawMessage `json:"tool_calls"`
			ToolCallID string            `json:"tool_call_id"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			continue
		}
		switch base.Role {
		case "system":
			messages = append(messages, protocol.Message{Role: protocol.RoleSystem, Content: parseOpenAIContent(base.Content)})
		case "user":
			messages = append(messages, protocol.Message{Role: protocol.RoleUser, Content: parseOpenAIContent(base.Content), Name: base.Name})
		case "assistant":
			pmsg := protocol.Message{Role: protocol.RoleAssistant, Content: parseOpenAIContent(base.Content)}
			if len(base.ToolCalls) > 0 {
				pmsg.ToolCalls = parseOpenAIToolCalls(base.ToolCalls)
			}
			messages = append(messages, pmsg)
		case "tool":
			messages = append(messages, protocol.Message{Role: protocol.RoleTool, Content: parseOpenAIContent(base.Content), ToolCallID: base.ToolCallID, Name: base.Name})
		}
	}
	return messages
}

func parseOpenAIContent(raw json.RawMessage) []protocol.ContentBlock {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return protocol.TextContent(text)
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	var blocks []protocol.ContentBlock
	for _, part := range parts {
		var base struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(part, &base) != nil {
			continue
		}
		switch base.Type {
		case "text":
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(part, &p) == nil && p.Text != "" {
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentText, Text: p.Text})
			}
		case "image_url":
			var p struct {
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			}
			if json.Unmarshal(part, &p) == nil && p.ImageURL.URL != "" {
				mediaType, data := decodeDataURI(p.ImageURL.URL)
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentImage, MediaType: mediaType, Data: data, URL: p.ImageURL.URL})
			}
		case "tool_use":
			var p struct {
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(part, &p) == nil {
				input := p.Input
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentToolUse, ID: p.ID, Name: p.Name, Input: input})
			}
		case "tool_result":
			var p struct {
				ToolUseID string `json:"tool_use_id"`
				Content   any    `json:"content"`
				IsError   bool   `json:"is_error"`
			}
			if json.Unmarshal(part, &p) == nil {
				text := contentToString(p.Content)
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentToolResult, Text: text, ToolUseID: p.ToolUseID, IsError: p.IsError})
			}
		}
	}
	return blocks
}

func parseOpenAIToolCalls(raw []json.RawMessage) []protocol.ToolCall {
	calls := make([]protocol.ToolCall, 0, len(raw))
	for _, r := range raw {
		var base struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		}
		if json.Unmarshal(r, &base) == nil {
			calls = append(calls, protocol.ToolCall{ID: base.ID, Name: base.Function.Name, Input: base.Function.Arguments})
		}
	}
	return calls
}

func parseAnthropicMessages(raw []json.RawMessage) []protocol.Message {
	messages := make([]protocol.Message, 0, len(raw))
	for _, msg := range raw {
		var base struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			continue
		}
		pmsg := protocol.Message{Role: base.Role}
		pmsg.Content = parseAnthropicContent(base.Content)
		messages = append(messages, pmsg)
	}
	return messages
}

func parseAnthropicContent(raw json.RawMessage) []protocol.ContentBlock {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return protocol.TextContent(text)
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	var blocks []protocol.ContentBlock
	for _, part := range parts {
		var base struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(part, &base) != nil {
			continue
		}
		switch base.Type {
		case "text":
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(part, &p) == nil && p.Text != "" {
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentText, Text: p.Text})
			}
		case "image":
			var p struct {
				Source struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
					URL       string `json:"url"`
				} `json:"source"`
			}
			if json.Unmarshal(part, &p) == nil {
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentImage, MediaType: p.Source.MediaType, Data: p.Source.Data, URL: p.Source.URL})
			}
		case "tool_use":
			var p struct {
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(part, &p) == nil {
				input := p.Input
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentToolUse, ID: p.ID, Name: p.Name, Input: input})
			}
		case "tool_result":
			var p struct {
				ToolUseID string          `json:"tool_use_id"`
				Content   json.RawMessage `json:"content"`
				IsError   bool            `json:"is_error"`
			}
			if json.Unmarshal(part, &p) == nil {
				block := protocol.ContentBlock{Type: protocol.ContentToolResult, ToolUseID: p.ToolUseID, IsError: p.IsError}
				trimmed := bytes.TrimSpace(p.Content)
				if len(trimmed) > 0 {
					switch trimmed[0] {
					case '"':
						var s string
						if json.Unmarshal(trimmed, &s) == nil {
							block.Text = s
						}
					case '[', '{':
						// Preserve structured tool_result payload (e.g. multi-block
						// text + inline images) verbatim so downstream emitters can
						// forward it without flattening to a plain string. trimmed
						// already aliases the json.RawMessage buffer owned by the
						// outer Unmarshal, so no extra copy is needed — base64
						// screenshot payloads can be large and copying doubles
						// memory.
						block.Content = trimmed
					default:
						// JSON primitive (null/true/false/number). Out of spec for
						// Anthropic tool_result.content, but the previous
						// contentToString path stringified the JSON literal rather
						// than dropping it; mirror that so upstream still sees a
						// well-formed string payload instead of an empty content.
						block.Text = string(trimmed)
					}
				}
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}

func parseTools(raw []json.RawMessage) []protocol.Tool {
	tools := make([]protocol.Tool, 0, len(raw))
	for _, r := range raw {
		var base struct {
			Type     string `json:"type"`
			Name     string `json:"name"`
			Function *struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
			InputSchema json.RawMessage `json:"input_schema"`
		}
		if json.Unmarshal(r, &base) != nil {
			continue
		}
		t := protocol.Tool{}
		switch base.Type {
		case "function":
			if base.Function != nil {
				t.Name = base.Function.Name
				t.Description = base.Function.Description
				t.InputSchema = base.Function.Parameters
			}
		default:
			if base.Name != "" {
				t.Name = base.Name
			} else if base.Function != nil {
				t.Name = base.Function.Name
				t.Description = base.Function.Description
				t.InputSchema = base.Function.Parameters
			}
		}
		if base.InputSchema != nil {
			t.InputSchema = base.InputSchema
		}
		if t.Name != "" {
			tools = append(tools, t)
		}
	}
	return tools
}

func extractSystem(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []any:
		var out string
		for _, item := range s {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if typ, _ := m["type"].(string); typ == "text" {
				if text, ok := m["text"].(string); ok {
					out += text
				}
			}
		}
		return out
	}
	return ""
}

func contentToString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []any:
		var out string
		for _, item := range s {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := m["text"].(string); ok {
				out += text
			}
		}
		return out
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func decodeDataURI(uri string) (mediaType, data string) {
	before, after, ok := strings.Cut(uri, ",")
	if !ok {
		return "", uri
	}
	media := strings.TrimPrefix(before, "data:")
	if idx := strings.Index(media, ";"); idx >= 0 {
		media = media[:idx]
	}
	return media, after
}
