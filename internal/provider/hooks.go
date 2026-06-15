package provider

import (
	"context"
	"time"

	"github.com/zhfeng/llm-gateway/internal/protocol"
)

type CallKind string

const (
	CallKindComplete CallKind = "complete"
	CallKindStream   CallKind = "stream"
)

type CallInfo struct {
	ProviderName string
	ProviderType string
	Kind         CallKind
	Request      *protocol.Request
	StartedAt    time.Time
}

type CallResult struct {
	Response *protocol.Response
	Err      error
	EndedAt  time.Time
}

type Hook interface {
	BeforeProviderCall(ctx context.Context, info CallInfo) error
	AfterProviderCall(ctx context.Context, info CallInfo, result CallResult)
}

func WithHooks(base Provider, hooks ...Hook) Provider {
	if base == nil || len(hooks) == 0 {
		return base
	}
	return &hookedProvider{base: base, hooks: hooks}
}

type hookedProvider struct {
	base  Provider
	hooks []Hook
}

func (p *hookedProvider) Name() string { return p.base.Name() }
func (p *hookedProvider) Type() string { return p.base.Type() }

func (p *hookedProvider) ListModels(ctx context.Context) ([]protocol.ModelInfo, error) {
	return p.base.ListModels(ctx)
}

func (p *hookedProvider) HealthCheck(ctx context.Context) error {
	return p.base.HealthCheck(ctx)
}

func (p *hookedProvider) Complete(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	info := p.callInfo(CallKindComplete, req)
	if err := p.before(ctx, info); err != nil {
		return nil, err
	}
	resp, err := p.base.Complete(ctx, req)
	p.after(ctx, info, CallResult{Response: resp, Err: err, EndedAt: time.Now()})
	return resp, err
}

func (p *hookedProvider) Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	info := p.callInfo(CallKindStream, req)
	if err := p.before(ctx, info); err != nil {
		return nil, err
	}
	events, err := p.base.Stream(ctx, req)
	if err != nil {
		p.after(ctx, info, CallResult{Err: err, EndedAt: time.Now()})
		return nil, err
	}
	out := make(chan protocol.StreamEvent)
	go p.forwardStream(ctx, info, events, out)
	return out, nil
}

func (p *hookedProvider) callInfo(kind CallKind, req *protocol.Request) CallInfo {
	return CallInfo{ProviderName: p.base.Name(), ProviderType: p.base.Type(), Kind: kind, Request: req, StartedAt: time.Now()}
}

func (p *hookedProvider) before(ctx context.Context, info CallInfo) error {
	for _, hook := range p.hooks {
		if err := hook.BeforeProviderCall(ctx, info); err != nil {
			return err
		}
	}
	return nil
}

func (p *hookedProvider) after(ctx context.Context, info CallInfo, result CallResult) {
	for _, hook := range p.hooks {
		hook.AfterProviderCall(ctx, info, result)
	}
}

func (p *hookedProvider) forwardStream(ctx context.Context, info CallInfo, in <-chan protocol.StreamEvent, out chan<- protocol.StreamEvent) {
	defer close(out)
	var resp *protocol.Response
	var err error
	defer func() {
		p.after(ctx, info, CallResult{Response: resp, Err: err, EndedAt: time.Now()})
	}()
	for {
		select {
		case event, ok := <-in:
			if !ok {
				return
			}
			if event.Type == protocol.StreamMessageStop {
				resp = event.Response
			}
			if event.Type == protocol.StreamError {
				err = event.Err
			}
			select {
			case out <- event:
			case <-ctx.Done():
				err = ctx.Err()
				return
			}
		case <-ctx.Done():
			err = ctx.Err()
			return
		}
	}
}
