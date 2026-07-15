package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
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

	rwr, err := h.usecase.CreateRoom(c.Request().Context(), userID, req.Name, req.Description)
	if err != nil {
		middleware.GetLogger(c).Error("failed to create room", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusCreated, toRoomResponse(rwr))
}

// Get handles GET /rooms/:roomId. It retrieves a single room by ID. The
// authenticated user must have access to the room. On success it returns
// HTTP 200 with a RoomResponse. It returns HTTP 403 if the user lacks
// permission and HTTP 404 if the room does not exist.
func (h *RoomHandler) Get(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	rwr, err := h.usecase.GetRoom(c.Request().Context(), userID, roomID)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, toRoomResponse(rwr))
}

// List handles GET /rooms. It returns all rooms the authenticated user has
// access to. On success it returns HTTP 200 with a JSON array of RoomResponse
// objects. It returns HTTP 500 for unexpected errors.
func (h *RoomHandler) List(c echo.Context) error {
	userID := middleware.GetUserID(c)

	rooms, err := h.usecase.ListRooms(c.Request().Context(), userID)
	if err != nil {
		middleware.GetLogger(c).Error("failed to list rooms", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	resp := make([]RoomResponse, len(rooms))
	for i, rwr := range rooms {
		resp[i] = toRoomResponse(rwr)
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

	rwr, err := h.usecase.UpdateRoom(c.Request().Context(), userID, roomID, req.Name, req.Description)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, toRoomResponse(rwr))
}

// UpdateAIContextCutoff handles PATCH /rooms/:roomId/ai-context-cutoff. It
// sets or clears the room's AI context cutoff datetime: a non-null
// cutoff_at excludes any message created before it from future AI context
// assembly, while a null or omitted cutoff_at clears the restriction. The
// caller must be at least admin in the room. On success it returns HTTP 200
// with the updated RoomResponse. It returns HTTP 400 for invalid input,
// HTTP 403 if the caller lacks permission, and HTTP 404 if the room does
// not exist.
func (h *RoomHandler) UpdateAIContextCutoff(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req UpdateRoomAIContextCutoffRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	rwr, err := h.usecase.UpdateAIContextCutoff(c.Request().Context(), userID, roomID, req.CutoffAt)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, toRoomResponse(rwr))
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

// ListMembers handles GET /rooms/:roomId/members. Any member of the room —
// including domainroom.RoleReader — may list its members. On success it
// returns HTTP 200 with a MemberListResponse. It returns HTTP 403 if the
// caller is not a member of the room.
func (h *RoomHandler) ListMembers(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	members, err := h.usecase.ListMembers(c.Request().Context(), userID, roomID)
	if err != nil {
		return handleRoomError(c, err)
	}

	resp := make([]MemberResponse, len(members))
	for i, m := range members {
		resp[i] = toMemberResponse(m)
	}

	return c.JSON(http.StatusOK, MemberListResponse{Members: resp})
}

// Leave handles DELETE /rooms/:roomId/members/:userId. It removes the
// authenticated user from the room. This endpoint only supports
// self-leave: it returns HTTP 403 if :userId does not match the
// authenticated caller. On success it returns HTTP 204 with no content. It
// returns HTTP 409 (ErrorResponse{Message: "owner must transfer ownership
// before leaving"}) if the caller is the room's current owner, since an
// owner must call TransferOwnership before they can leave.
func (h *RoomHandler) Leave(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	targetUserID := c.Param("userId")

	if err := h.usecase.LeaveRoom(c.Request().Context(), userID, roomID, targetUserID); err != nil {
		return handleRoomError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// ChangeRole handles PATCH /rooms/:roomId/members/:userId/role. It changes
// the target member's role. The request body's role must be one of
// "reader", "guest", "member", or "admin" — granting "master" through this
// endpoint returns HTTP 400 (ErrorResponse{Message: "invalid role"}); use
// the ownership-transfer endpoint to grant master. Route-level
// middleware.RequireRole (domainroom.ActionManageMembers) gates this
// endpoint at the HTTP layer, and the usecase re-checks it as defense in
// depth, so both return HTTP 403 for a caller lacking that capability. On
// success it returns HTTP 200 with the updated MemberResponse. It returns
// HTTP 409 if the target is the room's current owner (whose role can only
// change via ownership transfer).
func (h *RoomHandler) ChangeRole(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	targetUserID := c.Param("userId")

	var req ChangeMemberRoleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	newRole := domainroom.Role(req.Role)
	if !newRole.IsValid() || newRole == domainroom.RoleMaster {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid role"})
	}

	member, err := h.usecase.ChangeMemberRole(c.Request().Context(), userID, roomID, targetUserID, newRole)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, toMemberResponse(member))
}

// TransferOwnership handles PATCH /rooms/:roomId/owner. It transfers room
// ownership to another existing member: new_owner_id is required (HTTP 400
// otherwise). Only the room's current owner may call this (HTTP 403
// otherwise). On success it returns HTTP 200 with a RoomResponse whose role
// reflects the caller's post-transfer role.
func (h *RoomHandler) TransferOwnership(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req TransferOwnershipRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.NewOwnerID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "new_owner_id is required"})
	}

	rwr, err := h.usecase.TransferOwnership(c.Request().Context(), userID, roomID, req.NewOwnerID)
	if err != nil {
		return handleRoomError(c, err)
	}

	return c.JSON(http.StatusOK, toRoomResponse(rwr))
}

// toMemberResponse converts a domainroom.RoomMember into the JSON-facing
// MemberResponse.
func toMemberResponse(m *domainroom.RoomMember) MemberResponse {
	return MemberResponse{
		ID:       m.ID,
		RoomID:   m.RoomID,
		UserID:   m.UserID,
		Username: m.Username,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt,
	}
}

// toRoomResponse converts a domainroom.RoomWithRole (a Room paired with the
// requesting user's Role in it) into the JSON-facing RoomResponse.
func toRoomResponse(rwr *domainroom.RoomWithRole) RoomResponse {
	rm := rwr.Room
	return RoomResponse{
		ID:                rm.ID,
		Name:              rm.Name,
		Description:       rm.Description,
		OwnerID:           rm.OwnerID,
		Role:              string(rwr.Role),
		AIContextCutoffAt: rm.AIContextCutoffAt,
		CreatedAt:         rm.CreatedAt,
		UpdatedAt:         rm.UpdatedAt,
	}
}

func handleRoomError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "room not found"})
	}
	if errors.Is(err, domain.ErrForbidden) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
	}
	if errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "owner must transfer ownership before leaving"})
	}
	middleware.GetLogger(c).Error("unhandled room error", "error", err)
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}
