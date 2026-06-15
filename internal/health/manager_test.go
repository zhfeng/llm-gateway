package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/protocol"
	"github.com/zhfeng/llm-gateway/internal/provider"
)

type fakeProvider struct {
	err error
}

func (f fakeProvider) Name() string { return "p1" }
func (f fakeProvider) Type() string { return config.ProviderOpenAICompatible }
func (f fakeProvider) Complete(context.Context, *protocol.Request) (*protocol.Response, error) {
	return nil, nil
}
func (f fakeProvider) Stream(context.Context, *protocol.Request) (<-chan protocol.StreamEvent, error) {
	return nil, nil
}
func (f fakeProvider) ListModels(context.Context) ([]protocol.ModelInfo, error) { return nil, nil }
func (f fakeProvider) HealthCheck(context.Context) error                        { return f.err }

func TestDisabledManagerTreatsProvidersRoutable(t *testing.T) {
	m := NewManager(nil, Config{})
	if !m.IsRoutable("p1") || !m.Healthy() {
		t.Fatal("disabled manager should be routable and healthy")
	}
}

func TestManagerThresholds(t *testing.T) {
	m := NewManager(nil, Config{Enabled: true, FailureThreshold: 2, SuccessThreshold: 2, ProviderEnabled: map[string]bool{"p1": true}})
	err := errors.New("down")
	m.RecordCheck("p1", err, time.Millisecond)
	if !m.IsRoutable("p1") {
		t.Fatal("first failure should not exceed threshold")
	}
	m.RecordCheck("p1", err, time.Millisecond)
	if m.IsRoutable("p1") {
		t.Fatal("second failure should mark unhealthy")
	}
	m.RecordCheck("p1", nil, time.Millisecond)
	if m.IsRoutable("p1") {
		t.Fatal("first success should not reach success threshold")
	}
	m.RecordCheck("p1", nil, time.Millisecond)
	if !m.IsRoutable("p1") {
		t.Fatal("second success should mark healthy")
	}
}

func TestCheckAllRecordsProviderHealth(t *testing.T) {
	m := NewManager(map[string]provider.Provider{"p1": fakeProvider{}}, Config{Enabled: true, Timeout: time.Second, FailureThreshold: 1, SuccessThreshold: 1, ProviderEnabled: map[string]bool{"p1": true}})
	m.CheckAll(context.Background())
	statuses := m.Snapshot()
	if len(statuses) != 1 || statuses[0].State != StateHealthy || statuses[0].LastCheckedAt.IsZero() {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}
