package app

import "github.com/labstack/echo/v4"

// registerRoomRoutes registers the authenticated room CRUD endpoints on the
// given group (the shared authenticated group built in NewRouter).
func registerRoomRoutes(g *echo.Group, c *Container) {
	g.POST("/rooms", c.RoomHandler.Create)
	g.GET("/rooms", c.RoomHandler.List)
	g.GET("/rooms/:roomId", c.RoomHandler.Get)
	g.PUT("/rooms/:roomId", c.RoomHandler.Update)
	g.DELETE("/rooms/:roomId", c.RoomHandler.Delete)
}
