package security

import "errors"

type Principal struct {
	ID     string
	Role   string
	Scopes []string
}

type TokenAuthenticator struct {
	token string
}

func NewTokenAuthenticator(token string) *TokenAuthenticator {
	return &TokenAuthenticator{token: token}
}

func (a *TokenAuthenticator) Authenticate(token string) (Principal, error) {
	if a.token == "" {
		return Principal{ID: "local", Role: "operator", Scopes: []string{"operator.admin"}}, nil
	}
	if token != a.token {
		return Principal{}, errors.New("security: invalid token")
	}
	return Principal{ID: "operator", Role: "operator", Scopes: []string{"operator.admin"}}, nil
}
