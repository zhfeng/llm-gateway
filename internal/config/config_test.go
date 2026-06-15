package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	t.Setenv("TEST_PROVIDER_KEY", "provider-key")
	t.Setenv("TEST_GATEWAY_KEY", "gateway-key")
	path := writeConfig(t, `{
		"auth": {"api_keys_env": ["TEST_GATEWAY_KEY"]},
		"providers": [{
			"name": "p1",
			"type": "openai_compatible",
			"base_url": "http://example.test/v1",
			"api_key_env": "TEST_PROVIDER_KEY"
		}],
		"models": {"m1": {"provider": "p1", "provider_model": "pm1"}}
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if rt.ProviderAPIKeys["p1"] != "provider-key" {
		t.Fatalf("provider key not resolved: %#v", rt.ProviderAPIKeys)
	}
	if len(rt.GatewayAPIKeys) != 1 || rt.GatewayAPIKeys[0] != "gateway-key" {
		t.Fatalf("gateway key not resolved: %#v", rt.GatewayAPIKeys)
	}
}

func TestLoadAuthDisableDefaultsFalse(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Config.Auth.Disable {
		t.Fatal("expected auth.disable to default to false")
	}
}

func TestLoadAuthDisable(t *testing.T) {
	path := writeConfig(t, `{
		"auth": {"disable": true},
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Config.Auth.Disable {
		t.Fatal("expected auth.disable to be true")
	}
}

func TestLoadDebugLogMessages(t *testing.T) {
	path := writeConfig(t, `{
		"debug": {"log_messages": true},
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Config.Debug.LogMessages {
		t.Fatal("expected debug.log_messages to be true")
	}
}

func TestLoadProviderTransport(t *testing.T) {
	forceHTTP2 := false
	path := writeConfig(t, `{
		"providers": [{
			"name": "p1",
			"type": "openai_compatible",
			"base_url": "http://example.test/v1",
			"transport": {
				"max_idle_conns": 2048,
				"max_idle_conns_per_host": 512,
				"idle_conn_timeout": "45s",
				"dial_timeout": "5s",
				"dial_keep_alive": "20s",
				"tls_handshake_timeout": "4s",
				"expect_continue_timeout": "500ms",
				"force_attempt_http2": false
			}
		}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	transport := rt.ProviderTransports["p1"]
	if transport.MaxIdleConns != 2048 || transport.MaxIdleConnsPerHost != 512 || transport.IdleConnTimeout != 45*time.Second || transport.DialTimeout != 5*time.Second || transport.DialKeepAlive != 20*time.Second || transport.TLSHandshakeTimeout != 4*time.Second || transport.ExpectContinueTimeout != 500*time.Millisecond || transport.ForceAttemptHTTP2 != forceHTTP2 {
		t.Fatalf("unexpected transport: %+v", transport)
	}
}

func TestLoadProviderTransportDefaults(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	transport := rt.ProviderTransports["p1"]
	if transport.MaxIdleConns != 1024 || transport.MaxIdleConnsPerHost != 256 || transport.IdleConnTimeout != 90*time.Second || transport.DialTimeout != 10*time.Second || transport.DialKeepAlive != 30*time.Second || transport.TLSHandshakeTimeout != 10*time.Second || transport.ExpectContinueTimeout != time.Second || !transport.ForceAttemptHTTP2 {
		t.Fatalf("unexpected default transport: %+v", transport)
	}
}

func TestLoadWeightedModelTargets(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [
			{"name": "p1", "type": "openai_compatible", "base_url": "http://p1.test/v1"},
			{"name": "p2", "type": "anthropic_compatible", "base_url": "http://p2.test/v1"}
		],
		"models": {"m1": {"policy": {"type": "weighted"}, "targets": [
			{"provider": "p1", "provider_model": "pm1", "weight": 80},
			{"provider": "p2", "provider_model": "pm2", "weight": 20}
		]}}
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.Config.Models["m1"].Targets[0].Weight; got != 80 {
		t.Fatalf("weight = %d", got)
	}
}

func TestLoadWeightedModelTargetsDefaultPolicy(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://p1.test/v1"}],
		"models": {"m1": {"targets": [{"provider": "p1", "provider_model": "pm1", "weight": 1}]}}
	}`)
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsInvalidWeightedModelTargets(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown provider", `{"models":{"m1":{"targets":[{"provider":"missing","provider_model":"pm1","weight":1}]}}}`},
		{"empty targets with no legacy", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://p1.test/v1"}],"models":{"m1":{"policy":{"type":"weighted"},"targets":[]}}}`},
		{"missing provider model", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://p1.test/v1"}],"models":{"m1":{"targets":[{"provider":"p1","weight":1}]}}}`},
		{"non-positive weight", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://p1.test/v1"}],"models":{"m1":{"targets":[{"provider":"p1","provider_model":"pm1","weight":0}]}}}`},
		{"unsupported policy", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://p1.test/v1"}],"models":{"m1":{"policy":{"type":"latency"},"targets":[{"provider":"p1","provider_model":"pm1","weight":1}]}}}`},
		{"mixed legacy and targets", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://p1.test/v1"}],"models":{"m1":{"provider":"p1","provider_model":"pm1","targets":[{"provider":"p1","provider_model":"pm2","weight":1}]}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadProviderHADefaults(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	probe := rt.ProviderHealthProbes["p1"]
	if probe.Method != "GET" || probe.Path != "/models" || !probe.ExpectedStatus[200] {
		t.Fatalf("unexpected health probe defaults: %+v", probe)
	}
	if rt.ProviderConcurrencyLimits["p1"] != 0 {
		t.Fatalf("unexpected concurrency default: %+v", rt.ProviderConcurrencyLimits)
	}
	breaker := rt.ProviderCircuitBreakers["p1"]
	if breaker.Enabled || breaker.FailureThreshold != 5 || breaker.SuccessThreshold != 1 || breaker.OpenTimeout != 30*time.Second {
		t.Fatalf("unexpected circuit breaker defaults: %+v", breaker)
	}
}

func TestLoadProviderHAConfig(t *testing.T) {
	enabled := true
	path := writeConfig(t, `{
		"providers": [{
			"name": "p1",
			"type": "openai_compatible",
			"base_url": "http://example.test/v1",
			"health": {"probe_path": "health", "probe_method": "HEAD", "expected_status": [200, 204]},
			"concurrency": {"max_in_flight": 7},
			"circuit_breaker": {"enabled": true, "failure_threshold": 3, "success_threshold": 2, "open_timeout": "5s"}
		}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	probe := rt.ProviderHealthProbes["p1"]
	if probe.Method != "HEAD" || probe.Path != "/health" || !probe.ExpectedStatus[200] || !probe.ExpectedStatus[204] {
		t.Fatalf("unexpected health probe config: %+v", probe)
	}
	if rt.ProviderConcurrencyLimits["p1"] != 7 {
		t.Fatalf("unexpected concurrency config: %+v", rt.ProviderConcurrencyLimits)
	}
	breaker := rt.ProviderCircuitBreakers["p1"]
	if breaker.Enabled != enabled || breaker.FailureThreshold != 3 || breaker.SuccessThreshold != 2 || breaker.OpenTimeout != 5*time.Second {
		t.Fatalf("unexpected circuit breaker config: %+v", breaker)
	}
}

func TestLoadRejectsInvalidProviderHAConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"method", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1","health":{"probe_method":"POST"}}]}`},
		{"status", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1","health":{"expected_status":[99]}}]}`},
		{"concurrency", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1","concurrency":{"max_in_flight":-1}}]}`},
		{"failure threshold", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1","circuit_breaker":{"failure_threshold":-1}}]}`},
		{"success threshold", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1","circuit_breaker":{"success_threshold":-1}}]}`},
		{"open timeout", `{"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1","circuit_breaker":{"open_timeout":"bad"}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadHealthAndRetryDefaults(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if rt.HealthEnabled || rt.HealthInterval != 30*time.Second || rt.HealthTimeout != 5*time.Second || rt.HealthFailureThreshold != 2 || rt.HealthSuccessThreshold != 1 || rt.ProviderHealthEnabled["p1"] {
		t.Fatalf("unexpected health defaults: %+v", rt)
	}
	if rt.RetryEnabled || rt.RetryMaxAttempts != 1 || rt.RetryBackoff != 200*time.Millisecond || rt.RetryMaxBackoff != time.Second || !rt.RetryOnStatus[503] {
		t.Fatalf("unexpected retry defaults: %+v", rt)
	}
}

func TestLoadHealthAndRetryConfig(t *testing.T) {
	path := writeConfig(t, `{
		"health": {"enabled": true, "interval": "10s", "timeout": "2s", "failure_threshold": 3, "success_threshold": 2},
		"routing": {"retry": {"enabled": true, "max_attempts": 3, "backoff": "100ms", "max_backoff": "500ms", "retry_on_status": [500], "retry_on_network_error": true, "retry_on_timeout": true}},
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1", "health": {"enabled": false}}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.HealthEnabled || rt.HealthInterval != 10*time.Second || rt.HealthTimeout != 2*time.Second || rt.HealthFailureThreshold != 3 || rt.HealthSuccessThreshold != 2 || rt.ProviderHealthEnabled["p1"] {
		t.Fatalf("unexpected health config: %+v", rt)
	}
	if !rt.RetryEnabled || rt.RetryMaxAttempts != 3 || rt.RetryBackoff != 100*time.Millisecond || rt.RetryMaxBackoff != 500*time.Millisecond || !rt.RetryOnStatus[500] || rt.RetryOnStatus[503] || !rt.RetryOnNetworkError || !rt.RetryOnTimeout {
		t.Fatalf("unexpected retry config: %+v", rt)
	}
}

func TestLoadRejectsInvalidHealthAndRetryConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"health interval", `{"health":{"interval":"bad"},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"health timeout", `{"health":{"timeout":"bad"},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"failure threshold", `{"health":{"failure_threshold":-1},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"success threshold", `{"health":{"success_threshold":-1},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"max attempts", `{"routing":{"retry":{"max_attempts":-1}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"backoff", `{"routing":{"retry":{"backoff":"bad"}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"max backoff", `{"routing":{"retry":{"max_backoff":"bad"}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"retry status", `{"routing":{"retry":{"retry_on_status":[99]}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadStickyWeightedDefaults(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.StickyWeightedEnabled || rt.StickyWeightedHeader != "X-LLM-Gateway-Sticky-Key" || rt.StickyWeightedFallback != "auth_key" || rt.StickyWeightedTTL != 24*time.Hour || rt.StickyWeightedMaxEntries != 10000 {
		t.Fatalf("unexpected sticky defaults: %+v", rt)
	}
}

func TestLoadStickyWeightedConfig(t *testing.T) {
	enabled := false
	path := writeConfig(t, `{
		"routing": {"sticky_weighted": {"enabled": false, "header": "X-Session", "fallback": "none", "ttl": "2h", "max_entries": 5}},
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}]
	}`)
	rt, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if rt.StickyWeightedEnabled != enabled || rt.StickyWeightedHeader != "X-Session" || rt.StickyWeightedFallback != "none" || rt.StickyWeightedTTL != 2*time.Hour || rt.StickyWeightedMaxEntries != 5 {
		t.Fatalf("unexpected sticky config: %+v", rt)
	}
}

func TestLoadRejectsInvalidStickyWeightedConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"fallback", `{"routing":{"sticky_weighted":{"fallback":"ip"}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"ttl", `{"routing":{"sticky_weighted":{"ttl":"bad"}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
		{"max entries", `{"routing":{"sticky_weighted":{"max_entries":-1}},"providers":[{"name":"p1","type":"openai_compatible","base_url":"http://example.test/v1"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadRejectsInvalidModelFilterPattern(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1", "include_models": ["["]}]
	}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid include_models pattern error")
	}
}

func TestLoadRejectsDuplicateProviders(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [
			{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"},
			{"name": "p1", "type": "anthropic_compatible", "base_url": "http://example.test/v1"}
		]
	}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected duplicate provider error")
	}
}

func TestLoadRejectsUnknownModelProvider(t *testing.T) {
	path := writeConfig(t, `{
		"providers": [{"name": "p1", "type": "openai_compatible", "base_url": "http://example.test/v1"}],
		"models": {"m1": {"provider": "missing", "provider_model": "pm1"}}
	}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected unknown provider error")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
