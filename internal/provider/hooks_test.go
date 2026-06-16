package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/protocol"
)

type hookTestProvider struct {
	completeResp *protocol.Response
	completeErr  error
	streamEvents []protocol.StreamEvent
	streamErr    error
	calls        *[]string
}

func (p hookTestProvider) Name() string                                             { return "test-provider" }
func (p hookTestProvider) Type() string                                             { return config.ProviderOpenAICompatible }
func (p hookTestProvider) ListModels(context.Context) ([]protocol.ModelInfo, error) { return nil, nil }
func (p hookTestProvider) HealthCheck(context.Context) error                        { return nil }

func (p hookTestProvider) Complete(context.Context, *protocol.Request) (*protocol.Response, error) {
	*p.calls = append(*p.calls, "complete")
	return p.completeResp, p.completeErr
}

func (p hookTestProvider) Stream(context.Context, *protocol.Request) (<-chan protocol.StreamEvent, error) {
	*p.calls = append(*p.calls, "stream")
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	ch := make(chan protocol.StreamEvent, len(p.streamEvents))
	for _, event := range p.streamEvents {
		ch <- event
	}
	close(ch)
	return ch, nil
}

type recordingHook struct {
	beforeErr error
	calls     *[]string
	before    []CallInfo
	after     []CallResult
}

func (h *recordingHook) BeforeProviderCall(ctx context.Context, info CallInfo) error {
	*h.calls = append(*h.calls, "before")
	h.before = append(h.before, info)
	return h.beforeErr
}

func (h *recordingHook) AfterProviderCall(ctx context.Context, info CallInfo, result CallResult) {
	*h.calls = append(*h.calls, "after")
	h.after = append(h.after, result)
}

func TestHookedProviderCompleteLifecycle(t *testing.T) {
	calls := []string{}
	req := &protocol.Request{Model: "m"}
	resp := &protocol.Response{Model: "m"}
	hook := &recordingHook{calls: &calls}
	prov := WithHooks(hookTestProvider{completeResp: resp, calls: &calls}, hook)

	got, err := prov.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got != resp {
		t.Fatal("response was not preserved")
	}
	if !reflect.DeepEqual(calls, []string{"before", "complete", "after"}) {
		t.Fatalf("calls = %v", calls)
	}
	if len(hook.before) != 1 || hook.before[0].ProviderName != "test-provider" || hook.before[0].ProviderType != config.ProviderOpenAICompatible || hook.before[0].Kind != CallKindComplete || hook.before[0].Request != req {
		t.Fatalf("unexpected before info: %+v", hook.before)
	}
	if len(hook.after) != 1 || hook.after[0].Response != resp || hook.after[0].Err != nil {
		t.Fatalf("unexpected after result: %+v", hook.after)
	}
}

func TestHookedProviderCompleteBeforeRejects(t *testing.T) {
	calls := []string{}
	rejectErr := errors.New("rate limited")
	hook := &recordingHook{beforeErr: rejectErr, calls: &calls}
	prov := WithHooks(hookTestProvider{calls: &calls}, hook)

	_, err := prov.Complete(context.Background(), &protocol.Request{Model: "m"})
	if !errors.Is(err, rejectErr) {
		t.Fatalf("err = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"before"}) {
		t.Fatalf("calls = %v", calls)
	}
	if len(hook.after) != 0 {
		t.Fatalf("after should not run: %+v", hook.after)
	}
}

func TestHookedProviderCompleteErrorReachesAfter(t *testing.T) {
	calls := []string{}
	providerErr := errors.New("provider failed")
	hook := &recordingHook{calls: &calls}
	prov := WithHooks(hookTestProvider{completeErr: providerErr, calls: &calls}, hook)

	_, err := prov.Complete(context.Background(), &protocol.Request{Model: "m"})
	if !errors.Is(err, providerErr) {
		t.Fatalf("err = %v", err)
	}
	if len(hook.after) != 1 || !errors.Is(hook.after[0].Err, providerErr) {
		t.Fatalf("unexpected after result: %+v", hook.after)
	}
}

func TestHookedProviderStreamLifecycle(t *testing.T) {
	calls := []string{}
	resp := &protocol.Response{Model: "m"}
	events := []protocol.StreamEvent{{Type: protocol.StreamTextDelta, Text: "hi"}, {Type: protocol.StreamMessageStop, Response: resp}}
	hook := &recordingHook{calls: &calls}
	prov := WithHooks(hookTestProvider{streamEvents: events, calls: &calls}, hook)

	ch, err := prov.Stream(context.Background(), &protocol.Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var got []protocol.StreamEvent
	for event := range ch {
		got = append(got, event)
	}
	if !reflect.DeepEqual(got, events) {
		t.Fatalf("events = %+v", got)
	}
	if !reflect.DeepEqual(calls, []string{"before", "stream", "after"}) {
		t.Fatalf("calls = %v", calls)
	}
	if len(hook.after) != 1 || hook.after[0].Response != resp || hook.after[0].Err != nil {
		t.Fatalf("unexpected after result: %+v", hook.after)
	}
}

func TestHookedProviderStreamImmediateErrorReachesAfter(t *testing.T) {
	calls := []string{}
	streamErr := errors.New("stream open failed")
	hook := &recordingHook{calls: &calls}
	prov := WithHooks(hookTestProvider{streamErr: streamErr, calls: &calls}, hook)

	ch, err := prov.Stream(context.Background(), &protocol.Request{Model: "m"})
	if ch != nil {
		t.Fatal("expected nil channel")
	}
	if !errors.Is(err, streamErr) {
		t.Fatalf("err = %v", err)
	}
	if len(hook.after) != 1 || !errors.Is(hook.after[0].Err, streamErr) {
		t.Fatalf("unexpected after result: %+v", hook.after)
	}
}

func TestForwardingLogHookLifecycle(t *testing.T) {
	hook := NewForwardingLogHook()
	info := CallInfo{ProviderName: "test-provider", ProviderType: config.ProviderOpenAICompatible, Kind: CallKindComplete, Request: &protocol.Request{Model: "m", ProviderModel: "pm"}}
	if err := hook.BeforeProviderCall(context.Background(), info); err != nil {
		t.Fatal(err)
	}
	hook.AfterProviderCall(context.Background(), info, CallResult{Err: errors.New("provider failed")})
}

func TestHookedProviderStreamEventErrorReachesAfter(t *testing.T) {
	calls := []string{}
	streamErr := errors.New("stream event failed")
	events := []protocol.StreamEvent{{Type: protocol.StreamError, Err: streamErr}}
	hook := &recordingHook{calls: &calls}
	prov := WithHooks(hookTestProvider{streamEvents: events, calls: &calls}, hook)

	ch, err := prov.Stream(context.Background(), &protocol.Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var got []protocol.StreamEvent
	for event := range ch {
		got = append(got, event)
	}
	if !reflect.DeepEqual(got, events) {
		t.Fatalf("events = %+v", got)
	}
	if len(hook.after) != 1 || !errors.Is(hook.after[0].Err, streamErr) {
		t.Fatalf("unexpected after result: %+v", hook.after)
	}
}
