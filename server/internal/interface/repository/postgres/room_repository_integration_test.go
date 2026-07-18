//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

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
// must fail at the database with a CHECK violation specifically — not merely
// fail for some other reason.
//
// The INSERT uses a fresh user who is *not* already a member of the room
// (rather than reusing the room's owner, who RoomRepository.Create already
// added as a room_members row): inserting a second row for
// (room_id, owner_id) would also violate the UNIQUE(room_id, user_id)
// constraint, which could make this test pass for the wrong reason (a
// unique violation, not the role CHECK constraint this test targets) if
// Postgres happened to report that constraint first.
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

	// A fresh user, never added to room_members, so this INSERT cannot also
	// collide with the UNIQUE(room_id, user_id) constraint.
	nonMember := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "role-check-non-member@example.com",
		Username:     "role-check-non-member",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, nonMember); err != nil {
		t.Fatalf("create non-member user: %v", err)
	}

	_, err := pool.Exec(ctx,
		`INSERT INTO room_members (id, room_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, 'superadmin', $4)`,
		uuid.New().String(), rm.ID, nonMember.ID, time.Now(),
	)
	if err == nil {
		t.Fatal("expected an INSERT with role='superadmin' to violate the room_members_role_check CHECK constraint, got nil error")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected a *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != pgerrcode.CheckViolation {
		t.Fatalf("expected check_violation (%s), got code %s: %v", pgerrcode.CheckViolation, pgErr.Code, err)
	}
	if pgErr.ConstraintName != "room_members_role_check" {
		t.Fatalf("expected constraint room_members_role_check, got %s", pgErr.ConstraintName)
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
// persists the new role for an existing membership, and that GetMember
// populates RoomMember.Username via its JOIN against users (Step 42) --
// not just ListMembers.
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
	if got.Username != member.Username {
		t.Fatalf("expected GetMember to populate Username %q, got %q", member.Username, got.Username)
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

// TestTransferOwnershipStaleOwnerCAS proves that
// RoomRepository.TransferOwnership's owner_id update is a compare-and-swap
// against the caller-supplied oldOwnerID (Step 27): once ownership has
// already moved, a second call using the now-stale previous owner ID fails
// with domain.ErrNotFound and leaves the room's actual current owner/role
// assignments untouched, rather than blindly overwriting them (regression
// test for the `UPDATE rooms SET owner_id = ...` previously lacking an
// `AND owner_id = ...` guard, which let a concurrent/stale transfer race the
// legitimate one).
func TestTransferOwnershipStaleOwnerCAS(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "cas-owner")
	firstNewOwner := createTestUser(ctx, t, userRepo, "cas-first-new-owner")
	secondNewOwner := createTestUser(ctx, t, userRepo, "cas-second-new-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "CAS Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: firstNewOwner.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember(firstNewOwner) failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: secondNewOwner.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember(secondNewOwner) failed: %v", err)
	}

	// The legitimate transfer: owner -> firstNewOwner.
	if err := roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, firstNewOwner.ID); err != nil {
		t.Fatalf("first TransferOwnership failed: %v", err)
	}

	// A second call still using the now-stale original owner ID must fail:
	// owner.ID is no longer rooms.owner_id.
	err := roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, secondNewOwner.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for a stale-owner TransferOwnership, got %v", err)
	}

	// The room's actual state must reflect only the first, legitimate
	// transfer -- untouched by the failed stale-owner attempt.
	updatedRoom, getErr := roomRepo.GetByID(ctx, rm.ID)
	if getErr != nil {
		t.Fatalf("GetByID failed: %v", getErr)
	}
	if updatedRoom.OwnerID != firstNewOwner.ID {
		t.Fatalf("expected owner_id to remain %s, got %s", firstNewOwner.ID, updatedRoom.OwnerID)
	}

	secondNewOwnerMember, getErr := roomRepo.GetMember(ctx, rm.ID, secondNewOwner.ID)
	if getErr != nil {
		t.Fatalf("GetMember(secondNewOwner) failed: %v", getErr)
	}
	if secondNewOwnerMember.Role != domainroom.RoleMember {
		t.Fatalf("expected secondNewOwner's role to remain member after the failed stale-owner transfer, got %s", secondNewOwnerMember.Role)
	}
}
