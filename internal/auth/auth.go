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

type Authenticator interface {
	Authenticate(r *http.Request) (*Identity, bool)
	Name() string
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
