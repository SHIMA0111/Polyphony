// Package invitation implements the business logic for room invitations:
// creating, listing, accepting, and rejecting invitations that bring a new
// user into a room with an assigned role. It sits between the HTTP handlers
// (server/internal/interface/handler) and the invitation/room/user
// repository ports (server/internal/domain/invitation,
// server/internal/domain/room, server/internal/domain/user).
package invitation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domaininvitation "github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// DefaultExpiresInHours is the invitation lifetime used when
// CreateInvitation's expiresInHours argument is nil.
const DefaultExpiresInHours = 168 // 7 days

// MinExpiresInHours and MaxExpiresInHours bound the accepted range for an
// explicitly requested expiresInHours. Callers (the handler layer) are
// expected to reject out-of-range values with HTTP 400 before ever reaching
// the usecase; CreateInvitation itself clamps defensively so it never
// persists an invitation with a nonsensical expiry even if called directly.
const (
	MinExpiresInHours = 1
	MaxExpiresInHours = 720 // 30 days
)

// InvitationUsecase provides invitation-related business logic: creating,
// listing, accepting, and rejecting room invitations.
type InvitationUsecase struct {
	invitationRepo domaininvitation.InvitationRepository
	roomRepo       domainroom.RoomRepository
	userRepo       domainuser.UserRepository
}

// NewInvitationUsecase creates a new InvitationUsecase.
func NewInvitationUsecase(
	invitationRepo domaininvitation.InvitationRepository,
	roomRepo domainroom.RoomRepository,
	userRepo domainuser.UserRepository,
) *InvitationUsecase {
	return &InvitationUsecase{
		invitationRepo: invitationRepo,
		roomRepo:       roomRepo,
		userRepo:       userRepo,
	}
}

// CreateInvitation creates a new invitation for roomID on behalf of
// inviterID. If inviteeUsername is non-nil, it is resolved to a specific
// user and the invitation is single-use (username-targeted); if nil, the
// invitation is a reusable link invitation. expiresInHours, if nil,
// defaults to DefaultExpiresInHours; the effective value is clamped to
// [MinExpiresInHours, MaxExpiresInHours].
//
// It returns domain.ErrForbidden if inviterID is not at least
// domainroom.RoleAdmin in roomID, or if role outranks the inviter's own
// role in that room (the privilege-escalation guard). If inviteeUsername is
// set, it returns domain.ErrNotFound if no user with that username exists,
// domain.ErrAlreadyMember if that user is already a member of roomID, and
// domain.ErrInvitationAlreadyExists if a pending invitation already exists
// for that (room, invitee) pair.
func (u *InvitationUsecase) CreateInvitation(
	ctx context.Context,
	inviterID, roomID string,
	inviteeUsername *string,
	role domainroom.Role,
	expiresInHours *int,
) (*domaininvitation.Invitation, error) {
	inviter, err := u.getMember(ctx, roomID, inviterID)
	if err != nil {
		return nil, err
	}
	if !inviter.Role.AtLeast(domainroom.RoleAdmin) {
		return nil, domain.ErrForbidden
	}
	// Privilege-escalation guard: the role assigned on the invitation must
	// not outrank the inviter's own effective role in the room. This also
	// rejects an unrecognized role value, since Role.AtLeast returns false
	// whenever either side is not one of the five defined roles.
	if !inviter.Role.AtLeast(role) {
		return nil, domain.ErrForbidden
	}

	var inviteeID *string
	if inviteeUsername != nil {
		invitee, err := u.userRepo.GetByUsername(ctx, *inviteeUsername)
		if err != nil {
			return nil, err
		}

		if _, err := u.roomRepo.GetMember(ctx, roomID, invitee.ID); err == nil {
			return nil, domain.ErrAlreadyMember
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}

		if _, err := u.invitationRepo.GetPendingByRoomAndInvitee(ctx, roomID, invitee.ID); err == nil {
			return nil, domain.ErrInvitationAlreadyExists
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}

		inviteeID = &invitee.ID
	}

	hours := DefaultExpiresInHours
	if expiresInHours != nil {
		hours = *expiresInHours
	}
	if hours < MinExpiresInHours {
		hours = MinExpiresInHours
	} else if hours > MaxExpiresInHours {
		hours = MaxExpiresInHours
	}

	now := time.Now()
	inv := &domaininvitation.Invitation{
		ID:         uuid.New().String(),
		RoomID:     roomID,
		InviterID:  inviterID,
		InviteeID:  inviteeID,
		InviteCode: newInviteCode(),
		Role:       role,
		Status:     domaininvitation.StatusPending,
		ExpiresAt:  now.Add(time.Duration(hours) * time.Hour),
		CreatedAt:  now,
	}

	if err := u.invitationRepo.Create(ctx, inv); err != nil {
		if !errors.Is(err, domaininvitation.ErrInviteCodeConflict) {
			return nil, err
		}
		// Retry once with a freshly generated code; the invite_code unique
		// constraint is the final backstop and a second collision is
		// vanishingly unlikely, so no elaborate retry loop is warranted.
		inv.InviteCode = newInviteCode()
		if err := u.invitationRepo.Create(ctx, inv); err != nil {
			return nil, err
		}
	}

	return inv, nil
}

// ListRoomInvitations returns every invitation created for roomID. The
// caller must be at least domainroom.RoleAdmin in roomID; it returns
// domain.ErrForbidden otherwise.
func (u *InvitationUsecase) ListRoomInvitations(ctx context.Context, userID, roomID string) ([]*domaininvitation.Invitation, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.AtLeast(domainroom.RoleAdmin) {
		return nil, domain.ErrForbidden
	}
	return u.invitationRepo.ListByRoomID(ctx, roomID)
}

// ListMyInvitations returns every pending invitation targeted at userID.
// Any authenticated user may call this for their own userID.
func (u *InvitationUsecase) ListMyInvitations(ctx context.Context, userID string) ([]*domaininvitation.Invitation, error) {
	return u.invitationRepo.ListPendingByInviteeID(ctx, userID)
}

// GetInvitationByCode resolves an invitation by its invite code, for
// preview purposes before accepting or rejecting it. Any authenticated
// user may call this (userID is accepted for interface symmetry and future
// per-caller filtering, but is not currently used to restrict the result).
// It returns domain.ErrNotFound if code does not match any invitation.
func (u *InvitationUsecase) GetInvitationByCode(ctx context.Context, userID, code string) (*domaininvitation.Invitation, error) {
	return u.invitationRepo.GetByCode(ctx, code)
}

// AcceptInvitation accepts the invitation identified by invitationID on
// behalf of userID, inserting a room_members row with the role recorded on
// the invitation, and returns that new membership.
//
// It returns domain.ErrNotFound if invitationID does not exist,
// domain.ErrForbidden if the invitation is username-targeted at a
// different user, domain.ErrAlreadyMember if userID is already a member of
// the room, domain.ErrInvitationNotPending if a username-targeted
// invitation has already been accepted/rejected/revoked, and
// domain.ErrInvitationExpired if the invitation's ExpiresAt has passed.
//
// Username-targeted invitations are single-use: on success their status
// transitions to domaininvitation.StatusAccepted. Link invitations
// (InviteeID == nil) remain domaininvitation.StatusPending — they are
// reusable by any authenticated user holding the code until expiry or
// explicit revocation.
func (u *InvitationUsecase) AcceptInvitation(ctx context.Context, userID, invitationID string) (*domainroom.RoomMember, error) {
	inv, err := u.invitationRepo.GetByID(ctx, invitationID)
	if err != nil {
		return nil, err
	}

	if inv.InviteeID != nil && *inv.InviteeID != userID {
		return nil, domain.ErrForbidden
	}

	if _, err := u.roomRepo.GetMember(ctx, inv.RoomID, userID); err == nil {
		return nil, domain.ErrAlreadyMember
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	// Single-use semantics only apply to username-targeted invitations;
	// link invitations stay StatusPending forever (until expiry/revoke) so
	// this check would otherwise always fail spuriously on the second use.
	if inv.InviteeID != nil && inv.Status != domaininvitation.StatusPending {
		return nil, domain.ErrInvitationNotPending
	}

	if time.Now().After(inv.ExpiresAt) {
		return nil, domain.ErrInvitationExpired
	}

	member := &domainroom.RoomMember{
		ID:       uuid.New().String(),
		RoomID:   inv.RoomID,
		UserID:   userID,
		Role:     inv.Role,
		JoinedAt: time.Now(),
	}
	if err := u.roomRepo.AddMember(ctx, member); err != nil {
		return nil, err
	}

	if inv.InviteeID != nil {
		if err := u.invitationRepo.UpdateStatus(ctx, inv.ID, domaininvitation.StatusAccepted); err != nil {
			return nil, err
		}
	}

	return member, nil
}

// RejectInvitation rejects the username-targeted invitation identified by
// invitationID on behalf of userID.
//
// It returns domain.ErrNotFound if invitationID does not exist,
// domain.ErrForbidden if the invitation is a link invitation (InviteeID ==
// nil — link invitations have no single invitee and so cannot be
// rejected) or if it is username-targeted at a different user, and
// domain.ErrInvitationNotPending if it has already been
// accepted/rejected/revoked.
func (u *InvitationUsecase) RejectInvitation(ctx context.Context, userID, invitationID string) error {
	inv, err := u.invitationRepo.GetByID(ctx, invitationID)
	if err != nil {
		return err
	}

	if inv.InviteeID == nil {
		return domain.ErrForbidden
	}
	if *inv.InviteeID != userID {
		return domain.ErrForbidden
	}
	if inv.Status != domaininvitation.StatusPending {
		return domain.ErrInvitationNotPending
	}

	return u.invitationRepo.UpdateStatus(ctx, inv.ID, domaininvitation.StatusRejected)
}

// getMember loads the caller's membership in roomID, translating a missing
// membership (domain.ErrNotFound) into domain.ErrForbidden so that a
// non-member can never distinguish "room does not exist" from "room exists
// but I'm not a member of it" via the returned error. See
// domainroom.GetMemberOrForbidden (shared with usecase/message and
// usecase/room, which each keep this same thin wrapper).
func (u *InvitationUsecase) getMember(ctx context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	return domainroom.GetMemberOrForbidden(ctx, u.roomRepo, roomID, userID)
}

// newInviteCode generates a unique, URL-safe invitation code: a UUID with
// its dashes stripped. The invite_code column's unique constraint is the
// final backstop against collision; see CreateInvitation's retry-once logic.
func newInviteCode() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}
