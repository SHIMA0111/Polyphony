package auth

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// TestAuthUsecaseRegister verifies Register succeeds and returns a token
// pair for a new, unique email/username.
func TestAuthUsecaseRegister(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := NewAuthUsecase(svc)
	ctx := context.Background()

	pair, err := uc.Register(ctx, "test@example.com", "user", "pass")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if pair.AccessToken != "tok" {
		t.Fatalf("expected tok, got %s", pair.AccessToken)
	}
}

// TestAuthUsecaseRegisterDuplicate verifies Register returns
// domain.ErrEmailAlreadyExists when registering a second account with an
// email already in use.
func TestAuthUsecaseRegisterDuplicate(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := NewAuthUsecase(svc)
	ctx := context.Background()

	_, _ = uc.Register(ctx, "dup@example.com", "user", "pass")
	_, err := uc.Register(ctx, "dup@example.com", "user2", "pass")
	if err != domain.ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

// TestAuthUsecaseLogin verifies Login succeeds with correct credentials and
// returns domain.ErrInvalidCredentials with an incorrect password.
func TestAuthUsecaseLogin(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := NewAuthUsecase(svc)
	ctx := context.Background()

	_, _ = uc.Register(ctx, "test@example.com", "user", "pass")

	pair, err := uc.Login(ctx, "test@example.com", "correct")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if pair.AccessToken != "tok" {
		t.Fatalf("expected tok, got %s", pair.AccessToken)
	}

	_, err = uc.Login(ctx, "test@example.com", "wrong")
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
