package app

import (
	"github.com/go-redis/redis_rate/v10"
	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// registerMessageRoutes registers the authenticated message endpoints on the
// given group (the shared authenticated group built in NewRouter). The two
// AI-invoke routes (SendAI/RegenerateAI) additionally carry a per-
// authenticated-user Redis rate limit (Step 33), since they are the only
// routes here that trigger a metered, provider-billed AI call.
func registerMessageRoutes(g *echo.Group, c *Container) {
	aiInvokeRateLimit := middleware.RateLimit(middleware.RateLimitConfig{
		Limiter:   c.RateLimiter,
		Limit:     redis_rate.PerMinute(c.Config.RateLimitAIInvokePerMinute),
		KeyPrefix: "ai_invoke",
		KeyFunc:   middleware.PerUserOrIPKeyFunc,
	})

	g.POST("/rooms/:roomId/messages", c.MessageHandler.Send)
	g.GET("/rooms/:roomId/messages", c.MessageHandler.List)
	g.POST("/rooms/:roomId/messages/ai", c.MessageHandler.SendAI, aiInvokeRateLimit)
	g.POST("/rooms/:roomId/messages/:messageId/regenerate", c.MessageHandler.RegenerateAI, aiInvokeRateLimit)
	g.DELETE("/rooms/:roomId/messages/:messageId", c.MessageHandler.Delete)
	g.PATCH("/rooms/:roomId/messages/:messageId", c.MessageHandler.UpdateExclude)
}
