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

type openAIRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Stop                any             `json:"stop,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	StreamOptions       any             `json:"stream_options,omitempty"`
	Tools               []openAITool    `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

type openAIToolCall struct {
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAICompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *HTTPProvider) completeOpenAI(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	payload := toOpenAIRequest(req, false)
	if p.logMessages {
		logJSON("provider request body", "provider", p.name, "protocol", "openai", "payload", payload)
	}
	hreq, err := p.newJSONRequest(ctx, http.MethodPost, p.endpoint("chat/completions"), payload)
	if err != nil {
		return nil, err
	}
	resp, err := p.do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var decoded openAICompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if p.logMessages {
		logJSON("provider response body", "provider", p.name, "protocol", "openai", "payload", decoded)
	}
	return fromOpenAICompletion(decoded), nil
}

func (p *HTTPProvider) streamOpenAI(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	payload := toOpenAIRequest(req, true)
	if p.logMessages {
		logJSON("provider request body", "provider", p.name, "protocol", "openai", "stream", true, "payload", payload)
	}
	hreq, err := p.newJSONRequest(ctx, http.MethodPost, p.endpoint("chat/completions"), payload)
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
		toolCalls := map[int]*protocol.ToolCall{}
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
				slog.Info("provider stream event", "provider", p.name, "protocol", "openai", "event", ev.Name, "data", string(ev.Data))
			}
			if string(ev.Data) == "[DONE]" {
				return nil
			}
			var chunk struct {
				ID      string `json:"id"`
				Model   string `json:"model"`
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							Index    int            `json:"index"`
							ID       string         `json:"id"`
							Type     string         `json:"type"`
							Function openAIFunction `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(ev.Data, &chunk); err != nil {
				return err
			}
			if chunk.ID != "" {
				acc.ID = chunk.ID
			}
			if chunk.Model != "" {
				acc.Model = chunk.Model
			}
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				acc.Usage.InputTokens = chunk.Usage.PromptTokens
				acc.Usage.OutputTokens = chunk.Usage.CompletionTokens
				if !send(protocol.StreamEvent{Type: protocol.StreamTextDelta, Usage: acc.Usage}) {
					return ctx.Err()
				}
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					acc.Content = appendText(acc.Content, choice.Delta.Content)
					if !send(protocol.StreamEvent{Type: protocol.StreamTextDelta, Text: choice.Delta.Content}) {
						return ctx.Err()
					}
				}
				for _, tc := range choice.Delta.ToolCalls {
					call := toolCalls[tc.Index]
					if call == nil {
						call = &protocol.ToolCall{ID: tc.ID, Name: tc.Function.Name}
						toolCalls[tc.Index] = call
					}
					if tc.ID != "" {
						call.ID = tc.ID
					}
					if tc.Function.Name != "" {
						call.Name = tc.Function.Name
					}
					if len(tc.Function.Arguments) > 0 {
						call.Input = append(call.Input, tc.Function.Arguments...)
						if !send(protocol.StreamEvent{Type: protocol.StreamToolCall, ToolCallID: call.ID, ToolName: call.Name, ToolInput: string(tc.Function.Arguments)}) {
							return ctx.Err()
						}
					}
				}
				if choice.FinishReason != "" {
					acc.StopReason = protocol.MapOpenAIStopReason(choice.FinishReason)
				}
			}
			return nil
		})
		for i := 0; i < len(toolCalls); i++ {
			if call := toolCalls[i]; call != nil {
				acc.ToolCalls = append(acc.ToolCalls, *call)
			}
		}
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

func toOpenAIRequest(req *protocol.Request, streaming bool) openAIRequest {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: protocol.RoleSystem, Content: req.System})
	}
	for _, msg := range req.Messages {
		messages = append(messages, toOpenAIMessage(msg))
	}
	tools := make([]openAITool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		params := tool.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, openAITool{Type: "function", Function: openAIFunction{Name: tool.Name, Description: tool.Description, Parameters: params}})
	}
	out := openAIRequest{
		Model:       req.ProviderModel,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      streaming,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
	}
	if streaming {
		out.StreamOptions = map[string]bool{"include_usage": true}
	}
	if len(req.StopSequences) == 1 {
		out.Stop = req.StopSequences[0]
	} else if len(req.StopSequences) > 1 {
		out.Stop = req.StopSequences
	}
	return out
}

func toOpenAIMessage(msg protocol.Message) openAIMessage {
	out := openAIMessage{Role: msg.Role, ToolCallID: msg.ToolCallID, Name: msg.Name}
	if msg.Role == protocol.RoleTool {
		out.Content = protocol.ContentTextValue(msg.Content)
		return out
	}
	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = make([]openAIToolCall, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, openAIToolCall{ID: call.ID, Type: "function", Function: openAIFunction{Name: call.Name, Arguments: call.Input}})
		}
	}
	out.Content = openAIContent(msg.Content)
	return out
}

func openAIContent(blocks []protocol.ContentBlock) any {
	if len(blocks) == 0 {
		return ""
	}
	if len(blocks) == 1 && blocks[0].Type == protocol.ContentText {
		return blocks[0].Text
	}
	parts := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case protocol.ContentText:
			parts = append(parts, map[string]any{"type": "text", "text": block.Text})
		case protocol.ContentImage:
			url := block.URL
			if url == "" && block.Data != "" {
				url = "data:" + block.MediaType + ";base64," + block.Data
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
		case protocol.ContentToolResult:
			parts = append(parts, map[string]any{"type": "text", "text": block.Text})
		}
	}
	return parts
}

func fromOpenAICompletion(resp openAICompletionResponse) *protocol.Response {
	out := &protocol.Response{ID: resp.ID, Model: resp.Model, Role: protocol.RoleAssistant}
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		out.Content = protocol.TextContent(contentString(choice.Message.Content))
		out.ToolCalls = fromOpenAIToolCalls(choice.Message.ToolCalls)
		out.StopReason = protocol.MapOpenAIStopReason(choice.FinishReason)
	}
	out.Usage.InputTokens = resp.Usage.PromptTokens
	out.Usage.OutputTokens = resp.Usage.CompletionTokens
	return out
}

func fromOpenAIToolCalls(calls []openAIToolCall) []protocol.ToolCall {
	out := make([]protocol.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, protocol.ToolCall{ID: call.ID, Name: call.Function.Name, Input: call.Function.Arguments})
	}
	return out
}

func contentString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var out string
		for _, part := range v {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := m["text"].(string); ok {
				out += text
			}
		}
		return out
	default:
		return ""
	}
}

func appendText(blocks []protocol.ContentBlock, text string) []protocol.ContentBlock {
	if len(blocks) > 0 && blocks[len(blocks)-1].Type == protocol.ContentText {
		blocks[len(blocks)-1].Text += text
		return blocks
	}
	return append(blocks, protocol.ContentBlock{Type: protocol.ContentText, Text: text})
}
