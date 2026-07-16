// Package middleware provides Echo middleware: JWT authentication and request-scoped structured logging.
package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
)

const userIDKey = "user_id"

// cookieTokenPrefix marks a token value extracted from a session cookie
// (rather than an Authorization header) before it is passed to
// TokenValidator.ValidateToken. This mirrors (and must stay in sync with)
// the identical constant in interface/auth/kratos.go, which documents the
// full contract: KratosAuthService.ValidateToken strips this prefix and
// forwards the remainder as a Cookie header, while SimpleJWTService never
// receives a prefixed value in practice since SimpleJWT never sets a cookie.
const cookieTokenPrefix = "cookie:"

type errorResponse struct {
	Message string `json:"message"`
}

// JWTAuth returns an Echo middleware that authenticates requests using the
// given TokenValidator, accepting credentials from either an
// "Authorization: Bearer <token>" header or a named session cookie.
//
// When the Authorization header is present, its bearer token is passed to
// ValidateToken unprefixed, exactly as before this cookie support was added
// (this covers both SimpleJWT JWTs and Kratos native/API session tokens
// returned by KratosAuthService.Register/Login, both of which
// KratosAuthService.ValidateToken treats as opaque X-Session-Token values).
//
// When the Authorization header is absent, JWTAuth falls back to reading a
// cookie named cookieName (e.g. "ory_kratos_session"); if present, its value
// is passed to ValidateToken with a "cookie:" prefix, per the contract
// documented on KratosAuthService.ValidateToken.
//
// It stores the authenticated user ID in the Echo context for downstream
// handlers via GetUserID. If neither the header nor the named cookie is
// present, the header is malformed, or the resolved token/cookie is
// invalid/expired, it returns HTTP 401.
func JWTAuth(tokenValidator auth.TokenValidator, cookieName string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token, ok := extractCredential(c, cookieName)
			if !ok {
				return c.JSON(http.StatusUnauthorized, errorResponse{Message: "missing authorization header or session cookie"})
			}
			if token == "" {
				return c.JSON(http.StatusUnauthorized, errorResponse{Message: "invalid authorization header format"})
			}

			claims, err := tokenValidator.ValidateToken(c.Request().Context(), token)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, errorResponse{Message: "invalid or expired token"})
			}

			c.Set(userIDKey, claims.UserID)
			return next(c)
		}
	}
}

// extractCredential resolves the credential to pass to TokenValidator.ValidateToken
// from the current request. It returns (token, true) on success, where token
// is either a bare bearer token (Authorization header) or a "cookie:"-prefixed
// cookie value (see KratosAuthService.ValidateToken); ("", true) if an
// Authorization header is present but malformed; and ("", false) if neither
// an Authorization header nor the named cookie is present at all.
func extractCredential(c echo.Context, cookieName string) (string, bool) {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			return "", true
		}
		return parts[1], true
	}

	cookie, err := c.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookieTokenPrefix + cookie.Value, true
}

// GetUserID extracts the authenticated user ID from the Echo context. It
// returns an empty string if the user ID is not set, which indicates that the
// JWTAuth middleware was not applied or the request is unauthenticated.
func GetUserID(c echo.Context) string {
	id, _ := c.Get(userIDKey).(string)
	return id
}
