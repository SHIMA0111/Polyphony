package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	userusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/user"
)

func TestMeHandler200NoPasswordHash(t *testing.T) {
	repo := &mocks.UserRepo{}
	now := time.Now()
	repo.Users = map[string]*domainuser.User{}
	repo.Users["user-1"] = &domainuser.User{
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
	repo := &mocks.UserRepo{}
	uc := userusecase.NewUserUsecase(repo)
	h := NewUserHandler(uc)

	svc := &mocks.AuthService{}
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
