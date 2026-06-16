package static

import (
	"net/http"
	"strings"

	"github.com/zhfeng/llm-gateway/internal/auth"
)

type Authenticator struct {
	keys map[string]struct{}
}

func NewAuthenticator(keys []string) *Authenticator {
	keyMap := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k != "" {
			keyMap[k] = struct{}{}
		}
	}
	return &Authenticator{keys: keyMap}
}

func (a *Authenticator) Name() string {
	return "static"
}

func (a *Authenticator) Authenticate(r *http.Request) (*auth.Identity, bool) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if provided == "" {
		provided = r.Header.Get("x-api-key")
	}

	if _, ok := a.keys[provided]; ok {
		return &auth.Identity{
			ID:          "static_key",
			Type:        "api_key",
			Permissions: auth.Permissions{},
		}, true
	}
	return nil, false
}
