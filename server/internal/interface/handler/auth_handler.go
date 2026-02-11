package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	authusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/auth"
)

// AuthHandler handles authentication HTTP requests.
type AuthHandler struct {
	usecase *authusecase.AuthUsecase
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(usecase *authusecase.AuthUsecase) *AuthHandler {
	return &AuthHandler{usecase: usecase}
}

// Register handles POST /auth/register.
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusCreated, TokenResponse{
		AccessToken: pair.AccessToken,
		TokenType:   pair.TokenType,
	})
}

// Login handles POST /auth/login.
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusOK, TokenResponse{
		AccessToken: pair.AccessToken,
		TokenType:   pair.TokenType,
	})
}
