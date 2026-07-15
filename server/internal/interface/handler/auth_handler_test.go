package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
)

func TestRegisterHandler201(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := authusecase.NewAuthUsecase(svc)
	h := NewAuthHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"test@example.com","username":"user","password":"pass"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestRegisterHandler400(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := authusecase.NewAuthUsecase(svc)
	h := NewAuthHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"","username":"","password":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRegisterHandler409(t *testing.T) {
	svc := &mocks.AuthService{}
	svc.SeedRegistered("dup@example.com")
	uc := authusecase.NewAuthUsecase(svc)
	h := NewAuthHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"dup@example.com","username":"user","password":"pass"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Register(c); err != nil {
		t.Fatalf("Register handler error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestLoginHandler200(t *testing.T) {
	svc := &mocks.AuthService{}
	svc.SeedRegistered("test@example.com")
	uc := authusecase.NewAuthUsecase(svc)
	h := NewAuthHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"test@example.com","password":"correct"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLoginHandler401(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := authusecase.NewAuthUsecase(svc)
	h := NewAuthHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"no@example.com","password":"wrong"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login handler error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestAuthHandlerLogout proves that a request routed through a real
// middleware.JWTAuth instance (which populates middleware.GetToken) and then
// into AuthHandler.Logout returns HTTP 200.
func TestAuthHandlerLogout(t *testing.T) {
	svc := &mocks.AuthService{}
	uc := authusecase.NewAuthUsecase(svc)
	h := NewAuthHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := middleware.JWTAuth(svc, "ory_kratos_session")
	handler := mw(h.Logout)

	if err := handler(c); err != nil {
		t.Fatalf("Logout handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
