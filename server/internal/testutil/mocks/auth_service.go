package mocks

import (
	"context"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
)

// AuthService is a configurable fake implementing domainauth.AuthService.
//
// By default it tracks registered emails in memory: Register fails with
// domain.ErrEmailAlreadyExists for an email already seen, Login succeeds only
// for a registered email with password "correct", and ValidateToken always
// returns a fixed claim for user "user-1". Any of these three behaviors can
// be overridden per test by setting the corresponding *Func field.
//
// The zero value (mocks.AuthService{}) is ready to use.
type AuthService struct {
	// RegisterFunc, if set, overrides the default Register behavior.
	RegisterFunc func(ctx context.Context, email, username, password string) (*domainauth.TokenPair, error)
	// LoginFunc, if set, overrides the default Login behavior.
	LoginFunc func(ctx context.Context, email, password string) (*domainauth.TokenPair, error)
	// ValidateTokenFunc, if set, overrides the default ValidateToken behavior.
	ValidateTokenFunc func(ctx context.Context, token string) (*domainauth.Claims, error)
	// RevokeFunc, if set, overrides the default Revoke behavior (a no-op
	// returning nil). Setting it lets a test make AuthService satisfy
	// domainauth.Revoker with assertable behavior (e.g. to verify
	// AuthUsecase.Logout/CachedAuthService.Revoke delegate to it correctly);
	// leaving it unset keeps every existing test's no-op behavior unchanged.
	RevokeFunc func(ctx context.Context, token string) error

	mu         sync.Mutex
	Registered map[string]bool // default-Register bookkeeping of already-registered emails
}

// SeedRegistered marks email as already registered under the default
// Register/Login behavior, letting a test exercise duplicate-registration or
// successful-login scenarios without first calling Register.
func (a *AuthService) SeedRegistered(email string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Registered == nil {
		a.Registered = make(map[string]bool)
	}
	a.Registered[email] = true
}

// Register creates a new user with a hashed password and returns a token
// pair. The default implementation returns domain.ErrEmailAlreadyExists if
// email was already registered (via a prior Register call or SeedRegistered),
// otherwise records it as registered and returns a fixed token pair.
func (a *AuthService) Register(ctx context.Context, email, username, password string) (*domainauth.TokenPair, error) {
	if a.RegisterFunc != nil {
		return a.RegisterFunc(ctx, email, username, password)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Registered == nil {
		a.Registered = make(map[string]bool)
	}
	if a.Registered[email] {
		return nil, domain.ErrEmailAlreadyExists
	}
	a.Registered[email] = true
	return &domainauth.TokenPair{AccessToken: "tok", TokenType: "Bearer"}, nil
}

// Login authenticates a user and returns a token pair. The default
// implementation succeeds only if email is registered and password equals
// "correct"; otherwise it returns domain.ErrInvalidCredentials.
func (a *AuthService) Login(ctx context.Context, email, password string) (*domainauth.TokenPair, error) {
	if a.LoginFunc != nil {
		return a.LoginFunc(ctx, email, password)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.Registered[email] || password != "correct" {
		return nil, domain.ErrInvalidCredentials
	}
	return &domainauth.TokenPair{AccessToken: "tok", TokenType: "Bearer"}, nil
}

// ValidateToken validates a token and returns its claims. The default
// implementation accepts any token and always returns claims for "user-1";
// set ValidateTokenFunc to test invalid/expired token handling.
func (a *AuthService) ValidateToken(ctx context.Context, token string) (*domainauth.Claims, error) {
	if a.ValidateTokenFunc != nil {
		return a.ValidateTokenFunc(ctx, token)
	}
	return &domainauth.Claims{UserID: "user-1"}, nil
}

// Revoke satisfies domainauth.Revoker, making AuthService usable to test
// Logout/CachedAuthService.Revoke's delegation to a Revoker. The default
// implementation is a no-op returning nil; set RevokeFunc to assert Revoke
// is called with the expected token.
func (a *AuthService) Revoke(ctx context.Context, token string) error {
	if a.RevokeFunc != nil {
		return a.RevokeFunc(ctx, token)
	}
	return nil
}
