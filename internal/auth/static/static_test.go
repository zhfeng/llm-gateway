package static

import (
	"net/http"
	"testing"
)

func TestAuthenticator_BearerToken(t *testing.T) {
	a := NewAuthenticator([]string{"valid-key"})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-key")

	id, ok := a.Authenticate(req)
	if !ok {
		t.Fatal("expected valid Bearer token to authenticate")
	}
	if id.Type != "api_key" {
		t.Errorf("expected Type api_key, got %s", id.Type)
	}
}

func TestAuthenticator_XAPIKey(t *testing.T) {
	a := NewAuthenticator([]string{"valid-key"})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("x-api-key", "valid-key")

	_, ok := a.Authenticate(req)
	if !ok {
		t.Fatal("expected valid x-api-key to authenticate")
	}
}

func TestAuthenticator_InvalidKey(t *testing.T) {
	a := NewAuthenticator([]string{"valid-key"})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")

	_, ok := a.Authenticate(req)
	if ok {
		t.Fatal("expected invalid key to reject")
	}
}

func TestAuthenticator_EmptyKeys(t *testing.T) {
	a := NewAuthenticator([]string{})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer any-key")

	_, ok := a.Authenticate(req)
	if ok {
		t.Fatal("expected empty keys map to reject all")
	}
}

func TestAuthenticator_EmptyStringsIgnored(t *testing.T) {
	a := NewAuthenticator([]string{"", "valid"})

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid")

	_, ok := a.Authenticate(req)
	if !ok {
		t.Fatal("expected valid key to work even when empty strings are present")
	}
}

func TestAuthenticator_Name(t *testing.T) {
	a := NewAuthenticator([]string{})
	if a.Name() != "static" {
		t.Errorf("expected Name static, got %s", a.Name())
	}
}
