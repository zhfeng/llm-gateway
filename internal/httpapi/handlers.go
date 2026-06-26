package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zhfeng/llm-gateway/internal/gwerror"
	"github.com/zhfeng/llm-gateway/internal/health"
	"github.com/zhfeng/llm-gateway/internal/models"
	"github.com/zhfeng/llm-gateway/internal/protocol"
	"github.com/zhfeng/llm-gateway/internal/provider"
	dstream "github.com/zhfeng/llm-gateway/internal/stream"
)

type HandlerGroup struct {
	registry               *models.Registry
	health                 *health.Manager
	maxBodyBytes           int64
	logMessages            bool
	retryEnabled           bool
	retryMaxAttempts       int
	retryBackoff           time.Duration
	retryMaxBackoff        time.Duration
	retryOnStatus          map[int]bool
	retryOnNetworkError    bool
	retryOnTimeout         bool
	stickyWeightedEnabled  bool
	stickyWeightedHeader   string
	stickyWeightedFallback string
}

type Options struct {
	RetryEnabled           bool
	RetryMaxAttempts       int
	RetryBackoff           time.Duration
	RetryMaxBackoff        time.Duration
	RetryOnStatus          map[int]bool
	RetryOnNetworkError    bool
	RetryOnTimeout         bool
	StickyWeightedEnabled  bool
	StickyWeightedHeader   string
	StickyWeightedFallback string
}

func New(registry *models.Registry, healthManager *health.Manager, maxBodyBytes int64, logMessages bool, opts Options) *HandlerGroup {
	return &HandlerGroup{registry: registry, health: healthManager, maxBodyBytes: maxBodyBytes, logMessages: logMessages, retryEnabled: opts.RetryEnabled, retryMaxAttempts: opts.RetryMaxAttempts, retryBackoff: opts.RetryBackoff, retryMaxBackoff: opts.RetryMaxBackoff, retryOnStatus: opts.RetryOnStatus, retryOnNetworkError: opts.RetryOnNetworkError, retryOnTimeout: opts.RetryOnTimeout, stickyWeightedEnabled: opts.StickyWeightedEnabled, stickyWeightedHeader: opts.StickyWeightedHeader, stickyWeightedFallback: opts.StickyWeightedFallback}
}

func (h *HandlerGroup) stickyKeyFromRequest(r *http.Request) string {
	if !h.stickyWeightedEnabled {
		return ""
	}
	if h.stickyWeightedHeader != "" {
		if value := r.Header.Get(h.stickyWeightedHeader); value != "" {
			return hashStickyKey("header", value)
		}
	}
	if h.stickyWeightedFallback == "auth_key" {
		if value := authKeyFromRequest(r); value != "" {
			return hashStickyKey("auth", value)
		}
	}
	return ""
}

func authKeyFromRequest(r *http.Request) string {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if provided == "" {
		provided = r.Header.Get("x-api-key")
	}
	return provided
}

func hashStickyKey(source, value string) string {
	sum := sha256.Sum256([]byte(value))
	return source + ":" + hex.EncodeToString(sum[:])
}

func (h *HandlerGroup) ListModels(w http.ResponseWriter, r *http.Request) {
	models := h.registry.Models()
	data := make([]map[string]any, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]any{"id": model.ID, "object": "model", "owned_by": model.OwnedBy})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (h *HandlerGroup) readBody(r io.Reader) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, h.maxBodyBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > h.maxBodyBytes {
		return nil, true, nil
	}
	return body, false, nil
}

func (h *HandlerGroup) streamWithRetry(ctx context.Context, req *protocol.Request, route models.Route) (<-chan protocol.StreamEvent, error) {
	attempts := h.attempts(route)
	if !h.retryEnabled || h.retryMaxAttempts < 1 {
		attempts = attempts[:1]
	}
	var lastErr error
	for i, attempt := range attempts {
		if i >= h.retryMaxAttempts {
			break
		}
		attemptReq := *req
		attemptReq.ProviderModel = attempt.ProviderModel
		events, err := attempt.Provider.Stream(ctx, &attemptReq)
		h.recordProviderResult(attempt.ProviderName, err)
		if err == nil {
			return events, nil
		}
		lastErr = err
		if !h.shouldRetry(ctx, err) {
			break
		}
		if i+1 < len(attempts) && i+1 < h.retryMaxAttempts && h.retryBackoff > 0 {
			timer := time.NewTimer(h.backoffForAttempt(i))
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

func (h *HandlerGroup) completeWithRetry(ctx context.Context, req *protocol.Request, route models.Route) (*protocol.Response, error) {
	attempts := h.attempts(route)
	if !h.retryEnabled || h.retryMaxAttempts < 1 {
		attempts = attempts[:1]
	}
	var lastErr error
	for i, attempt := range attempts {
		if i >= h.retryMaxAttempts {
			break
		}
		attemptReq := *req
		attemptReq.ProviderModel = attempt.ProviderModel
		resp, err := attempt.Provider.Complete(ctx, &attemptReq)
		h.recordProviderResult(attempt.ProviderName, err)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !h.shouldRetry(ctx, err) {
			break
		}
		if i+1 < len(attempts) && i+1 < h.retryMaxAttempts && h.retryBackoff > 0 {
			timer := time.NewTimer(h.backoffForAttempt(i))
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

type attemptRoute struct {
	Provider      provider.Provider
	ProviderName  string
	ProviderModel string
}

func (h *HandlerGroup) attempts(route models.Route) []attemptRoute {
	primary := attemptRoute{Provider: route.Provider, ProviderName: route.ProviderName, ProviderModel: route.ProviderModel}
	attempts := []attemptRoute{primary}
	for _, target := range route.Targets {
		if target.ProviderName == primary.ProviderName && target.ProviderModel == primary.ProviderModel {
			continue
		}
		attempts = append(attempts, attemptRoute{Provider: target.Provider, ProviderName: target.ProviderName, ProviderModel: target.ProviderModel})
	}
	if h.health == nil {
		return attempts
	}
	preferred := make([]attemptRoute, 0, len(attempts))
	deferred := make([]attemptRoute, 0, len(attempts))
	for _, attempt := range attempts {
		if h.health.IsRoutable(attempt.ProviderName) {
			preferred = append(preferred, attempt)
		} else {
			deferred = append(deferred, attempt)
		}
	}
	if len(preferred) == 0 {
		return attempts
	}
	return append(preferred, deferred...)
}

func (h *HandlerGroup) shouldRetry(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var ge *gwerror.Error
	if errors.As(err, &ge) {
		return h.retryOnStatus[ge.Status]
	}
	if h.retryOnTimeout && errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return h.retryOnTimeout
		}
		return h.retryOnNetworkError
	}
	return false
}

func (h *HandlerGroup) backoffForAttempt(attempt int) time.Duration {
	backoff := h.retryBackoff
	for i := 0; i < attempt; i++ {
		backoff *= 2
		if h.retryMaxBackoff > 0 && backoff > h.retryMaxBackoff {
			return h.retryMaxBackoff
		}
	}
	return backoff
}

func (h *HandlerGroup) recordProviderResult(providerName string, err error) {
	if h.health != nil && !provider.IsAdmissionError(err) {
		h.health.RecordResult(providerName, err)
	}
}

func (h *HandlerGroup) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, tooLarge, err := h.readBody(r.Body)
	if err != nil {
		gwerror.WriteOpenAI(w, gwerror.New(http.StatusBadRequest, "invalid_request_error", "read body: "+err.Error()))
		return
	}
	if tooLarge {
		gwerror.WriteOpenAI(w, gwerror.New(http.StatusRequestEntityTooLarge, "invalid_request_error", "request body too large"))
		return
	}
	if h.logMessages {
		slog.Info("gateway request body", "protocol", "openai", "path", r.URL.Path, "body", string(body))
	}
	var raw struct {
		Model               string            `json:"model"`
		Messages            []json.RawMessage `json:"messages"`
		System              string            `json:"system"`
		MaxTokens           int               `json:"max_tokens"`
		MaxCompletionTokens int               `json:"max_completion_tokens"`
		Temperature         *float64          `json:"temperature"`
		TopP                *float64          `json:"top_p"`
		Stop                any               `json:"stop"`
		Stream              bool              `json:"stream"`
		Tools               []json.RawMessage `json:"tools"`
		ToolChoice          any               `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		gwerror.WriteOpenAI(w, gwerror.New(http.StatusBadRequest, "invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}
	req := &protocol.Request{Model: raw.Model, System: raw.System, Temperature: raw.Temperature, TopP: raw.TopP, Stream: raw.Stream, ToolChoice: raw.ToolChoice}
	if raw.MaxTokens > 0 {
		req.MaxTokens = raw.MaxTokens
	}
	if raw.MaxCompletionTokens > 0 {
		req.MaxTokens = raw.MaxCompletionTokens
	}
	req.StopSequences = parseStop(raw.Stop)
	req.Messages = parseOpenAIMessages(raw.Messages)
	req.Tools = parseTools(raw.Tools)

	route, ok := h.registry.ResolveWithOptions(raw.Model, models.ResolveOptions{StickyKey: h.stickyKeyFromRequest(r)})
	if !ok {
		gwerror.WriteOpenAI(w, gwerror.New(http.StatusForbidden, "permission_error", "model does not exist"))
		return
	}
	req.ProviderModel = route.ProviderModel

	if raw.Stream {
		h.streamOpenAI(w, r, req, route)
	} else {
		resp, err := h.completeWithRetry(r.Context(), req, route)
		if err != nil {
			gwerror.WriteOpenAI(w, err)
			return
		}
		h.writeOpenAICompletion(w, resp)
	}
}

func (h *HandlerGroup) Messages(w http.ResponseWriter, r *http.Request) {
	body, tooLarge, err := h.readBody(r.Body)
	if err != nil {
		gwerror.WriteAnthropic(w, gwerror.New(http.StatusBadRequest, "invalid_request_error", "read body: "+err.Error()))
		return
	}
	if tooLarge {
		gwerror.WriteAnthropic(w, gwerror.New(http.StatusRequestEntityTooLarge, "invalid_request_error", "request body too large"))
		return
	}
	if h.logMessages {
		slog.Info("gateway request body", "protocol", "anthropic", "path", r.URL.Path, "body", string(body))
	}
	var raw struct {
		Model         string            `json:"model"`
		Messages      []json.RawMessage `json:"messages"`
		System        any               `json:"system"`
		MaxTokens     int               `json:"max_tokens"`
		Temperature   *float64          `json:"temperature"`
		TopP          *float64          `json:"top_p"`
		StopSequences []string          `json:"stop_sequences"`
		Stream        bool              `json:"stream"`
		Tools         []json.RawMessage `json:"tools"`
		ToolChoice    any               `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		gwerror.WriteAnthropic(w, gwerror.New(http.StatusBadRequest, "invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}
	req := &protocol.Request{Model: raw.Model, Temperature: raw.Temperature, TopP: raw.TopP, Stream: raw.Stream, StopSequences: raw.StopSequences, ToolChoice: raw.ToolChoice}
	if raw.MaxTokens > 0 {
		req.MaxTokens = raw.MaxTokens
	}
	req.System = extractSystem(raw.System)
	req.Messages = parseAnthropicMessages(raw.Messages)
	req.Tools = parseTools(raw.Tools)

	route, ok := h.registry.ResolveWithOptions(raw.Model, models.ResolveOptions{StickyKey: h.stickyKeyFromRequest(r)})
	if !ok {
		gwerror.WriteAnthropic(w, gwerror.New(http.StatusForbidden, "permission_error", "model does not exist"))
		return
	}
	req.ProviderModel = route.ProviderModel

	if raw.Stream {
		h.streamAnthropic(w, r, req, route)
	} else {
		resp, err := h.completeWithRetry(r.Context(), req, route)
		if err != nil {
			gwerror.WriteAnthropic(w, err)
			return
		}
		h.writeAnthropicMessage(w, resp)
	}
}

func (h *HandlerGroup) streamOpenAI(w http.ResponseWriter, r *http.Request, req *protocol.Request, route models.Route) {
	events, err := h.streamWithRetry(r.Context(), req, route)
	if err != nil {
		gwerror.WriteOpenAI(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	dstream.Flush(w)
	dstream.WriteEvent(w, "", openAIRoleDeltaChunk(req.Model))
	dstream.Flush(w)
	for event := range events {
		if event.Type == protocol.StreamError {
			dstream.WriteEvent(w, "", openAIStreamError(event.Err))
			dstream.Flush(w)
			return
		}
		if event.Type == protocol.StreamTextDelta && event.Text != "" {
			dstream.WriteEvent(w, "", openAITextDeltaChunk(req.Model, event.Text))
			dstream.Flush(w)
			continue
		}
		if event.Type == protocol.StreamThinking && event.Text != "" {
			dstream.WriteEvent(w, "", openAIReasoningDeltaChunk(req.Model, event.Text))
			dstream.Flush(w)
			continue
		}
		if event.Type == protocol.StreamToolCall && event.ToolInput != "" {
			dstream.WriteEvent(w, "", openAIToolDeltaChunk(req.Model, event))
			dstream.Flush(w)
			continue
		}
		if event.Type == protocol.StreamMessageStop {
			if event.Response != nil {
				dstream.WriteEvent(w, "", openAIStreamChunk(event.Response))
			}
			io.WriteString(w, "data: [DONE]\n\n")
			dstream.Flush(w)
			return
		}
	}
}

func (h *HandlerGroup) streamAnthropic(w http.ResponseWriter, r *http.Request, req *protocol.Request, route models.Route) {
	events, err := h.streamWithRetry(r.Context(), req, route)
	if err != nil {
		gwerror.WriteAnthropic(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(http.StatusOK)
	dstream.Flush(w)
	dstream.WriteEvent(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"type": "message", "id": "", "model": req.Model, "role": "assistant", "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}})
	dstream.Flush(w)
	blockStarted := false
	blockType := ""
	startBlock := func(kind string, event protocol.StreamEvent) {
		if blockStarted {
			return
		}
		blockStarted = true
		blockType = kind
		contentBlock := map[string]any{"type": "text", "text": ""}
		if kind == "tool_use" {
			contentBlock = map[string]any{"type": "tool_use", "id": event.ToolCallID, "name": event.ToolName, "input": map[string]any{}}
		}
		dstream.WriteEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": contentBlock})
	}
	stopBlock := func() {
		if blockStarted {
			dstream.WriteEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
			blockStarted = false
			blockType = ""
		}
	}
	for event := range events {
		if event.Type == protocol.StreamError {
			dstream.WriteEvent(w, "error", map[string]any{"type": "error", "error": map[string]any{"type": "server_error", "message": event.Err.Error()}})
			dstream.Flush(w)
			return
		}
		if event.Type == protocol.StreamTextDelta && event.Text != "" {
			if blockType != "text" {
				stopBlock()
				startBlock("text", event)
			}
			dstream.WriteEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": event.Text}})
			dstream.Flush(w)
			continue
		}
		if event.Type == protocol.StreamToolCall && event.ToolInput != "" {
			if blockType != "tool_use" {
				stopBlock()
				startBlock("tool_use", event)
			}
			dstream.WriteEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": event.ToolInput}})
			dstream.Flush(w)
			continue
		}
		if event.Type == protocol.StreamMessageStop {
			stopBlock()
			if event.Response != nil {
				usage := map[string]any{"input_tokens": event.Response.Usage.InputTokens, "output_tokens": event.Response.Usage.OutputTokens}
				addCacheUsageFields(usage, event.Response.Usage)
				dstream.WriteEvent(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": event.Response.StopReason, "stop_sequence": nil}, "usage": usage})
			}
			dstream.WriteEvent(w, "message_stop", map[string]any{"type": "message_stop"})
			dstream.Flush(w)
			return
		}
	}
}

func (h *HandlerGroup) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *HandlerGroup) writeOpenAICompletion(w http.ResponseWriter, resp *protocol.Response) {
	choice := map[string]any{
		"index": 0,
		"message": map[string]any{
			"role":    "assistant",
			"content": protocol.ContentTextValue(resp.Content),
		},
		"finish_reason": protocol.OpenAIStopReason(resp.StopReason),
	}
	if len(resp.ToolCalls) > 0 {
		calls := make([]map[string]any, 0, len(resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			calls = append(calls, map[string]any{
				"id":   call.ID,
				"type": "function",
				"function": map[string]any{
					"name":      call.Name,
					"arguments": string(call.Input),
				},
			})
		}
		choice["message"].(map[string]any)["tool_calls"] = calls
	}
	usage := map[string]any{
		"prompt_tokens":     resp.Usage.InputTokens,
		"completion_tokens": resp.Usage.OutputTokens,
		"total_tokens":      resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}
	addCacheUsageFields(usage, resp.Usage)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"id":      resp.ID,
		"object":  "chat.completion",
		"model":   resp.Model,
		"created": time.Now().Unix(),
		"choices": []any{choice},
		"usage":   usage,
	})
}

func (h *HandlerGroup) writeAnthropicMessage(w http.ResponseWriter, resp *protocol.Response) {
	content := make([]map[string]any, 0, len(resp.Content)+len(resp.ToolCalls))
	for _, block := range resp.Content {
		switch block.Type {
		case protocol.ContentText:
			content = append(content, map[string]any{"type": "text", "text": block.Text})
		}
	}
	for _, call := range resp.ToolCalls {
		content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": call.Input})
	}
	if len(content) == 0 {
		content = append(content, map[string]any{"type": "text", "text": ""})
	}
	usage := map[string]any{
		"input_tokens":  resp.Usage.InputTokens,
		"output_tokens": resp.Usage.OutputTokens,
	}
	addCacheUsageFields(usage, resp.Usage)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"id":          resp.ID,
		"type":        "message",
		"role":        resp.Role,
		"content":     content,
		"model":       resp.Model,
		"stop_reason": resp.StopReason,
		"usage":       usage,
	})
}

func openAIRoleDeltaChunk(model string) map[string]any {
	return map[string]any{
		"object": "chat.completion.chunk",
		"model":  model,
		"choices": []any{map[string]any{
			"index":         0,
			"delta":         map[string]any{"role": "assistant"},
			"finish_reason": nil,
		}},
	}
}

func openAITextDeltaChunk(model, text string) map[string]any {
	return map[string]any{
		"object": "chat.completion.chunk",
		"model":  model,
		"choices": []any{map[string]any{
			"index":         0,
			"delta":         map[string]any{"content": text},
			"finish_reason": nil,
		}},
	}
}

func openAIReasoningDeltaChunk(model, text string) map[string]any {
	return map[string]any{
		"object": "chat.completion.chunk",
		"model":  model,
		"choices": []any{map[string]any{
			"index":         0,
			"delta":         map[string]any{"reasoning_content": text},
			"finish_reason": nil,
		}},
	}
}

func openAIToolDeltaChunk(model string, event protocol.StreamEvent) map[string]any {
	return map[string]any{
		"object": "chat.completion.chunk",
		"model":  model,
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0,
				"id":    event.ToolCallID,
				"type":  "function",
				"function": map[string]any{
					"name":      event.ToolName,
					"arguments": event.ToolInput,
				},
			}}},
			"finish_reason": nil,
		}},
	}
}

func openAIStreamChunk(resp *protocol.Response) map[string]any {
	choice := map[string]any{
		"index": 0,
		"delta": map[string]any{
			"role":    "assistant",
			"content": "",
		},
		"finish_reason": protocol.OpenAIStopReason(resp.StopReason),
	}
	usage := map[string]any{
		"prompt_tokens":     resp.Usage.InputTokens,
		"completion_tokens": resp.Usage.OutputTokens,
		"total_tokens":      resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}
	addCacheUsageFields(usage, resp.Usage)
	return map[string]any{
		"id":      resp.ID,
		"object":  "chat.completion.chunk",
		"model":   resp.Model,
		"choices": []any{choice},
		"usage":   usage,
	}
}

func addCacheUsageFields(usage map[string]any, u protocol.Usage) {
	if u.CacheCreationInputTokens > 0 {
		usage["cache_creation_input_tokens"] = u.CacheCreationInputTokens
	}
	if u.CacheReadInputTokens > 0 {
		usage["cache_read_input_tokens"] = u.CacheReadInputTokens
	}
	if u.CacheCreation5mInputTokens > 0 || u.CacheCreation1hInputTokens > 0 {
		cc := map[string]any{}
		if u.CacheCreation5mInputTokens > 0 {
			cc["ephemeral_5m_input_tokens"] = u.CacheCreation5mInputTokens
		}
		if u.CacheCreation1hInputTokens > 0 {
			cc["ephemeral_1h_input_tokens"] = u.CacheCreation1hInputTokens
		}
		usage["cache_creation"] = cc
	}
}

func openAIStreamError(err error) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"type":    "server_error",
			"code":    nil,
		},
	}
}
