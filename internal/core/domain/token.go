// Package domain contains the core entities and value objects of the
// application. It has zero dependencies on any framework, transport or
// storage technology — that is the whole point of keeping it at the center
// of the hexagon.
package domain

import "time"

// expirySkew is subtracted from the token's real expiry so that callers
// treat a token as expired slightly before Spotify actually rejects it,
// avoiding races between "IsExpired" checks and the request that follows.
const expirySkew = 30 * time.Second

// Token represents an OAuth 2.0 credential issued by Spotify's Accounts
// service. It is a plain value object: it knows nothing about how it was
// obtained (browser flow, refresh, ...) or where it is persisted.
type Token struct {
	AccessToken  string
	TokenType    string
	Scope        string
	RefreshToken string
	ExpiresAt    time.Time
}

// IsExpired reports whether the token is expired, or close enough to
// expiring that it should be refreshed before use.
func (t Token) IsExpired() bool {
	if t.AccessToken == "" {
		return true
	}
	return time.Now().After(t.ExpiresAt.Add(-expirySkew))
}

// IsZero reports whether the token holds no credentials at all, e.g. when
// nothing has been persisted yet.
func (t Token) IsZero() bool {
	return t.AccessToken == "" && t.RefreshToken == ""
}
