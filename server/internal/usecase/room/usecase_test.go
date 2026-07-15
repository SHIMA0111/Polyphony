package room

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func TestCreateRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, err := uc.CreateRoom(ctx, "user-1", "Test Room", "A test room")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if rm.Name != "Test Room" {
		t.Fatalf("expected Test Room, got %s", rm.Name)
	}
	if rm.OwnerID != "user-1" {
		t.Fatalf("expected owner user-1, got %s", rm.OwnerID)
	}
}

func TestGetRoomNotMember(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.GetRoom(ctx, "user-2", rm.ID)
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateRoomNotOwner(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	// Add user-2 as member
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rm.ID, UserID: "user-2", Role: "member",
	})

	_, err := uc.UpdateRoom(ctx, "user-2", rm.ID, "New Name", "new desc")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestDeleteRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	err := uc.DeleteRoom(ctx, "user-1", rm.ID)
	if err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	_, err = uc.GetRoom(ctx, "user-1", rm.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

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
