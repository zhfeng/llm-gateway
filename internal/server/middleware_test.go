package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/zhfeng/llm-gateway/internal/config"
	"github.com/zhfeng/llm-gateway/internal/models"
)

func TestChainAppliesMiddlewareInOrder(t *testing.T) {
	calls := []string{}
	handler := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "handler")
		}),
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, "first-before")
				next.ServeHTTP(w, r)
				calls = append(calls, "first-after")
			})
		},
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, "second-before")
				next.ServeHTTP(w, r)
				calls = append(calls, "second-after")
			})
		},
	)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"first-before", "second-before", "handler", "second-after", "first-after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestHealthRoutesBypassAuth(t *testing.T) {
	handler := newHandler(testRuntime(false, []string{"secret"}), testRegistry(), nil)

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body=%s", path, w.Code, http.StatusOK, w.Body.String())
		}
	}
}

func TestAPIRoutesRequireAuth(t *testing.T) {
	handler := newHandler(testRuntime(false, []string{"secret"}), testRegistry(), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestAPIRoutesAllowValidAuth(t *testing.T) {
	handler := newHandler(testRuntime(false, []string{"secret"}), testRegistry(), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func testRuntime(authDisabled bool, keys []string) *config.Runtime {
	return &config.Runtime{
		Config: config.Config{
			Auth:   config.AuthConfig{Disable: authDisabled},
			Server: config.ServerConfig{MaxBodyBytes: 1 << 20},
		},
		GatewayAPIKeys:            keys,
		RetryMaxAttempts:          1,
		RetryOnStatus:             map[int]bool{},
		StickyWeightedEnabled:     true,
		StickyWeightedHeader:      "X-LLM-Gateway-Sticky-Key",
		StickyWeightedFallback:    "auth_key",
		StickyWeightedTTL:         time.Hour,
		StickyWeightedMaxEntries:  10000,
		ProviderConcurrencyLimits: map[string]int{},
		ProviderCircuitBreakers:   map[string]config.CircuitBreakerRuntime{},
	}
}

func testRegistry() *models.Registry {
	return models.New(config.Config{Models: map[string]config.ModelRoute{}}, nil, time.Hour, true, time.Hour, 10000)
}
