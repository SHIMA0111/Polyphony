package app

import (
	"github.com/labstack/echo/v4"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// registerRoomRoutes registers the authenticated room CRUD endpoints on the
// given group (the shared authenticated group built in NewRouter).
func registerRoomRoutes(g *echo.Group, c *Container) {
	g.POST("/rooms", c.RoomHandler.Create)
	g.GET("/rooms", c.RoomHandler.List)
	g.GET("/rooms/:roomId", c.RoomHandler.Get)
	g.PUT("/rooms/:roomId", c.RoomHandler.Update)
	g.DELETE("/rooms/:roomId", c.RoomHandler.Delete)

	g.GET("/rooms/:roomId/members", c.RoomHandler.ListMembers)
	g.DELETE("/rooms/:roomId/members/:userId", c.RoomHandler.Leave)
	g.PATCH("/rooms/:roomId/members/:userId/role", c.RoomHandler.ChangeRole,
		middleware.RequireRole(c.RoomRepo, domainroom.ActionManageMembers))
	g.PATCH("/rooms/:roomId/owner", c.RoomHandler.TransferOwnership)
	g.PATCH("/rooms/:roomId/ai-context-cutoff", c.RoomHandler.UpdateAIContextCutoff)
	g.PATCH("/rooms/:roomId/settings", c.RoomHandler.UpdateSettings)

	g.POST("/rooms/:roomId/fork", c.RoomHandler.Fork,
		middleware.RequireRole(c.RoomRepo, domainroom.ActionManageRoom))
	g.GET("/rooms/:roomId/fork-jobs/:jobId", c.RoomHandler.GetForkJobStatus)
}
