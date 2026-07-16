package app

import "github.com/labstack/echo/v4"

// registerUserRoutes registers the authenticated whoami endpoint on the
// given group (the shared authenticated group built in NewRouter).
func registerUserRoutes(g *echo.Group, c *Container) {
	g.GET("/users/me", c.UserHandler.Me)
}
