package app

import "github.com/labstack/echo/v4"

// registerInvitationRoutes registers the authenticated room invitation
// endpoints on the given group (the shared authenticated group built in
// NewRouter).
func registerInvitationRoutes(g *echo.Group, c *Container) {
	g.POST("/rooms/:roomId/invitations", c.InvitationHandler.Create)
	g.GET("/rooms/:roomId/invitations", c.InvitationHandler.ListForRoom)
	g.GET("/invitations", c.InvitationHandler.ListMine)
	g.GET("/invitations/by-code/:code", c.InvitationHandler.GetByCode)
	g.POST("/invitations/:invitationId/accept", c.InvitationHandler.Accept)
	g.POST("/invitations/:invitationId/reject", c.InvitationHandler.Reject)
}
