package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// RoomHandler handles room HTTP requests.
type RoomHandler struct {
	usecase *roomusecase.RoomUsecase
}

// NewRoomHandler creates a new RoomHandler.
func NewRoomHandler(usecase *roomusecase.RoomUsecase) *RoomHandler {
	return &RoomHandler{usecase: usecase}
}

// Create handles POST /rooms.
func (h *RoomHandler) Create(c echo.Context) error {
	userID := middleware.GetUserID(c)

	var req CreateRoomRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "name is required"})
	}

	rm, err := h.usecase.CreateRoom(c.Request().Context(), userID, req.Name, req.Description)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusCreated, RoomResponse{
		ID:          rm.ID,
		Name:        rm.Name,
		Description: rm.Description,
		OwnerID:     rm.OwnerID,
		CreatedAt:   rm.CreatedAt,
		UpdatedAt:   rm.UpdatedAt,
	})
}

// Get handles GET /rooms/:roomId.
func (h *RoomHandler) Get(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	rm, err := h.usecase.GetRoom(c.Request().Context(), userID, roomID)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, RoomResponse{
		ID:          rm.ID,
		Name:        rm.Name,
		Description: rm.Description,
		OwnerID:     rm.OwnerID,
		CreatedAt:   rm.CreatedAt,
		UpdatedAt:   rm.UpdatedAt,
	})
}

// List handles GET /rooms.
func (h *RoomHandler) List(c echo.Context) error {
	userID := middleware.GetUserID(c)

	rooms, err := h.usecase.ListRooms(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	resp := make([]RoomResponse, len(rooms))
	for i, rm := range rooms {
		resp[i] = RoomResponse{
			ID:          rm.ID,
			Name:        rm.Name,
			Description: rm.Description,
			OwnerID:     rm.OwnerID,
			CreatedAt:   rm.CreatedAt,
			UpdatedAt:   rm.UpdatedAt,
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// Update handles PUT /rooms/:roomId.
func (h *RoomHandler) Update(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req UpdateRoomRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "name is required"})
	}

	rm, err := h.usecase.UpdateRoom(c.Request().Context(), userID, roomID, req.Name, req.Description)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, RoomResponse{
		ID:          rm.ID,
		Name:        rm.Name,
		Description: rm.Description,
		OwnerID:     rm.OwnerID,
		CreatedAt:   rm.CreatedAt,
		UpdatedAt:   rm.UpdatedAt,
	})
}

// Delete handles DELETE /rooms/:roomId.
func (h *RoomHandler) Delete(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	if err := h.usecase.DeleteRoom(c.Request().Context(), userID, roomID); err != nil {
		return handleRoomError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func handleRoomError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "room not found"})
	}
	if errors.Is(err, domain.ErrForbidden) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
	}
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}
