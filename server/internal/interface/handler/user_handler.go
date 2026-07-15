package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	userusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/user"
)

// UserHandler handles HTTP requests for the authenticated caller's own user
// identity. It delegates business logic to UserUsecase.
type UserHandler struct {
	usecase *userusecase.UserUsecase
}

// NewUserHandler creates a new UserHandler with the given UserUsecase.
func NewUserHandler(usecase *userusecase.UserUsecase) *UserHandler {
	return &UserHandler{usecase: usecase}
}

// Me handles GET /users/me (authenticated). It returns the authenticated
// user's own identity as a UserResponse (id, email, username, created_at —
// never the password hash). It returns HTTP 404 if the user record no longer
// exists and HTTP 500 for unexpected errors.
func (h *UserHandler) Me(c echo.Context) error {
	userID := middleware.GetUserID(c)

	u, err := h.usecase.Get(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return c.JSON(http.StatusNotFound, ErrorResponse{Message: "user not found"})
		}
		middleware.GetLogger(c).Error("failed to fetch current user", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusOK, UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Username:  u.Username,
		CreatedAt: u.CreatedAt,
	})
}
