package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/gwerror"
	"github.com/zhfeng/llm-gateway/internal/health"
	"github.com/zhfeng/llm-gateway/internal/models"
	"github.com/zhfeng/llm-gateway/internal/protocol"
	"github.com/zhfeng/llm-gateway/internal/provider"
)

func TestChatCompletionsUnknownModelReturns403(t *testing.T) {
	h := New(models.New(config.Config{Models: map[string]config.ModelRoute{}}, nil, time.Hour, true, time.Hour, 10000), nil, 1<<20, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"missing","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "model does not exist") {
		t.Fatalf("body does not contain expected message: %s", w.Body.String())
	}
}

func TestMessagesUnknownModelReturns403(t *testing.T) {
	h := New(models.New(config.Config{Models: map[string]config.ModelRoute{}}, nil, time.Hour, true, time.Hour, 10000), nil, 1<<20, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"missing","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Messages(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "model does not exist") {
		t.Fatalf("body does not contain expected message: %s", w.Body.String())
	}
}

type streamProvider struct {
	events      []protocol.StreamEvent
	seen        *[]string
	completeErr error
	streamErr   error
	block       <-chan struct{}
	entered     chan<- struct{}
}

func (s streamProvider) Name() string { return "p1" }
func (s streamProvider) Type() string { return config.ProviderAnthropicCompatible }
func (s streamProvider) Complete(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	if s.seen != nil {
		*s.seen = append(*s.seen, req.ProviderModel)
	}
	if s.completeErr != nil {
		return nil, s.completeErr
	}
	if s.entered != nil {
		s.entered <- struct{}{}
	}
	if s.block != nil {
		<-s.block
	}
	return &protocol.Response{Model: req.Model, Role: protocol.RoleAssistant, Content: protocol.TextContent("ok")}, nil
}
func (s streamProvider) Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	if s.seen != nil {
		*s.seen = append(*s.seen, req.ProviderModel)
	}
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan protocol.StreamEvent, len(s.events))
	for _, event := range s.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}
func (s streamProvider) ListModels(context.Context) ([]protocol.ModelInfo, error) { return nil, nil }
func (s streamProvider) HealthCheck(context.Context) error                        { return nil }

func TestChatCompletionsOversizedBodyReturns413(t *testing.T) {
	h := New(models.New(config.Config{Models: map[string]config.ModelRoute{}}, nil, time.Hour, true, time.Hour, 10000), nil, 8, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m"} `))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
}

func TestMessagesOversizedBodyReturns413(t *testing.T) {
	h := New(models.New(config.Config{Models: map[string]config.ModelRoute{}}, nil, time.Hour, true, time.Hour, 10000), nil, 8, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"m"} `))
	w := httptest.NewRecorder()

	h.Messages(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
}

func TestOpenAICompletionMapsAnthropicRefusalToContentFilter(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_refusal",
			"model":       "provider-model",
			"role":        "assistant",
			"content":     []any{map[string]any{"type": "text", "text": "I can't help with that."}},
			"stop_reason": "refusal",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 2},
		})
	}))
	defer upstream.Close()

	prov, err := provider.New(config.ProviderConfig{Name: "p1", Type: config.ProviderAnthropicCompatible, BaseURL: upstream.URL + "/v1"}, "key", nil, time.Second, config.TransportRuntime{}, config.ProviderHealthProbeRuntime{}, false)
	if err != nil {
		t.Fatal(err)
	}
	registry := models.New(config.Config{Providers: []config.ProviderConfig{{Name: "p1", Type: config.ProviderAnthropicCompatible}}, Models: map[string]config.ModelRoute{"m": {Provider: "p1", ProviderModel: "provider-model"}}}, map[string]provider.Provider{"p1": prov}, time.Hour, true, time.Hour, 10000)
	h := New(registry, nil, 1<<20, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Choices) != 1 {
		t.Fatalf("choices = %d; want 1", len(body.Choices))
	}
	if got := body.Choices[0].FinishReason; got != "content_filter" {
		t.Fatalf("finish_reason = %q; want %q", got, "content_filter")
	}
}

func TestOpenAIStreamMapsAnthropicRefusalToContentFilter(t *testing.T) {
	prov := streamProvider{events: []protocol.StreamEvent{
		{Type: protocol.StreamMessageStop, Response: &protocol.Response{Model: "m", Role: protocol.RoleAssistant, StopReason: protocol.MapAnthropicStopReason("refusal")}},
	}}
	registry := models.New(config.Config{Providers: []config.ProviderConfig{{Name: "p1", Type: config.ProviderAnthropicCompatible}}, Models: map[string]config.ModelRoute{"m": {Provider: "p1", ProviderModel: "pm"}}}, map[string]provider.Provider{"p1": prov}, time.Hour, true, time.Hour, 10000)
	h := New(registry, nil, 1<<20, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"finish_reason":"content_filter"`) {
		t.Fatalf("body missing content_filter finish_reason: %s", body)
	}
	if strings.Contains(body, `"finish_reason":"refusal"`) {
		t.Fatalf("body contains unmapped refusal finish_reason: %s", body)
	}
}

func TestOpenAIToolDeltaChunkIncludesToolIndex(t *testing.T) {
	chunk := openAIToolDeltaChunk("m", protocol.StreamEvent{ToolCallID: "call_1", ToolName: "lookup", ToolInput: `{"q":"x"}`})
	choices := chunk["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	toolCalls := delta["tool_calls"].([]any)
	if got := toolCalls[0].(map[string]any)["index"]; got != 0 {
		t.Fatalf("tool index = %#v", got)
	}
}

func TestOpenAIReasoningDeltaChunkUsesReasoningContent(t *testing.T) {
	chunk := openAIReasoningDeltaChunk("m", "abc")
	choices := chunk["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	if got := delta["reasoning_content"]; got != "abc" {
		t.Fatalf("reasoning_content = %#v; want %q", got, "abc")
	}
	if _, ok := delta["content"]; ok {
		t.Fatalf("reasoning chunk must not carry content field: %#v", delta)
	}
}

func TestStickyHeaderRoutesRepeatedRequestsToSameTarget(t *testing.T) {
	seen := []string{}
	h := stickyTestHandler(&seen, true, "auth_key")
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("X-LLM-Gateway-Sticky-Key", "session")
		w := httptest.NewRecorder()
		h.ChatCompletions(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
		}
	}
	for _, model := range seen[1:] {
		if model != seen[0] {
			t.Fatalf("sticky routing changed targets: %v", seen)
		}
	}
}

func TestStickyHeaderTakesPrecedenceOverAuthFallback(t *testing.T) {
	seen := []string{}
	h := stickyTestHandler(&seen, true, "auth_key")
	for _, header := range []string{"session-a", "session-a"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("X-LLM-Gateway-Sticky-Key", header)
		req.Header.Set("Authorization", "Bearer shared")
		w := httptest.NewRecorder()
		h.Messages(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
		}
	}
	if seen[0] != seen[1] {
		t.Fatalf("header stickiness not stable: %v", seen)
	}
}

func TestStickyAuthFallbackRoutesRepeatedRequestsToSameTarget(t *testing.T) {
	seen := []string{}
	h := stickyTestHandler(&seen, true, "auth_key")
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Authorization", "Bearer auth-session")
		w := httptest.NewRecorder()
		h.ChatCompletions(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
		}
	}
	for _, model := range seen[1:] {
		if model != seen[0] {
			t.Fatalf("auth fallback stickiness changed targets: %v", seen)
		}
	}
}

func TestStickyDisabledDoesNotDeriveKey(t *testing.T) {
	seen := []string{}
	h := stickyTestHandler(&seen, false, "auth_key")
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("X-LLM-Gateway-Sticky-Key", "session")
	if key := h.stickyKeyFromRequest(req); key != "" {
		t.Fatalf("sticky key = %q", key)
	}
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func stickyTestHandler(seen *[]string, stickyEnabled bool, fallback string) *HandlerGroup {
	prov := streamProvider{seen: seen}
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "pm1", Weight: 1}, {Provider: "p2", ProviderModel: "pm2", Weight: 1}}}}}
	providers := map[string]provider.Provider{"p1": prov, "p2": prov}
	registry := models.New(cfg, providers, time.Hour, stickyEnabled, time.Hour, 10000)
	return New(registry, nil, 1<<20, false, Options{StickyWeightedEnabled: stickyEnabled, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: fallback, RetryMaxAttempts: 1})
}

func TestCompleteWithRetryFailsOverToSecondTarget(t *testing.T) {
	seen := []string{}
	retryErr := gwerror.New(http.StatusServiceUnavailable, "provider_error", "down")
	h := retryTestHandler(&seen, retryErr, nil, true)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("unexpected attempts: %v", seen)
	}
}

func TestCompleteWithRetryDoesNotRetryNonRetryableError(t *testing.T) {
	seen := []string{}
	nonRetryable := gwerror.New(http.StatusUnauthorized, "authentication_error", "bad key")
	h := retryTestHandler(&seen, nonRetryable, nil, true)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if len(seen) != 1 {
		t.Fatalf("unexpected attempts: %v", seen)
	}
}

func TestStreamWithRetryFailsOverBeforeHeaders(t *testing.T) {
	seen := []string{}
	h := retryTestHandler(&seen, nil, gwerror.New(http.StatusServiceUnavailable, "provider_error", "stream open failed"), true)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.Messages(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("unexpected attempts: %v", seen)
	}
}

func retryTestHandler(seen *[]string, completeErr error, streamErr error, retryEnabled bool) *HandlerGroup {
	p1 := streamProvider{seen: seen, completeErr: completeErr, streamErr: streamErr}
	p2 := streamProvider{seen: seen, events: []protocol.StreamEvent{{Type: protocol.StreamTextDelta, Text: "ok"}, {Type: protocol.StreamMessageStop, Response: &protocol.Response{Model: "m", StopReason: protocol.StopEndTurn}}}}
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "pm1", Weight: 1}, {Provider: "p2", ProviderModel: "pm2", Weight: 0}}}}}
	providers := map[string]provider.Provider{"p1": p1, "p2": p2}
	registry := models.New(cfg, providers, time.Hour, false, time.Hour, 10000)
	return New(registry, nil, 1<<20, false, Options{RetryEnabled: retryEnabled, RetryMaxAttempts: 2, RetryBackoff: 0, RetryOnStatus: map[int]bool{http.StatusServiceUnavailable: true}, RetryOnNetworkError: true, RetryOnTimeout: true, StickyWeightedEnabled: false})
}

func TestAdmissionErrorFailsOverAndDoesNotPoisonHealth(t *testing.T) {
	seen := []string{}
	block := make(chan struct{})
	entered := make(chan struct{}, 1)
	p1 := provider.WithConcurrencyLimit(streamProvider{seen: &seen, block: block, entered: entered}, 1)
	go p1.Complete(context.Background(), &protocol.Request{})
	<-entered
	p2 := streamProvider{seen: &seen}
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "pm1", Weight: 1}, {Provider: "p2", ProviderModel: "pm2", Weight: 0}}}}}
	providers := map[string]provider.Provider{"p1": p1, "p2": p2}
	registry := models.New(cfg, providers, time.Hour, false, time.Hour, 10000)
	healthManager := health.NewManager(providers, health.Config{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, ProviderEnabled: map[string]bool{"p1": true, "p2": true}})
	h := New(registry, healthManager, 1<<20, false, Options{RetryEnabled: true, RetryMaxAttempts: 2, RetryBackoff: 0, RetryOnStatus: map[int]bool{http.StatusTooManyRequests: true}, StickyWeightedEnabled: false})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if len(seen) != 2 || seen[1] != "pm2" {
		t.Fatalf("unexpected attempts: %v", seen)
	}
	if !healthManager.IsRoutable("p1") {
		t.Fatal("local admission error should not poison provider health")
	}
	close(block)
}

func TestAnthropicStreamIncludesLifecycleEvents(t *testing.T) {
	prov := streamProvider{events: []protocol.StreamEvent{
		{Type: protocol.StreamTextDelta, Text: "hello"},
		{Type: protocol.StreamMessageStop, Response: &protocol.Response{Model: "m", StopReason: protocol.StopEndTurn}},
	}}
	registry := models.New(config.Config{Providers: []config.ProviderConfig{{Name: "p1", Type: config.ProviderAnthropicCompatible}}, Models: map[string]config.ModelRoute{"m": {Provider: "p1", ProviderModel: "pm"}}}, map[string]provider.Provider{"p1": prov}, time.Hour, true, time.Hour, 10000)
	h := New(registry, nil, 1<<20, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.Messages(w, req)

	body := w.Body.String()
	for _, event := range []string{"event: message_start", "event: content_block_start", "event: content_block_delta", "event: content_block_stop", "event: message_delta", "event: message_stop"} {
		if !strings.Contains(body, event) {
			t.Fatalf("stream missing %s; body=%s", event, body)
		}
	}
}

func TestOpenAIStreamSurfacesThinkingAsReasoningContent(t *testing.T) {
	prov := streamProvider{events: []protocol.StreamEvent{
		{Type: protocol.StreamThinking, Text: "let me think"},
		{Type: protocol.StreamTextDelta, Text: "hello"},
		{Type: protocol.StreamMessageStop, Response: &protocol.Response{Model: "m", Role: protocol.RoleAssistant, Content: protocol.TextContent("hello"), StopReason: protocol.StopEndTurn}},
	}}
	registry := models.New(config.Config{Providers: []config.ProviderConfig{{Name: "p1", Type: config.ProviderAnthropicCompatible}}, Models: map[string]config.ModelRoute{"m": {Provider: "p1", ProviderModel: "pm"}}}, map[string]provider.Provider{"p1": prov}, time.Hour, true, time.Hour, 10000)
	h := New(registry, nil, 1<<20, false, Options{StickyWeightedEnabled: true, StickyWeightedHeader: "X-LLM-Gateway-Sticky-Key", StickyWeightedFallback: "auth_key", RetryMaxAttempts: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()

	h.ChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"reasoning_content":"let me think"`) {
		t.Fatalf("body missing reasoning_content delta: %s", body)
	}
	if !strings.Contains(body, `"content":"hello"`) {
		t.Fatalf("body missing text delta: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("body missing terminator: %s", body)
	}
}
