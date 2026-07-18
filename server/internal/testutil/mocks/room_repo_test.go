package mocks

import (
	"context"
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// TestRoomRepoUpdateMemberRoleOwnerProtected is a unit test for the
// owner-protection recheck RoomRepo.UpdateMemberRole added to mirror
// postgres.RoomRepository.UpdateMemberRole's `SELECT owner_id ... FOR
// UPDATE` guard: attempting to change the current owner's own membership
// role must return room.ErrOwnerRoleProtected, and must leave the
// membership's role untouched.
func TestRoomRepoUpdateMemberRoleOwnerProtected(t *testing.T) {
	repo := &RoomRepo{}
	ctx := context.Background()

	rm := &room.Room{ID: "room-1", OwnerID: "owner-1"}
	if err := repo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err := repo.UpdateMemberRole(ctx, "room-1", "owner-1", room.RoleAdmin)
	if !errors.Is(err, room.ErrOwnerRoleProtected) {
		t.Fatalf("expected room.ErrOwnerRoleProtected, got %v", err)
	}

	member, err := repo.GetMember(ctx, "room-1", "owner-1")
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if member.Role != room.RoleMaster {
		t.Fatalf("expected owner's role to remain master after a rejected update, got %s", member.Role)
	}
}

// TestRoomRepoUpdateMemberRoleNonOwnerSucceeds verifies that
// UpdateMemberRole's new owner recheck does not affect the ordinary case: a
// non-owner member's role still updates successfully.
func TestRoomRepoUpdateMemberRoleNonOwnerSucceeds(t *testing.T) {
	repo := &RoomRepo{}
	ctx := context.Background()

	rm := &room.Room{ID: "room-1", OwnerID: "owner-1"}
	if err := repo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.AddMember(ctx, &room.RoomMember{ID: "m-1", RoomID: "room-1", UserID: "member-1", Role: room.RoleMember}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := repo.UpdateMemberRole(ctx, "room-1", "member-1", room.RoleAdmin); err != nil {
		t.Fatalf("UpdateMemberRole failed: %v", err)
	}

	member, err := repo.GetMember(ctx, "room-1", "member-1")
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if member.Role != room.RoleAdmin {
		t.Fatalf("expected role admin, got %s", member.Role)
	}
}

// TestRoomRepoUpdateMemberRoleRoomNotFound verifies UpdateMemberRole returns
// domain.ErrNotFound when the room itself does not exist, mirroring
// postgres.RoomRepository.UpdateMemberRole's `SELECT owner_id FROM rooms ...`
// finding no row.
func TestRoomRepoUpdateMemberRoleRoomNotFound(t *testing.T) {
	repo := &RoomRepo{}
	ctx := context.Background()

	err := repo.UpdateMemberRole(ctx, "nonexistent-room", "member-1", room.RoleAdmin)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}
