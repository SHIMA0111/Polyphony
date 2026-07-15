package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	userusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/user"
)

// mockUserRepoForHandler is a minimal in-memory UserRepository used only by
// user_handler_test.go.
type mockUserRepoForHandler struct {
	users map[string]*domainuser.User
}

func newMockUserRepoForHandler() *mockUserRepoForHandler {
	return &mockUserRepoForHandler{users: make(map[string]*domainuser.User)}
}

func (m *mockUserRepoForHandler) Create(_ context.Context, u *domainuser.User) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepoForHandler) GetByID(_ context.Context, id string) (*domainuser.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepoForHandler) GetByEmail(_ context.Context, _ string) (*domainuser.User, error) {
	return nil, domain.ErrNotFound
}

func (m *mockUserRepoForHandler) GetByUsername(_ context.Context, _ string) (*domainuser.User, error) {
	return nil, domain.ErrNotFound
}

func (m *mockUserRepoForHandler) Update(_ context.Context, u *domainuser.User) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepoForHandler) Delete(_ context.Context, id string) error {
	delete(m.users, id)
	return nil
}

func TestMeHandler200NoPasswordHash(t *testing.T) {
	repo := newMockUserRepoForHandler()
	now := time.Now()
	repo.users["user-1"] = &domainuser.User{
		ID:           "user-1",
		Email:        "test@example.com",
		Username:     "tester",
		PasswordHash: "super-secret-hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	uc := userusecase.NewUserUsecase(repo)
	h := NewUserHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.Me(c); err != nil {
		t.Fatalf("Me handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if _, ok := body["password_hash"]; ok {
		t.Fatal("response body must not contain password_hash")
	}
	if body["id"] != "user-1" || body["email"] != "test@example.com" || body["username"] != "tester" {
		t.Fatalf("unexpected response body: %v", body)
	}
	if _, ok := body["created_at"]; !ok {
		t.Fatal("expected created_at in response body")
	}
}

func TestMeHandler401WithoutToken(t *testing.T) {
	repo := newMockUserRepoForHandler()
	uc := userusecase.NewUserUsecase(repo)
	h := NewUserHandler(uc)

	svc := newMockAuthService()
	mw := middleware.JWTAuth(svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(h.Me)
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
