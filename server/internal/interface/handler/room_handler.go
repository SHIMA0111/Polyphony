package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// RoomHandler handles HTTP requests for room endpoints, including creating,
// reading, listing, updating, and deleting chat rooms. It delegates business
// logic to RoomUsecase.
type RoomHandler struct {
	usecase *roomusecase.RoomUsecase
}

// NewRoomHandler creates a new RoomHandler with the given RoomUsecase.
func NewRoomHandler(usecase *roomusecase.RoomUsecase) *RoomHandler {
	return &RoomHandler{usecase: usecase}
}

// Create handles POST /rooms. It creates a new chat room owned by the
// authenticated user. The request body must include a non-empty name and may
// include an optional description. On success it returns HTTP 201 with a
// RoomResponse. It returns HTTP 400 for invalid or incomplete input and
// HTTP 500 for unexpected errors.
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

// Get handles GET /rooms/:roomId. It retrieves a single room by ID. The
// authenticated user must have access to the room. On success it returns
// HTTP 200 with a RoomResponse. It returns HTTP 403 if the user lacks
// permission and HTTP 404 if the room does not exist.
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

// List handles GET /rooms. It returns all rooms the authenticated user has
// access to. On success it returns HTTP 200 with a JSON array of RoomResponse
// objects. It returns HTTP 500 for unexpected errors.
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

// Update handles PUT /rooms/:roomId. It updates the name and description of an
// existing room. The request body must include a non-empty name. On success it
// returns HTTP 200 with the updated RoomResponse. It returns HTTP 400 for
// invalid input, HTTP 403 if the user lacks permission, and HTTP 404 if the
// room does not exist.
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

// Delete handles DELETE /rooms/:roomId. It deletes the specified room. Only
// the room owner or a user with sufficient privileges may delete a room. On
// success it returns HTTP 204 with no content. It returns HTTP 403 if the user
// lacks permission and HTTP 404 if the room does not exist.
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
