package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domaininvitation "github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
)

// InvitationHandler handles HTTP requests for room invitation endpoints:
// creating, listing, previewing, accepting, and rejecting invitations. It
// delegates business logic to InvitationUsecase.
type InvitationHandler struct {
	usecase *invitationusecase.InvitationUsecase
}

// NewInvitationHandler creates a new InvitationHandler with the given
// InvitationUsecase.
func NewInvitationHandler(usecase *invitationusecase.InvitationUsecase) *InvitationHandler {
	return &InvitationHandler{usecase: usecase}
}

// Create handles POST /rooms/:roomId/invitations. It creates a new
// invitation for the room, either username-targeted (single-use) or a
// reusable link invitation, with an assigned role and expiry. The caller
// must be at least domainroom.RoleAdmin in the room. On success it returns
// HTTP 201 with an InvitationResponse. It returns HTTP 400 for invalid
// input (missing/unrecognized role, or expires_in_hours out of range) and
// delegates other error mapping to handleInvitationError (e.g. HTTP 403 if
// the caller lacks permission or the requested role outranks their own,
// HTTP 404 if invitee_username does not match any user, HTTP 409 if the
// invitee is already a member or already has a pending invitation for this
// room).
func (h *InvitationHandler) Create(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req CreateInvitationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	role := domainroom.Role(req.Role)
	if !role.IsValid() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "role is required and must be one of: reader, guest, member, admin, master"})
	}
	if req.ExpiresInHours != nil && (*req.ExpiresInHours < invitationusecase.MinExpiresInHours || *req.ExpiresInHours > invitationusecase.MaxExpiresInHours) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "expires_in_hours must be between 1 and 720"})
	}

	inv, err := h.usecase.CreateInvitation(c.Request().Context(), userID, roomID, req.InviteeUsername, role, req.ExpiresInHours)
	if err != nil {
		return handleInvitationError(c, err)
	}

	return c.JSON(http.StatusCreated, toInvitationResponse(inv))
}

// ListForRoom handles GET /rooms/:roomId/invitations. It returns every
// invitation created for the room, regardless of status. The caller must
// be at least domainroom.RoleAdmin in the room. On success it returns HTTP
// 200 with an InvitationListResponse. It returns HTTP 403 if the caller
// lacks permission.
func (h *InvitationHandler) ListForRoom(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	invitations, err := h.usecase.ListRoomInvitations(c.Request().Context(), userID, roomID)
	if err != nil {
		return handleInvitationError(c, err)
	}

	return c.JSON(http.StatusOK, toInvitationListResponse(invitations))
}

// ListMine handles GET /invitations. It returns every pending invitation
// targeted at the authenticated caller. On success it returns HTTP 200
// with an InvitationListResponse.
func (h *InvitationHandler) ListMine(c echo.Context) error {
	userID := middleware.GetUserID(c)

	invitations, err := h.usecase.ListMyInvitations(c.Request().Context(), userID)
	if err != nil {
		middleware.GetLogger(c).Error("failed to list invitations", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusOK, toInvitationListResponse(invitations))
}

// GetByCode handles GET /invitations/by-code/:code. It resolves a
// link/username invitation by its invite code for preview, before the
// caller decides to accept or reject it. On success it returns HTTP 200
// with an InvitationResponse. It returns HTTP 404 if code does not match
// any invitation.
func (h *InvitationHandler) GetByCode(c echo.Context) error {
	userID := middleware.GetUserID(c)
	code := c.Param("code")

	inv, err := h.usecase.GetInvitationByCode(c.Request().Context(), userID, code)
	if err != nil {
		return handleInvitationError(c, err)
	}

	return c.JSON(http.StatusOK, toInvitationResponse(inv))
}

// Accept handles POST /invitations/:invitationId/accept. It accepts the
// invitation on behalf of the authenticated caller, inserting a
// room_members row with the role recorded on the invitation. On success it
// returns HTTP 200 with a RoomMembershipResponse. It returns HTTP 403 if
// the invitation is username-targeted at a different user, HTTP 404 if the
// invitation does not exist, HTTP 409 if the caller is already a member of
// the room or the invitation is no longer pending, and HTTP 410 if the
// invitation has expired.
func (h *InvitationHandler) Accept(c echo.Context) error {
	userID := middleware.GetUserID(c)
	invitationID := c.Param("invitationId")

	member, err := h.usecase.AcceptInvitation(c.Request().Context(), userID, invitationID)
	if err != nil {
		return handleInvitationError(c, err)
	}

	return c.JSON(http.StatusOK, RoomMembershipResponse{
		ID:       member.ID,
		RoomID:   member.RoomID,
		UserID:   member.UserID,
		Role:     string(member.Role),
		JoinedAt: member.JoinedAt,
	})
}

// Reject handles POST /invitations/:invitationId/reject. It rejects the
// invitation on behalf of the authenticated caller. Link invitations
// (reusable, not targeted at a single user) cannot be rejected. On success
// it returns HTTP 204 with no content. It returns HTTP 403 if the
// invitation is a link invitation or is targeted at a different user,
// HTTP 404 if the invitation does not exist, and HTTP 409 if the
// invitation is no longer pending.
func (h *InvitationHandler) Reject(c echo.Context) error {
	userID := middleware.GetUserID(c)
	invitationID := c.Param("invitationId")

	if err := h.usecase.RejectInvitation(c.Request().Context(), userID, invitationID); err != nil {
		return handleInvitationError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// toInvitationResponse converts a domaininvitation.Invitation into the
// JSON-facing InvitationResponse.
func toInvitationResponse(inv *domaininvitation.Invitation) InvitationResponse {
	return InvitationResponse{
		ID:         inv.ID,
		RoomID:     inv.RoomID,
		InviterID:  inv.InviterID,
		InviteeID:  inv.InviteeID,
		InviteCode: inv.InviteCode,
		Role:       string(inv.Role),
		Status:     string(inv.Status),
		ExpiresAt:  inv.ExpiresAt,
		CreatedAt:  inv.CreatedAt,
	}
}

// toInvitationListResponse converts a slice of domaininvitation.Invitation
// into the JSON-facing InvitationListResponse.
func toInvitationListResponse(invitations []*domaininvitation.Invitation) InvitationListResponse {
	resp := make([]InvitationResponse, len(invitations))
	for i, inv := range invitations {
		resp[i] = toInvitationResponse(inv)
	}
	return InvitationListResponse{Invitations: resp}
}

// handleInvitationError maps a domain error returned by InvitationUsecase
// to the corresponding HTTP response, mirroring handleRoomError.
func handleInvitationError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "invitation not found"})
	case errors.Is(err, domain.ErrForbidden):
		return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
	case errors.Is(err, domain.ErrAlreadyMember):
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "user is already a member of the room"})
	case errors.Is(err, domain.ErrInvitationAlreadyExists):
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "invitation already exists"})
	case errors.Is(err, domain.ErrInvitationNotPending):
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "invitation is not pending"})
	case errors.Is(err, domain.ErrInvitationExpired):
		return c.JSON(http.StatusGone, ErrorResponse{Message: "invitation expired"})
	default:
		middleware.GetLogger(c).Error("unhandled invitation error", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}
}
