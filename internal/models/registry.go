package models

import (
	"context"
	"math/rand"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/protocol"
	"github.com/zhfeng/llm-gateway/internal/provider"
)

type Route struct {
	Model         string
	Provider      provider.Provider
	ProviderName  string
	ProviderModel string
	Source        string
	Policy        string
	Targets       []RouteTarget
}

type RouteTarget struct {
	Provider      provider.Provider
	ProviderName  string
	ProviderModel string
	Weight        int
}

type Registry struct {
	providers        map[string]provider.Provider
	cfg              config.Config
	ttl              time.Duration
	stickyEnabled    bool
	stickyTTL        time.Duration
	stickyMaxEntries int
	snapshot         atomic.Value
	mu               sync.Mutex
	refreshing       map[string]bool
	stickyMu         sync.Mutex
	sticky           map[string]stickyTarget
}

type ResolveOptions struct {
	StickyKey string
}

type stickyTarget struct {
	ProviderName  string
	ProviderModel string
	ExpiresAt     time.Time
}

type snapshot struct {
	routes map[string]Route
	models []protocol.ModelInfo
	cached map[string]cachedModels
}

type cachedModels struct {
	models   []protocol.ModelInfo
	cachedAt time.Time
}

func New(cfg config.Config, providers map[string]provider.Provider, ttl time.Duration, stickyEnabled bool, stickyTTL time.Duration, stickyMaxEntries int) *Registry {
	r := &Registry{cfg: cfg, providers: providers, ttl: ttl, stickyEnabled: stickyEnabled, stickyTTL: stickyTTL, stickyMaxEntries: stickyMaxEntries, refreshing: map[string]bool{}, sticky: map[string]stickyTarget{}}
	routes := r.staticRoutes(nil)
	r.snapshot.Store(snapshot{routes: routes, models: r.modelList(routes), cached: map[string]cachedModels{}})
	return r
}

func (r *Registry) Resolve(model string) (Route, bool) {
	return r.ResolveWithOptions(model, ResolveOptions{})
}

func (r *Registry) ResolveWithOptions(model string, opts ResolveOptions) (Route, bool) {
	s := r.snapshot.Load().(snapshot)
	route, ok := s.routes[model]
	if !ok {
		return Route{}, false
	}
	if len(route.Targets) == 0 {
		return route, true
	}
	if !r.stickyEnabled || opts.StickyKey == "" {
		return applyTarget(route, selectWeightedTarget(route.Targets, rand.Intn(totalWeight(route.Targets)))), true
	}
	return r.resolveSticky(model, opts.StickyKey, route), true
}

func (r *Registry) Models() []protocol.ModelInfo {
	s := r.snapshot.Load().(snapshot)
	out := make([]protocol.ModelInfo, len(s.models))
	copy(out, s.models)
	return out
}

func (r *Registry) RefreshAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, pcfg := range r.cfg.Providers {
		if !pcfg.DiscoverModels {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_ = r.RefreshProvider(ctx, name)
		}(pcfg.Name)
	}
	wg.Wait()
}

func (r *Registry) RefreshProvider(ctx context.Context, name string) error {
	p := r.providers[name]
	if p == nil {
		return nil
	}
	if !r.startRefresh(name) {
		return nil
	}
	defer r.finishRefresh(name)

	models, err := p.ListModels(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	current := r.snapshot.Load().(snapshot)
	cached := copyCache(current.cached)
	cached[name] = cachedModels{models: models, cachedAt: time.Now()}
	routes := r.staticRoutes(cached)
	r.snapshot.Store(snapshot{routes: routes, models: r.modelList(routes), cached: cached})
	r.mu.Unlock()
	return nil
}

func (r *Registry) startRefresh(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.refreshing[name] {
		return false
	}
	r.refreshing[name] = true
	return true
}

func (r *Registry) finishRefresh(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.refreshing, name)
}

func (r *Registry) staticRoutes(cached map[string]cachedModels) map[string]Route {
	routes := map[string]Route{}
	if cached != nil {
		for _, pcfg := range r.cfg.Providers {
			cache, ok := cached[pcfg.Name]
			if !ok || time.Since(cache.cachedAt) > r.ttl && !r.cfg.ModelDiscovery.StaleWhileRevalidate {
				continue
			}
			p := r.providers[pcfg.Name]
			for _, model := range cache.models {
				if !modelAllowed(model.ID, pcfg.IncludeModels, pcfg.ExcludeModels) {
					continue
				}
				exposed := pcfg.ModelPrefix + model.ID
				routes[exposed] = Route{Model: exposed, Provider: p, ProviderName: pcfg.Name, ProviderModel: model.ID, Source: "dynamic"}
			}
		}
	}
	for name, cfg := range r.cfg.Models {
		if len(cfg.Targets) == 0 {
			p := r.providers[cfg.Provider]
			routes[name] = Route{Model: name, Provider: p, ProviderName: cfg.Provider, ProviderModel: cfg.ProviderModel, Source: "static"}
			continue
		}
		targets := make([]RouteTarget, 0, len(cfg.Targets))
		for _, target := range cfg.Targets {
			targets = append(targets, RouteTarget{Provider: r.providers[target.Provider], ProviderName: target.Provider, ProviderModel: target.ProviderModel, Weight: target.Weight})
		}
		policy := cfg.Policy.Type
		if policy == "" {
			policy = config.ModelPolicyWeighted
		}
		routes[name] = Route{Model: name, ProviderName: "llm-gateway", Source: "static", Policy: policy, Targets: targets}
	}
	return routes
}

func (r *Registry) modelList(routes map[string]Route) []protocol.ModelInfo {
	out := make([]protocol.ModelInfo, 0, len(routes))
	for id, route := range routes {
		out = append(out, protocol.ModelInfo{ID: id, OwnedBy: route.ProviderName})
	}
	return out
}

func (r *Registry) resolveSticky(model, stickyKey string, route Route) Route {
	key := model + "\x00" + stickyKey
	now := time.Now()
	r.stickyMu.Lock()
	defer r.stickyMu.Unlock()
	if binding, ok := r.sticky[key]; ok {
		if now.Before(binding.ExpiresAt) {
			if target, ok := findTarget(route.Targets, binding.ProviderName, binding.ProviderModel); ok {
				return applyTarget(route, target)
			}
		}
		delete(r.sticky, key)
	}
	target := selectWeightedTarget(route.Targets, rand.Intn(totalWeight(route.Targets)))
	if r.stickyMaxEntries == 0 || len(r.sticky) < r.stickyMaxEntries || r.pruneExpiredSticky(now) {
		r.sticky[key] = stickyTarget{ProviderName: target.ProviderName, ProviderModel: target.ProviderModel, ExpiresAt: now.Add(r.stickyTTL)}
	}
	return applyTarget(route, target)
}

func (r *Registry) pruneExpiredSticky(now time.Time) bool {
	for key, binding := range r.sticky {
		if !now.Before(binding.ExpiresAt) {
			delete(r.sticky, key)
		}
	}
	return len(r.sticky) < r.stickyMaxEntries
}

func findTarget(targets []RouteTarget, providerName, providerModel string) (RouteTarget, bool) {
	for _, target := range targets {
		if target.ProviderName == providerName && target.ProviderModel == providerModel {
			return target, true
		}
	}
	return RouteTarget{}, false
}

func applyTarget(route Route, target RouteTarget) Route {
	route.Provider = target.Provider
	route.ProviderName = target.ProviderName
	route.ProviderModel = target.ProviderModel
	return route
}

func totalWeight(targets []RouteTarget) int {
	total := 0
	for _, target := range targets {
		total += target.Weight
	}
	return total
}

func selectWeightedTarget(targets []RouteTarget, pick int) RouteTarget {
	seen := 0
	for _, target := range targets {
		seen += target.Weight
		if pick < seen {
			return target
		}
	}
	return targets[len(targets)-1]
}

func modelAllowed(model string, include, exclude []string) bool {
	if len(include) > 0 && !matchesAny(include, model) {
		return false
	}
	return !matchesAny(exclude, model)
}

func matchesAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, value); ok {
			return true
		}
	}
	return false
}

func copyCache(in map[string]cachedModels) map[string]cachedModels {
	out := make(map[string]cachedModels, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
