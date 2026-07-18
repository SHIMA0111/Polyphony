package mocks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// TestRoomRepoRemoveMemberRejectsCurrentOwner verifies that RoomRepo's
// in-memory fake, like postgres.RoomRepository, independently refuses to
// delete the room's current owner's membership -- mirroring
// UpdateMemberRole's existing owner-protection check -- rather than relying
// solely on the usecase layer's own pre-check.
func TestRoomRepoRemoveMemberRejectsCurrentOwner(t *testing.T) {
	ctx := context.Background()
	rm := &room.Room{ID: "room-1", Name: "Room", OwnerID: "owner-1", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	repo := &RoomRepo{
		Rooms: map[string]*room.Room{rm.ID: rm},
		Members: map[string]map[string]*room.RoomMember{
			rm.ID: {
				"owner-1": {ID: "m-1", RoomID: rm.ID, UserID: "owner-1", Role: room.RoleMaster, JoinedAt: time.Now()},
			},
		},
	}

	err := repo.RemoveMember(ctx, rm.ID, "owner-1")
	if !errors.Is(err, room.ErrOwnerRoleProtected) {
		t.Fatalf("expected room.ErrOwnerRoleProtected for the current owner, got %v", err)
	}

	got, getErr := repo.GetMember(ctx, rm.ID, "owner-1")
	if getErr != nil {
		t.Fatalf("GetMember failed: %v", getErr)
	}
	if got.Role != room.RoleMaster {
		t.Fatalf("expected the owner's membership to remain intact, got role %s", got.Role)
	}
}

// TestRoomRepoRemoveMemberSucceedsForNonOwner is a control case for
// TestRoomRepoRemoveMemberRejectsCurrentOwner: a non-owner member must
// still be removable normally.
func TestRoomRepoRemoveMemberSucceedsForNonOwner(t *testing.T) {
	ctx := context.Background()
	rm := &room.Room{ID: "room-1", Name: "Room", OwnerID: "owner-1", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	repo := &RoomRepo{
		Rooms: map[string]*room.Room{rm.ID: rm},
		Members: map[string]map[string]*room.RoomMember{
			rm.ID: {
				"owner-1":  {ID: "m-1", RoomID: rm.ID, UserID: "owner-1", Role: room.RoleMaster, JoinedAt: time.Now()},
				"member-2": {ID: "m-2", RoomID: rm.ID, UserID: "member-2", Role: room.RoleMember, JoinedAt: time.Now()},
			},
		},
	}

	if err := repo.RemoveMember(ctx, rm.ID, "member-2"); err != nil {
		t.Fatalf("RemoveMember failed for a non-owner member: %v", err)
	}

	if _, err := repo.GetMember(ctx, rm.ID, "member-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the member to be gone, got %v", err)
	}
}
