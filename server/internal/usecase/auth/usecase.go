package auth

import (
	"context"

	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
)

// AuthUsecase wraps AuthService to provide authentication use cases.
type AuthUsecase struct {
	authService domainauth.AuthService
}

// NewAuthUsecase creates a new AuthUsecase.
func NewAuthUsecase(authService domainauth.AuthService) *AuthUsecase {
	return &AuthUsecase{authService: authService}
}

// Register creates a new user account and returns a token pair.
func (u *AuthUsecase) Register(ctx context.Context, email, username, password string) (*domainauth.TokenPair, error) {
	return u.authService.Register(ctx, email, username, password)
}

// Login authenticates a user and returns a token pair.
func (u *AuthUsecase) Login(ctx context.Context, email, password string) (*domainauth.TokenPair, error) {
	return u.authService.Login(ctx, email, password)
}
