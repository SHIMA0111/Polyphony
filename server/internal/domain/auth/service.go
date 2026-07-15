// Package auth defines the authentication service port (token issuance and validation).
package auth

import "context"

// TokenPair holds access token and its type.
type TokenPair struct {
	AccessToken string
	TokenType   string
}

// Claims holds the decoded claims from a validated token.
type Claims struct {
	UserID string
}

// AuthService defines authentication operations.
// This is the swap point for Phase 9 (Ory Kratos).
type AuthService interface {
	// Register creates a new user with a hashed password and returns a token pair.
	Register(ctx context.Context, email, username, password string) (*TokenPair, error)

	// Login authenticates a user and returns a token pair.
	Login(ctx context.Context, email, password string) (*TokenPair, error)

	// ValidateToken validates a token and returns the claims.
	// Returns ErrInvalidToken if the token is invalid or expired.
	ValidateToken(ctx context.Context, token string) (*Claims, error)
}

// Revoker is an optional capability interface an AuthService implementation
// may satisfy to support server-side session revocation on logout (Step 33).
// It is deliberately kept separate from AuthService rather than folded into
// it as a fourth method: stateless mechanisms (e.g. SimpleJWTService's HMAC
// JWTs) have no server-side session to revoke, so requiring every
// AuthService to implement Revoke would force a fake/no-op implementation on
// backends for which "logout" has no real meaning beyond the client
// discarding its token. Callers (usecase/auth.AuthUsecase.Logout) must type-
// assert an AuthService value against this interface and treat its absence
// as a no-op rather than an error.
type Revoker interface {
	// Revoke invalidates token server-side so a subsequent ValidateToken call
	// with the same token fails, even if the token has not otherwise expired.
	// It returns an error only for unexpected failures; an already-expired or
	// already-revoked token is not an error (revocation is idempotent).
	Revoke(ctx context.Context, token string) error
}
