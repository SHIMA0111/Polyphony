package room

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// TestCreateRoom verifies CreateRoom persists a room with the given name
// and owner.
func TestCreateRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, err := uc.CreateRoom(ctx, "user-1", "Test Room", "A test room")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if rwr.Room.Name != "Test Room" {
		t.Fatalf("expected Test Room, got %s", rwr.Room.Name)
	}
	if rwr.Room.OwnerID != "user-1" {
		t.Fatalf("expected owner user-1, got %s", rwr.Room.OwnerID)
	}
	if rwr.Role != domainroom.RoleMaster {
		t.Fatalf("expected creator role master, got %s", rwr.Role)
	}
}

// TestGetRoomNotMember verifies GetRoom returns domain.ErrForbidden when the
// requesting user is not a member of the room.
func TestGetRoomNotMember(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.GetRoom(ctx, "user-2", rwr.Room.ID)
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGetRoomReturnsRole(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	created, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: created.Room.ID, UserID: "user-2", Role: domainroom.RoleGuest,
	})

	rwr, err := uc.GetRoom(ctx, "user-2", created.Room.ID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}
	if rwr.Role != domainroom.RoleGuest {
		t.Fatalf("expected role guest, got %s", rwr.Role)
	}
}

func TestUpdateRoomMemberForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	// Add user-2 as a plain member (not admin/master) — cannot manage room.
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	_, err := uc.UpdateRoom(ctx, "user-2", rwr.Room.ID, "New Name", "new desc")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateRoomAdminAllowed(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	// Add user-2 as admin — admin may manage room settings.
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})

	updated, err := uc.UpdateRoom(ctx, "user-2", rwr.Room.ID, "New Name", "new desc")
	if err != nil {
		t.Fatalf("expected admin to update room, got error: %v", err)
	}
	if updated.Room.Name != "New Name" {
		t.Fatalf("expected updated name, got %s", updated.Room.Name)
	}
}

func TestDeleteRoomAdminForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	// Add user-2 as admin — admin may manage the room but may not delete it.
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})

	err := uc.DeleteRoom(ctx, "user-2", rwr.Room.ID)
	if err != domain.ErrForbidden {
		t.Fatalf("expected admin DeleteRoom to be forbidden, got %v", err)
	}
}

// TestDeleteRoom verifies DeleteRoom removes the room so a subsequent
// GetRoom fails.
func TestDeleteRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	err := uc.DeleteRoom(ctx, "user-1", rwr.Room.ID)
	if err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	_, err = uc.GetRoom(ctx, "user-1", rwr.Room.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// TestListRoomsEmpty verifies ListRooms returns an empty slice (not an
// error) for a user who is not a member of any room.
func TestListRoomsEmpty(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rooms, err := uc.ListRooms(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListRooms failed: %v", err)
	}
	if len(rooms) != 0 {
		t.Fatalf("expected 0 rooms, got %d", len(rooms))
	}
}

func TestListRoomsIncludesRole(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	rooms, err := uc.ListRooms(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListRooms failed: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}
	if rooms[0].Room.ID != rwr.Room.ID {
		t.Fatalf("expected room %s, got %s", rwr.Room.ID, rooms[0].Room.ID)
	}
	if rooms[0].Role != domainroom.RoleMaster {
		t.Fatalf("expected role master, got %s", rooms[0].Role)
	}
}
