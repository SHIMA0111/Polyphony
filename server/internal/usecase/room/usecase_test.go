package room

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

// TestGetRoomReturnsRole verifies GetRoom returns the caller's own
// membership role (e.g. domainroom.RoleGuest) alongside the room.
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

// TestUpdateRoomMemberForbidden verifies UpdateRoom returns
// domain.ErrForbidden for a plain domainroom.RoleMember, who lacks the
// ActionManageRoom permission.
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

// TestUpdateRoomAdminAllowed verifies UpdateRoom succeeds for a
// domainroom.RoleAdmin member, who holds ActionManageRoom permission.
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

// TestDeleteRoomAdminForbidden verifies DeleteRoom returns
// domain.ErrForbidden for a domainroom.RoleAdmin member: deleting a room is
// reserved for domainroom.RoleMaster (the ActionDeleteRoom permission).
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

// --- UpdateAIContextCutoff (Step 23: AI context control) ---

func TestUpdateAIContextCutoffAdminCanSet(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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

// TestUpdateRoomAndAIContextCutoffDoNotClobberEachOther is a regression
// test for Step 26: UpdateRoom and UpdateAIContextCutoff must persist via
// disjoint partial updates (RoomRepository.UpdateDetails /
// UpdateAIContextCutoff), not a shared full-row Update, so setting the
// cutoff does not lose a previously-set name/description and vice versa --
// simulating what a full-row Update built from a stale in-memory Room would
// have lost under either ordering.
func TestUpdateRoomAndAIContextCutoffDoNotClobberEachOther(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Original Name", "Original Description")

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-1", rwr.Room.ID, &cutoff); err != nil {
		t.Fatalf("UpdateAIContextCutoff failed: %v", err)
	}

	updated, err := uc.UpdateRoom(ctx, "user-1", rwr.Room.ID, "New Name", "New Description")
	if err != nil {
		t.Fatalf("UpdateRoom failed: %v", err)
	}
	if updated.Room.Name != "New Name" || updated.Room.Description != "New Description" {
		t.Fatalf("expected name/description to update, got %+v", updated.Room)
	}
	// The cutoff set moments earlier must survive UpdateRoom's partial
	// update, not be clobbered back to nil by a stale full-row write.
	if updated.Room.AIContextCutoffAt == nil || !updated.Room.AIContextCutoffAt.Equal(cutoff) {
		t.Fatalf("expected UpdateRoom to preserve the previously-set cutoff %v, got %v", cutoff, updated.Room.AIContextCutoffAt)
	}

	// And the reverse: fetch fresh from the repo (not the usecase's locally
	// mutated struct) to prove the persisted row itself, not just the
	// in-memory return value, has both fields.
	persisted, err := repo.GetByID(ctx, rwr.Room.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if persisted.Name != "New Name" || persisted.Description != "New Description" {
		t.Fatalf("expected persisted name/description to be updated, got %+v", persisted)
	}
	if persisted.AIContextCutoffAt == nil || !persisted.AIContextCutoffAt.Equal(cutoff) {
		t.Fatalf("expected persisted cutoff %v to survive UpdateRoom, got %v", cutoff, persisted.AIContextCutoffAt)
	}
}

// TestConcurrentUpdateRoomAndAIContextCutoffNoLostUpdate is a concurrency
// regression test for Step 26: UpdateRoom and UpdateAIContextCutoff, run
// concurrently against the same room, must both land -- neither call's
// GetByID-then-write may clobber the other's change. This is the scenario
// the old shared full-row Update was vulnerable to: if both calls' GetByID
// reads happened before either wrote, whichever wrote last would overwrite
// the other's field-group with its own stale copy. UpdateDetails and
// UpdateAIContextCutoff being genuinely disjoint partial updates (touching
// only their own columns) makes that interleaving harmless by construction.
//
// Run with -race to also catch any data race in the repo mock itself.
func TestConcurrentUpdateRoomAndAIContextCutoffNoLostUpdate(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Original Name", "Original Description")
	cutoff := time.Now()

	var wg sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})

	var updateRoomErr, updateCutoffErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		_, updateRoomErr = uc.UpdateRoom(ctx, "user-1", rwr.Room.ID, "New Name", "New Description")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		_, updateCutoffErr = uc.UpdateAIContextCutoff(ctx, "user-1", rwr.Room.ID, &cutoff)
	}()

	ready.Wait()
	close(start)
	wg.Wait()

	if updateRoomErr != nil {
		t.Fatalf("UpdateRoom failed: %v", updateRoomErr)
	}
	if updateCutoffErr != nil {
		t.Fatalf("UpdateAIContextCutoff failed: %v", updateCutoffErr)
	}

	persisted, err := repo.GetByID(ctx, rwr.Room.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if persisted.Name != "New Name" || persisted.Description != "New Description" {
		t.Fatalf("expected both concurrent calls' name/description change to land, got %+v", persisted)
	}
	if persisted.AIContextCutoffAt == nil || !persisted.AIContextCutoffAt.Equal(cutoff) {
		t.Fatalf("expected both concurrent calls' cutoff change to land (%v), got %v", cutoff, persisted.AIContextCutoffAt)
	}
}

func TestUpdateAIContextCutoffMasterCanClear(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	cutoff := time.Now()
	if _, err := uc.UpdateAIContextCutoff(ctx, "user-2", rwr.Room.ID, &cutoff); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for non-member, got %v", err)
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

// TestListRoomsIncludesRole verifies ListRooms returns each room paired with
// the caller's own membership role.
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

func TestListMembersReturnsForReader(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.ListMembers(ctx, "user-2", rwr.Room.ID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestLeaveRoomSucceedsForNonOwnerMember(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	err := uc.LeaveRoom(ctx, "user-1", rwr.Room.ID, "user-1")
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected ErrOwnerRoleProtected, got %v", err)
	}
}

func TestLeaveRoomForbiddenWhenTargetNotCaller(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
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

// TestChangeMemberRolePreservesUsername proves that ChangeMemberRole's
// returned RoomMember carries through the Username that GetMember resolved
// (Step 42 -- RoomRepository.GetMember now JOINs users to populate it,
// mirroring ListMembers, instead of always leaving it "").
func TestChangeMemberRolePreservesUsername(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m3", RoomID: rwr.Room.ID, UserID: "user-3", Role: domainroom.RoleMember, Username: "carol",
	})

	updated, err := uc.ChangeMemberRole(ctx, "user-2", rwr.Room.ID, "user-3", domainroom.RoleGuest)
	if err != nil {
		t.Fatalf("ChangeMemberRole failed: %v", err)
	}
	if updated.Username != "carol" {
		t.Fatalf("expected Username %q to be preserved, got %q", "carol", updated.Username)
	}
}

// TestChangeMemberRoleRejectsMasterRole proves that ChangeMemberRole rejects
// newRole == domainroom.RoleMaster as defense in depth, even though the
// handler layer already blocks it before reaching the usecase.
func TestChangeMemberRoleRejectsMasterRole(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m3", RoomID: rwr.Room.ID, UserID: "user-3", Role: domainroom.RoleMember,
	})

	_, err := uc.ChangeMemberRole(ctx, "user-2", rwr.Room.ID, "user-3", domainroom.RoleMaster)
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected ErrOwnerRoleProtected promoting a member to master, got %v", err)
	}
}

func TestChangeMemberRoleOwnerProtected(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
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

// TestTransferOwnershipStaleOwnerRepoCAS is a mock-level regression test
// (Step 27) for RoomRepository.TransferOwnership's compare-and-swap on the
// room's current owner: a second TransferOwnership call using an
// already-stale oldOwnerID (ownership already moved by a prior call) must
// fail with domain.ErrNotFound and must not touch room_members roles,
// exercising the same WHERE-clause guard mocks.RoomRepo.TransferOwnership
// mirrors from the real postgres implementation.
func TestTransferOwnershipStaleOwnerRepoCAS(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m3", RoomID: rwr.Room.ID, UserID: "user-3", Role: domainroom.RoleMember,
	})

	// Legitimate transfer: user-1 -> user-2.
	if _, err := uc.TransferOwnership(ctx, "user-1", rwr.Room.ID, "user-2"); err != nil {
		t.Fatalf("first TransferOwnership failed: %v", err)
	}

	// A direct repo-level call using the now-stale oldOwnerID ("user-1")
	// must fail: user-1 is no longer the room's owner. (The usecase itself
	// would reject this earlier via its own callerID == rm.OwnerID check --
	// this test exercises the repository's own CAS directly, standing in
	// for a race where the usecase's own check passed against a value that
	// went stale before the repo call landed.)
	err := repo.TransferOwnership(ctx, rwr.Room.ID, "user-1", "user-3")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for a stale-owner TransferOwnership, got %v", err)
	}

	// user-3's role must remain untouched by the failed stale-owner call.
	user3Member, err := repo.GetMember(ctx, rwr.Room.ID, "user-3")
	if err != nil {
		t.Fatalf("GetMember(user-3) failed: %v", err)
	}
	if user3Member.Role != domainroom.RoleMember {
		t.Fatalf("expected user-3's role to remain member after the failed stale-owner transfer, got %s", user3Member.Role)
	}
}

func TestTransferOwnershipForbiddenForNonOwner(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.TransferOwnership(ctx, "user-1", rwr.Room.ID, "user-2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTransferOwnershipNoOpWhenTargetIsCurrentOwner(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
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
