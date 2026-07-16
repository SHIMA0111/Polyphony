package app

import "github.com/labstack/echo/v4"

// registerHealthRoutes registers the public liveness-probe endpoint.
func registerHealthRoutes(e *echo.Echo, c *Container) {
	e.GET("/health", c.HealthHandler.Health)
}
