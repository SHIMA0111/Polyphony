package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domaingroup "github.com/SHIMA0111/multi-user-ai/server/internal/domain/group"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	groupusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/group"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
)

// GroupHandler handles HTTP requests for personal group endpoints:
// creating, reading, listing, updating, and deleting groups the
// authenticated caller owns, managing each group's membership, and
// batch-inviting a group's members into a room. It delegates business logic
// to GroupUsecase.
type GroupHandler struct {
	usecase *groupusecase.GroupUsecase
}

// NewGroupHandler creates a new GroupHandler with the given GroupUsecase.
func NewGroupHandler(usecase *groupusecase.GroupUsecase) *GroupHandler {
	return &GroupHandler{usecase: usecase}
}

// maxGroupNameLength mirrors groups.name's VARCHAR(255) column limit
// (server/schema.sql). Create and Update reject an over-length name with
// HTTP 400 rather than letting it reach the DB, where it would surface as
// an unhandled HTTP 500.
const maxGroupNameLength = 255

// Create handles POST /groups. It creates a new personal group owned by the
// authenticated caller. The request body must include a non-empty name and
// may include an optional description. On success it returns HTTP 201 with
// a GroupResponse. It returns HTTP 400 for invalid or incomplete input.
func (h *GroupHandler) Create(c echo.Context) error {
	userID := middleware.GetUserID(c)

	var req CreateGroupRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "name is required"})
	}
	if len(req.Name) > maxGroupNameLength {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "name must be at most 255 characters"})
	}

	g, err := h.usecase.CreateGroup(c.Request().Context(), userID, req.Name, req.Description)
	if err != nil {
		middleware.GetLogger(c).Error("failed to create group", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusCreated, toGroupResponse(g))
}

// List handles GET /groups. It returns every group owned by the
// authenticated caller. On success it returns HTTP 200 with a
// GroupListResponse.
func (h *GroupHandler) List(c echo.Context) error {
	userID := middleware.GetUserID(c)

	groups, err := h.usecase.ListGroups(c.Request().Context(), userID)
	if err != nil {
		middleware.GetLogger(c).Error("failed to list groups", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	resp := make([]GroupResponse, len(groups))
	for i, g := range groups {
		resp[i] = toGroupResponse(g)
	}
	return c.JSON(http.StatusOK, GroupListResponse{Groups: resp})
}

// Get handles GET /groups/:groupId. Only the group's owner may retrieve it.
// On success it returns HTTP 200 with a GroupResponse. It returns HTTP 403
// if the caller does not own the group and HTTP 404 if it does not exist.
func (h *GroupHandler) Get(c echo.Context) error {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")

	g, err := h.usecase.GetGroup(c.Request().Context(), userID, groupID)
	if err != nil {
		return handleGroupError(c, err)
	}
	return c.JSON(http.StatusOK, toGroupResponse(g))
}

// Update handles PUT /groups/:groupId. Only the group's owner may update
// it. The request body must include a non-empty name. On success it
// returns HTTP 200 with the updated GroupResponse. It returns HTTP 400 for
// invalid input, HTTP 403 if the caller does not own the group, and
// HTTP 404 if it does not exist.
func (h *GroupHandler) Update(c echo.Context) error {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")

	var req UpdateGroupRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "name is required"})
	}
	if len(req.Name) > maxGroupNameLength {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "name must be at most 255 characters"})
	}

	g, err := h.usecase.UpdateGroup(c.Request().Context(), userID, groupID, req.Name, req.Description)
	if err != nil {
		return handleGroupError(c, err)
	}
	return c.JSON(http.StatusOK, toGroupResponse(g))
}

// Delete handles DELETE /groups/:groupId. Only the group's owner may delete
// it. On success it returns HTTP 204 with no content. It returns HTTP 403
// if the caller does not own the group and HTTP 404 if it does not exist.
func (h *GroupHandler) Delete(c echo.Context) error {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")

	if err := h.usecase.DeleteGroup(c.Request().Context(), userID, groupID); err != nil {
		return handleGroupError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AddMember handles POST /groups/:groupId/members. Only the group's owner
// may add a member. The request body's username is required and resolved
// to an existing user. On success it returns HTTP 201 with a
// GroupMemberResponse. It returns HTTP 400 for invalid input, HTTP 403 if
// the caller does not own the group, HTTP 404 if the group or username
// does not exist, and HTTP 409 if the user is already a member of the
// group.
func (h *GroupHandler) AddMember(c echo.Context) error {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")

	var req AddGroupMemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.Username == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "username is required"})
	}

	member, err := h.usecase.AddMember(c.Request().Context(), userID, groupID, req.Username)
	if err != nil {
		return handleGroupError(c, err)
	}
	return c.JSON(http.StatusCreated, toGroupMemberResponse(member))
}

// ListMembers handles GET /groups/:groupId/members. Only the group's owner
// may list its members. On success it returns HTTP 200 with a
// GroupMemberListResponse. It returns HTTP 403 if the caller does not own
// the group and HTTP 404 if it does not exist.
func (h *GroupHandler) ListMembers(c echo.Context) error {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")

	members, err := h.usecase.ListMembers(c.Request().Context(), userID, groupID)
	if err != nil {
		return handleGroupError(c, err)
	}

	resp := make([]GroupMemberResponse, len(members))
	for i, m := range members {
		resp[i] = toGroupMemberResponse(m)
	}
	return c.JSON(http.StatusOK, GroupMemberListResponse{Members: resp})
}

// RemoveMember handles DELETE /groups/:groupId/members/:userId. Only the
// group's owner may remove a member. On success it returns HTTP 204 with
// no content. It returns HTTP 403 if the caller does not own the group and
// HTTP 404 if the group or membership does not exist.
func (h *GroupHandler) RemoveMember(c echo.Context) error {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")
	targetUserID := c.Param("userId")

	if err := h.usecase.RemoveMember(c.Request().Context(), userID, groupID, targetUserID); err != nil {
		return handleGroupError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// BatchInviteByGroup handles POST /rooms/:roomId/invitations/batch-by-group.
// It invites every member of the request's group_id into the room with the
// requested role, reusing invitationusecase.InvitationUsecase.CreateInvitation
// unchanged once per member. The caller must own the group and be at least
// domainroom.RoleAdmin in the room. role must be a valid, non-"master" role
// (HTTP 400 otherwise — "master" may never be granted through an
// invitation path). On success it returns HTTP 200 with a
// BatchInviteByGroupResponse containing both the successfully created
// invitations and a per-member skip list; a single RBAC failure (caller
// lacks Admin+ in the room) is returned as one top-level error (HTTP 403)
// rather than N per-member skips. It returns HTTP 403 if the caller does
// not own the group.
//
// If GroupUsecase.BatchInviteToRoom aborts partway through on an
// unexpected per-member error, it returns a non-nil partial
// *groupusecase.BatchInviteResult alongside the error (see its GoDoc). In
// that case this handler does not discard the partial work: it renders the
// partial result with aborted=true and an error message, using the
// domain-error-mapped HTTP status rather than always 200, so the caller can
// see exactly which members were already invited/skipped before the abort.
func (h *GroupHandler) BatchInviteByGroup(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req BatchInviteByGroupRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.GroupID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "group_id is required"})
	}

	role := domainroom.Role(req.Role)
	if !role.IsValid() || role == domainroom.RoleMaster {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid role"})
	}
	if req.ExpiresInHours != nil && (*req.ExpiresInHours < invitationusecase.MinExpiresInHours || *req.ExpiresInHours > invitationusecase.MaxExpiresInHours) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "expires_in_hours must be between 1 and 720"})
	}

	result, err := h.usecase.BatchInviteToRoom(c.Request().Context(), userID, roomID, req.GroupID, role, req.ExpiresInHours)
	if err != nil {
		if result == nil {
			return handleGroupError(c, err)
		}
		// A partial result: BatchInviteToRoom aborted mid-batch on an
		// unexpected per-member error after already inviting/skipping some
		// members. Render that partial work instead of discarding it, with
		// an explicit failure indication, at the same HTTP status
		// handleGroupError would have used for a total failure.
		middleware.GetLogger(c).Error("batch invite aborted mid-batch", "error", err)
		resp := toBatchInviteByGroupResponse(result)
		resp.Aborted = true
		resp.Error = "batch invite aborted before all group members were processed"
		return c.JSON(groupErrorStatus(err), resp)
	}

	return c.JSON(http.StatusOK, toBatchInviteByGroupResponse(result))
}

// toGroupResponse converts a domaingroup.Group into the JSON-facing
// GroupResponse.
func toGroupResponse(g *domaingroup.Group) GroupResponse {
	return GroupResponse{
		ID:          g.ID,
		OwnerID:     g.OwnerID,
		Name:        g.Name,
		Description: g.Description,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// toGroupMemberResponse converts a domaingroup.GroupMemberWithUsername into
// the JSON-facing GroupMemberResponse.
func toGroupMemberResponse(m *domaingroup.GroupMemberWithUsername) GroupMemberResponse {
	return GroupMemberResponse{
		ID:       m.ID,
		GroupID:  m.GroupID,
		UserID:   m.UserID,
		Username: m.Username,
		AddedAt:  m.AddedAt,
	}
}

// toBatchInviteByGroupResponse converts a groupusecase.BatchInviteResult
// into the JSON-facing BatchInviteByGroupResponse, reusing
// toInvitationResponse (defined by the invitation handler) for each invited
// entry.
func toBatchInviteByGroupResponse(result *groupusecase.BatchInviteResult) BatchInviteByGroupResponse {
	invited := make([]InvitationResponse, len(result.Invited))
	for i, inv := range result.Invited {
		invited[i] = toInvitationResponse(inv)
	}
	skipped := make([]BatchInviteSkipResponse, len(result.Skipped))
	for i, s := range result.Skipped {
		skipped[i] = BatchInviteSkipResponse{
			UserID:   s.UserID,
			Username: s.Username,
			Reason:   s.Reason,
		}
	}
	return BatchInviteByGroupResponse{Invited: invited, Skipped: skipped}
}

// handleGroupError maps a domain error returned by GroupUsecase to the
// corresponding HTTP response, mirroring handleRoomError/handleInvitationError.
func handleGroupError(c echo.Context, err error) error {
	status := groupErrorStatus(err)
	if status == http.StatusInternalServerError {
		middleware.GetLogger(c).Error("unhandled group error", "error", err)
	}

	var message string
	switch {
	case errors.Is(err, domain.ErrNotFound):
		message = "not found"
	case errors.Is(err, domain.ErrForbidden):
		message = "forbidden"
	case errors.Is(err, domain.ErrAlreadyMember):
		message = "user is already a member of this group"
	default:
		message = "internal server error"
	}
	return c.JSON(status, ErrorResponse{Message: message})
}

// groupErrorStatus maps a domain error returned by GroupUsecase to the HTTP
// status handleGroupError would use, without writing a response. Shared
// with BatchInviteByGroup's partial-result path, which needs the same
// status mapping but writes a BatchInviteByGroupResponse body instead of an
// ErrorResponse.
func groupErrorStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrAlreadyMember):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
