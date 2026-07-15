package app

import "github.com/labstack/echo/v4"

// registerAuthRoutes registers the public registration and login endpoints.
func registerAuthRoutes(e *echo.Echo, c *Container) {
	e.POST("/auth/register", c.AuthHandler.Register)
	e.POST("/auth/login", c.AuthHandler.Login)
}
