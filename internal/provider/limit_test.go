package provider

import (
	"context"
	"testing"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/protocol"
)

type blockingProvider struct {
	block       chan struct{}
	entered     chan struct{}
	healthCalls int
}

func (p *blockingProvider) Name() string                                             { return "p1" }
func (p *blockingProvider) Type() string                                             { return config.ProviderOpenAICompatible }
func (p *blockingProvider) ListModels(context.Context) ([]protocol.ModelInfo, error) { return nil, nil }
func (p *blockingProvider) HealthCheck(context.Context) error {
	p.healthCalls++
	return nil
}
func (p *blockingProvider) Complete(context.Context, *protocol.Request) (*protocol.Response, error) {
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	<-p.block
	return &protocol.Response{Model: "m"}, nil
}
func (p *blockingProvider) Stream(context.Context, *protocol.Request) (<-chan protocol.StreamEvent, error) {
	ch := make(chan protocol.StreamEvent)
	go func() {
		<-p.block
		close(ch)
	}()
	return ch, nil
}

func TestConcurrencyLimitRejectsSecondComplete(t *testing.T) {
	base := &blockingProvider{block: make(chan struct{}), entered: make(chan struct{}, 1)}
	limited := WithConcurrencyLimit(base, 1)
	firstDone := make(chan struct{})
	go func() {
		limited.Complete(context.Background(), &protocol.Request{})
		close(firstDone)
	}()
	<-base.entered
	if _, err := limited.Complete(context.Background(), &protocol.Request{}); !IsAdmissionError(err) {
		t.Fatalf("expected admission error, got %v", err)
	}
	close(base.block)
	<-firstDone
	if _, err := limited.Complete(context.Background(), &protocol.Request{}); err != nil {
		t.Fatalf("slot was not released: %v", err)
	}
}

func TestConcurrencyLimitHoldsStreamSlotUntilClose(t *testing.T) {
	base := &blockingProvider{block: make(chan struct{})}
	limited := WithConcurrencyLimit(base, 1)
	events, err := limited.Stream(context.Background(), &protocol.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Stream(context.Background(), &protocol.Request{}); !IsAdmissionError(err) {
		t.Fatalf("expected admission error, got %v", err)
	}
	close(base.block)
	for range events {
	}
	if _, err := limited.Stream(context.Background(), &protocol.Request{}); err != nil {
		t.Fatalf("slot was not released: %v", err)
	}
}

func TestConcurrencyLimitBypassesHealthCheck(t *testing.T) {
	base := &blockingProvider{block: make(chan struct{}), entered: make(chan struct{}, 1)}
	limited := WithConcurrencyLimit(base, 1)
	go limited.Complete(context.Background(), &protocol.Request{})
	<-base.entered
	if err := limited.HealthCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	if base.healthCalls != 1 {
		t.Fatalf("health calls = %d", base.healthCalls)
	}
	close(base.block)
}

func TestConcurrencyLimitDisabledReturnsBase(t *testing.T) {
	base := &blockingProvider{block: make(chan struct{})}
	if got := WithConcurrencyLimit(base, 0); got != base {
		t.Fatal("expected base provider")
	}
}

func TestAdmissionErrorUnwraps(t *testing.T) {
	err := newConcurrencyLimitError("p1")
	if !IsAdmissionError(err) {
		t.Fatalf("unexpected admission error: %v", err)
	}
}
