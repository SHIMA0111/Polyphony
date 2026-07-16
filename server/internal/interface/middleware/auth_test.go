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

// testCookieName is the session cookie name used across the cookie-based
// extraction test cases below.
const testCookieName = "ory_kratos_session"

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
	mw := JWTAuth(svc, testCookieName)

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
	mw := JWTAuth(svc, testCookieName)

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
	mw := JWTAuth(svc, testCookieName)

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
	mw := JWTAuth(svc, testCookieName)

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

// TestJWTAuthValidCookie proves that, when no Authorization header is
// present, JWTAuth falls back to the named session cookie and forwards its
// value to ValidateToken with the "cookie:" prefix.
func TestJWTAuthValidCookie(t *testing.T) {
	svc := &mocks.AuthService{
		ValidateTokenFunc: func(_ context.Context, token string) (*domainauth.Claims, error) {
			if token == "cookie:valid-cookie-value" {
				return &domainauth.Claims{UserID: "user-1"}, nil
			}
			return nil, domain.ErrInvalidToken
		},
	}
	mw := JWTAuth(svc, testCookieName)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: "valid-cookie-value"})
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

// TestJWTAuthMissingHeaderAndCookie proves that JWTAuth returns 401 when
// neither an Authorization header nor the named cookie is present.
func TestJWTAuthMissingHeaderAndCookie(t *testing.T) {
	svc := newFixedTokenAuthService("valid-token")
	mw := JWTAuth(svc, testCookieName)

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

// TestJWTAuthCookieRejectedByValidator proves that JWTAuth returns 401 when
// the named cookie is present but the validator rejects its value.
func TestJWTAuthCookieRejectedByValidator(t *testing.T) {
	svc := &mocks.AuthService{
		ValidateTokenFunc: func(_ context.Context, _ string) (*domainauth.Claims, error) {
			return nil, domain.ErrInvalidToken
		},
	}
	mw := JWTAuth(svc, testCookieName)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: "rejected-cookie-value"})
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
