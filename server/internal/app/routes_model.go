package app

import "github.com/labstack/echo/v4"

// registerModelRoutes registers the public LLM model-listing endpoint.
func registerModelRoutes(e *echo.Echo, c *Container) {
	e.GET("/models", c.ModelHandler.List)
}
