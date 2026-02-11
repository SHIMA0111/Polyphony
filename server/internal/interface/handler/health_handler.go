package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthHandler handles health check HTTP requests.
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health handles GET /health.
func (h *HealthHandler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
