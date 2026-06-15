package provider

import (
	"context"

	"github.com/zhfeng/llm-gateway/internal/protocol"
)

func WithConcurrencyLimit(base Provider, max int) Provider {
	if base == nil || max <= 0 {
		return base
	}
	return &limitedProvider{base: base, sem: make(chan struct{}, max)}
}

type limitedProvider struct {
	base Provider
	sem  chan struct{}
}

func (p *limitedProvider) Name() string { return p.base.Name() }
func (p *limitedProvider) Type() string { return p.base.Type() }
func (p *limitedProvider) ListModels(ctx context.Context) ([]protocol.ModelInfo, error) {
	return p.base.ListModels(ctx)
}
func (p *limitedProvider) HealthCheck(ctx context.Context) error { return p.base.HealthCheck(ctx) }

func (p *limitedProvider) Complete(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	release, err := p.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	return p.base.Complete(ctx, req)
}

func (p *limitedProvider) Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	release, err := p.acquire()
	if err != nil {
		return nil, err
	}
	events, err := p.base.Stream(ctx, req)
	if err != nil {
		release()
		return nil, err
	}
	out := make(chan protocol.StreamEvent)
	go func() {
		defer release()
		defer close(out)
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (p *limitedProvider) acquire() (func(), error) {
	select {
	case p.sem <- struct{}{}:
		return func() { <-p.sem }, nil
	default:
		return nil, newConcurrencyLimitError(p.Name())
	}
}
