package app

import "github.com/labstack/echo/v4"

// registerWebSocketRoutes registers the WebSocket ticket-issuance and
// connection endpoints. POST /ws/ticket is registered on the given
// authenticated group (it needs the caller's identity, established by
// JWTAuth). GET /rooms/:roomId/ws is registered directly on e as a public
// route, since browsers cannot attach an Authorization header to a
// WebSocket handshake request; WebSocketHandler.Handle enforces
// authentication itself via the ticket query parameter.
func registerWebSocketRoutes(e *echo.Echo, g *echo.Group, c *Container) {
	g.POST("/ws/ticket", c.WebSocketHandler.IssueTicket)
	e.GET("/rooms/:roomId/ws", c.WebSocketHandler.Handle)
}
