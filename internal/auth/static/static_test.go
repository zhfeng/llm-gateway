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

func TestFactory_WithStringKeys(t *testing.T) {
	a, err := Factory(map[string]any{"keys": []string{"alpha", "beta"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer alpha")
	if _, ok := a.Authenticate(req); !ok {
		t.Fatal("expected Factory-built authenticator to accept a configured key")
	}
}

func TestFactory_WithJSONKeys(t *testing.T) {
	// encoding/json produces []any for JSON arrays, so Factory must accept
	// that shape — this mirrors what happens after config unmarshalling.
	a, err := Factory(map[string]any{"keys": []any{"alpha", "beta"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer beta")
	if _, ok := a.Authenticate(req); !ok {
		t.Fatal("expected Factory-built authenticator to accept a configured key from []any")
	}
}

func TestFactory_MissingKeys(t *testing.T) {
	_, err := Factory(map[string]any{})
	if err == nil {
		t.Fatal("expected error when config.keys is missing, got nil")
	}
}

func TestFactory_NilConfig(t *testing.T) {
	_, err := Factory(nil)
	if err == nil {
		t.Fatal("expected error when config is nil, got nil")
	}
}

func TestFactory_KeysWrongType(t *testing.T) {
	_, err := Factory(map[string]any{"keys": "not-an-array"})
	if err == nil {
		t.Fatal("expected error when config.keys is not an array, got nil")
	}
}

func TestFactory_KeysElementNotString(t *testing.T) {
	_, err := Factory(map[string]any{"keys": []any{"ok", 42}})
	if err == nil {
		t.Fatal("expected error when a config.keys element is not a string, got nil")
	}
}

func TestFactory_EmptyKeys(t *testing.T) {
	// An empty keys array would build an inert authenticator that silently
	// rejects every request. Factory must fail fast at startup instead.
	_, err := Factory(map[string]any{"keys": []string{}})
	if err == nil {
		t.Fatal("expected error when config.keys is empty, got nil")
	}
}

func TestFactory_Name(t *testing.T) {
	a, err := Factory(map[string]any{"keys": []string{"k"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name() != "static" {
		t.Errorf("expected Name static, got %s", a.Name())
	}
}
