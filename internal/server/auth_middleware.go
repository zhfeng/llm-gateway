package server

import (
	"context"
	"errors"
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

			identity, authenticated, err := authn.Authenticate(r)
			if err != nil {
				// An authenticator recognised the credential but rejected it
				// (e.g. an expired token). Surface its specific reason instead
				// of the generic "invalid API key" so the client gets an
				// actionable signal.
				var ae *auth.AuthError
				if errors.As(err, &ae) {
					gwerror.WriteOpenAI(w, gwerror.New(http.StatusUnauthorized, ae.Type, ae.Message))
				} else {
					gwerror.WriteOpenAI(w, gwerror.New(http.StatusUnauthorized, "authentication_error", err.Error()))
				}
				return
			}
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
