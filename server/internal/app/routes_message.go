package app

import "github.com/labstack/echo/v4"

// registerMessageRoutes registers the authenticated message endpoints on the
// given group (the shared authenticated group built in NewRouter).
func registerMessageRoutes(g *echo.Group, c *Container) {
	g.POST("/rooms/:roomId/messages", c.MessageHandler.Send)
	g.GET("/rooms/:roomId/messages", c.MessageHandler.List)
	g.POST("/rooms/:roomId/messages/ai", c.MessageHandler.SendAI)
	g.POST("/rooms/:roomId/messages/:messageId/regenerate", c.MessageHandler.RegenerateAI)
	g.DELETE("/rooms/:roomId/messages/:messageId", c.MessageHandler.Delete)
	g.PATCH("/rooms/:roomId/messages/:messageId", c.MessageHandler.UpdateExclude)
}
