package models

import (
	"context"
	"testing"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/protocol"
	"github.com/zhfeng/llm-gateway/internal/provider"
)

type fakeProvider struct {
	name   string
	models []protocol.ModelInfo
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Type() string { return config.ProviderOpenAICompatible }
func (f fakeProvider) Complete(context.Context, *protocol.Request) (*protocol.Response, error) {
	return nil, nil
}
func (f fakeProvider) Stream(context.Context, *protocol.Request) (<-chan protocol.StreamEvent, error) {
	return nil, nil
}
func (f fakeProvider) ListModels(context.Context) ([]protocol.ModelInfo, error) { return f.models, nil }
func (f fakeProvider) HealthCheck(context.Context) error                        { return nil }

func TestRegistryDoesNotPassthroughUnknownModel(t *testing.T) {
	cfg := config.Config{
		Providers: []config.ProviderConfig{{Name: "p1", Type: config.ProviderOpenAICompatible}},
		Models:    map[string]config.ModelRoute{},
	}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1"}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	if _, ok := registry.Resolve("missing"); ok {
		t.Fatal("expected unknown model not to resolve")
	}
}

func TestRegistryStaticWinsDynamicConflict(t *testing.T) {
	cfg := config.Config{
		ModelDiscovery: config.ModelDiscoveryConfig{StaleWhileRevalidate: true},
		Providers:      []config.ProviderConfig{{Name: "p1", Type: config.ProviderOpenAICompatible, DiscoverModels: true}},
		Models:         map[string]config.ModelRoute{"model-a": {Provider: "p1", ProviderModel: "static-model"}},
	}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1", models: []protocol.ModelInfo{{ID: "model-a"}}}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	if err := registry.RefreshProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	route, ok := registry.Resolve("model-a")
	if !ok {
		t.Fatal("expected model-a to resolve")
	}
	if route.ProviderModel != "static-model" || route.Source != "static" {
		t.Fatalf("static route did not win: %+v", route)
	}
}

func TestRegistryFiltersDynamicModels(t *testing.T) {
	cfg := config.Config{
		ModelDiscovery: config.ModelDiscoveryConfig{StaleWhileRevalidate: true},
		Providers: []config.ProviderConfig{{
			Name:           "p1",
			Type:           config.ProviderOpenAICompatible,
			DiscoverModels: true,
			ModelPrefix:    "p1/",
			IncludeModels:  []string{"doubao-*", "deepseek-*"},
			ExcludeModels:  []string{"*-embedding-*"},
		}},
		Models: map[string]config.ModelRoute{},
	}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1", models: []protocol.ModelInfo{{ID: "doubao-pro"}, {ID: "doubao-embedding-text"}, {ID: "deepseek-v3"}, {ID: "qwen3"}}}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	if err := registry.RefreshProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve("p1/doubao-pro"); !ok {
		t.Fatal("expected included doubao model")
	}
	if _, ok := registry.Resolve("p1/deepseek-v3"); !ok {
		t.Fatal("expected included deepseek model")
	}
	if _, ok := registry.Resolve("p1/doubao-embedding-text"); ok {
		t.Fatal("expected excluded embedding model not to resolve")
	}
	if _, ok := registry.Resolve("p1/qwen3"); ok {
		t.Fatal("expected non-included qwen model not to resolve")
	}
}

func TestRegistryAppliesDynamicPrefix(t *testing.T) {
	cfg := config.Config{
		ModelDiscovery: config.ModelDiscoveryConfig{StaleWhileRevalidate: true},
		Providers:      []config.ProviderConfig{{Name: "p1", Type: config.ProviderOpenAICompatible, DiscoverModels: true, ModelPrefix: "p1/"}},
		Models:         map[string]config.ModelRoute{},
	}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1", models: []protocol.ModelInfo{{ID: "model-a"}}}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	if err := registry.RefreshProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	route, ok := registry.Resolve("p1/model-a")
	if !ok {
		t.Fatal("expected prefixed model to resolve")
	}
	if route.ProviderModel != "model-a" || route.Source != "dynamic" {
		t.Fatalf("unexpected dynamic route: %+v", route)
	}
}

func TestRegistryResolvesWeightedTargets(t *testing.T) {
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "pm1", Weight: 1}}}}}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1"}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	route, ok := registry.Resolve("m")
	if !ok {
		t.Fatal("expected model to resolve")
	}
	if route.ProviderName != "p1" || route.ProviderModel != "pm1" || route.Provider == nil || route.Policy != config.ModelPolicyWeighted {
		t.Fatalf("unexpected route: %+v", route)
	}
}

func TestSelectWeightedTargetBoundaries(t *testing.T) {
	targets := []RouteTarget{{ProviderName: "a", Weight: 2}, {ProviderName: "b", Weight: 3}, {ProviderName: "c", Weight: 5}}
	cases := map[int]string{0: "a", 1: "a", 2: "b", 4: "b", 5: "c", 9: "c"}
	for pick, want := range cases {
		if got := selectWeightedTarget(targets, pick).ProviderName; got != want {
			t.Fatalf("pick %d selected %q, want %q", pick, got, want)
		}
	}
}

func TestRegistryMultiTargetStaticWinsDynamicConflict(t *testing.T) {
	cfg := config.Config{
		ModelDiscovery: config.ModelDiscoveryConfig{StaleWhileRevalidate: true},
		Providers:      []config.ProviderConfig{{Name: "p1", Type: config.ProviderOpenAICompatible, DiscoverModels: true}},
		Models:         map[string]config.ModelRoute{"model-a": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "static-model", Weight: 1}}}},
	}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1", models: []protocol.ModelInfo{{ID: "model-a"}}}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	if err := registry.RefreshProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	route, ok := registry.Resolve("model-a")
	if !ok {
		t.Fatal("expected model-a to resolve")
	}
	if route.ProviderModel != "static-model" || route.Source != "static" {
		t.Fatalf("static route did not win: %+v", route)
	}
}

func TestRegistryModelsListsMultiTargetAliasOnce(t *testing.T) {
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "pm1", Weight: 1}, {Provider: "p2", ProviderModel: "pm2", Weight: 1}}}}}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1"}, "p2": fakeProvider{name: "p2"}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)

	models := registry.Models()
	if len(models) != 1 || models[0].ID != "m" || models[0].OwnedBy != "llm-gateway" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestRegistryStickyKeyReusesTarget(t *testing.T) {
	registry := stickyTestRegistry(time.Hour)
	first, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "session"})
	if !ok {
		t.Fatal("expected model to resolve")
	}
	for i := 0; i < 20; i++ {
		next, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "session"})
		if !ok {
			t.Fatal("expected model to resolve")
		}
		if next.ProviderName != first.ProviderName || next.ProviderModel != first.ProviderModel {
			t.Fatalf("sticky target changed from %+v to %+v", first, next)
		}
	}
}

func TestRegistryStickyDifferentKeysBindIndependently(t *testing.T) {
	registry := stickyTestRegistry(time.Hour)
	a, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "a"})
	if !ok {
		t.Fatal("expected model to resolve")
	}
	b, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "b"})
	if !ok {
		t.Fatal("expected model to resolve")
	}
	if len(registry.sticky) != 2 {
		t.Fatalf("sticky entries = %d; a=%+v b=%+v", len(registry.sticky), a, b)
	}
}

func TestRegistryStickyExpiredBindingReselects(t *testing.T) {
	registry := stickyTestRegistry(time.Hour)
	registry.sticky["m\x00session"] = stickyTarget{ProviderName: "p1", ProviderModel: "pm1", ExpiresAt: time.Now().Add(-time.Second)}
	if _, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "session"}); !ok {
		t.Fatal("expected model to resolve")
	}
	if !time.Now().Before(registry.sticky["m\x00session"].ExpiresAt) {
		t.Fatalf("sticky binding was not refreshed: %+v", registry.sticky["m\x00session"])
	}
}

func TestRegistryStickyInvalidBindingReselects(t *testing.T) {
	registry := stickyTestRegistry(time.Hour)
	registry.sticky["m\x00session"] = stickyTarget{ProviderName: "missing", ProviderModel: "pm", ExpiresAt: time.Now().Add(time.Hour)}
	route, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "session"})
	if !ok {
		t.Fatal("expected model to resolve")
	}
	if route.ProviderName == "missing" {
		t.Fatalf("invalid binding was used: %+v", route)
	}
}

func TestRegistryStickyDoesNotAffectSingleProviderRoutes(t *testing.T) {
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Provider: "p1", ProviderModel: "pm1"}}}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1"}}
	registry := New(cfg, providers, time.Hour, true, time.Hour, 10000)
	route, ok := registry.ResolveWithOptions("m", ResolveOptions{StickyKey: "session"})
	if !ok {
		t.Fatal("expected model to resolve")
	}
	if route.ProviderName != "p1" || len(registry.sticky) != 0 {
		t.Fatalf("unexpected route or sticky entries: route=%+v sticky=%+v", route, registry.sticky)
	}
}

func stickyTestRegistry(ttl time.Duration) *Registry {
	cfg := config.Config{Models: map[string]config.ModelRoute{"m": {Targets: []config.ModelRouteTarget{{Provider: "p1", ProviderModel: "pm1", Weight: 1}, {Provider: "p2", ProviderModel: "pm2", Weight: 1}}}}}
	providers := map[string]provider.Provider{"p1": fakeProvider{name: "p1"}, "p2": fakeProvider{name: "p2"}}
	return New(cfg, providers, time.Hour, true, ttl, 10000)
}
