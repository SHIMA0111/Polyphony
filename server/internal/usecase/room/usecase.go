// Package room implements the room use cases: creating, listing, reading,
// updating, and deleting chat rooms on behalf of an authenticated user.
package room

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainevent "github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
)

// RoomUsecase provides room-related business logic.
type RoomUsecase struct {
	roomRepo    domainroom.RoomRepository
	msgRepo     domainmessage.MessageRepository
	forkJobRepo domainroomfork.ForkJobRepository
	hub         domainevent.MessageHub
}

// NewRoomUsecase creates a new RoomUsecase. msgRepo and forkJobRepo are used
// only by the room-fork feature (see fork.go's ForkRoom/GetForkJobStatus/
// runForkJob): msgRepo drives the fork worker's message-copy loop
// (CountAndMaxSequence/ListByRoomAfter/CreateBatch/ReserveSequenceRange) and
// forkJobRepo persists roomfork.Job progress/state transitions. hub is used
// only by LeaveRoom, to revoke the departing member's live WebSocket
// subscription on the room (see LeaveRoom's doc comment) — every other
// RoomUsecase method (CreateRoom, UpdateRoom, ...) uses only roomRepo.
func NewRoomUsecase(roomRepo domainroom.RoomRepository, msgRepo domainmessage.MessageRepository, forkJobRepo domainroomfork.ForkJobRepository, hub domainevent.MessageHub) *RoomUsecase {
	return &RoomUsecase{roomRepo: roomRepo, msgRepo: msgRepo, forkJobRepo: forkJobRepo, hub: hub}
}

// CreateRoom creates a new room with the given user as owner. The creating
// user is always added as the room's RoleMaster member (mirroring
// RoomRepository.Create), so the returned RoomWithRole.Role is always
// domainroom.RoleMaster.
func (u *RoomUsecase) CreateRoom(ctx context.Context, userID, name, description string) (*domainroom.RoomWithRole, error) {
	now := time.Now()
	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		OwnerID:     userID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := u.roomRepo.Create(ctx, rm); err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: domainroom.RoleMaster}, nil
}

// GetRoom retrieves a room along with the requesting user's role in it. Any
// valid member may retrieve a room (no Action check beyond membership); it
// returns domain.ErrForbidden if userID is not a member of roomID.
func (u *RoomUsecase) GetRoom(ctx context.Context, userID, roomID string) (*domainroom.RoomWithRole, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: member.Role}, nil
}

// ListRooms returns all rooms the user is a member of, each paired with the
// user's role in that room.
func (u *RoomUsecase) ListRooms(ctx context.Context, userID string) ([]*domainroom.RoomWithRole, error) {
	return u.roomRepo.ListByUserIDWithRole(ctx, userID)
}

// UpdateRoom updates a room's name and description. The caller must be at
// least domainroom.RoleAdmin in the room (domainroom.ActionManageRoom) — so
// both admin and master may update room settings, but reader/guest/member
// may not. It returns domain.ErrForbidden if the caller lacks that role.
func (u *RoomUsecase) UpdateRoom(ctx context.Context, userID, roomID, name, description string) (*domainroom.RoomWithRole, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if err := domainroom.Authorize(member.Role, domainroom.ActionManageRoom); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	rm.Name = name
	rm.Description = description
	rm.UpdatedAt = time.Now()

	if err = u.roomRepo.UpdateDetails(ctx, roomID, name, description); err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: member.Role}, nil
}

// UpdateAIContextCutoff sets or clears a room's AI context cutoff datetime.
// A non-nil cutoff excludes any message created before it from future AI
// context assembly (ai.ContextBuilder.Build); a nil cutoff clears the
// restriction. The caller must be at least domainroom.RoleAdmin in the room
// (domainroom.ActionManageRoom) — so both admin and master may set the
// cutoff, but reader/guest/member may not. It returns domain.ErrForbidden if
// the caller lacks that role or is not a member of the room. It follows the
// same get-check-mutate-persist shape as UpdateRoom and, like UpdateRoom,
// returns a domainroom.RoomWithRole pairing the updated room with the
// caller's role, so handlers can map the result to RoomResponse without
// special-casing this endpoint.
func (u *RoomUsecase) UpdateAIContextCutoff(ctx context.Context, userID, roomID string, cutoff *time.Time) (*domainroom.RoomWithRole, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if err := domainroom.Authorize(member.Role, domainroom.ActionManageRoom); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	rm.AIContextCutoffAt = cutoff
	rm.UpdatedAt = time.Now()

	if err = u.roomRepo.UpdateAIContextCutoff(ctx, roomID, cutoff); err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: member.Role}, nil
}

// DeleteRoom deletes a room. The caller must be domainroom.RoleMaster
// (domainroom.ActionDeleteRoom) — even admin may not delete the room. It
// returns domain.ErrForbidden if the caller is not master.
func (u *RoomUsecase) DeleteRoom(ctx context.Context, userID, roomID string) error {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if err := domainroom.Authorize(member.Role, domainroom.ActionDeleteRoom); err != nil {
		return err
	}

	return u.roomRepo.Delete(ctx, roomID)
}

// ListMembers returns all members of a room. Any member — including
// domainroom.RoleReader — may list members; no domainroom.Action capability
// is required beyond plain membership. It returns domain.ErrForbidden if
// callerID is not a member of roomID. Each returned RoomMember carries a
// populated Username (see domainroom.RoomMember.Username).
func (u *RoomUsecase) ListMembers(ctx context.Context, callerID, roomID string) ([]*domainroom.RoomMember, error) {
	if _, err := u.getMember(ctx, roomID, callerID); err != nil {
		return nil, err
	}
	return u.roomRepo.ListMembers(ctx, roomID)
}

// LeaveRoom removes targetUserID from roomID's membership. This endpoint
// only supports self-leave: it returns domain.ErrForbidden if
// targetUserID != callerID (kicking other members is out of scope for this
// step) or if the caller is not a member of the room. If callerID is the
// room's current owner (domainroom.Room.OwnerID), it returns
// domainroom.ErrOwnerRoleProtected instead of leaving — the owner must
// transfer ownership (see TransferOwnership) before they can leave.
//
// On success, it also calls hub.Revoke to close any live WebSocket
// subscription targetUserID holds on roomID: once membership is gone, the
// room's messages must stop reaching them immediately, not just on their
// next HTTP request's membership check. This is unconditional (unlike a
// role change via ChangeMemberRole, which never revokes — see that method's
// doc comment) because leaving removes access entirely rather than merely
// changing its level.
func (u *RoomUsecase) LeaveRoom(ctx context.Context, callerID, roomID, targetUserID string) error {
	if targetUserID != callerID {
		return domain.ErrForbidden
	}
	if _, err := u.getMember(ctx, roomID, callerID); err != nil {
		return err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return err
	}
	if callerID == rm.OwnerID {
		return domainroom.ErrOwnerRoleProtected
	}

	if err := u.roomRepo.RemoveMember(ctx, roomID, targetUserID); err != nil {
		return err
	}

	if u.hub != nil {
		u.hub.Revoke(ctx, roomID, targetUserID)
	}
	return nil
}

// ChangeMemberRole changes targetUserID's role within roomID to newRole. The
// caller must hold domainroom.ActionManageMembers in the room (checked here
// as defense in depth alongside the route-level middleware.RequireRole); it
// returns domain.ErrForbidden if the caller lacks that capability. It
// returns domainroom.ErrOwnerRoleProtected if targetUserID is the room's
// current owner — the owner's role can only change via TransferOwnership,
// never directly — or if newRole is domainroom.RoleMaster, which would
// leave the room with two masters instead of transferring ownership away
// from the current one. Rejecting newRole == domainroom.RoleMaster is
// already validated at the handler layer (HTTP 400 before this method is
// ever called via HTTP); this method re-checks it as defense in depth for
// any other caller, per this codebase's pattern of never trusting the
// handler layer alone for an invariant the usecase can enforce directly.
// It returns domain.ErrNotFound if targetUserID is not a member of roomID.
//
// Unlike LeaveRoom, this never calls hub.Revoke: a role change (even a
// demotion, e.g. to domainroom.RoleReader) never removes room membership
// itself, and every role above "not a member" still carries at least
// read/view access to the room — so targetUserID's live WebSocket
// subscription remains valid and must keep receiving events after this
// call, just as it would after a promotion.
func (u *RoomUsecase) ChangeMemberRole(ctx context.Context, callerID, roomID, targetUserID string, newRole domainroom.Role) (*domainroom.RoomMember, error) {
	caller, err := u.getMember(ctx, roomID, callerID)
	if err != nil {
		return nil, err
	}
	if err := domainroom.Authorize(caller.Role, domainroom.ActionManageMembers); err != nil {
		return nil, err
	}
	if newRole == domainroom.RoleMaster {
		return nil, domainroom.ErrOwnerRoleProtected
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if targetUserID == rm.OwnerID {
		return nil, domainroom.ErrOwnerRoleProtected
	}

	target, err := u.roomRepo.GetMember(ctx, roomID, targetUserID)
	if err != nil {
		return nil, err
	}

	if err := u.roomRepo.UpdateMemberRole(ctx, roomID, targetUserID, newRole); err != nil {
		return nil, err
	}
	target.Role = newRole
	return target, nil
}

// TransferOwnership transfers room ownership from its current owner
// (roomID's rooms.owner_id) to newOwnerID, and returns the room paired with
// the caller's post-transfer role (always domainroom.RoleAdmin, since the
// caller must have been the previous owner to reach that point — see
// below). Only the current owner may call this; it returns
// domain.ErrForbidden if callerID != room.OwnerID. If newOwnerID already is
// the current owner, it is a no-op that returns the room unchanged with no
// error (idempotent). Otherwise it returns domain.ErrNotFound if newOwnerID
// is not already a member of the room — ownership can only transfer to an
// existing member — then atomically updates rooms.owner_id, promotes
// newOwnerID to domainroom.RoleMaster, and demotes the previous owner to
// domainroom.RoleAdmin (see roomRepo.TransferOwnership) so the room always
// has exactly one master.
func (u *RoomUsecase) TransferOwnership(ctx context.Context, callerID, roomID, newOwnerID string) (*domainroom.RoomWithRole, error) {
	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if callerID != rm.OwnerID {
		return nil, domain.ErrForbidden
	}

	if newOwnerID == rm.OwnerID {
		return &domainroom.RoomWithRole{Room: rm, Role: domainroom.RoleMaster}, nil
	}

	if _, err := u.roomRepo.GetMember(ctx, roomID, newOwnerID); err != nil {
		return nil, err
	}

	if err := u.roomRepo.TransferOwnership(ctx, roomID, rm.OwnerID, newOwnerID); err != nil {
		return nil, err
	}

	rm.OwnerID = newOwnerID
	return &domainroom.RoomWithRole{Room: rm, Role: domainroom.RoleAdmin}, nil
}

// getMember loads the caller's membership in roomID, translating a missing
// membership (domain.ErrNotFound) into domain.ErrForbidden so that a
// non-member can never distinguish "room does not exist" from "room exists
// but I'm not a member of it" via the returned error. See
// domainroom.GetMemberOrForbidden (shared with usecase/message and
// usecase/invitation, which each keep this same thin wrapper).
func (u *RoomUsecase) getMember(ctx context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	return domainroom.GetMemberOrForbidden(ctx, u.roomRepo, roomID, userID)
}
