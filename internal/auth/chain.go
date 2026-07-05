package auth

import "net/http"

type AuthenticatorChain struct {
	authenticators []Authenticator
}

func NewAuthenticatorChain(authenticators ...Authenticator) *AuthenticatorChain {
	return &AuthenticatorChain{authenticators: authenticators}
}

func (c *AuthenticatorChain) Add(a Authenticator) {
	c.authenticators = append(c.authenticators, a)
}

func (c *AuthenticatorChain) Authenticate(r *http.Request) (*Identity, bool, error) {
	for _, a := range c.authenticators {
		id, ok, err := a.Authenticate(r)
		if err != nil {
			// An authenticator recognised the credential but rejected it
			// (invalid/expired). Stop the chain and surface the error.
			return nil, false, err
		}
		if ok {
			return id, true, nil
		}
	}
	return nil, false, nil
}

func (c *AuthenticatorChain) HasAuthenticators() bool {
	return len(c.authenticators) > 0
}

type AuthorizerChain struct {
	authorizers []Authorizer
}

func NewAuthorizerChain(authorizers ...Authorizer) *AuthorizerChain {
	return &AuthorizerChain{authorizers: authorizers}
}

func (c *AuthorizerChain) Add(a Authorizer) {
	c.authorizers = append(c.authorizers, a)
}

func (c *AuthorizerChain) Authorize(r *http.Request, id *Identity) bool {
	for _, a := range c.authorizers {
		if !a.Authorize(r, id) {
			return false
		}
	}
	return true
}
