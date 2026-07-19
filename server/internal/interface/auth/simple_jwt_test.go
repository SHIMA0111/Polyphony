package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func TestRegisterAndLogin(t *testing.T) {
	repo := &mocks.UserRepo{}
	svc := NewSimpleJWTService(repo, "test-secret")
	ctx := context.Background()

	pair, err := svc.Register(ctx, "test@example.com", "testuser", "password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if pair.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
	if pair.TokenType != "Bearer" {
		t.Fatalf("expected Bearer token type, got %s", pair.TokenType)
	}

	// Login with correct credentials
	loginPair, err := svc.Login(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if loginPair.AccessToken == "" {
		t.Fatal("expected non-empty access token on login")
	}

	// Login with wrong password
	_, err = svc.Login(ctx, "test@example.com", "wrong")
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// Login with non-existent email
	_, err = svc.Login(ctx, "no@example.com", "password123")
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	repo := &mocks.UserRepo{}
	svc := NewSimpleJWTService(repo, "test-secret")
	ctx := context.Background()

	pair, err := svc.Register(ctx, "test@example.com", "testuser", "password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	claims, err := svc.ValidateToken(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.UserID == "" {
		t.Fatal("expected non-empty user ID in claims")
	}
}

func TestExpiredToken(t *testing.T) {
	svc := NewSimpleJWTService(&mocks.UserRepo{}, "test-secret")
	ctx := context.Background()

	// Create an expired token manually
	claims := jwt.RegisteredClaims{
		Subject:   "some-id",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("SignedString failed: %v", err)
	}

	_, err = svc.ValidateToken(ctx, tokenStr)
	if err != domain.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWrongSecret(t *testing.T) {
	svc := NewSimpleJWTService(&mocks.UserRepo{}, "secret-a")
	ctx := context.Background()

	// Create token with different secret
	claims := jwt.RegisteredClaims{
		Subject:   "some-id",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("secret-b"))
	if err != nil {
		t.Fatalf("SignedString failed: %v", err)
	}

	_, err = svc.ValidateToken(ctx, tokenStr)
	if err != domain.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestDuplicateEmail(t *testing.T) {
	repo := &mocks.UserRepo{}
	svc := NewSimpleJWTService(repo, "test-secret")
	ctx := context.Background()

	_, err := svc.Register(ctx, "dup@example.com", "user1", "password123")
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	_, err = svc.Register(ctx, "dup@example.com", "user2", "password123")
	if err != domain.ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := hashPassword("mypassword")
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}

	if !verifyPassword("mypassword", hash) {
		t.Fatal("verifyPassword should return true for correct password")
	}

	if verifyPassword("wrongpassword", hash) {
		t.Fatal("verifyPassword should return false for wrong password")
	}
}
