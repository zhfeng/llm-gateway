package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// errorAuthenticator simulates an authenticator that recognises a credential
// but rejects it as invalid/expired (e.g. an expired JWT). It returns an
// AuthError so the middleware can surface a specific reason in the 401 body.
type errorAuthenticator struct{}

func (e *errorAuthenticator) Authenticate(_ *http.Request) (*auth.Identity, bool, error) {
	return nil, false, &auth.AuthError{Type: "expired_token", Message: "token has expired"}
}

func (e *errorAuthenticator) Name() string { return "error" }

func TestAuthMiddleware_AuthErrorBody(t *testing.T) {
	authn := auth.NewAuthenticatorChain(&errorAuthenticator{})
	authz := auth.NewAuthorizerChain()
	middleware := AuthMiddleware(authn, authz, false)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called for an auth error")
	})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()

	middleware(next).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "expired_token") {
		t.Errorf("expected response body to contain the specific error type 'expired_token', got: %s", body)
	}
	if !strings.Contains(body, "token has expired") {
		t.Errorf("expected response body to contain the specific error message, got: %s", body)
	}
}

type rejectingAuthorizer struct{}

func (r *rejectingAuthorizer) Authorize(_ *http.Request, _ *auth.Identity) bool {
	return false
}

func (r *rejectingAuthorizer) Name() string {
	return "reject"
}
