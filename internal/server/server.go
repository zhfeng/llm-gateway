package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/zhfeng/llm-gateway/internal/auth"
	"github.com/zhfeng/llm-gateway/internal/auth/static"
	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/health"
	"github.com/zhfeng/llm-gateway/internal/httpapi"
	"github.com/zhfeng/llm-gateway/internal/models"
)

func Start(ctx context.Context, cfg *config.Runtime, registry *models.Registry, healthManager *health.Manager) error {
	if cfg.Config.Auth.Disable {
		slog.Warn("gateway API key authentication is disabled; this is very dangerous in production and should only be used for local testing")
	}
	server := &http.Server{
		Addr:              cfg.Config.Server.Addr,
		Handler:           newHandler(cfg, registry, healthManager),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}
	slog.Info("starting gateway", "addr", cfg.Config.Server.Addr)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}

func newHandler(cfg *config.Runtime, registry *models.Registry, healthManager *health.Manager) http.Handler {
	authn := auth.NewAuthenticatorChain()
	authz := auth.NewAuthorizerChain()

	if len(cfg.GatewayAPIKeys) > 0 {
		authn.Add(static.NewAuthenticator(cfg.GatewayAPIKeys))
	}

	authMiddleware := AuthMiddleware(authn, authz, cfg.Config.Auth.Disable)

	h := httpapi.New(registry, healthManager, cfg.Config.Server.MaxBodyBytes, cfg.Config.Debug.LogMessages, httpapi.Options{RetryEnabled: cfg.RetryEnabled, RetryMaxAttempts: cfg.RetryMaxAttempts, RetryBackoff: cfg.RetryBackoff, RetryMaxBackoff: cfg.RetryMaxBackoff, RetryOnStatus: cfg.RetryOnStatus, RetryOnNetworkError: cfg.RetryOnNetworkError, RetryOnTimeout: cfg.RetryOnTimeout, StickyWeightedEnabled: cfg.StickyWeightedEnabled, StickyWeightedHeader: cfg.StickyWeightedHeader, StickyWeightedFallback: cfg.StickyWeightedFallback})

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/v1/models", h.ListModels)
	apiMux.HandleFunc("/v1/chat/completions", h.ChatCompletions)
	apiMux.HandleFunc("/v1/messages", h.Messages)

	mux := http.NewServeMux()
	mux.Handle("/v1/", Chain(apiMux, authMiddleware))
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

	return Chain(mux, requestID, requestMetrics)
}

func requestMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		_ = time.Since(start)
	})
}
