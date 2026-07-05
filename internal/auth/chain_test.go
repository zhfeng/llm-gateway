package auth

import (
	"net/http"
	"testing"
)

type mockAuthenticator struct {
	name       string
	shouldAuth bool
	err        error
	called     bool
}

func (m *mockAuthenticator) Authenticate(r *http.Request) (*Identity, bool, error) {
	m.called = true
	if m.err != nil {
		return nil, false, m.err
	}
	if m.shouldAuth {
		return &Identity{ID: m.name, Type: "mock"}, true, nil
	}
	return nil, false, nil
}

func (m *mockAuthenticator) Name() string {
	return m.name
}

func TestAuthenticatorChain_FirstWins(t *testing.T) {
	chain := NewAuthenticatorChain(
		&mockAuthenticator{name: "first", shouldAuth: true},
		&mockAuthenticator{name: "second", shouldAuth: true},
	)

	req, _ := http.NewRequest("GET", "/", nil)
	id, ok, _ := chain.Authenticate(req)
	if !ok {
		t.Fatal("expected authentication to succeed")
	}
	if id.ID != "first" {
		t.Errorf("expected first authenticator to win, got %s", id.ID)
	}
}

func TestAuthenticatorChain_FallsThrough(t *testing.T) {
	chain := NewAuthenticatorChain(
		&mockAuthenticator{name: "first", shouldAuth: false},
		&mockAuthenticator{name: "second", shouldAuth: true},
	)

	req, _ := http.NewRequest("GET", "/", nil)
	id, ok, _ := chain.Authenticate(req)
	if !ok {
		t.Fatal("expected authentication to succeed")
	}
	if id.ID != "second" {
		t.Errorf("expected second authenticator to be used after first fails, got %s", id.ID)
	}
}

func TestAuthenticatorChain_StopsOnError(t *testing.T) {
	// An authenticator that recognises the credential but rejects it
	// (err != nil) must stop the chain — the second authenticator must
	// never be called, and the error must be surfaced.
	wantErr := &AuthError{Type: "expired_token", Message: "token has expired"}
	first := &mockAuthenticator{name: "first", err: wantErr}
	second := &mockAuthenticator{name: "second", shouldAuth: true}
	chain := NewAuthenticatorChain(first, second)

	req, _ := http.NewRequest("GET", "/", nil)
	id, ok, err := chain.Authenticate(req)

	if ok {
		t.Fatal("expected authentication to fail when first returns error")
	}
	if id != nil {
		t.Fatal("expected nil identity when chain returns error")
	}
	if err != wantErr {
		t.Fatalf("expected chain to surface first authenticator's error, got %v", err)
	}
	if second.called {
		t.Fatal("expected second authenticator to NOT be called after first returns error")
	}
}

func TestAuthenticatorChain_EmptyRejects(t *testing.T) {
	chain := NewAuthenticatorChain()

	req, _ := http.NewRequest("GET", "/", nil)
	_, ok, err := chain.Authenticate(req)
	if ok {
		t.Fatal("expected empty chain to reject all")
	}
	if err != nil {
		t.Fatalf("expected empty chain to return no error, got %v", err)
	}
}

func TestAuthenticatorChain_HasAuthenticators(t *testing.T) {
	empty := NewAuthenticatorChain()
	if empty.HasAuthenticators() {
		t.Fatal("expected empty chain to have no authenticators")
	}

	withAuth := NewAuthenticatorChain(&mockAuthenticator{name: "test"})
	if !withAuth.HasAuthenticators() {
		t.Fatal("expected chain with authenticator to have authenticators")
	}
}

func TestAuthenticatorChain_Add(t *testing.T) {
	chain := NewAuthenticatorChain()
	if chain.HasAuthenticators() {
		t.Fatal("expected empty chain to have no authenticators")
	}

	chain.Add(&mockAuthenticator{name: "test"})
	if !chain.HasAuthenticators() {
		t.Fatal("expected chain to have authenticator after Add")
	}
}

type mockAuthorizer struct {
	name       string
	shouldPass bool
}

func (m *mockAuthorizer) Authorize(r *http.Request, id *Identity) bool {
	return m.shouldPass
}

func (m *mockAuthorizer) Name() string {
	return m.name
}

func TestAuthorizerChain_ANDLogic(t *testing.T) {
	id := &Identity{ID: "test"}
	req, _ := http.NewRequest("GET", "/", nil)

	allPass := NewAuthorizerChain(
		&mockAuthorizer{shouldPass: true},
		&mockAuthorizer{shouldPass: true},
	)
	if !allPass.Authorize(req, id) {
		t.Fatal("expected all pass authorizers to succeed")
	}

	oneFails := NewAuthorizerChain(
		&mockAuthorizer{shouldPass: true},
		&mockAuthorizer{shouldPass: false},
	)
	if oneFails.Authorize(req, id) {
		t.Fatal("expected one failing authorizer to reject")
	}

	empty := NewAuthorizerChain()
	if !empty.Authorize(req, id) {
		t.Fatal("expected empty authorizer chain to pass")
	}
}

func TestAuthorizerChain_Add(t *testing.T) {
	chain := NewAuthorizerChain()
	chain.Add(&mockAuthorizer{shouldPass: true})

	id := &Identity{ID: "test"}
	req, _ := http.NewRequest("GET", "/", nil)

	if !chain.Authorize(req, id) {
		t.Fatal("expected added authorizer to be used")
	}
}
