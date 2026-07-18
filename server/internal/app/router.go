package app

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// NewRouter builds the Echo router for the API server: it registers the
// global middleware chain (panic recovery, request ID, CORS, request
// logging) and then delegates route registration to one small registrar
// function per handler group, so that later steps add a new registrar file
// or call instead of editing this function.
func NewRouter(c *Container) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	// The API server is exposed directly in docker-compose (no reverse
	// proxy in front of it), so a client-supplied X-Forwarded-For or
	// X-Real-IP header cannot be trusted: echo.ExtractIPFromRealIPHeader /
	// ExtractIPFromXFFHeader would let a caller forge whichever IP it wants
	// and thereby dodge or collide with another caller's rate-limit bucket
	// (see interface/middleware's Redis token-bucket limiter, which keys on
	// c.RealIP()). ExtractIPDirect always uses the TCP peer address instead.
	// Phase 21 puts an ALB in front of the service; at that point this
	// should switch to ExtractIPFromXFFHeader scoped to the ALB's CIDR so
	// the real client IP (rather than the ALB's) is used for rate limiting.
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(echomw.Recover())
	e.Use(echomw.RequestID())
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: strings.Split(c.Config.CORSOrigins, ","),
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType, echo.HeaderAuthorization},
		// AllowCredentials lets the browser send/receive the Kratos session
		// cookie cross-origin (Step 20). This is only valid because
		// c.Config.CORSOrigins is never "*" (it defaults to
		// http://localhost:3000 and is env-driven) — credentialed CORS with
		// a wildcard origin is rejected by browsers.
		AllowCredentials: true,
	}))
	e.Use(middleware.RequestLogger(c.Logger))

	registerHealthRoutes(e, c)
	registerModelRoutes(e, c)

	// Shared authenticated route group, used by every registrar below that
	// needs the caller's identity.
	authGroup := e.Group("", middleware.JWTAuth(c.AuthUC, c.Config.KratosCookieName))
	registerAuthRoutes(e, authGroup, c)
	registerRoomRoutes(authGroup, c)
	registerMessageRoutes(authGroup, c)
	registerUserRoutes(authGroup, c)
	registerAttachmentRoutes(authGroup, c)
	registerInvitationRoutes(authGroup, c)
	registerGroupRoutes(authGroup, c)
	registerWebSocketRoutes(e, authGroup, c)
	registerTokenRoutes(authGroup, c)
	registerBillingRoutes(e, authGroup, c)

	return e
}
