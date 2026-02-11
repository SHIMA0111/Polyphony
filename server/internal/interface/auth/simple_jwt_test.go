package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// mockUserRepo implements user.UserRepository for testing.
type mockUserRepo struct {
	users map[string]*user.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*user.User)}
}

func (m *mockUserRepo) Create(_ context.Context, u *user.User) error {
	for _, existing := range m.users {
		if existing.Email == u.Email {
			return domain.ErrEmailAlreadyExists
		}
		if existing.Username == u.Username {
			return domain.ErrUsernameAlreadyExists
		}
	}
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*user.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*user.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*user.User, error) {
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) Update(_ context.Context, u *user.User) error {
	if _, ok := m.users[u.ID]; !ok {
		return domain.ErrNotFound
	}
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.users, id)
	return nil
}

func TestRegisterAndLogin(t *testing.T) {
	repo := newMockUserRepo()
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
	repo := newMockUserRepo()
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
	svc := NewSimpleJWTService(newMockUserRepo(), "test-secret")
	ctx := context.Background()

	// Create an expired token manually
	claims := jwt.RegisteredClaims{
		Subject:   "some-id",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte("test-secret"))

	_, err := svc.ValidateToken(ctx, tokenStr)
	if err != domain.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWrongSecret(t *testing.T) {
	svc := NewSimpleJWTService(newMockUserRepo(), "secret-a")
	ctx := context.Background()

	// Create token with different secret
	claims := jwt.RegisteredClaims{
		Subject:   "some-id",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte("secret-b"))

	_, err := svc.ValidateToken(ctx, tokenStr)
	if err != domain.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestDuplicateEmail(t *testing.T) {
	repo := newMockUserRepo()
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
