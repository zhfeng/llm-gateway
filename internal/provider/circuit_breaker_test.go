package provider

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/gwerror"
	"github.com/zhfeng/llm-gateway/internal/protocol"
)

type breakerProvider struct {
	err    error
	events []protocol.StreamEvent
}

func (p *breakerProvider) Name() string                                             { return "p1" }
func (p *breakerProvider) Type() string                                             { return config.ProviderOpenAICompatible }
func (p *breakerProvider) ListModels(context.Context) ([]protocol.ModelInfo, error) { return nil, nil }
func (p *breakerProvider) HealthCheck(context.Context) error                        { return nil }
func (p *breakerProvider) Complete(context.Context, *protocol.Request) (*protocol.Response, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &protocol.Response{Model: "m"}, nil
}
func (p *breakerProvider) Stream(context.Context, *protocol.Request) (<-chan protocol.StreamEvent, error) {
	ch := make(chan protocol.StreamEvent, len(p.events))
	for _, event := range p.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func TestCircuitBreakerOpensAfterFailures(t *testing.T) {
	base := &breakerProvider{err: gwerror.New(http.StatusServiceUnavailable, "provider_error", "down")}
	wrapped := WithCircuitBreaker(base, config.CircuitBreakerRuntime{Enabled: true, FailureThreshold: 2, SuccessThreshold: 1, OpenTimeout: time.Hour})
	for i := 0; i < 2; i++ {
		if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); err == nil {
			t.Fatal("expected provider error")
		}
	}
	if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); !IsAdmissionError(err) {
		t.Fatalf("expected circuit admission error, got %v", err)
	}
}

func TestCircuitBreakerHalfOpenClosesOnSuccess(t *testing.T) {
	base := &breakerProvider{err: gwerror.New(http.StatusServiceUnavailable, "provider_error", "down")}
	wrapped := WithCircuitBreaker(base, config.CircuitBreakerRuntime{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, OpenTimeout: time.Millisecond})
	wrapped.Complete(context.Background(), &protocol.Request{})
	time.Sleep(2 * time.Millisecond)
	base.err = nil
	if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); err != nil {
		t.Fatalf("half-open success failed: %v", err)
	}
	if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); err != nil {
		t.Fatalf("closed circuit rejected request: %v", err)
	}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	base := &breakerProvider{err: gwerror.New(http.StatusServiceUnavailable, "provider_error", "down")}
	wrapped := WithCircuitBreaker(base, config.CircuitBreakerRuntime{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, OpenTimeout: time.Millisecond})
	wrapped.Complete(context.Background(), &protocol.Request{})
	time.Sleep(2 * time.Millisecond)
	wrapped.Complete(context.Background(), &protocol.Request{})
	if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); !IsAdmissionError(err) {
		t.Fatalf("expected reopened circuit, got %v", err)
	}
}

func TestCircuitBreakerRecordsStreamError(t *testing.T) {
	base := &breakerProvider{events: []protocol.StreamEvent{{Type: protocol.StreamError, Err: gwerror.New(http.StatusServiceUnavailable, "provider_error", "down")}}}
	wrapped := WithCircuitBreaker(base, config.CircuitBreakerRuntime{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, OpenTimeout: time.Hour})
	events, err := wrapped.Stream(context.Background(), &protocol.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); !IsAdmissionError(err) {
		t.Fatalf("expected open circuit, got %v", err)
	}
}

func TestCircuitBreakerSuccessfulStreamCountsAsSuccess(t *testing.T) {
	base := &breakerProvider{err: gwerror.New(http.StatusServiceUnavailable, "provider_error", "down")}
	wrapped := WithCircuitBreaker(base, config.CircuitBreakerRuntime{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, OpenTimeout: time.Millisecond})
	wrapped.Complete(context.Background(), &protocol.Request{})
	time.Sleep(2 * time.Millisecond)
	base.err = nil
	base.events = []protocol.StreamEvent{{Type: protocol.StreamMessageStop, Response: &protocol.Response{Model: "m"}}}
	events, err := wrapped.Stream(context.Background(), &protocol.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	if _, err := wrapped.Complete(context.Background(), &protocol.Request{}); err != nil {
		t.Fatalf("closed circuit rejected request: %v", err)
	}
}
