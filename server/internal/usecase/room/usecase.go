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

// CreateRoom creates a new room with the given user as owner.
func (u *RoomUsecase) CreateRoom(ctx context.Context, userID, name, description string) (*domainroom.Room, error) {
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

	return rm, nil
}

// GetRoom retrieves a room. Only members can access it.
func (u *RoomUsecase) GetRoom(ctx context.Context, userID, roomID string) (*domainroom.Room, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	return u.roomRepo.GetByID(ctx, roomID)
}

// ListRooms returns all rooms the user is a member of.
func (u *RoomUsecase) ListRooms(ctx context.Context, userID string) ([]*domainroom.Room, error) {
	return u.roomRepo.ListByUserID(ctx, userID)
}

// UpdateRoom updates a room. Only the owner can update.
func (u *RoomUsecase) UpdateRoom(ctx context.Context, userID, roomID, name, description string) (*domainroom.Room, error) {
	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	if rm.OwnerID != userID {
		return nil, domain.ErrForbidden
	}

	rm.Name = name
	rm.Description = description
	rm.UpdatedAt = time.Now()

	if err = u.roomRepo.Update(ctx, rm); err != nil {
		return nil, err
	}

	return rm, nil
}

// DeleteRoom deletes a room. Only the owner can delete.
func (u *RoomUsecase) DeleteRoom(ctx context.Context, userID, roomID string) error {
	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	if rm.OwnerID != userID {
		return domain.ErrForbidden
	}

	return u.roomRepo.Delete(ctx, roomID)
}

func (u *RoomUsecase) checkMembership(ctx context.Context, roomID, userID string) error {
	_, err := u.roomRepo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden
		}
		return err
	}
	return nil
}
