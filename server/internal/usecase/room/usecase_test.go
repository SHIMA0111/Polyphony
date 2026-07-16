package room

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func TestCreateRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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

func TestGetRoomNotMember(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.GetRoom(ctx, "user-2", rwr.Room.ID)
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGetRoomReturnsRole(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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

func TestDeleteRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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

// --- UpdateAIContextCutoff (Step 23: AI context control) ---

func TestUpdateAIContextCutoffAdminCanSet(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})

	cutoff := time.Now()
	updated, err := uc.UpdateAIContextCutoff(ctx, "user-2", rwr.Room.ID, &cutoff)
	if err != nil {
		t.Fatalf("expected admin to set cutoff, got error: %v", err)
	}
	if updated.Room.AIContextCutoffAt == nil || !updated.Room.AIContextCutoffAt.Equal(cutoff) {
		t.Fatalf("expected cutoff %v, got %v", cutoff, updated.Room.AIContextCutoffAt)
	}
}

func TestUpdateAIContextCutoffMasterCanClear(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-1", rwr.Room.ID, &cutoff); err != nil {
		t.Fatalf("expected master to set cutoff, got error: %v", err)
	}

	cleared, err := uc.UpdateAIContextCutoff(ctx, "user-1", rwr.Room.ID, nil)
	if err != nil {
		t.Fatalf("expected master to clear cutoff, got error: %v", err)
	}
	if cleared.Room.AIContextCutoffAt != nil {
		t.Fatalf("expected cleared cutoff to be nil, got %v", cleared.Room.AIContextCutoffAt)
	}
}

func TestUpdateAIContextCutoffMemberForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-2", rwr.Room.ID, &cutoff); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for member, got %v", err)
	}
}

func TestUpdateAIContextCutoffGuestForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleGuest,
	})

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-2", rwr.Room.ID, &cutoff); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for guest, got %v", err)
	}
}

func TestUpdateAIContextCutoffReaderForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleReader,
	})

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-2", rwr.Room.ID, &cutoff); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for reader, got %v", err)
	}
}

func TestUpdateAIContextCutoffNonMemberForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-2", rwr.Room.ID, &cutoff); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for non-member, got %v", err)
	}
}

func TestListRoomsEmpty(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
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

func TestListMembersReturnsForReader(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleReader,
	})

	members, err := uc.ListMembers(ctx, "user-2", rwr.Room.ID)
	if err != nil {
		t.Fatalf("ListMembers failed for reader: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}

func TestListMembersForbiddenForNonMember(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.ListMembers(ctx, "user-2", rwr.Room.ID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestLeaveRoomSucceedsForNonOwnerMember(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	if err := uc.LeaveRoom(ctx, "user-2", rwr.Room.ID, "user-2"); err != nil {
		t.Fatalf("LeaveRoom failed: %v", err)
	}
	if _, err := repo.GetMember(ctx, rwr.Room.ID, "user-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected membership to be removed, got err=%v", err)
	}
}

func TestLeaveRoomOwnerProtected(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	err := uc.LeaveRoom(ctx, "user-1", rwr.Room.ID, "user-1")
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected ErrOwnerRoleProtected, got %v", err)
	}
}

// TestLeaveRoomRevokesLiveSubscription proves that a successful LeaveRoom
// closes the departing member's live MessageHub subscription on the room
// (via hub.Revoke), so their WebSocket connection stops receiving room
// events immediately rather than continuing until some later membership
// check happens to run.
func TestLeaveRoomRevokesLiveSubscription(t *testing.T) {
	repo := &mocks.RoomRepo{}
	hub := event.NewInProcessHub()
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, hub)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	ch, unsubscribe := hub.Subscribe(ctx, rwr.Room.ID, "user-2")
	defer unsubscribe()

	if err := uc.LeaveRoom(ctx, "user-2", rwr.Room.ID, "user-2"); err != nil {
		t.Fatalf("LeaveRoom failed: %v", err)
	}

	select {
	case _, open := <-ch:
		if open {
			t.Fatal("expected the subscription channel to be closed (revoked), not to still deliver values")
		}
	default:
		t.Fatal("expected the subscription channel to be closed by LeaveRoom, but a read would have blocked")
	}
}

func TestLeaveRoomForbiddenWhenTargetNotCaller(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	err := uc.LeaveRoom(ctx, "user-2", rwr.Room.ID, "user-3")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestChangeMemberRoleSucceedsForAdmin(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m3", RoomID: rwr.Room.ID, UserID: "user-3", Role: domainroom.RoleMember,
	})

	updated, err := uc.ChangeMemberRole(ctx, "user-2", rwr.Room.ID, "user-3", domainroom.RoleGuest)
	if err != nil {
		t.Fatalf("ChangeMemberRole failed: %v", err)
	}
	if updated.Role != domainroom.RoleGuest {
		t.Fatalf("expected role guest, got %s", updated.Role)
	}
}

func TestChangeMemberRoleForbiddenForInsufficientRole(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m3", RoomID: rwr.Room.ID, UserID: "user-3", Role: domainroom.RoleGuest,
	})

	for _, callerID := range []string{"user-2", "user-3"} {
		_, err := uc.ChangeMemberRole(ctx, callerID, rwr.Room.ID, "user-1", domainroom.RoleReader)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("caller %s: expected ErrForbidden, got %v", callerID, err)
		}
	}
}

func TestChangeMemberRoleOwnerProtected(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})

	_, err := uc.ChangeMemberRole(ctx, "user-2", rwr.Room.ID, "user-1", domainroom.RoleGuest)
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected ErrOwnerRoleProtected, got %v", err)
	}
}

func TestTransferOwnershipSucceeds(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	result, err := uc.TransferOwnership(ctx, "user-1", rwr.Room.ID, "user-2")
	if err != nil {
		t.Fatalf("TransferOwnership failed: %v", err)
	}
	if result.Room.OwnerID != "user-2" {
		t.Fatalf("expected new owner user-2, got %s", result.Room.OwnerID)
	}

	newOwnerMember, err := repo.GetMember(ctx, rwr.Room.ID, "user-2")
	if err != nil {
		t.Fatalf("GetMember(user-2) failed: %v", err)
	}
	if newOwnerMember.Role != domainroom.RoleMaster {
		t.Fatalf("expected new owner role master, got %s", newOwnerMember.Role)
	}

	oldOwnerMember, err := repo.GetMember(ctx, rwr.Room.ID, "user-1")
	if err != nil {
		t.Fatalf("GetMember(user-1) failed: %v", err)
	}
	if oldOwnerMember.Role != domainroom.RoleAdmin {
		t.Fatalf("expected old owner role admin, got %s", oldOwnerMember.Role)
	}
}

func TestTransferOwnershipForbiddenForNonOwner(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})

	_, err := uc.TransferOwnership(ctx, "user-2", rwr.Room.ID, "user-2")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestTransferOwnershipNotFoundForNonMemberTarget(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.TransferOwnership(ctx, "user-1", rwr.Room.ID, "user-2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTransferOwnershipNoOpWhenTargetIsCurrentOwner(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	result, err := uc.TransferOwnership(ctx, "user-1", rwr.Room.ID, "user-1")
	if err != nil {
		t.Fatalf("expected no-op transfer to succeed, got error: %v", err)
	}
	if result.Room.OwnerID != "user-1" {
		t.Fatalf("expected owner unchanged (user-1), got %s", result.Room.OwnerID)
	}
}
