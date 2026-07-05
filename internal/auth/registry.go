package auth

import (
	"fmt"
	"sync"

	"github.com/zhfeng/llm-gateway/internal/config"
)

// AuthenticatorFactory constructs an Authenticator from a plugin config map
// (the `config` field of an `auth.authenticators[]` entry). Implementations
// should validate their config and return an error on invalid input so that
// misconfiguration fails fast at startup instead of being silently dropped.
type AuthenticatorFactory func(cfg map[string]any) (Authenticator, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]AuthenticatorFactory{}
)

// RegisterAuthenticator registers a factory for the given authenticator type.
// Registering the same type twice replaces the previous factory; this keeps
// registration idempotent so callers (and tests) may re-register freely.
func RegisterAuthenticator(typ string, factory AuthenticatorFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[typ] = factory
}

// LookupAuthenticator returns the factory registered for typ, or false if no
// factory has been registered.
func LookupAuthenticator(typ string) (AuthenticatorFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[typ]
	return f, ok
}

// BuildAuthenticators constructs the authenticators described by specs using
// the registered factories. An unknown type or a factory error fails fast:
// the error is returned (and surfaces as a startup error from the server) so
// that a typo'd `type` can never silently disable authentication.
func BuildAuthenticators(specs []config.AuthProviderConfig) ([]Authenticator, error) {
	authenticators := make([]Authenticator, 0, len(specs))
	for i, spec := range specs {
		factory, ok := LookupAuthenticator(spec.Type)
		if !ok {
			return nil, fmt.Errorf("auth.authenticators[%d]: unknown authenticator type %q", i, spec.Type)
		}
		a, err := factory(spec.Config)
		if err != nil {
			return nil, fmt.Errorf("auth.authenticators[%d] (%s): %w", i, spec.Type, err)
		}
		if a == nil {
			return nil, fmt.Errorf("auth.authenticators[%d] (%s): factory returned nil authenticator", i, spec.Type)
		}
		authenticators = append(authenticators, a)
	}
	return authenticators, nil
}
