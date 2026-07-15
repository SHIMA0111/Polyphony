package app

import "github.com/labstack/echo/v4"

// registerAttachmentRoutes registers the authenticated attachment endpoints
// on the given group (the shared authenticated group built in NewRouter).
func registerAttachmentRoutes(g *echo.Group, c *Container) {
	g.POST("/rooms/:roomId/attachments/upload-url", c.AttachmentHandler.RequestUpload)
	g.POST("/rooms/:roomId/messages/:messageId/attachments", c.AttachmentHandler.Attach)
	g.GET("/rooms/:roomId/messages/:messageId/attachments", c.AttachmentHandler.List)
}
