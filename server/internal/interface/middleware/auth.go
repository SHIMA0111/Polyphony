package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
)

const userIDKey = "user_id"

type errorResponse struct {
	Message string `json:"message"`
}

// JWTAuth returns middleware that validates Bearer tokens using the given TokenValidator.
func JWTAuth(tokenValidator auth.TokenValidator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, errorResponse{Message: "missing authorization header"})
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return c.JSON(http.StatusUnauthorized, errorResponse{Message: "invalid authorization header format"})
			}

			claims, err := tokenValidator.ValidateToken(c.Request().Context(), parts[1])
			if err != nil {
				return c.JSON(http.StatusUnauthorized, errorResponse{Message: "invalid or expired token"})
			}

			c.Set(userIDKey, claims.UserID)
			return next(c)
		}
	}
}

// GetUserID extracts the authenticated user ID from the Echo context.
func GetUserID(c echo.Context) string {
	id, _ := c.Get(userIDKey).(string)
	return id
}
