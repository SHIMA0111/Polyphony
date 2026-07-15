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
	e.Use(echomw.Recover())
	e.Use(echomw.RequestID())
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: strings.Split(c.Config.CORSOrigins, ","),
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType, echo.HeaderAuthorization},
	}))
	e.Use(middleware.RequestLogger(c.Logger))

	registerHealthRoutes(e, c)
	registerModelRoutes(e, c)
	registerAuthRoutes(e, c)

	// Shared authenticated route group, used by every registrar below that
	// needs the caller's identity.
	authGroup := e.Group("", middleware.JWTAuth(c.AuthUC))
	registerRoomRoutes(authGroup, c)
	registerMessageRoutes(authGroup, c)
	registerUserRoutes(authGroup, c)

	return e
}
