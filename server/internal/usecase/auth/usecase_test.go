package auth

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
)

type mockAuthService struct {
	registered map[string]bool
}

func newMockAuthService() *mockAuthService {
	return &mockAuthService{registered: make(map[string]bool)}
}

func (m *mockAuthService) Register(_ context.Context, email, _, _ string) (*domainauth.TokenPair, error) {
	if m.registered[email] {
		return nil, domain.ErrEmailAlreadyExists
	}
	m.registered[email] = true
	return &domainauth.TokenPair{AccessToken: "tok", TokenType: "Bearer"}, nil
}

func (m *mockAuthService) Login(_ context.Context, email, password string) (*domainauth.TokenPair, error) {
	if !m.registered[email] || password != "correct" {
		return nil, domain.ErrInvalidCredentials
	}
	return &domainauth.TokenPair{AccessToken: "tok", TokenType: "Bearer"}, nil
}

func (m *mockAuthService) ValidateToken(_ context.Context, _ string) (*domainauth.Claims, error) {
	return &domainauth.Claims{UserID: "user-1"}, nil
}

func TestAuthUsecaseRegister(t *testing.T) {
	svc := newMockAuthService()
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

func TestAuthUsecaseRegisterDuplicate(t *testing.T) {
	svc := newMockAuthService()
	uc := NewAuthUsecase(svc)
	ctx := context.Background()

	_, _ = uc.Register(ctx, "dup@example.com", "user", "pass")
	_, err := uc.Register(ctx, "dup@example.com", "user2", "pass")
	if err != domain.ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

func TestAuthUsecaseLogin(t *testing.T) {
	svc := newMockAuthService()
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
