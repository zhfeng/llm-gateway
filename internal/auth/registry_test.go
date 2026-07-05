package auth

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/zhfeng/llm-gateway/internal/config"
)

// stubAuthenticator is a minimal Authenticator used only for registry tests.
type stubAuthenticator struct{ name string }

func (s *stubAuthenticator) Authenticate(r *http.Request) (*Identity, bool, error) {
	return nil, false, nil
}
func (s *stubAuthenticator) Name() string { return s.name }

func TestRegisterAndLookupAuthenticator(t *testing.T) {
	// Use a unique type name to avoid colliding with other tests that may
	// register into the global registry.
	typ := "test-register-" + t.Name()
	factory := func(cfg map[string]any) (Authenticator, error) {
		return &stubAuthenticator{name: typ}, nil
	}

	RegisterAuthenticator(typ, factory)

	got, ok := LookupAuthenticator(typ)
	if !ok {
		t.Fatalf("LookupAuthenticator(%q) = false, want true", typ)
	}
	if got == nil {
		t.Fatal("LookupAuthenticator returned nil factory")
	}

	a, err := got(map[string]any{})
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if a.Name() != typ {
		t.Errorf("authenticator Name = %q, want %q", a.Name(), typ)
	}
}

func TestLookupAuthenticatorUnknownType(t *testing.T) {
	typ := "definitely-not-registered-" + t.Name()
	if _, ok := LookupAuthenticator(typ); ok {
		t.Fatalf("expected LookupAuthenticator(%q) to return false for unknown type", typ)
	}
}

func TestBuildAuthenticatorsEmpty(t *testing.T) {
	authenticators, err := BuildAuthenticators(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authenticators) != 0 {
		t.Fatalf("expected 0 authenticators, got %d", len(authenticators))
	}
}

func TestBuildAuthenticatorsUnknownType(t *testing.T) {
	typ := "unknown-type-" + t.Name()
	specs := []config.AuthProviderConfig{
		{Type: typ, Config: map[string]any{}},
	}
	_, err := BuildAuthenticators(specs)
	if err == nil {
		t.Fatal("expected error for unknown authenticator type, got nil")
	}
}

func TestBuildAuthenticatorsFactoryError(t *testing.T) {
	typ := "test-factory-error-" + t.Name()
	wantErr := errors.New("boom")
	RegisterAuthenticator(typ, func(cfg map[string]any) (Authenticator, error) {
		return nil, wantErr
	})

	specs := []config.AuthProviderConfig{
		{Type: typ, Config: map[string]any{}},
	}
	_, err := BuildAuthenticators(specs)
	if err == nil {
		t.Fatal("expected error from factory, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to wrap %v, got %v", wantErr, err)
	}
}

func TestBuildAuthenticatorsMultiple(t *testing.T) {
	typA := "test-multi-a-" + t.Name()
	typB := "test-multi-b-" + t.Name()
	RegisterAuthenticator(typA, func(cfg map[string]any) (Authenticator, error) {
		return &stubAuthenticator{name: "a"}, nil
	})
	RegisterAuthenticator(typB, func(cfg map[string]any) (Authenticator, error) {
		return &stubAuthenticator{name: "b"}, nil
	})

	specs := []config.AuthProviderConfig{
		{Type: typA, Config: map[string]any{}},
		{Type: typB, Config: map[string]any{}},
	}
	authenticators, err := BuildAuthenticators(specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authenticators) != 2 {
		t.Fatalf("expected 2 authenticators, got %d", len(authenticators))
	}

	names := []string{authenticators[0].Name(), authenticators[1].Name()}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestBuildAuthenticatorsNilFromFactory(t *testing.T) {
	typ := "test-nil-factory-" + t.Name()
	RegisterAuthenticator(typ, func(cfg map[string]any) (Authenticator, error) {
		return nil, nil
	})

	specs := []config.AuthProviderConfig{
		{Type: typ, Config: map[string]any{}},
	}
	_, err := BuildAuthenticators(specs)
	if err == nil {
		t.Fatal("expected error for nil authenticator from factory, got nil")
	}
}

func TestRegisterAuthenticatorPanicsOnEmptyType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty type, got none")
		}
	}()
	RegisterAuthenticator("", func(cfg map[string]any) (Authenticator, error) {
		return &stubAuthenticator{}, nil
	})
}

func TestRegisterAuthenticatorPanicsOnNilFactory(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil factory, got none")
		}
	}()
	RegisterAuthenticator("test-nil-panic-"+t.Name(), nil)
}
