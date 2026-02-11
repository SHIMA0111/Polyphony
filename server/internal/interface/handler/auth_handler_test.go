package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
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

func TestRegisterHandler201(t *testing.T) {
	svc := newMockAuthService()
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
	svc := newMockAuthService()
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
	svc := newMockAuthService()
	svc.registered["dup@example.com"] = true
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
	svc := newMockAuthService()
	svc.registered["test@example.com"] = true
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
	svc := newMockAuthService()
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
