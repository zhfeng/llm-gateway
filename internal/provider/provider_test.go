package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/protocol"
)

func defaultTransport() config.TransportRuntime {
	return config.TransportRuntime{
		MaxIdleConns:          1024,
		MaxIdleConnsPerHost:   256,
		IdleConnTimeout:       90 * time.Second,
		DialTimeout:           10 * time.Second,
		DialKeepAlive:         30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
}

func defaultHealthProbe() config.ProviderHealthProbeRuntime {
	return config.ProviderHealthProbeRuntime{Method: http.MethodGet, Path: "/models", ExpectedStatus: map[int]bool{http.StatusOK: true}}
}

func TestAnthropicEndpointAddsV1WhenMissing(t *testing.T) {
	p, err := New(config.ProviderConfig{Name: "volcengine", Type: config.ProviderAnthropicCompatible, BaseURL: "https://ark.cn-beijing.volces.com/api/coding"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.(*HTTPProvider).endpoint("models"), "https://ark.cn-beijing.volces.com/api/coding/v1/models"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestAnthropicEndpointDoesNotDuplicateV1(t *testing.T) {
	p, err := New(config.ProviderConfig{Name: "anthropic", Type: config.ProviderAnthropicCompatible, BaseURL: "https://api.anthropic.com/v1"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.(*HTTPProvider).endpoint("messages"), "https://api.anthropic.com/v1/messages"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestOpenAICompatibleComplete(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("missing auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl_test",
			"model": "provider-model",
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "hello"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 2},
		})
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "openai", Type: config.ProviderOpenAICompatible, BaseURL: upstream.URL + "/v1"}, "key", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Complete(context.Background(), &protocol.Request{Model: "m", ProviderModel: "provider-model", Messages: []protocol.Message{{Role: protocol.RoleUser, Content: protocol.TextContent("hi")}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := protocol.ContentTextValue(resp.Content); got != "hello" {
		t.Fatalf("unexpected content %q", got)
	}
	if resp.StopReason != protocol.StopEndTurn {
		t.Fatalf("unexpected stop reason %q", resp.StopReason)
	}
}

func TestAnthropicCompatibleBearerAuthHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("missing bearer auth: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "ark-code-latest"}}})
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "volcengine", Type: config.ProviderAnthropicCompatible, BaseURL: upstream.URL + "/v1", APIKeyHeader: "Authorization", APIKeyScheme: "Bearer"}, "key", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAnthropicCompatibleComplete(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "key" {
			t.Fatalf("missing api key: %s", r.Header.Get("x-api-key"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_test",
			"model":       "provider-model",
			"role":        "assistant",
			"content":     []any{map[string]any{"type": "text", "text": "hello"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 2},
		})
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "anthropic", Type: config.ProviderAnthropicCompatible, BaseURL: upstream.URL + "/v1"}, "key", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Complete(context.Background(), &protocol.Request{Model: "m", ProviderModel: "provider-model", MaxTokens: 10, Messages: []protocol.Message{{Role: protocol.RoleUser, Content: protocol.TextContent("hi")}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := protocol.ContentTextValue(resp.Content); got != "hello" {
		t.Fatalf("unexpected content %q", got)
	}
	if resp.Usage.InputTokens != 1 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestHealthCheck(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "openai", Type: config.ProviderOpenAICompatible, BaseURL: upstream.URL + "/v1"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHealthCheckCustomProbe(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/health" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	probe := config.ProviderHealthProbeRuntime{Method: http.MethodHead, Path: "/health", ExpectedStatus: map[int]bool{http.StatusNoContent: true}}
	p, err := New(config.ProviderConfig{Name: "openai", Type: config.ProviderOpenAICompatible, BaseURL: upstream.URL + "/v1"}, "", nil, time.Second, defaultTransport(), probe, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHealthCheckUnexpectedStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "openai", Type: config.ProviderOpenAICompatible, BaseURL: upstream.URL + "/v1"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestHealthCheckProviderError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "down"}})
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "openai", Type: config.ProviderOpenAICompatible, BaseURL: upstream.URL + "/v1"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestListModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "m1", "owned_by": "owner"}}})
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "openai", Type: config.ProviderOpenAICompatible, BaseURL: upstream.URL + "/v1"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "m1" || models[0].OwnedBy != "owner" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestDefaultToolSchemasAreValidJSON(t *testing.T) {
	req := &protocol.Request{ProviderModel: "provider-model", Messages: []protocol.Message{{Role: protocol.RoleUser, Content: protocol.TextContent("hi")}}, Tools: []protocol.Tool{{Name: "lookup"}}}
	if _, err := json.Marshal(toAnthropicRequest(req, false)); err != nil {
		t.Fatalf("anthropic request marshal failed: %v", err)
	}
	if _, err := json.Marshal(toOpenAIRequest(req, false)); err != nil {
		t.Fatalf("openai request marshal failed: %v", err)
	}
}

func TestAnthropicRequestIncludesSystemMessages(t *testing.T) {
	payload := toAnthropicRequest(&protocol.Request{ProviderModel: "provider-model", System: "top", Messages: []protocol.Message{{Role: protocol.RoleSystem, Content: protocol.TextContent("from message")}, {Role: protocol.RoleUser, Content: protocol.TextContent("hi")}}}, false)
	if payload.System != "top\nfrom message" {
		t.Fatalf("system = %#v", payload.System)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Role != protocol.RoleUser {
		t.Fatalf("unexpected messages: %+v", payload.Messages)
	}
}

func TestAnthropicMessageConvertsToolRoleToToolResult(t *testing.T) {
	msg := toAnthropicMessage(protocol.Message{Role: protocol.RoleTool, ToolCallID: "call_1", Content: protocol.TextContent("result")})
	if msg.Role != protocol.RoleUser || len(msg.Content) != 1 {
		t.Fatalf("unexpected message: %+v", msg)
	}
	part := msg.Content[0]
	if part.Type != "tool_result" || part.ToolUseID != "call_1" || part.Content != "result" {
		t.Fatalf("unexpected tool result: %+v", part)
	}
}

func TestAnthropicMessageToolResultBlockPreservesStringContent(t *testing.T) {
	msg := toAnthropicMessage(protocol.Message{
		Role: protocol.RoleUser,
		Content: []protocol.ContentBlock{{
			Type:      protocol.ContentToolResult,
			ToolUseID: "call_1",
			Text:      "plain string result",
		}},
	})
	if len(msg.Content) != 1 {
		t.Fatalf("unexpected content len: %+v", msg.Content)
	}
	part := msg.Content[0]
	if part.Type != "tool_result" || part.ToolUseID != "call_1" {
		t.Fatalf("unexpected tool result header: %+v", part)
	}
	if s, ok := part.Content.(string); !ok || s != "plain string result" {
		t.Fatalf("expected string content, got %T %v", part.Content, part.Content)
	}
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(out, []byte(`"content":"plain string result"`)) {
		t.Fatalf("expected string content in JSON, got %s", out)
	}
}

func TestAnthropicMessageToolResultBlockPreservesArrayContent(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"hello"},{"type":"text","text":"world"}]`)
	msg := toAnthropicMessage(protocol.Message{
		Role: protocol.RoleUser,
		Content: []protocol.ContentBlock{{
			Type:      protocol.ContentToolResult,
			ToolUseID: "call_1",
			Content:   raw,
		}},
	})
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	expected := `"content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]`
	if !bytes.Contains(out, []byte(expected)) {
		t.Fatalf("expected array content preserved, got %s", out)
	}
}

func TestAnthropicMessageToolResultBlockPreservesMixedImageContent(t *testing.T) {
	raw := json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}},{"type":"text","text":"see image"}]`)
	msg := toAnthropicMessage(protocol.Message{
		Role: protocol.RoleUser,
		Content: []protocol.ContentBlock{{
			Type:      protocol.ContentToolResult,
			ToolUseID: "call_2",
			Content:   raw,
		}},
	})
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(out, []byte(`"media_type":"image/png"`)) || !bytes.Contains(out, []byte(`"data":"AAAA"`)) {
		t.Fatalf("expected image source preserved, got %s", out)
	}
	if !bytes.Contains(out, []byte(`"text":"see image"`)) {
		t.Fatalf("expected text block preserved, got %s", out)
	}
}

func TestAnthropicMessageRoleToolPreservesStructuredContent(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"structured"}]`)
	msg := toAnthropicMessage(protocol.Message{
		Role:       protocol.RoleTool,
		ToolCallID: "call_3",
		Content: []protocol.ContentBlock{{
			Type:      protocol.ContentToolResult,
			ToolUseID: "call_3",
			Content:   raw,
		}},
	})
	if msg.Role != protocol.RoleUser || len(msg.Content) != 1 {
		t.Fatalf("unexpected message: %+v", msg)
	}
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(out, []byte(`"content":[{"type":"text","text":"structured"}]`)) {
		t.Fatalf("expected structured content preserved in RoleTool path, got %s", out)
	}
}

func TestAnthropicMessageRoleToolPreservesToolResultText(t *testing.T) {
	// A tool-role message whose Content is a tool_result ContentBlock carrying
	// only Text (no structured Content) used to fall through to
	// ContentTextValue, which only concatenates ContentText blocks and would
	// therefore emit an empty tool_result. The dedicated fallback should
	// surface the Text instead.
	msg := toAnthropicMessage(protocol.Message{
		Role:       protocol.RoleTool,
		ToolCallID: "call_4",
		Content: []protocol.ContentBlock{{
			Type:      protocol.ContentToolResult,
			ToolUseID: "call_4",
			Text:      "tool said hi",
		}},
	})
	if msg.Role != protocol.RoleUser || len(msg.Content) != 1 {
		t.Fatalf("unexpected message: %+v", msg)
	}
	part := msg.Content[0]
	if part.Type != "tool_result" || part.ToolUseID != "call_4" {
		t.Fatalf("unexpected tool result header: %+v", part)
	}
	if s, ok := part.Content.(string); !ok || s != "tool said hi" {
		t.Fatalf("expected tool_result.Text surfaced as string content, got %T %v", part.Content, part.Content)
	}
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(out, []byte(`"content":"tool said hi"`)) {
		t.Fatalf("expected text content in JSON, got %s", out)
	}
}

func TestOpenAIStreamOptionsOnlyForStreaming(t *testing.T) {
	req := &protocol.Request{ProviderModel: "provider-model", Messages: []protocol.Message{{Role: protocol.RoleUser, Content: protocol.TextContent("hi")}}}
	nonStreaming, err := json.Marshal(toOpenAIRequest(req, false))
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid(nonStreaming) && string(nonStreaming) != "" && containsJSONKey(nonStreaming, "stream_options") {
		t.Fatalf("non-streaming request includes stream_options: %s", nonStreaming)
	}
	streaming, err := json.Marshal(toOpenAIRequest(req, true))
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSONKey(streaming, "stream_options") {
		t.Fatalf("streaming request omits stream_options: %s", streaming)
	}
}

func TestAnthropicStreamToolInputAccumulatesDeltas(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_start\n"))
		w.Write([]byte("data: {\"message\":{\"id\":\"msg\",\"model\":\"m\",\"usage\":{\"input_tokens\":1}}}\n\n"))
		w.Write([]byte("event: content_block_start\n"))
		w.Write([]byte("data: {\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"lookup\"}}\n\n"))
		w.Write([]byte("event: content_block_delta\n"))
		w.Write([]byte("data: {\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\"\"}}\n\n"))
		w.Write([]byte("event: content_block_delta\n"))
		w.Write([]byte("data: {\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\":\\\"SF\\\"}\"}}\n\n"))
		w.Write([]byte("event: content_block_stop\n"))
		w.Write([]byte("data: {}\n\n"))
		w.Write([]byte("event: message_delta\n"))
		w.Write([]byte("data: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":2}}\n\n"))
		w.Write([]byte("event: message_stop\n"))
		w.Write([]byte("data: {}\n\n"))
	}))
	defer upstream.Close()

	p, err := New(config.ProviderConfig{Name: "anthropic", Type: config.ProviderAnthropicCompatible, BaseURL: upstream.URL + "/v1"}, "", nil, time.Second, defaultTransport(), defaultHealthProbe(), false)
	if err != nil {
		t.Fatal(err)
	}
	events, err := p.Stream(context.Background(), &protocol.Request{Model: "m", ProviderModel: "provider-model", Messages: []protocol.Message{{Role: protocol.RoleUser, Content: protocol.TextContent("hi")}}})
	if err != nil {
		t.Fatal(err)
	}
	var final *protocol.Response
	for event := range events {
		if event.Type == protocol.StreamMessageStop {
			final = event.Response
		}
	}
	if final == nil || len(final.ToolCalls) != 1 {
		t.Fatalf("missing final tool call: %+v", final)
	}
	if string(final.ToolCalls[0].Input) != `{"city":"SF"}` {
		t.Fatalf("tool input = %s", final.ToolCalls[0].Input)
	}
}

func containsJSONKey(data []byte, key string) bool {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return false
	}
	_, ok := obj[key]
	return ok
}

func TestAnthropicMessageEmptyContentUsesPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		msg  protocol.Message
	}{
		{
			name: "user message with nil content",
			msg:  protocol.Message{Role: protocol.RoleUser, Content: nil},
		},
		{
			name: "user message with empty string content",
			msg:  protocol.Message{Role: protocol.RoleUser, Content: protocol.TextContent("")},
		},
		{
			name: "assistant message with empty content and no tool calls",
			msg:  protocol.Message{Role: protocol.RoleAssistant, Content: protocol.TextContent("")},
		},
		{
			// protocol.TextContent("") returns nil, so the cases above collapse
			// onto the nil-content path. These explicit single-empty-text-block
			// cases exercise the "only empty text blocks" branch of
			// isEmptyTextParts, which is the shape produced when a client sends
			// {"role":"user","content":[{"type":"text","text":""}]} verbatim.
			name: "user message with single empty text block",
			msg: protocol.Message{
				Role:    protocol.RoleUser,
				Content: []protocol.ContentBlock{{Type: protocol.ContentText, Text: ""}},
			},
		},
		{
			name: "assistant message with single empty text block and no tool calls",
			msg: protocol.Message{
				Role:    protocol.RoleAssistant,
				Content: []protocol.ContentBlock{{Type: protocol.ContentText, Text: ""}},
			},
		},
		{
			name: "user message with multiple empty text blocks",
			msg: protocol.Message{
				Role: protocol.RoleUser,
				Content: []protocol.ContentBlock{
					{Type: protocol.ContentText, Text: ""},
					{Type: protocol.ContentText, Text: ""},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toAnthropicMessage(tt.msg)
			if len(got.Content) != 1 || got.Content[0].Type != "text" || got.Content[0].Text != "..." {
				t.Fatalf("expected single {text, \"...\"} placeholder, got: %+v", got.Content)
			}
		})
	}
}

func TestAnthropicMessageAssistantWithToolCallsPreservesToolUse(t *testing.T) {
	msg := toAnthropicMessage(protocol.Message{
		Role:      protocol.RoleAssistant,
		ToolCalls: []protocol.ToolCall{{ID: "call_1", Name: "get_weather", Input: json.RawMessage(`{"city":"SF"}`)}},
	})
	if len(msg.Content) != 1 || msg.Content[0].Type != "tool_use" {
		t.Fatalf("expected tool_use block preserved, got: %+v", msg.Content)
	}
	if msg.Content[0].ID != "call_1" || msg.Content[0].Name != "get_weather" {
		t.Fatalf("unexpected tool_use block: %+v", msg.Content[0])
	}
}

func TestAnthropicRequestPrependsSyntheticUserWhenFirstMessageIsAssistant(t *testing.T) {
	tests := []struct {
		name     string
		req      *protocol.Request
		wantLen  int
		wantRole []string
	}{
		{
			name: "first non-system message is assistant",
			req: &protocol.Request{
				ProviderModel: "provider-model",
				Messages: []protocol.Message{
					{Role: protocol.RoleAssistant, Content: protocol.TextContent("seed prefill")},
					{Role: protocol.RoleUser, Content: protocol.TextContent("hi")},
				},
			},
			wantLen:  3,
			wantRole: []string{protocol.RoleUser, protocol.RoleAssistant, protocol.RoleUser},
		},
		{
			name: "system message followed by assistant first",
			req: &protocol.Request{
				ProviderModel: "provider-model",
				Messages: []protocol.Message{
					{Role: protocol.RoleSystem, Content: protocol.TextContent("you are helpful")},
					{Role: protocol.RoleAssistant, Content: protocol.TextContent("seed prefill")},
					{Role: protocol.RoleUser, Content: protocol.TextContent("hi")},
				},
			},
			wantLen:  3,
			wantRole: []string{protocol.RoleUser, protocol.RoleAssistant, protocol.RoleUser},
		},
		{
			name: "single assistant message",
			req: &protocol.Request{
				ProviderModel: "provider-model",
				Messages: []protocol.Message{
					{Role: protocol.RoleAssistant, Content: protocol.TextContent("alone")},
				},
			},
			wantLen:  2,
			wantRole: []string{protocol.RoleUser, protocol.RoleAssistant},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := toAnthropicRequest(tt.req, false)
			if len(payload.Messages) != tt.wantLen {
				t.Fatalf("len = %d, want %d; messages = %+v", len(payload.Messages), tt.wantLen, payload.Messages)
			}
			for i, role := range tt.wantRole {
				if payload.Messages[i].Role != role {
					t.Fatalf("messages[%d].Role = %q, want %q", i, payload.Messages[i].Role, role)
				}
			}
			first := payload.Messages[0]
			if len(first.Content) != 1 || first.Content[0].Type != "text" || first.Content[0].Text != "..." {
				t.Fatalf("synthetic first message = %+v, want single text \"...\" block", first.Content)
			}
		})
	}
}

func TestAnthropicRequestLeavesFirstUserMessageUntouched(t *testing.T) {
	req := &protocol.Request{
		ProviderModel: "provider-model",
		Messages: []protocol.Message{
			{Role: protocol.RoleUser, Content: protocol.TextContent("hello")},
			{Role: protocol.RoleAssistant, Content: protocol.TextContent("hi there")},
		},
	}
	payload := toAnthropicRequest(req, false)
	if len(payload.Messages) != 2 {
		t.Fatalf("expected 2 messages unchanged, got %d: %+v", len(payload.Messages), payload.Messages)
	}
	if payload.Messages[0].Role != protocol.RoleUser || payload.Messages[1].Role != protocol.RoleAssistant {
		t.Fatalf("unexpected roles: %+v", payload.Messages)
	}
	if got := payload.Messages[0].Content[0].Text; got != "hello" {
		t.Fatalf("first user message mutated: text = %q", got)
	}
}

func TestAnthropicRequestEmptyMessagesUnchanged(t *testing.T) {
	req := &protocol.Request{
		ProviderModel: "provider-model",
		Messages:      []protocol.Message{},
	}
	payload := toAnthropicRequest(req, false)
	if len(payload.Messages) != 0 {
		t.Fatalf("expected empty messages slice unchanged, got %d: %+v", len(payload.Messages), payload.Messages)
	}
}

func TestAnthropicRequestSystemOnlyMessagesUnchanged(t *testing.T) {
	req := &protocol.Request{
		ProviderModel: "provider-model",
		Messages: []protocol.Message{
			{Role: protocol.RoleSystem, Content: protocol.TextContent("you are helpful")},
		},
	}
	payload := toAnthropicRequest(req, false)
	if payload.System != "you are helpful" {
		t.Fatalf("system = %q, want \"you are helpful\"", payload.System)
	}
	if len(payload.Messages) != 0 {
		t.Fatalf("expected empty messages slice (no synthetic injection on system-only), got %d: %+v", len(payload.Messages), payload.Messages)
	}
}
