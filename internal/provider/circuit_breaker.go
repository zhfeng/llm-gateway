package provider

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/gwerror"
	"github.com/zhfeng/llm-gateway/internal/protocol"
)

type circuitState string

const (
	circuitClosed   circuitState = "closed"
	circuitOpen     circuitState = "open"
	circuitHalfOpen circuitState = "half_open"
)

func WithCircuitBreaker(base Provider, cfg config.CircuitBreakerRuntime) Provider {
	if base == nil || !cfg.Enabled {
		return base
	}
	return &circuitBreakerProvider{base: base, failureThreshold: cfg.FailureThreshold, successThreshold: cfg.SuccessThreshold, openTimeout: cfg.OpenTimeout, state: circuitClosed}
}

type circuitBreakerProvider struct {
	base             Provider
	failureThreshold int
	successThreshold int
	openTimeout      time.Duration
	mu               sync.Mutex
	state            circuitState
	openedAt         time.Time
	failures         int
	successes        int
	trialInFlight    bool
}

func (p *circuitBreakerProvider) Name() string { return p.base.Name() }
func (p *circuitBreakerProvider) Type() string { return p.base.Type() }
func (p *circuitBreakerProvider) ListModels(ctx context.Context) ([]protocol.ModelInfo, error) {
	return p.base.ListModels(ctx)
}
func (p *circuitBreakerProvider) HealthCheck(ctx context.Context) error {
	return p.base.HealthCheck(ctx)
}

func (p *circuitBreakerProvider) Complete(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	finish, err := p.allow()
	if err != nil {
		return nil, err
	}
	resp, err := p.base.Complete(ctx, req)
	finish(err)
	return resp, err
}

func (p *circuitBreakerProvider) Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	finish, err := p.allow()
	if err != nil {
		return nil, err
	}
	events, err := p.base.Stream(ctx, req)
	if err != nil {
		finish(err)
		return nil, err
	}
	out := make(chan protocol.StreamEvent)
	go func() {
		defer close(out)
		var streamErr error
		defer func() { finish(streamErr) }()
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.Type == protocol.StreamError {
					streamErr = event.Err
				}
				select {
				case out <- event:
				case <-ctx.Done():
					streamErr = ctx.Err()
					return
				}
			case <-ctx.Done():
				streamErr = ctx.Err()
				return
			}
		}
	}()
	return out, nil
}

func (p *circuitBreakerProvider) allow() (func(error), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == circuitOpen {
		if time.Since(p.openedAt) < p.openTimeout {
			return nil, newCircuitOpenError(p.Name())
		}
		p.state = circuitHalfOpen
		p.successes = 0
		p.failures = 0
	}
	if p.state == circuitHalfOpen {
		if p.trialInFlight {
			return nil, newCircuitOpenError(p.Name())
		}
		p.trialInFlight = true
	}
	return p.recordResult, nil
}

func (p *circuitBreakerProvider) recordResult(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == circuitHalfOpen {
		p.trialInFlight = false
	}
	if !isCircuitFailure(err) {
		p.failures = 0
		p.successes++
		if p.state == circuitHalfOpen && p.successes >= p.successThreshold {
			p.state = circuitClosed
			p.successes = 0
		}
		return
	}
	p.successes = 0
	p.failures++
	if p.state == circuitHalfOpen || p.failures >= p.failureThreshold {
		p.state = circuitOpen
		p.openedAt = time.Now()
		p.failures = 0
	}
}

func isCircuitFailure(err error) bool {
	if err == nil || IsAdmissionError(err) || errors.Is(err, context.Canceled) {
		return false
	}
	var ge *gwerror.Error
	if errors.As(err, &ge) {
		return ge.Status == 408 || ge.Status == 429 || ge.Status >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
