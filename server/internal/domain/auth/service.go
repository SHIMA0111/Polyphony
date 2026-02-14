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
