package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// UpdateSettings handles PATCH /rooms/:roomId/settings. It sets or clears
// the room's default AI provider/model (see
// roomusecase.RoomUsecase.UpdateSettings and UpdateRoomSettingsRequest for
// the exact nil/empty-string-sentinel/value convention each field follows).
// The caller must be at least admin in the room. On success it returns
// HTTP 200 with the updated RoomResponse. It returns HTTP 400 for a
// malformed request body, HTTP 403 if the caller lacks permission, and
// HTTP 404 if the room does not exist.
//
// Kept in its own file (rather than room_handler.go) so that adding this
// method does not create merge conflicts with other same-wave steps that
// touch RoomHandler's existing CRUD methods.
func (h *RoomHandler) UpdateSettings(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req UpdateRoomSettingsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	rwr, err := h.usecase.UpdateSettings(c.Request().Context(), userID, roomID, req.AIProvider, req.AIModel)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, toRoomResponse(rwr))
}
