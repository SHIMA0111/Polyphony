package app

import (
	"github.com/go-redis/redis_rate/v10"
	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// registerAuthRoutes registers the public registration and login endpoints
// on e (rate-limited per client IP — Step 33) and the authenticated logout
// endpoint on authGroup (the shared authenticated group built in NewRouter),
// mirroring registerWebSocketRoutes's e/authGroup split.
func registerAuthRoutes(e *echo.Echo, authGroup *echo.Group, c *Container) {
	loginRateLimit := middleware.RateLimit(middleware.RateLimitConfig{
		Limiter:   c.RateLimiter,
		Limit:     redis_rate.PerMinute(c.Config.RateLimitLoginPerMinute),
		KeyPrefix: "auth",
		KeyFunc:   middleware.PerIPKeyFunc,
	})

	e.POST("/auth/register", c.AuthHandler.Register, loginRateLimit)
	e.POST("/auth/login", c.AuthHandler.Login, loginRateLimit)

	authGroup.POST("/auth/logout", c.AuthHandler.Logout)
}
