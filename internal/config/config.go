package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ProviderAnthropicCompatible = "anthropic_compatible"
	ProviderOpenAICompatible    = "openai_compatible"
	ModelPolicyWeighted         = "weighted"
)

type Config struct {
	Server         ServerConfig          `json:"server"`
	Auth           AuthConfig            `json:"auth"`
	Debug          DebugConfig           `json:"debug"`
	Health         HealthConfig          `json:"health"`
	Routing        RoutingConfig         `json:"routing"`
	ModelDiscovery ModelDiscoveryConfig  `json:"model_discovery"`
	Providers      []ProviderConfig      `json:"providers"`
	Models         map[string]ModelRoute `json:"models"`
}

type ServerConfig struct {
	Addr              string `json:"addr"`
	ReadHeaderTimeout string `json:"read_header_timeout"`
	ReadTimeout       string `json:"read_timeout"`
	WriteTimeout      string `json:"write_timeout"`
	IdleTimeout       string `json:"idle_timeout"`
	MaxBodyBytes      int64  `json:"max_body_bytes"`
}

type AuthConfig struct {
	Disable    bool     `json:"disable"`
	APIKeys    []string `json:"api_keys"`
	APIKeysEnv []string `json:"api_keys_env"`

	Authenticators []AuthProviderConfig `json:"authenticators"`
}

type AuthProviderConfig struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

type DebugConfig struct {
	LogMessages bool `json:"log_messages"`
}

type HealthConfig struct {
	Enabled          bool   `json:"enabled"`
	Interval         string `json:"interval"`
	Timeout          string `json:"timeout"`
	FailureThreshold int    `json:"failure_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
}

type RoutingConfig struct {
	StickyWeighted StickyWeightedConfig `json:"sticky_weighted"`
	Retry          RetryConfig          `json:"retry"`
}

type StickyWeightedConfig struct {
	Enabled    *bool  `json:"enabled"`
	Header     string `json:"header"`
	Fallback   string `json:"fallback"`
	TTL        string `json:"ttl"`
	MaxEntries int    `json:"max_entries"`
}

type RetryConfig struct {
	Enabled             bool   `json:"enabled"`
	MaxAttempts         int    `json:"max_attempts"`
	Backoff             string `json:"backoff"`
	MaxBackoff          string `json:"max_backoff"`
	RetryOnStatus       []int  `json:"retry_on_status"`
	RetryOnNetworkError bool   `json:"retry_on_network_error"`
	RetryOnTimeout      bool   `json:"retry_on_timeout"`
}

type ModelDiscoveryConfig struct {
	TTL                  string `json:"ttl"`
	StaleWhileRevalidate bool   `json:"stale_while_revalidate"`
}

type ProviderConfig struct {
	Name           string                       `json:"name"`
	Type           string                       `json:"type"`
	BaseURL        string                       `json:"base_url"`
	APIKey         string                       `json:"api_key"`
	APIKeyEnv      string                       `json:"api_key_env"`
	Headers        map[string]string            `json:"headers"`
	APIKeyHeader   string                       `json:"api_key_header"`
	APIKeyScheme   string                       `json:"api_key_scheme"`
	Timeout        string                       `json:"timeout"`
	Transport      TransportConfig              `json:"transport"`
	Health         ProviderHealthConfig         `json:"health"`
	Concurrency    ProviderConcurrencyConfig    `json:"concurrency"`
	CircuitBreaker ProviderCircuitBreakerConfig `json:"circuit_breaker"`
	DiscoverModels bool                         `json:"discover_models"`
	ModelPrefix    string                       `json:"model_prefix"`
	IncludeModels  []string                     `json:"include_models"`
	ExcludeModels  []string                     `json:"exclude_models"`
}

type ProviderHealthConfig struct {
	Enabled        *bool  `json:"enabled"`
	ProbePath      string `json:"probe_path"`
	ProbeMethod    string `json:"probe_method"`
	ExpectedStatus []int  `json:"expected_status"`
}

type ProviderConcurrencyConfig struct {
	MaxInFlight int `json:"max_in_flight"`
}

type ProviderCircuitBreakerConfig struct {
	Enabled          *bool  `json:"enabled"`
	FailureThreshold int    `json:"failure_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
	OpenTimeout      string `json:"open_timeout"`
}

type TransportConfig struct {
	MaxIdleConns          int    `json:"max_idle_conns"`
	MaxIdleConnsPerHost   int    `json:"max_idle_conns_per_host"`
	IdleConnTimeout       string `json:"idle_conn_timeout"`
	DialTimeout           string `json:"dial_timeout"`
	DialKeepAlive         string `json:"dial_keep_alive"`
	TLSHandshakeTimeout   string `json:"tls_handshake_timeout"`
	ExpectContinueTimeout string `json:"expect_continue_timeout"`
	ForceAttemptHTTP2     *bool  `json:"force_attempt_http2"`
}

type ModelRoute struct {
	Provider      string             `json:"provider"`
	ProviderModel string             `json:"provider_model"`
	Policy        ModelRoutePolicy   `json:"policy"`
	Targets       []ModelRouteTarget `json:"targets"`
}

type ModelRoutePolicy struct {
	Type string `json:"type"`
}

type ModelRouteTarget struct {
	Provider      string `json:"provider"`
	ProviderModel string `json:"provider_model"`
	Weight        int    `json:"weight"`
}

type Runtime struct {
	Config                    Config
	GatewayAPIKeys            []string
	ReadHeaderTimeout         time.Duration
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
	ModelDiscoveryTTL         time.Duration
	HealthEnabled             bool
	HealthInterval            time.Duration
	HealthTimeout             time.Duration
	HealthFailureThreshold    int
	HealthSuccessThreshold    int
	ProviderHealthEnabled     map[string]bool
	ProviderHealthProbes      map[string]ProviderHealthProbeRuntime
	ProviderConcurrencyLimits map[string]int
	ProviderCircuitBreakers   map[string]CircuitBreakerRuntime
	RetryEnabled              bool
	RetryMaxAttempts          int
	RetryBackoff              time.Duration
	RetryMaxBackoff           time.Duration
	RetryOnStatus             map[int]bool
	RetryOnNetworkError       bool
	RetryOnTimeout            bool
	StickyWeightedEnabled     bool
	StickyWeightedHeader      string
	StickyWeightedFallback    string
	StickyWeightedTTL         time.Duration
	StickyWeightedMaxEntries  int
	ProviderTimeouts          map[string]time.Duration
	ProviderTransports        map[string]TransportRuntime
	ProviderAPIKeys           map[string]string
	ProviderHeadersCanonical  map[string]map[string]string
}

type TransportRuntime struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	IdleConnTimeout       time.Duration
	DialTimeout           time.Duration
	DialKeepAlive         time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ForceAttemptHTTP2     bool
}

type ProviderHealthProbeRuntime struct {
	Method         string
	Path           string
	ExpectedStatus map[int]bool
}

type CircuitBreakerRuntime struct {
	Enabled          bool
	FailureThreshold int
	SuccessThreshold int
	OpenTimeout      time.Duration
}

func Load(path string) (*Runtime, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	rt, err := buildRuntime(cfg)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func defaultConfig() Config {
	return Config{
		Auth: AuthConfig{
			Disable: false,
		},
		Server: ServerConfig{
			Addr:              "127.0.0.1:8080",
			ReadHeaderTimeout: "10s",
			ReadTimeout:       "30s",
			WriteTimeout:      "0s",
			IdleTimeout:       "120s",
			MaxBodyBytes:      10 << 20,
		},
		ModelDiscovery: ModelDiscoveryConfig{
			TTL:                  "10m",
			StaleWhileRevalidate: true,
		},
		Models: map[string]ModelRoute{},
	}
}

func buildRuntime(cfg Config) (*Runtime, error) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = "127.0.0.1:8080"
	}
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = 10 << 20
	}
	if cfg.Models == nil {
		cfg.Models = map[string]ModelRoute{}
	}

	readHeaderTimeout, err := parseDurationDefault(cfg.Server.ReadHeaderTimeout, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("server.read_header_timeout: %w", err)
	}
	readTimeout, err := parseDurationDefault(cfg.Server.ReadTimeout, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("server.read_timeout: %w", err)
	}
	writeTimeout, err := parseDurationDefault(cfg.Server.WriteTimeout, 0)
	if err != nil {
		return nil, fmt.Errorf("server.write_timeout: %w", err)
	}
	idleTimeout, err := parseDurationDefault(cfg.Server.IdleTimeout, 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("server.idle_timeout: %w", err)
	}
	discoveryTTL, err := parseDurationDefault(cfg.ModelDiscovery.TTL, 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("model_discovery.ttl: %w", err)
	}
	stickyEnabled := true
	if cfg.Routing.StickyWeighted.Enabled != nil {
		stickyEnabled = *cfg.Routing.StickyWeighted.Enabled
	}
	stickyHeader := cfg.Routing.StickyWeighted.Header
	if stickyHeader == "" {
		stickyHeader = "X-LLM-Gateway-Sticky-Key"
	}
	stickyFallback := cfg.Routing.StickyWeighted.Fallback
	if stickyFallback == "" {
		stickyFallback = "auth_key"
	}
	if stickyFallback != "auth_key" && stickyFallback != "none" {
		return nil, fmt.Errorf("routing.sticky_weighted.fallback has unsupported value %q", stickyFallback)
	}
	stickyTTL, err := parseDurationDefault(cfg.Routing.StickyWeighted.TTL, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("routing.sticky_weighted.ttl: %w", err)
	}
	stickyMaxEntries := cfg.Routing.StickyWeighted.MaxEntries
	if stickyMaxEntries < 0 {
		return nil, errors.New("routing.sticky_weighted.max_entries must be non-negative")
	}
	if stickyMaxEntries == 0 {
		stickyMaxEntries = 10000
	}
	healthInterval, err := parseDurationDefault(cfg.Health.Interval, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("health.interval: %w", err)
	}
	healthTimeout, err := parseDurationDefault(cfg.Health.Timeout, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("health.timeout: %w", err)
	}
	healthFailureThreshold := cfg.Health.FailureThreshold
	if healthFailureThreshold == 0 {
		healthFailureThreshold = 2
	}
	if healthFailureThreshold < 0 {
		return nil, errors.New("health.failure_threshold must be positive")
	}
	healthSuccessThreshold := cfg.Health.SuccessThreshold
	if healthSuccessThreshold == 0 {
		healthSuccessThreshold = 1
	}
	if healthSuccessThreshold < 0 {
		return nil, errors.New("health.success_threshold must be positive")
	}
	retryMaxAttempts := cfg.Routing.Retry.MaxAttempts
	if retryMaxAttempts == 0 {
		retryMaxAttempts = 1
	}
	if retryMaxAttempts < 1 {
		return nil, errors.New("routing.retry.max_attempts must be at least 1")
	}
	retryBackoff, err := parseDurationDefault(cfg.Routing.Retry.Backoff, 200*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("routing.retry.backoff: %w", err)
	}
	retryMaxBackoff, err := parseDurationDefault(cfg.Routing.Retry.MaxBackoff, time.Second)
	if err != nil {
		return nil, fmt.Errorf("routing.retry.max_backoff: %w", err)
	}
	if retryBackoff < 0 || retryMaxBackoff < 0 {
		return nil, errors.New("routing.retry backoff durations must be non-negative")
	}
	retryOnStatus, err := parseRetryStatus(cfg.Routing.Retry.RetryOnStatus)
	if err != nil {
		return nil, err
	}

	seenProviders := map[string]struct{}{}
	providerTimeouts := map[string]time.Duration{}
	providerTransports := map[string]TransportRuntime{}
	providerHealthEnabled := map[string]bool{}
	providerHealthProbes := map[string]ProviderHealthProbeRuntime{}
	providerConcurrencyLimits := map[string]int{}
	providerCircuitBreakers := map[string]CircuitBreakerRuntime{}
	providerAPIKeys := map[string]string{}
	providerHeaders := map[string]map[string]string{}
	for i, p := range cfg.Providers {
		if strings.TrimSpace(p.Name) == "" {
			return nil, fmt.Errorf("providers[%d].name is required", i)
		}
		if _, ok := seenProviders[p.Name]; ok {
			return nil, fmt.Errorf("duplicate provider %q", p.Name)
		}
		seenProviders[p.Name] = struct{}{}
		if p.Type != ProviderAnthropicCompatible && p.Type != ProviderOpenAICompatible {
			return nil, fmt.Errorf("provider %q has unsupported type %q", p.Name, p.Type)
		}
		if strings.TrimSpace(p.BaseURL) == "" {
			return nil, fmt.Errorf("provider %q base_url is required", p.Name)
		}
		timeout, err := parseDurationDefault(p.Timeout, 120*time.Second)
		if err != nil {
			return nil, fmt.Errorf("provider %q timeout: %w", p.Name, err)
		}
		transport, err := parseTransportRuntime(p.Transport)
		if err != nil {
			return nil, fmt.Errorf("provider %q transport: %w", p.Name, err)
		}
		healthProbe, err := parseProviderHealthProbe(p.Health)
		if err != nil {
			return nil, fmt.Errorf("provider %q health: %w", p.Name, err)
		}
		circuitBreaker, err := parseCircuitBreakerRuntime(p.CircuitBreaker)
		if err != nil {
			return nil, fmt.Errorf("provider %q circuit_breaker: %w", p.Name, err)
		}
		if p.Concurrency.MaxInFlight < 0 {
			return nil, fmt.Errorf("provider %q concurrency.max_in_flight must be non-negative", p.Name)
		}
		if err := validatePatterns(p.IncludeModels); err != nil {
			return nil, fmt.Errorf("provider %q include_models: %w", p.Name, err)
		}
		if err := validatePatterns(p.ExcludeModels); err != nil {
			return nil, fmt.Errorf("provider %q exclude_models: %w", p.Name, err)
		}
		providerTimeouts[p.Name] = timeout
		providerTransports[p.Name] = transport
		providerHealthProbes[p.Name] = healthProbe
		providerConcurrencyLimits[p.Name] = p.Concurrency.MaxInFlight
		providerCircuitBreakers[p.Name] = circuitBreaker
		providerHealthEnabled[p.Name] = cfg.Health.Enabled
		if p.Health.Enabled != nil {
			providerHealthEnabled[p.Name] = *p.Health.Enabled
		}
		providerAPIKeys[p.Name] = resolveValue(p.APIKey, p.APIKeyEnv)
		providerHeaders[p.Name] = canonicalHeaders(p.Headers)
	}

	for name, route := range cfg.Models {
		if err := validateModelRoute(name, route, seenProviders); err != nil {
			return nil, err
		}
	}

	return &Runtime{
		Config:                    cfg,
		GatewayAPIKeys:            resolveAPIKeys(cfg.Auth),
		ReadHeaderTimeout:         readHeaderTimeout,
		ReadTimeout:               readTimeout,
		WriteTimeout:              writeTimeout,
		IdleTimeout:               idleTimeout,
		ModelDiscoveryTTL:         discoveryTTL,
		HealthEnabled:             cfg.Health.Enabled,
		HealthInterval:            healthInterval,
		HealthTimeout:             healthTimeout,
		HealthFailureThreshold:    healthFailureThreshold,
		HealthSuccessThreshold:    healthSuccessThreshold,
		ProviderHealthEnabled:     providerHealthEnabled,
		ProviderHealthProbes:      providerHealthProbes,
		ProviderConcurrencyLimits: providerConcurrencyLimits,
		ProviderCircuitBreakers:   providerCircuitBreakers,
		RetryEnabled:              cfg.Routing.Retry.Enabled,
		RetryMaxAttempts:          retryMaxAttempts,
		RetryBackoff:              retryBackoff,
		RetryMaxBackoff:           retryMaxBackoff,
		RetryOnStatus:             retryOnStatus,
		RetryOnNetworkError:       cfg.Routing.Retry.RetryOnNetworkError,
		RetryOnTimeout:            cfg.Routing.Retry.RetryOnTimeout,
		StickyWeightedEnabled:     stickyEnabled,
		StickyWeightedHeader:      stickyHeader,
		StickyWeightedFallback:    stickyFallback,
		StickyWeightedTTL:         stickyTTL,
		StickyWeightedMaxEntries:  stickyMaxEntries,
		ProviderTimeouts:          providerTimeouts,
		ProviderTransports:        providerTransports,
		ProviderAPIKeys:           providerAPIKeys,
		ProviderHeadersCanonical:  providerHeaders,
	}, nil
}

func validateModelRoute(name string, route ModelRoute, providers map[string]struct{}) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("models contains an empty model name")
	}
	hasLegacy := route.Provider != "" || route.ProviderModel != ""
	hasTargets := len(route.Targets) > 0
	if hasLegacy && hasTargets {
		return fmt.Errorf("model %q cannot set both provider/provider_model and targets", name)
	}
	if !hasTargets {
		if route.Provider == "" {
			return fmt.Errorf("model %q provider is required", name)
		}
		if _, ok := providers[route.Provider]; !ok {
			return fmt.Errorf("model %q references unknown provider %q", name, route.Provider)
		}
		if route.ProviderModel == "" {
			return fmt.Errorf("model %q provider_model is required", name)
		}
		return nil
	}
	policy := route.Policy.Type
	if policy == "" {
		policy = ModelPolicyWeighted
	}
	if policy != ModelPolicyWeighted {
		return fmt.Errorf("model %q has unsupported policy %q", name, policy)
	}
	for i, target := range route.Targets {
		if target.Provider == "" {
			return fmt.Errorf("model %q targets[%d].provider is required", name, i)
		}
		if _, ok := providers[target.Provider]; !ok {
			return fmt.Errorf("model %q targets[%d] references unknown provider %q", name, i, target.Provider)
		}
		if target.ProviderModel == "" {
			return fmt.Errorf("model %q targets[%d].provider_model is required", name, i)
		}
		if target.Weight <= 0 {
			return fmt.Errorf("model %q targets[%d].weight must be positive", name, i)
		}
	}
	return nil
}

func parseDurationDefault(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}

func parseProviderHealthProbe(cfg ProviderHealthConfig) (ProviderHealthProbeRuntime, error) {
	method := strings.ToUpper(strings.TrimSpace(cfg.ProbeMethod))
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "HEAD" {
		return ProviderHealthProbeRuntime{}, fmt.Errorf("probe_method must be GET or HEAD")
	}
	path := strings.TrimSpace(cfg.ProbePath)
	if path == "" {
		path = "/models"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	expected, err := parseExpectedStatus(cfg.ExpectedStatus)
	if err != nil {
		return ProviderHealthProbeRuntime{}, err
	}
	return ProviderHealthProbeRuntime{Method: method, Path: path, ExpectedStatus: expected}, nil
}

func parseExpectedStatus(statuses []int) (map[int]bool, error) {
	if len(statuses) == 0 {
		statuses = []int{200}
	}
	out := make(map[int]bool, len(statuses))
	for _, status := range statuses {
		if status < 100 || status > 599 {
			return nil, fmt.Errorf("expected_status contains invalid HTTP status %d", status)
		}
		out[status] = true
	}
	return out, nil
}

func parseCircuitBreakerRuntime(cfg ProviderCircuitBreakerConfig) (CircuitBreakerRuntime, error) {
	enabled := false
	if cfg.Enabled != nil {
		enabled = *cfg.Enabled
	}
	failureThreshold := cfg.FailureThreshold
	if failureThreshold == 0 {
		failureThreshold = 5
	}
	if failureThreshold < 0 {
		return CircuitBreakerRuntime{}, errors.New("failure_threshold must be non-negative")
	}
	successThreshold := cfg.SuccessThreshold
	if successThreshold == 0 {
		successThreshold = 1
	}
	if successThreshold < 0 {
		return CircuitBreakerRuntime{}, errors.New("success_threshold must be non-negative")
	}
	openTimeout, err := parseDurationDefault(cfg.OpenTimeout, 30*time.Second)
	if err != nil {
		return CircuitBreakerRuntime{}, fmt.Errorf("open_timeout: %w", err)
	}
	if openTimeout < 0 {
		return CircuitBreakerRuntime{}, errors.New("open_timeout must be non-negative")
	}
	return CircuitBreakerRuntime{Enabled: enabled, FailureThreshold: failureThreshold, SuccessThreshold: successThreshold, OpenTimeout: openTimeout}, nil
}

func parseRetryStatus(statuses []int) (map[int]bool, error) {
	if len(statuses) == 0 {
		statuses = []int{408, 429, 500, 502, 503, 504}
	}
	out := make(map[int]bool, len(statuses))
	for _, status := range statuses {
		if status < 100 || status > 599 {
			return nil, fmt.Errorf("routing.retry.retry_on_status contains invalid HTTP status %d", status)
		}
		out[status] = true
	}
	return out, nil
}

func parseTransportRuntime(cfg TransportConfig) (TransportRuntime, error) {
	idleConnTimeout, err := parseDurationDefault(cfg.IdleConnTimeout, 90*time.Second)
	if err != nil {
		return TransportRuntime{}, fmt.Errorf("idle_conn_timeout: %w", err)
	}
	dialTimeout, err := parseDurationDefault(cfg.DialTimeout, 10*time.Second)
	if err != nil {
		return TransportRuntime{}, fmt.Errorf("dial_timeout: %w", err)
	}
	dialKeepAlive, err := parseDurationDefault(cfg.DialKeepAlive, 30*time.Second)
	if err != nil {
		return TransportRuntime{}, fmt.Errorf("dial_keep_alive: %w", err)
	}
	tlsHandshakeTimeout, err := parseDurationDefault(cfg.TLSHandshakeTimeout, 10*time.Second)
	if err != nil {
		return TransportRuntime{}, fmt.Errorf("tls_handshake_timeout: %w", err)
	}
	expectContinueTimeout, err := parseDurationDefault(cfg.ExpectContinueTimeout, time.Second)
	if err != nil {
		return TransportRuntime{}, fmt.Errorf("expect_continue_timeout: %w", err)
	}
	forceAttemptHTTP2 := true
	if cfg.ForceAttemptHTTP2 != nil {
		forceAttemptHTTP2 = *cfg.ForceAttemptHTTP2
	}
	out := TransportRuntime{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       idleConnTimeout,
		DialTimeout:           dialTimeout,
		DialKeepAlive:         dialKeepAlive,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
		ForceAttemptHTTP2:     forceAttemptHTTP2,
	}
	if out.MaxIdleConns == 0 {
		out.MaxIdleConns = 1024
	}
	if out.MaxIdleConnsPerHost == 0 {
		out.MaxIdleConnsPerHost = 256
	}
	return out, nil
}

func validatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			return errors.New("empty pattern")
		}
		if _, err := filepath.Match(pattern, "model"); err != nil {
			return err
		}
	}
	return nil
}

func resolveValue(value, env string) string {
	if value != "" {
		return value
	}
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}

func resolveAPIKeys(auth AuthConfig) []string {
	keys := make([]string, 0, len(auth.APIKeys)+len(auth.APIKeysEnv))
	for _, key := range auth.APIKeys {
		if key != "" {
			keys = append(keys, key)
		}
	}
	for _, env := range auth.APIKeysEnv {
		if value := os.Getenv(env); value != "" {
			keys = append(keys, value)
		}
	}
	return keys
}

func canonicalHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out[strings.ToLower(k)] = v
	}
	return out
}
