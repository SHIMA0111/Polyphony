// Package room implements the room use cases: creating, listing, reading,
// updating, and deleting chat rooms on behalf of an authenticated user.
package room

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// RoomUsecase provides room-related business logic.
type RoomUsecase struct {
	roomRepo domainroom.RoomRepository
}

// NewRoomUsecase creates a new RoomUsecase.
func NewRoomUsecase(roomRepo domainroom.RoomRepository) *RoomUsecase {
	return &RoomUsecase{roomRepo: roomRepo}
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
	if !member.Role.Allows(domainroom.ActionManageRoom) {
		return nil, domain.ErrForbidden
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	rm.Name = name
	rm.Description = description
	rm.UpdatedAt = time.Now()

	if err = u.roomRepo.Update(ctx, rm); err != nil {
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
	if !member.Role.Allows(domainroom.ActionDeleteRoom) {
		return domain.ErrForbidden
	}

	return u.roomRepo.Delete(ctx, roomID)
}

// getMember loads the caller's membership in roomID, translating a missing
// membership (domain.ErrNotFound) into domain.ErrForbidden so that a
// non-member can never distinguish "room does not exist" from "room exists
// but I'm not a member of it" via the returned error.
func (u *RoomUsecase) getMember(ctx context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	member, err := u.roomRepo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrForbidden
		}
		return nil, err
	}
	return member, nil
}
