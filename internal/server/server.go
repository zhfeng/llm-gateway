package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/health"
	"github.com/zhfeng/llm-gateway/internal/httpapi"
	"github.com/zhfeng/llm-gateway/internal/models"
)

func Start(cfg *config.Runtime, registry *models.Registry, healthManager *health.Manager) error {
	if cfg.Config.Auth.Disable {
		slog.Warn("gateway API key authentication is disabled; this is very dangerous in production and should only be used for local testing")
	}
	h := httpapi.New(registry, healthManager, cfg.GatewayAPIKeys, cfg.Config.Auth.Disable, cfg.Config.Server.MaxBodyBytes, cfg.Config.Debug.LogMessages, httpapi.Options{RetryEnabled: cfg.RetryEnabled, RetryMaxAttempts: cfg.RetryMaxAttempts, RetryBackoff: cfg.RetryBackoff, RetryMaxBackoff: cfg.RetryMaxBackoff, RetryOnStatus: cfg.RetryOnStatus, RetryOnNetworkError: cfg.RetryOnNetworkError, RetryOnTimeout: cfg.RetryOnTimeout, StickyWeightedEnabled: cfg.StickyWeightedEnabled, StickyWeightedHeader: cfg.StickyWeightedHeader, StickyWeightedFallback: cfg.StickyWeightedFallback})
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", h.Auth(h.ListModels))
	mux.HandleFunc("/v1/chat/completions", h.Auth(h.ChatCompletions))
	mux.HandleFunc("/v1/messages", h.Auth(h.Messages))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusOK
		body := map[string]any{"status": "ok"}
		if healthManager != nil {
			body["providers"] = healthManager.Snapshot()
			if healthManager.Enabled() && !healthManager.Healthy() {
				status = http.StatusServiceUnavailable
				body["status"] = "degraded"
			}
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(body)
	})

	server := &http.Server{
		Addr:              cfg.Config.Server.Addr,
		Handler:           requestLogger(mux),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}
	slog.Info("starting gateway", "addr", cfg.Config.Server.Addr)
	return server.ListenAndServe()
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
		slog.Info("completed", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
