package app

import "github.com/labstack/echo/v4"

// registerTokenRoutes registers the authenticated token estimation endpoint
// on the given group (the shared authenticated group built in NewRouter).
// Unlike the public GET /models, this endpoint requires an authenticated
// user since it is invoked while composing a message inside a room.
func registerTokenRoutes(g *echo.Group, c *Container) {
	g.POST("/tokens/estimate", c.TokenHandler.EstimateTokens)
}
