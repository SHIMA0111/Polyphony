package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// RequireRole returns an Echo middleware that authorizes the authenticated
// caller against a specific room-scoped action. It reads the room ID from
// the ":roomId" route param and the caller's user ID via GetUserID (so it
// must run after JWTAuth in the middleware chain), loads the caller's
// membership via roomRepo.GetMember, and denies the request with HTTP 403
// (ErrorResponse{Message:"forbidden"}) unless Authorize(member.Role, action)
// succeeds. A membership that does not exist (domain.ErrNotFound) is also
// reported as HTTP 403 rather than HTTP 404, so an unauthorized or
// non-member caller cannot use the response code to infer whether the room
// itself exists. Any other repository error is reported as HTTP 500. On
// success it calls next(c).
func RequireRole(roomRepo domainroom.RoomRepository, action domainroom.Action) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID := GetUserID(c)
			roomID := c.Param("roomId")

			member, err := roomRepo.GetMember(c.Request().Context(), roomID, userID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return c.JSON(http.StatusForbidden, errorResponse{Message: "forbidden"})
				}
				return c.JSON(http.StatusInternalServerError, errorResponse{Message: "internal server error"})
			}

			if err := Authorize(member.Role, action); err != nil {
				return c.JSON(http.StatusForbidden, errorResponse{Message: "forbidden"})
			}

			return next(c)
		}
	}
}

// Authorize checks whether role is permitted to perform action, applying
// the exact same capability matrix as RequireRole (see domainroom.Role.Allows
// for the matrix). It returns domain.ErrForbidden if the action is not
// allowed, or nil if it is. Usecases that already hold a loaded
// domainroom.RoomMember (e.g. after their own membership lookup) should call
// Authorize directly instead of going through RequireRole, so the
// authorization rule is defined exactly once but does not force a second
// repository round-trip.
func Authorize(role domainroom.Role, action domainroom.Action) error {
	if !role.Allows(action) {
		return domain.ErrForbidden
	}
	return nil
}
