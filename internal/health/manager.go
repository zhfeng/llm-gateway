package health

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/zhfeng/llm-gateway/internal/provider"
)

type State string

const (
	StateUnknown   State = "unknown"
	StateHealthy   State = "healthy"
	StateUnhealthy State = "unhealthy"
)

type Config struct {
	Enabled          bool
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
	SuccessThreshold int
	ProviderEnabled  map[string]bool
}

type ProviderStatus struct {
	Name             string        `json:"name"`
	State            State         `json:"state"`
	LastCheckedAt    time.Time     `json:"last_checked_at,omitempty"`
	LastHealthyAt    time.Time     `json:"last_healthy_at,omitempty"`
	LastError        string        `json:"last_error,omitempty"`
	ConsecutiveFails int           `json:"consecutive_fails"`
	ConsecutiveOKs   int           `json:"consecutive_oks"`
	Latency          time.Duration `json:"-"`
	LatencyMS        int64         `json:"latency_ms"`
}

type Manager struct {
	enabled          bool
	interval         time.Duration
	timeout          time.Duration
	failureThreshold int
	successThreshold int
	providers        map[string]provider.Provider
	providerEnabled  map[string]bool
	mu               sync.RWMutex
	statuses         map[string]ProviderStatus
}

func NewManager(providers map[string]provider.Provider, cfg Config) *Manager {
	m := &Manager{
		enabled:          cfg.Enabled,
		interval:         cfg.Interval,
		timeout:          cfg.Timeout,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		providers:        providers,
		providerEnabled:  cfg.ProviderEnabled,
		statuses:         map[string]ProviderStatus{},
	}
	for name := range providers {
		m.statuses[name] = ProviderStatus{Name: name, State: StateUnknown}
	}
	return m
}

func (m *Manager) Start(ctx context.Context) {
	if !m.enabled || m.interval <= 0 {
		return
	}
	m.CheckAll(ctx)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.CheckAll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) CheckAll(ctx context.Context) {
	for name, p := range m.providers {
		if !m.isProviderHealthEnabled(name) {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, m.timeout)
		start := time.Now()
		err := p.HealthCheck(checkCtx)
		cancel()
		m.RecordCheck(name, err, time.Since(start))
	}
}

func (m *Manager) RecordResult(providerName string, err error) {
	if !m.enabled || !m.isProviderHealthEnabled(providerName) {
		return
	}
	m.RecordCheck(providerName, err, 0)
}

func (m *Manager) RecordCheck(providerName string, err error, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.statuses[providerName]
	if status.Name == "" {
		status.Name = providerName
		status.State = StateUnknown
	}
	status.LastCheckedAt = time.Now()
	status.Latency = latency
	status.LatencyMS = latency.Milliseconds()
	if err != nil {
		status.ConsecutiveFails++
		status.ConsecutiveOKs = 0
		status.LastError = err.Error()
		if status.ConsecutiveFails >= m.failureThreshold {
			status.State = StateUnhealthy
		}
	} else {
		status.ConsecutiveOKs++
		status.ConsecutiveFails = 0
		status.LastError = ""
		status.LastHealthyAt = status.LastCheckedAt
		if status.ConsecutiveOKs >= m.successThreshold {
			status.State = StateHealthy
		}
	}
	m.statuses[providerName] = status
}

func (m *Manager) IsRoutable(providerName string) bool {
	if m == nil || !m.enabled || !m.isProviderHealthEnabled(providerName) {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.statuses[providerName]
	return status.State != StateUnhealthy
}

func (m *Manager) Snapshot() []ProviderStatus {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ProviderStatus, 0, len(m.statuses))
	for _, status := range m.statuses {
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) Healthy() bool {
	if m == nil || !m.enabled {
		return true
	}
	for _, status := range m.Snapshot() {
		if m.isProviderHealthEnabled(status.Name) && status.State == StateUnhealthy {
			return false
		}
	}
	return true
}

func (m *Manager) Enabled() bool {
	return m != nil && m.enabled
}

func (m *Manager) isProviderHealthEnabled(name string) bool {
	enabled, ok := m.providerEnabled[name]
	return ok && enabled
}
