package app

import "github.com/labstack/echo/v4"

// registerGroupRoutes registers the authenticated personal group CRUD and
// membership endpoints, plus the batch-invitation-by-group endpoint, on the
// given group (the shared authenticated group built in NewRouter).
func registerGroupRoutes(g *echo.Group, c *Container) {
	g.POST("/groups", c.GroupHandler.Create)
	g.GET("/groups", c.GroupHandler.List)
	g.GET("/groups/:groupId", c.GroupHandler.Get)
	g.PUT("/groups/:groupId", c.GroupHandler.Update)
	g.DELETE("/groups/:groupId", c.GroupHandler.Delete)
	g.POST("/groups/:groupId/members", c.GroupHandler.AddMember)
	g.GET("/groups/:groupId/members", c.GroupHandler.ListMembers)
	g.DELETE("/groups/:groupId/members/:userId", c.GroupHandler.RemoveMember)

	g.POST("/rooms/:roomId/invitations/batch-by-group", c.GroupHandler.BatchInviteByGroup)
}
