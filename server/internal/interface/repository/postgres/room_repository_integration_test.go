//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// TestRoomRepositoryCreateRollback proves that RoomRepository.Create's
// multi-statement transaction (insert rooms, insert room_sequences, insert
// room_members) rolls back cleanly via its `defer tx.Rollback(ctx)` when a
// later statement in the transaction fails: a second Create call reusing an
// already-existing room ID fails on the rooms primary key, and none of the
// three tables it writes to should retain a leftover row from that failed
// attempt.
func TestRoomRepositoryCreateRollback(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "rollback-owner@example.com",
		Username:     "rollback-owner",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        "Original Room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	// Reuse the same room ID: the rooms insert fails on the primary key,
	// so the room_sequences and room_members inserts in the same
	// transaction must never be observed either.
	duplicate := &domainroom.Room{
		ID:          rm.ID,
		Name:        "Duplicate Room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, duplicate); err == nil {
		t.Fatal("expected an error creating a room with a duplicate ID, got nil")
	}

	var roomCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM rooms WHERE id = $1`, rm.ID).Scan(&roomCount); err != nil {
		t.Fatalf("count rooms: %v", err)
	}
	if roomCount != 1 {
		t.Fatalf("expected exactly 1 rooms row after the failed duplicate Create, got %d", roomCount)
	}

	var seqCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM room_sequences WHERE room_id = $1`, rm.ID).Scan(&seqCount); err != nil {
		t.Fatalf("count room_sequences: %v", err)
	}
	if seqCount != 1 {
		t.Fatalf("expected exactly 1 room_sequences row after the failed duplicate Create, got %d", seqCount)
	}

	var memberCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM room_members WHERE room_id = $1`, rm.ID).Scan(&memberCount); err != nil {
		t.Fatalf("count room_members: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("expected exactly 1 room_members row after the failed duplicate Create, got %d", memberCount)
	}
}

// TestRoomMembersRoleCheckConstraint proves that room_members.role is
// enforced by a DB-level CHECK constraint (added by the
// add_room_members_role_check migration), independent of any Go-level
// validation: an INSERT with a role value outside the five defined roles
// must fail at the database.
func TestRoomMembersRoleCheckConstraint(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "role-check-owner@example.com",
		Username:     "role-check-owner",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        "Role Check Room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err := pool.Exec(ctx,
		`INSERT INTO room_members (id, room_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, 'superadmin', $4)`,
		uuid.New().String(), rm.ID, owner.ID, time.Now(),
	)
	if err == nil {
		t.Fatal("expected an INSERT with role='superadmin' to violate the room_members_role_check CHECK constraint, got nil error")
	}
}

// createTestUser is a small integration-test helper that persists a new
// user with a random email/username and returns it.
func createTestUser(ctx context.Context, t *testing.T, userRepo *UserRepository, label string) *domainuser.User {
	t.Helper()
	u := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        label + "-" + uuid.New().String() + "@example.com",
		Username:     label + "-" + uuid.New().String(),
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create %s user: %v", label, err)
	}
	return u
}

// TestUpdateMemberRolePersists proves that RoomRepository.UpdateMemberRole
// persists the new role for an existing membership.
func TestUpdateMemberRolePersists(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "role-update-owner")
	member := createTestUser(ctx, t, userRepo, "role-update-member")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Role Update Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: member.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := roomRepo.UpdateMemberRole(ctx, rm.ID, member.ID, domainroom.RoleAdmin); err != nil {
		t.Fatalf("UpdateMemberRole failed: %v", err)
	}

	got, err := roomRepo.GetMember(ctx, rm.ID, member.ID)
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if got.Role != domainroom.RoleAdmin {
		t.Fatalf("expected role admin, got %s", got.Role)
	}
}

// TestUpdateMemberRoleNotFound proves that RoomRepository.UpdateMemberRole
// returns domain.ErrNotFound when the (roomID, userID) membership does not
// exist.
func TestUpdateMemberRoleNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "role-notfound-owner")
	nonMember := createTestUser(ctx, t, userRepo, "role-notfound-nonmember")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Role NotFound Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err := roomRepo.UpdateMemberRole(ctx, rm.ID, nonMember.ID, domainroom.RoleAdmin)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

// TestTransferOwnershipAtomic proves that RoomRepository.TransferOwnership
// updates rooms.owner_id, promotes the new owner to master, and demotes the
// previous owner to admin, all as a single atomic transaction.
func TestTransferOwnershipAtomic(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "transfer-owner")
	newOwner := createTestUser(ctx, t, userRepo, "transfer-new-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Transfer Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: newOwner.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, newOwner.ID); err != nil {
		t.Fatalf("TransferOwnership failed: %v", err)
	}

	updatedRoom, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updatedRoom.OwnerID != newOwner.ID {
		t.Fatalf("expected owner_id %s, got %s", newOwner.ID, updatedRoom.OwnerID)
	}

	newOwnerMember, err := roomRepo.GetMember(ctx, rm.ID, newOwner.ID)
	if err != nil {
		t.Fatalf("GetMember(newOwner) failed: %v", err)
	}
	if newOwnerMember.Role != domainroom.RoleMaster {
		t.Fatalf("expected new owner role master, got %s", newOwnerMember.Role)
	}

	oldOwnerMember, err := roomRepo.GetMember(ctx, rm.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetMember(oldOwner) failed: %v", err)
	}
	if oldOwnerMember.Role != domainroom.RoleAdmin {
		t.Fatalf("expected old owner role admin, got %s", oldOwnerMember.Role)
	}
}

// TestTransferOwnershipRollbackOnMissingNewOwner proves that
// RoomRepository.TransferOwnership rolls back cleanly (no partial writes)
// when the new-owner membership row does not exist: rooms.owner_id and the
// previous owner's role must remain unchanged after the failed call.
func TestTransferOwnershipRollbackOnMissingNewOwner(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "rollback-owner")
	nonMember := createTestUser(ctx, t, userRepo, "rollback-nonmember")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Rollback Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err := roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, nonMember.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}

	// Verify no partial writes: owner_id and the original owner's role must
	// be unchanged.
	updatedRoom, getErr := roomRepo.GetByID(ctx, rm.ID)
	if getErr != nil {
		t.Fatalf("GetByID failed: %v", getErr)
	}
	if updatedRoom.OwnerID != owner.ID {
		t.Fatalf("expected owner_id to remain %s after rollback, got %s", owner.ID, updatedRoom.OwnerID)
	}

	ownerMember, getErr := roomRepo.GetMember(ctx, rm.ID, owner.ID)
	if getErr != nil {
		t.Fatalf("GetMember(owner) failed: %v", getErr)
	}
	if ownerMember.Role != domainroom.RoleMaster {
		t.Fatalf("expected original owner role to remain master after rollback, got %s", ownerMember.Role)
	}
}

// TestRoomRepositoryAIProviderModelRoundTrip proves that AIProvider/AIModel
// (Step 24: per-room AI provider/model settings) round-trip through
// GetByID and Update: a freshly created room has both as nil (NULL), and
// after Update sets them to non-nil values, GetByID, ListByUserID, and
// ListByUserIDWithRole all observe the same values.
func TestRoomRepositoryAIProviderModelRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "ai-settings-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "AI Settings Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Freshly created room: both columns are NULL.
	fetched, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.AIProvider != nil || fetched.AIModel != nil {
		t.Fatalf("expected nil ai_provider/ai_model on a freshly created room, got %v / %v",
			fetched.AIProvider, fetched.AIModel)
	}

	provider := "anthropic"
	model := "claude-opus-4"
	fetched.AIProvider = &provider
	fetched.AIModel = &model
	if err := roomRepo.Update(ctx, fetched); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	afterUpdate, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if afterUpdate.AIProvider == nil || *afterUpdate.AIProvider != provider {
		t.Fatalf("expected ai_provider %q, got %v", provider, afterUpdate.AIProvider)
	}
	if afterUpdate.AIModel == nil || *afterUpdate.AIModel != model {
		t.Fatalf("expected ai_model %q, got %v", model, afterUpdate.AIModel)
	}

	listed, err := roomRepo.ListByUserID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if !roomListContainsAISettings(listed, rm.ID, provider, model) {
		t.Fatalf("expected ListByUserID to include ai_provider/ai_model for room %s", rm.ID)
	}

	listedWithRole, err := roomRepo.ListByUserIDWithRole(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByUserIDWithRole failed: %v", err)
	}
	found := false
	for _, rwr := range listedWithRole {
		if rwr.Room.ID != rm.ID {
			continue
		}
		found = true
		if rwr.Room.AIProvider == nil || *rwr.Room.AIProvider != provider {
			t.Fatalf("expected ai_provider %q in ListByUserIDWithRole, got %v", provider, rwr.Room.AIProvider)
		}
		if rwr.Room.AIModel == nil || *rwr.Room.AIModel != model {
			t.Fatalf("expected ai_model %q in ListByUserIDWithRole, got %v", model, rwr.Room.AIModel)
		}
	}
	if !found {
		t.Fatalf("expected ListByUserIDWithRole to include room %s", rm.ID)
	}

	// Clearing back to nil round-trips as well.
	afterUpdate.AIProvider = nil
	afterUpdate.AIModel = nil
	if err := roomRepo.Update(ctx, afterUpdate); err != nil {
		t.Fatalf("Update (clear) failed: %v", err)
	}
	cleared, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID after clear failed: %v", err)
	}
	if cleared.AIProvider != nil || cleared.AIModel != nil {
		t.Fatalf("expected nil ai_provider/ai_model after clearing, got %v / %v", cleared.AIProvider, cleared.AIModel)
	}
}

// roomListContainsAISettings reports whether rooms contains roomID with the
// expected AIProvider/AIModel values.
func roomListContainsAISettings(rooms []*domainroom.Room, roomID, provider, model string) bool {
	for _, rm := range rooms {
		if rm.ID != roomID {
			continue
		}
		return rm.AIProvider != nil && *rm.AIProvider == provider && rm.AIModel != nil && *rm.AIModel == model
	}
	return false
}
