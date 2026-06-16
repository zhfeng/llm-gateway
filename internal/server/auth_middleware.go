package server

import (
	"context"
	"net/http"

	"github.com/zhfeng/llm-gateway/internal/auth"
	"github.com/zhfeng/llm-gateway/internal/gwerror"
)

func AuthMiddleware(authn *auth.AuthenticatorChain, authz *auth.AuthorizerChain, disabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if disabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authn.HasAuthenticators() {
				gwerror.WriteOpenAI(w, gwerror.New(http.StatusUnauthorized, "authentication_error", "gateway API key is required"))
				return
			}

			identity, authenticated := authn.Authenticate(r)
			if !authenticated {
				gwerror.WriteOpenAI(w, gwerror.New(http.StatusUnauthorized, "authentication_error", "invalid API key"))
				return
			}

			if !authz.Authorize(r, identity) {
				gwerror.WriteOpenAI(w, gwerror.New(http.StatusForbidden, "permission_error", "insufficient permissions"))
				return
			}

			ctx := context.WithValue(r.Context(), auth.IdentityKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
