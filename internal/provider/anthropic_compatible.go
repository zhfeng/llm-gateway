package provider

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/zhfeng/llm-gateway/internal/protocol"
	"github.com/zhfeng/llm-gateway/internal/stream"
)

type anthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []anthropicMessage `json:"messages"`
	System        any                `json:"system,omitempty"`
	MaxTokens     int                `json:"max_tokens"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content []anthropicContentPart `json:"content"`
}

type anthropicContentPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Source    any             `json:"source,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   any             `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	ID           string                 `json:"id"`
	Model        string                 `json:"model"`
	Role         string                 `json:"role"`
	Content      []anthropicContentPart `json:"content"`
	StopReason   string                 `json:"stop_reason"`
	StopSequence string                 `json:"stop_sequence"`
	Usage        struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreation            struct {
			Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
			Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
		} `json:"cache_creation"`
	} `json:"usage"`
}

func (p *HTTPProvider) completeAnthropic(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	payload := toAnthropicRequest(req, false)
	if p.logMessages {
		logJSON("provider request body", "provider", p.name, "protocol", "anthropic", "payload", payload)
	}
	hreq, err := p.newJSONRequest(ctx, http.MethodPost, p.endpoint("messages"), payload)
	if err != nil {
		return nil, err
	}
	resp, err := p.do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var decoded anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if p.logMessages {
		logJSON("provider response body", "provider", p.name, "protocol", "anthropic", "payload", decoded)
	}
	return fromAnthropicResponse(decoded), nil
}

func (p *HTTPProvider) streamAnthropic(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	payload := toAnthropicRequest(req, true)
	if p.logMessages {
		logJSON("provider request body", "provider", p.name, "protocol", "anthropic", "stream", true, "payload", payload)
	}
	hreq, err := p.newJSONRequest(ctx, http.MethodPost, p.endpoint("messages"), payload)
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Accept", "text/event-stream")
	resp, err := p.do(hreq)
	if err != nil {
		return nil, err
	}
	ch := make(chan protocol.StreamEvent)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		acc := &protocol.Response{Model: req.Model, Role: protocol.RoleAssistant}
		var currentTool *protocol.ToolCall
		send := func(event protocol.StreamEvent) bool {
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}
		err := stream.ReadEvents(ctx, resp.Body, func(ev stream.Event) error {
			if p.logMessages {
				slog.Info("provider stream event", "provider", p.name, "protocol", "anthropic", "event", ev.Name, "data", string(ev.Data))
			}
			switch ev.Name {
			case "message_start":
				var payload struct {
					Message anthropicResponse `json:"message"`
				}
				if err := json.Unmarshal(ev.Data, &payload); err != nil {
					return err
				}
				acc.ID = payload.Message.ID
				acc.Model = payload.Message.Model
				acc.Usage.InputTokens = payload.Message.Usage.InputTokens
				acc.Usage.CacheCreationInputTokens = payload.Message.Usage.CacheCreationInputTokens
				acc.Usage.CacheCreation5mInputTokens = payload.Message.Usage.CacheCreation.Ephemeral5mInputTokens
				acc.Usage.CacheCreation1hInputTokens = payload.Message.Usage.CacheCreation.Ephemeral1hInputTokens
				acc.Usage.CacheReadInputTokens = payload.Message.Usage.CacheReadInputTokens
				if acc.Usage.CacheCreationInputTokens == 0 && (acc.Usage.CacheCreation5mInputTokens > 0 || acc.Usage.CacheCreation1hInputTokens > 0) {
					acc.Usage.CacheCreationInputTokens = acc.Usage.CacheCreation5mInputTokens + acc.Usage.CacheCreation1hInputTokens
				}
			case "content_block_start":
				var payload struct {
					ContentBlock anthropicContentPart `json:"content_block"`
				}
				if err := json.Unmarshal(ev.Data, &payload); err != nil {
					return err
				}
				if payload.ContentBlock.Type == "tool_use" {
					currentTool = &protocol.ToolCall{ID: payload.ContentBlock.ID, Name: payload.ContentBlock.Name}
				}
			case "content_block_delta":
				var payload struct {
					Delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						Thinking    string `json:"thinking"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
				}
				if err := json.Unmarshal(ev.Data, &payload); err != nil {
					return err
				}
				switch payload.Delta.Type {
				case "text_delta":
					acc.Content = appendText(acc.Content, payload.Delta.Text)
					if !send(protocol.StreamEvent{Type: protocol.StreamTextDelta, Text: payload.Delta.Text}) {
						return ctx.Err()
					}
				case "thinking_delta":
					if !send(protocol.StreamEvent{Type: protocol.StreamThinking, Text: payload.Delta.Thinking}) {
						return ctx.Err()
					}
				case "signature_delta":
					// The signature itself is an encrypted attestation and is
					// not user-readable; we surface a newline so consumers
					// receive a separator between thinking blocks (mirrors
					// new-api's behavior for reasoning_content).
					if !send(protocol.StreamEvent{Type: protocol.StreamThinking, Text: "\n"}) {
						return ctx.Err()
					}
				case "input_json_delta":
					if currentTool != nil {
						currentTool.Input = append(currentTool.Input, payload.Delta.PartialJSON...)
						if !send(protocol.StreamEvent{Type: protocol.StreamToolCall, ToolCallID: currentTool.ID, ToolName: currentTool.Name, ToolInput: payload.Delta.PartialJSON}) {
							return ctx.Err()
						}
					}
				}
			case "content_block_stop":
				if currentTool != nil {
					if len(currentTool.Input) == 0 {
						currentTool.Input = json.RawMessage(`{}`)
					}
					acc.ToolCalls = append(acc.ToolCalls, *currentTool)
					currentTool = nil
				}
			case "message_delta":
				var payload struct {
					Delta struct {
						StopReason string `json:"stop_reason"`
					} `json:"delta"`
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						OutputTokens             int `json:"output_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
						CacheCreation            struct {
							Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
							Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
						} `json:"cache_creation"`
					} `json:"usage"`
				}
				if err := json.Unmarshal(ev.Data, &payload); err != nil {
					return err
				}
				acc.StopReason = protocol.MapAnthropicStopReason(payload.Delta.StopReason)
				acc.Usage.OutputTokens = payload.Usage.OutputTokens
				if payload.Usage.InputTokens > 0 {
					acc.Usage.InputTokens = payload.Usage.InputTokens
				}
				if payload.Usage.CacheCreationInputTokens > 0 {
					acc.Usage.CacheCreationInputTokens = payload.Usage.CacheCreationInputTokens
				}
				if payload.Usage.CacheCreation.Ephemeral5mInputTokens > 0 {
					acc.Usage.CacheCreation5mInputTokens = payload.Usage.CacheCreation.Ephemeral5mInputTokens
				}
				if payload.Usage.CacheCreation.Ephemeral1hInputTokens > 0 {
					acc.Usage.CacheCreation1hInputTokens = payload.Usage.CacheCreation.Ephemeral1hInputTokens
				}
				if payload.Usage.CacheReadInputTokens > 0 {
					acc.Usage.CacheReadInputTokens = payload.Usage.CacheReadInputTokens
				}
				if acc.Usage.CacheCreationInputTokens == 0 && (acc.Usage.CacheCreation5mInputTokens > 0 || acc.Usage.CacheCreation1hInputTokens > 0) {
					acc.Usage.CacheCreationInputTokens = acc.Usage.CacheCreation5mInputTokens + acc.Usage.CacheCreation1hInputTokens
				}
			case "message_stop":
				return nil
			case "error":
				return p.providerError(http.StatusBadGateway, ev.Data)
			}
			return nil
		})
		if err != nil && err != io.EOF && ctx.Err() == nil {
			send(protocol.StreamEvent{Type: protocol.StreamError, Err: err})
			return
		}
		if acc.StopReason == "" && len(acc.ToolCalls) > 0 {
			acc.StopReason = protocol.StopToolUse
		}
		if acc.StopReason == "" {
			acc.StopReason = protocol.StopEndTurn
		}
		send(protocol.StreamEvent{Type: protocol.StreamMessageStop, Response: acc})
	}()
	return ch, nil
}

func toAnthropicRequest(req *protocol.Request, streaming bool) anthropicRequest {
	messages := make([]anthropicMessage, 0, len(req.Messages))
	system := req.System
	for _, msg := range req.Messages {
		if msg.Role == protocol.RoleSystem {
			text := protocol.ContentTextValue(msg.Content)
			if text != "" {
				if system != "" {
					system += "\n"
				}
				system += text
			}
			continue
		}
		messages = append(messages, toAnthropicMessage(msg))
	}
	messages = mergeAnthropicMessages(messages)
	messages = ensureFirstMessageUser(messages)
	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, anthropicTool{Name: tool.Name, Description: tool.Description, InputSchema: schema})
	}
	out := anthropicRequest{
		Model:         req.ProviderModel,
		Messages:      messages,
		System:        system,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.StopSequences,
		Stream:        streaming,
		Tools:         tools,
		ToolChoice:    req.ToolChoice,
	}
	if out.MaxTokens == 0 {
		out.MaxTokens = 1024
	}
	return out
}

func toAnthropicMessage(msg protocol.Message) anthropicMessage {
	role := msg.Role
	if role == protocol.RoleTool {
		part := anthropicContentPart{Type: "tool_result", ToolUseID: msg.ToolCallID}
		if selected := selectToolResultBlock(msg.Content, msg.ToolCallID); selected != nil {
			// Propagate is_error from the selected block so error tool_results
			// aren't silently downgraded into successful ones on the wire.
			part.IsError = selected.IsError
			switch {
			case len(selected.Content) > 0:
				part.Content = selected.Content
			case selected.Text != "":
				// Tool-result block carrying only plain text (no structured
				// payload): forward that text directly so it isn't dropped by
				// ContentTextValue, which only concatenates ContentText blocks.
				part.Content = selected.Text
			default:
				part.Content = protocol.ContentTextValue(msg.Content)
			}
		} else {
			part.Content = protocol.ContentTextValue(msg.Content)
		}
		return anthropicMessage{Role: protocol.RoleUser, Content: []anthropicContentPart{part}}
	}
	parts := make([]anthropicContentPart, 0, len(msg.Content)+len(msg.ToolCalls))
	for _, block := range msg.Content {
		switch block.Type {
		case protocol.ContentText:
			parts = append(parts, anthropicContentPart{Type: "text", Text: block.Text})
		case protocol.ContentImage:
			source := map[string]any{"type": "base64", "media_type": block.MediaType, "data": block.Data}
			if block.URL != "" {
				source = map[string]any{"type": "url", "url": block.URL}
			}
			// Anthropic rejects PDFs inside an `image` block — they must be
			// routed to a `document` block. Mirrors new-api's
			// relay-claude.go:397-401. The response transform
			// (fromAnthropicResponse) doesn't currently round-trip binary
			// content at all — it only maps `text` and `tool_use` parts and
			// drops `image`/`document` blocks. Sub-item 1.6 in #10 only
			// covers the request path; response-side handling is out of
			// scope here.
			partType := "image"
			if block.MediaType == "application/pdf" {
				partType = "document"
			}
			parts = append(parts, anthropicContentPart{Type: partType, Source: source})
		case protocol.ContentToolResult:
			part := anthropicContentPart{Type: "tool_result", ToolUseID: block.ToolUseID, IsError: block.IsError}
			if len(block.Content) > 0 {
				// Forward the structured payload (array of text/image blocks)
				// untouched so multi-block tool_results survive the round-trip.
				part.Content = block.Content
			} else {
				part.Content = block.Text
			}
			parts = append(parts, part)
		case protocol.ContentToolUse:
			parts = append(parts, anthropicContentPart{Type: "tool_use", ID: block.ID, Name: block.Name, Input: block.Input})
		}
	}
	for _, call := range msg.ToolCalls {
		input := call.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		parts = append(parts, anthropicContentPart{Type: "tool_use", ID: sanitizeToolID(call.ID), Name: call.Name, Input: input})
	}
	if isEmptyTextParts(parts) {
		parts = []anthropicContentPart{{Type: "text", Text: "..."}}
	}
	return anthropicMessage{Role: role, Content: parts}
}

// selectToolResultBlock returns the tool_result content block that should
// drive the RoleTool fast path. When the message carries multiple tool_result
// blocks (e.g. a model that batched several tool calls) we prefer the one
// whose ToolUseID matches the message's ToolCallID so the right payload is
// associated with the right call. If no block matches — or toolCallID is
// empty — fall back to the first tool_result block so single-result messages
// keep working. Returns nil if msg.Content has no tool_result blocks at all,
// in which case the caller falls back to ContentTextValue.
func selectToolResultBlock(blocks []protocol.ContentBlock, toolCallID string) *protocol.ContentBlock {
	var first *protocol.ContentBlock
	for i := range blocks {
		b := &blocks[i]
		if b.Type != protocol.ContentToolResult {
			continue
		}
		if first == nil {
			first = b
		}
		if toolCallID != "" && b.ToolUseID == toolCallID {
			return b
		}
	}
	return first
}

// isEmptyTextParts reports whether parts is empty or contains only text blocks
// with empty strings. In either case the message would be rejected by Anthropic
// with a 400 invalid_request_error, so the caller substitutes a single
// {type:"text", text:"..."} placeholder (matching new-api's relay-claude.go).
func isEmptyTextParts(parts []anthropicContentPart) bool {
	for _, p := range parts {
		if p.Type != "text" || p.Text != "" {
			return false
		}
	}
	return true
}

// ensureFirstMessageUser prepends a synthetic user turn when the first
// (non-system) message is not role "user". Anthropic's Messages API rejects
// any conversation that does not begin with a user turn, so a client prefill
// that seeds an assistant message (or a re-played transcript that starts
// mid-conversation) would otherwise 400 upstream. Matches new-api's
// relay-claude.go:319-333 behavior, using the same "..." placeholder we use
// for empty content (sub-item 1.1).
func ensureFirstMessageUser(messages []anthropicMessage) []anthropicMessage {
	if len(messages) == 0 || messages[0].Role == protocol.RoleUser {
		return messages
	}
	synthetic := anthropicMessage{
		Role:    protocol.RoleUser,
		Content: []anthropicContentPart{{Type: "text", Text: "..."}},
	}
	return append([]anthropicMessage{synthetic}, messages...)
}

func mergeAnthropicMessages(messages []anthropicMessage) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		if len(out) > 0 && out[len(out)-1].Role == msg.Role {
			out[len(out)-1].Content = append(out[len(out)-1].Content, msg.Content...)
			continue
		}
		out = append(out, msg)
	}
	return out
}

func fromAnthropicResponse(resp anthropicResponse) *protocol.Response {
	out := &protocol.Response{ID: resp.ID, Model: resp.Model, Role: resp.Role, StopReason: protocol.MapAnthropicStopReason(resp.StopReason)}
	for _, part := range resp.Content {
		switch part.Type {
		case "text":
			out.Content = append(out.Content, protocol.ContentBlock{Type: protocol.ContentText, Text: part.Text})
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, protocol.ToolCall{ID: part.ID, Name: part.Name, Input: part.Input})
		}
	}
	out.Usage.InputTokens = resp.Usage.InputTokens
	out.Usage.OutputTokens = resp.Usage.OutputTokens
	out.Usage.CacheCreationInputTokens = resp.Usage.CacheCreationInputTokens
	out.Usage.CacheCreation5mInputTokens = resp.Usage.CacheCreation.Ephemeral5mInputTokens
	out.Usage.CacheCreation1hInputTokens = resp.Usage.CacheCreation.Ephemeral1hInputTokens
	out.Usage.CacheReadInputTokens = resp.Usage.CacheReadInputTokens
	if out.Usage.CacheCreationInputTokens == 0 && (out.Usage.CacheCreation5mInputTokens > 0 || out.Usage.CacheCreation1hInputTokens > 0) {
		out.Usage.CacheCreationInputTokens = out.Usage.CacheCreation5mInputTokens + out.Usage.CacheCreation1hInputTokens
	}
	return out
}

func sanitizeToolID(id string) string {
	if id == "" {
		return "toolu_compat"
	}
	out := make([]rune, 0, len(id))
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}
