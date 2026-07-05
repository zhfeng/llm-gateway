package static

import (
	"errors"
	"fmt"
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

// Factory is the registry entry for the "static" authenticator. It reads
// config.keys ([]string) and returns a static Authenticator, or an error if
// keys is missing or not a string array. It is registered as
// auth.RegisterAuthenticator("static", Factory) by the server.
func Factory(cfg map[string]any) (auth.Authenticator, error) {
	keys, err := readKeys(cfg)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("config.keys must contain at least one key")
	}
	return NewAuthenticator(keys), nil
}

// readKeys extracts a []string from cfg["keys"], accepting both []string
// (programmatic configs) and []any (the shape produced by encoding/json, which
// is what config.AuthProviderConfig.Config holds after unmarshalling).
func readKeys(cfg map[string]any) ([]string, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	raw, ok := cfg["keys"]
	if !ok {
		return nil, errors.New("config.keys is required")
	}
	if raw == nil {
		return nil, errors.New("config.keys must not be null")
	}
	// Fast path: a real []string (e.g. constructed in Go).
	if arr, ok := raw.([]string); ok {
		return arr, nil
	}
	// JSON path: arrays unmarshal into []interface{} (== []any).
	ifaceArr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("config.keys must be an array of strings, got %T", raw)
	}
	keys := make([]string, 0, len(ifaceArr))
	for i, v := range ifaceArr {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("config.keys[%d] must be a string, got %T", i, v)
		}
		keys = append(keys, s)
	}
	return keys, nil
}

func (a *Authenticator) Name() string {
	return "static"
}

func (a *Authenticator) Authenticate(r *http.Request) (*auth.Identity, bool, error) {
	provided := bearerToken(r.Header.Get("Authorization"))
	if provided == "" {
		provided = r.Header.Get("x-api-key")
	}

	if _, ok := a.keys[provided]; ok {
		return &auth.Identity{
			ID:          "static_key",
			Type:        "api_key",
			Permissions: auth.Permissions{},
		}, true, nil
	}
	// A missing or non-matching key means "this credential is not for me";
	// the chain should try the next authenticator, so return no error.
	return nil, false, nil
}

// bearerToken returns the token from an "Authorization: Bearer <token>"
// header, or "" if the header is absent or uses a scheme other than "Bearer"
// (e.g. "Basic …"). A non-Bearer Authorization header must not be treated as
// a candidate key — the caller falls back to x-api-key instead, so an
// upstream-injected "Basic" header can't shadow a valid x-api-key and turn a
// successful auth into a silent 401.
func bearerToken(authorization string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return ""
	}
	return authorization[len(prefix):]
}
