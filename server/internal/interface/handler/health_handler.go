package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthHandler handles health check HTTP requests. It provides a simple
// liveness probe for load balancers and orchestration systems.
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health handles GET /health. It returns HTTP 200 with a JSON body
// containing {"status": "ok"} to indicate the server is running.
func (h *HealthHandler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
