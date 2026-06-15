package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/health"
	"github.com/zhfeng/llm-gateway/internal/models"
	"github.com/zhfeng/llm-gateway/internal/provider"
	"github.com/zhfeng/llm-gateway/internal/server"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config.json")
	addr := flag.String("addr", "", "override server address")
	flag.Parse()

	rt, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if *addr != "" {
		rt.Config.Server.Addr = *addr
	}

	providers := make(map[string]provider.Provider, len(rt.Config.Providers))
	for _, pcfg := range rt.Config.Providers {
		p, err := provider.New(pcfg, rt.ProviderAPIKeys[pcfg.Name], rt.ProviderHeadersCanonical[pcfg.Name], rt.ProviderTimeouts[pcfg.Name], rt.ProviderTransports[pcfg.Name], rt.ProviderHealthProbes[pcfg.Name], rt.Config.Debug.LogMessages)
		if err != nil {
			slog.Error("create provider", "provider", pcfg.Name, "error", err)
			os.Exit(1)
		}
		p = provider.WithConcurrencyLimit(p, rt.ProviderConcurrencyLimits[pcfg.Name])
		p = provider.WithCircuitBreaker(p, rt.ProviderCircuitBreakers[pcfg.Name])
		providers[pcfg.Name] = p
	}

	registry := models.New(rt.Config, providers, rt.ModelDiscoveryTTL, rt.StickyWeightedEnabled, rt.StickyWeightedTTL, rt.StickyWeightedMaxEntries)
	healthManager := health.NewManager(providers, health.Config{Enabled: rt.HealthEnabled, Interval: rt.HealthInterval, Timeout: rt.HealthTimeout, FailureThreshold: rt.HealthFailureThreshold, SuccessThreshold: rt.HealthSuccessThreshold, ProviderEnabled: rt.ProviderHealthEnabled})
	healthCtx, stopHealth := context.WithCancel(context.Background())
	defer stopHealth()
	go healthManager.Start(healthCtx)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	go func() {
		defer cancel()
		registry.RefreshAll(ctx)
	}()

	if err := server.Start(rt, registry, healthManager); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
