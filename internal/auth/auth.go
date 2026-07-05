package auth

import (
	"context"
	"net/http"
)

type Identity struct {
	ID          string
	Type        string
	Permissions Permissions
	Metadata    map[string]string
}

type Permissions struct {
	Models   map[string]bool
	Metadata map[string]string
}

// Authenticator authenticates an incoming HTTP request.
//
// Authenticate returns three values:
//   - ident: the authenticated Identity, non-nil when ok is true.
//   - ok: true if this authenticator successfully authenticated the request.
//   - err: non-nil when the authenticator recognised the credential but
//     rejected it as invalid or expired (e.g. a malformed or expired JWT).
//
// Chain semantics:
//
//	ok=true,  err=nil  → authenticated; stop, use ident.
//	ok=false, err=nil  → credential not for this authenticator; try next.
//	ok=false, err!=nil → credential recognised but invalid/expired; stop
//	                     and surface err so the client gets a specific reason
//	                     instead of a generic "invalid API key".
type Authenticator interface {
	Authenticate(r *http.Request) (ident *Identity, ok bool, err error)
	Name() string
}

// AuthError is returned by an Authenticator when it recognises a credential
// but rejects it as invalid or expired (e.g. a malformed or expired JWT).
// The auth middleware surfaces Type and Message in the 401 response body so
// the client gets an actionable signal instead of a generic "invalid API key".
//
// An authenticator that simply does not recognise a credential (no matching
// header, wrong scheme) returns ok=false with a nil error, so the chain
// continues to the next authenticator.
type AuthError struct {
	Type    string // machine-readable, e.g. "expired_token", "malformed_token"
	Message string // human-readable
}

func (e *AuthError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Type
}

type Authorizer interface {
	Authorize(r *http.Request, id *Identity) bool
	Name() string
}

type contextKey string

const IdentityKey contextKey = "identity"

func FromContext(ctx context.Context) *Identity {
	if id, ok := ctx.Value(IdentityKey).(*Identity); ok {
		return id
	}
	return nil
}
