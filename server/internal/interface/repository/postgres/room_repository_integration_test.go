//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
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

// TestUpdateMemberRoleSerializedAgainstTransferOwnership races
// RoomRepository.TransferOwnership(owner -> newOwner) against
// RoomRepository.UpdateMemberRole(newOwner, RoleReader) -- an attempt to
// downgrade the very member who is concurrently becoming the room's new
// owner -- and asserts the room is never left with an owner whose
// room_members.role is not master.
//
// This exercises the row lock UpdateMemberRole's owner recheck
// (`SELECT owner_id FROM rooms WHERE id = $1 FOR UPDATE`) takes against the
// same rooms row TransferOwnership's `UPDATE rooms SET owner_id = ...`
// locks: without it, UpdateMemberRole could read owner_id = owner (still
// the pre-transfer value) with no lock, decide newOwner is not (yet) the
// owner and so is a legal target, and then have its
// `UPDATE room_members SET role = 'reader' ...` commit *after*
// TransferOwnership's own `UPDATE room_members SET role = 'master' ...` for
// newOwner already committed -- leaving newOwner as rooms.owner_id but with
// role 'reader'. The row lock instead serializes the two transactions, so
// whichever commits first is fully visible to the other before it proceeds:
// either UpdateMemberRole's downgrade lands before TransferOwnership (which
// then unconditionally promotes newOwner to master anyway, overwriting it),
// or UpdateMemberRole's owner recheck observes newOwner already installed as
// owner (post-TransferOwnership-commit) and rejects the downgrade with
// room.ErrOwnerRoleProtected.
func TestUpdateMemberRoleSerializedAgainstTransferOwnership(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "serialize-owner")
	newOwner := createTestUser(ctx, t, userRepo, "serialize-new-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Serialize Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: newOwner.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember(newOwner) failed: %v", err)
	}

	var wg sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})

	var transferErr, updateRoleErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		transferErr = roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, newOwner.ID)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		updateRoleErr = roomRepo.UpdateMemberRole(ctx, rm.ID, newOwner.ID, domainroom.RoleReader)
	}()

	ready.Wait()
	close(start)
	wg.Wait()

	if transferErr != nil {
		t.Fatalf("TransferOwnership failed: %v", transferErr)
	}
	if updateRoleErr != nil && !errors.Is(updateRoleErr, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected UpdateMemberRole to either succeed or fail with ErrOwnerRoleProtected, got %v", updateRoleErr)
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
	// The invariant under test: whoever ends up as rooms.owner_id must have
	// room_members.role = master -- never "owner" with a downgraded role.
	if newOwnerMember.Role != domainroom.RoleMaster {
		t.Fatalf("expected the room's owner to have role master, got %s (owner-with-non-master-role)", newOwnerMember.Role)
	}
}

// TestRemoveMemberOwnerProtected proves RoomRepository.RemoveMember rejects
// removing the room's current owner with domainroom.ErrOwnerRoleProtected,
// and leaves the owner's membership row intact, mirroring
// TestUpdateMemberRolePersists's owner-protection sibling test for
// UpdateMemberRole.
func TestRemoveMemberOwnerProtected(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "remove-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Remove Owner Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err := roomRepo.RemoveMember(ctx, rm.ID, owner.ID)
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected domainroom.ErrOwnerRoleProtected, got %v", err)
	}

	if _, err := roomRepo.GetMember(ctx, rm.ID, owner.ID); err != nil {
		t.Fatalf("expected the owner's membership to remain after a rejected removal, GetMember failed: %v", err)
	}
}

// TestRemoveMemberSerializedAgainstTransferOwnership proves a concurrent
// TransferOwnership(owner -> targetUser) and RemoveMember(targetUser) race
// on the same room resolves to one of exactly two consistent outcomes,
// mirroring TestUpdateMemberRoleSerializedAgainstTransferOwnership's
// pattern for the analogous UpdateMemberRole/TransferOwnership race:
//   - TransferOwnership wins the row lock first: targetUser becomes the new
//     owner, and RemoveMember must then fail with
//     domainroom.ErrOwnerRoleProtected (never silently remove the room's
//     new owner).
//   - RemoveMember wins the row lock first: targetUser (still a plain
//     member at that point) is removed, and TransferOwnership must then
//     fail with domain.ErrNotFound, since its "new owner is already a
//     member" UPDATE affects zero rows (never leave rooms.owner_id pointing
//     at a user with no room_members row).
//
// Whichever branch occurs, the room must never end up in a state where
// rooms.owner_id references a user with no corresponding room_members row.
func TestRemoveMemberSerializedAgainstTransferOwnership(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "remove-serialize-owner")
	target := createTestUser(ctx, t, userRepo, "remove-serialize-target")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Remove Serialize Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: target.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember(target) failed: %v", err)
	}

	var wg sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})

	var transferErr, removeErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		transferErr = roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, target.ID)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		removeErr = roomRepo.RemoveMember(ctx, rm.ID, target.ID)
	}()

	ready.Wait()
	close(start)
	wg.Wait()

	updatedRoom, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	switch {
	case transferErr == nil && errors.Is(removeErr, domainroom.ErrOwnerRoleProtected):
		if updatedRoom.OwnerID != target.ID {
			t.Fatalf("transfer succeeded but owner_id is %s, expected %s", updatedRoom.OwnerID, target.ID)
		}
		if _, err := roomRepo.GetMember(ctx, rm.ID, target.ID); err != nil {
			t.Fatalf("expected the new owner's membership to remain, GetMember failed: %v", err)
		}
	case removeErr == nil && errors.Is(transferErr, domain.ErrNotFound):
		if updatedRoom.OwnerID != owner.ID {
			t.Fatalf("removal won but owner_id changed to %s, expected it to remain %s", updatedRoom.OwnerID, owner.ID)
		}
		if _, err := roomRepo.GetMember(ctx, rm.ID, target.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected target's membership to be removed, GetMember returned %v", err)
		}
	default:
		t.Fatalf("expected exactly one of the two consistent outcomes, got transferErr=%v removeErr=%v", transferErr, removeErr)
	}
}
