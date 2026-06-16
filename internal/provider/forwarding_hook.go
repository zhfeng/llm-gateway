package provider

import (
	"context"
	"log/slog"
	"time"
)

type forwardingLogHook struct{}

func NewForwardingLogHook() Hook {
	return forwardingLogHook{}
}

func (forwardingLogHook) BeforeProviderCall(ctx context.Context, info CallInfo) error {
	model := ""
	providerModel := ""
	if info.Request != nil {
		model = info.Request.Model
		providerModel = info.Request.ProviderModel
	}
	slog.Debug("provider request forwarding", "provider", info.ProviderName, "provider_type", info.ProviderType, "kind", info.Kind, "model", model, "provider_model", providerModel)
	return nil
}

func (forwardingLogHook) AfterProviderCall(ctx context.Context, info CallInfo, result CallResult) {
	duration := result.EndedAt.Sub(info.StartedAt)
	if result.EndedAt.IsZero() {
		duration = time.Since(info.StartedAt)
	}
	attrs := []any{"provider", info.ProviderName, "provider_type", info.ProviderType, "kind", info.Kind, "duration", duration}
	if result.Err != nil {
		attrs = append(attrs, "error", result.Err)
	}
	slog.Debug("provider request forwarded", attrs...)
}
