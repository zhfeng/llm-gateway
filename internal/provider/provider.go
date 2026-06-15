package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/gwerror"
	"github.com/zhfeng/llm-gateway/internal/protocol"
)

type Provider interface {
	Name() string
	Type() string
	Complete(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
	Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error)
	ListModels(ctx context.Context) ([]protocol.ModelInfo, error)
	HealthCheck(ctx context.Context) error
}

type HTTPProvider struct {
	name         string
	typ          string
	baseURL      *url.URL
	apiKey       string
	apiKeyHeader string
	apiKeyScheme string
	headers      map[string]string
	client       *http.Client
	healthProbe  config.ProviderHealthProbeRuntime
	logMessages  bool
}

func New(cfg config.ProviderConfig, apiKey string, headers map[string]string, timeout time.Duration, transport config.TransportRuntime, healthProbe config.ProviderHealthProbeRuntime, logMessages bool) (Provider, error) {
	base, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("provider %q base_url: %w", cfg.Name, err)
	}
	p := &HTTPProvider{
		name:         cfg.Name,
		typ:          cfg.Type,
		baseURL:      base,
		apiKey:       apiKey,
		apiKeyHeader: cfg.APIKeyHeader,
		apiKeyScheme: cfg.APIKeyScheme,
		headers:      headers,
		healthProbe:  healthProbe,
		logMessages:  logMessages,
		client: &http.Client{
			Transport: tunedTransport(transport),
			Timeout:   timeout,
		},
	}
	return p, nil
}

func (p *HTTPProvider) Name() string { return p.name }
func (p *HTTPProvider) Type() string { return p.typ }

func tunedTransport(cfg config.TransportRuntime) *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: cfg.DialTimeout, KeepAlive: cfg.DialKeepAlive}).DialContext,
		ForceAttemptHTTP2:     cfg.ForceAttemptHTTP2,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
	}
}

func (p *HTTPProvider) endpoint(suffix string) string {
	u := *p.baseURL
	if p.typ == config.ProviderAnthropicCompatible && !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/v1") {
		u.Path = path.Join(u.Path, "v1", suffix)
	} else {
		u.Path = path.Join(u.Path, suffix)
	}
	return u.String()
}

func (p *HTTPProvider) newJSONRequest(ctx context.Context, method, endpoint string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range p.headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if p.apiKey != "" {
		header, value := p.authHeader()
		req.Header.Set(header, value)
	}
	if p.typ == config.ProviderAnthropicCompatible && req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	return req, nil
}

func (p *HTTPProvider) authHeader() (string, string) {
	header := p.apiKeyHeader
	if header == "" {
		if p.typ == config.ProviderAnthropicCompatible {
			header = "x-api-key"
		} else {
			header = "Authorization"
		}
	}
	scheme := p.apiKeyScheme
	if scheme == "" && header == "Authorization" {
		scheme = "Bearer"
	}
	if scheme != "" {
		return header, scheme + " " + p.apiKey
	}
	return header, p.apiKey
}

func (p *HTTPProvider) do(req *http.Request) (*http.Response, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, p.providerError(resp.StatusCode, data)
	}
	return resp, nil
}

func (p *HTTPProvider) providerError(status int, data []byte) error {
	message := strings.TrimSpace(string(data))
	typ := "provider_error"
	code := ""
	var envelope struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil {
		switch e := envelope.Error.(type) {
		case string:
			message = e
		case map[string]any:
			if v, ok := e["message"].(string); ok && v != "" {
				message = v
			}
			if v, ok := e["type"].(string); ok && v != "" {
				typ = v
			}
			if v, ok := e["code"].(string); ok && v != "" {
				code = v
			}
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &gwerror.Error{Status: status, Type: typ, Code: code, Message: message, Provider: p.name, Raw: data}
}

func (p *HTTPProvider) HealthCheck(ctx context.Context) error {
	req, err := p.newJSONRequest(ctx, p.healthProbe.Method, p.endpoint(strings.TrimPrefix(p.healthProbe.Path, "/")), nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if p.healthProbe.ExpectedStatus[resp.StatusCode] {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return p.providerError(resp.StatusCode, data)
	}
	return &gwerror.Error{Status: http.StatusBadGateway, Type: "provider_error", Message: fmt.Sprintf("health probe returned unexpected status %d", resp.StatusCode), Provider: p.name, Raw: data}
}

func (p *HTTPProvider) ListModels(ctx context.Context) ([]protocol.ModelInfo, error) {
	req, err := p.newJSONRequest(ctx, http.MethodGet, p.endpoint("models"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]protocol.ModelInfo, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		ownedBy := model.OwnedBy
		if ownedBy == "" {
			ownedBy = p.name
		}
		models = append(models, protocol.ModelInfo{ID: model.ID, OwnedBy: ownedBy})
	}
	return models, nil
}

func (p *HTTPProvider) Complete(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	switch p.typ {
	case config.ProviderOpenAICompatible:
		return p.completeOpenAI(ctx, req)
	case config.ProviderAnthropicCompatible:
		return p.completeAnthropic(ctx, req)
	default:
		return nil, gwerror.New(http.StatusBadGateway, "provider_error", "unsupported provider type")
	}
}

func (p *HTTPProvider) Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamEvent, error) {
	switch p.typ {
	case config.ProviderOpenAICompatible:
		return p.streamOpenAI(ctx, req)
	case config.ProviderAnthropicCompatible:
		return p.streamAnthropic(ctx, req)
	default:
		return nil, gwerror.New(http.StatusBadGateway, "provider_error", "unsupported provider type")
	}
}
