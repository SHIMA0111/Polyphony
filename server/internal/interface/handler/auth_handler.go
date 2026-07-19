// Package handler implements the Echo HTTP handlers and their request/response DTOs.
package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
)

// AuthHandler handles HTTP requests for authentication endpoints, including
// user registration and login. It delegates business logic to AuthUsecase.
type AuthHandler struct {
	usecase *authusecase.AuthUsecase
}

// NewAuthHandler creates a new AuthHandler with the given AuthUsecase.
func NewAuthHandler(usecase *authusecase.AuthUsecase) *AuthHandler {
	return &AuthHandler{usecase: usecase}
}

// Register handles POST /auth/register. It binds the request body to a
// RegisterRequest, validates that email, username, and password are non-empty,
// and creates a new user account. On success it returns HTTP 201 with a
// TokenResponse. It returns HTTP 400 for invalid or incomplete input,
// HTTP 409 if the email or username already exists, and HTTP 500 for
// unexpected errors.
func (h *AuthHandler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Email == "" || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "email, username, and password are required"})
	}

	pair, err := h.usecase.Register(c.Request().Context(), req.Email, req.Username, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			return c.JSON(http.StatusConflict, ErrorResponse{Message: "email already exists"})
		}
		if errors.Is(err, domain.ErrUsernameAlreadyExists) {
			return c.JSON(http.StatusConflict, ErrorResponse{Message: "username already exists"})
		}
		middleware.GetLogger(c).Error("failed to register user", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusCreated, TokenResponse{
		AccessToken: pair.AccessToken,
		TokenType:   pair.TokenType,
	})
}

// Login handles POST /auth/login. It binds the request body to a
// LoginRequest, validates that email and password are provided, and
// authenticates the user. On success it returns HTTP 200 with a TokenResponse.
// It returns HTTP 400 for invalid or incomplete input, HTTP 401 for invalid
// credentials, and HTTP 500 for unexpected errors.
func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "email and password are required"})
	}

	pair, err := h.usecase.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			return c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid credentials"})
		}
		middleware.GetLogger(c).Error("failed to log in user", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusOK, TokenResponse{
		AccessToken: pair.AccessToken,
		TokenType:   pair.TokenType,
	})
}

// Logout handles POST /auth/logout. It is only reachable behind
// middleware.JWTAuth, so middleware.GetToken(c) always returns the caller's
// authenticated credential when this runs. It delegates to
// AuthUsecase.Logout — for a backend with server-side session revocation
// (AUTH_MODE=kratos), this immediately invalidates the session; for a
// backend without one (AUTH_MODE=simple_jwt), Logout is a documented no-op.
// Either way, it returns HTTP 200 with an empty JSON object on success, and
// HTTP 500 (logged via middleware.GetLogger) if Logout returns an unexpected
// error.
func (h *AuthHandler) Logout(c echo.Context) error {
	token := middleware.GetToken(c)

	if err := h.usecase.Logout(c.Request().Context(), token); err != nil {
		middleware.GetLogger(c).Error("failed to log out user", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{})
}
