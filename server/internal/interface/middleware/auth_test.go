package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// newFixedTokenAuthService returns a mocks.AuthService whose ValidateToken
// only accepts validToken, mirroring the middleware's need for a
// deterministic valid/invalid token check independent of the default
// registered-email bookkeeping.
func newFixedTokenAuthService(validToken string) *mocks.AuthService {
	return &mocks.AuthService{
		ValidateTokenFunc: func(_ context.Context, token string) (*domainauth.Claims, error) {
			if token == validToken {
				return &domainauth.Claims{UserID: "user-1"}, nil
			}
			return nil, domain.ErrInvalidToken
		},
	}
}

func TestJWTAuthValidToken(t *testing.T) {
	svc := newFixedTokenAuthService("valid-token")
	mw := JWTAuth(svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(func(c echo.Context) error {
		uid := GetUserID(c)
		if uid != "user-1" {
			t.Fatalf("expected user-1, got %s", uid)
		}
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestJWTAuthMissingHeader(t *testing.T) {
	svc := newFixedTokenAuthService("valid-token")
	mw := JWTAuth(svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuthInvalidToken(t *testing.T) {
	svc := newFixedTokenAuthService("valid-token")
	mw := JWTAuth(svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuthInvalidFormat(t *testing.T) {
	svc := newFixedTokenAuthService("valid-token")
	mw := JWTAuth(svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic some-creds")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
