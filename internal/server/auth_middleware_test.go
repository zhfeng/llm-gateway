package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zhfeng/llm-gateway/internal/auth"
	"github.com/zhfeng/llm-gateway/internal/auth/static"
)

func TestAuthMiddleware_Disabled(t *testing.T) {
	authn := auth.NewAuthenticatorChain(static.NewAuthenticator([]string{"key"}))
	authz := auth.NewAuthorizerChain()
	middleware := AuthMiddleware(authn, authz, true)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	middleware(next).ServeHTTP(w, req)

	if !called {
		t.Fatal("expected next handler to be called when auth is disabled")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	authn := auth.NewAuthenticatorChain(static.NewAuthenticator([]string{"valid-key"}))
	authz := auth.NewAuthorizerChain()
	middleware := AuthMiddleware(authn, authz, false)

	var identityInContext *auth.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identityInContext = auth.FromContext(r.Context())
	})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()

	middleware(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if identityInContext == nil {
		t.Fatal("expected identity in context")
	}
	if identityInContext.Type != "api_key" {
		t.Errorf("expected Type api_key, got %s", identityInContext.Type)
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	authn := auth.NewAuthenticatorChain(static.NewAuthenticator([]string{"valid-key"}))
	authz := auth.NewAuthorizerChain()
	middleware := AuthMiddleware(authn, authz, false)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called for invalid key")
	})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()

	middleware(next).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_NoKeysConfigured(t *testing.T) {
	authn := auth.NewAuthenticatorChain()
	authz := auth.NewAuthorizerChain()
	middleware := AuthMiddleware(authn, authz, false)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called when no keys are configured")
	})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer any-key")
	w := httptest.NewRecorder()

	middleware(next).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_PermissionDenied(t *testing.T) {
	authn := auth.NewAuthenticatorChain(static.NewAuthenticator([]string{"valid-key"}))
	authz := auth.NewAuthorizerChain(&rejectingAuthorizer{})
	middleware := AuthMiddleware(authn, authz, false)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called when authorization fails")
	})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()

	middleware(next).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

type rejectingAuthorizer struct{}

func (r *rejectingAuthorizer) Authorize(_ *http.Request, _ *auth.Identity) bool {
	return false
}

func (r *rejectingAuthorizer) Name() string {
	return "reject"
}
