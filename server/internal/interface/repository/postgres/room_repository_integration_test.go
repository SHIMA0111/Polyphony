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
	// Step 42's review fix: GetMember must JOIN against users and populate
	// Username, exactly as ListMembers does, so a caller returning
	// GetMember's result directly (e.g. RoomUsecase.ChangeMemberRole) never
	// leaks an empty username to the API response.
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

// TestUpdateMemberRoleRejectsCurrentOwner proves that
// RoomRepository.UpdateMemberRole's row-locked owner recheck (see its
// GoDoc) rejects a role change for whoever rooms.owner_id currently points
// at, even when that ownership only became true after this test's own
// setup ran a completed TransferOwnership -- i.e. it rechecks at call time
// rather than trusting a caller's earlier, now-stale ownership snapshot.
// TestConcurrentTransferOwnershipAndUpdateMemberRoleSerializes below proves
// the same invariant holds when the two calls genuinely race.
func TestUpdateMemberRoleRejectsCurrentOwner(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "recheck-owner")
	target := createTestUser(ctx, t, userRepo, "recheck-target")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Recheck Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: target.ID, Role: domainroom.RoleAdmin, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, target.ID); err != nil {
		t.Fatalf("TransferOwnership failed: %v", err)
	}

	err := roomRepo.UpdateMemberRole(ctx, rm.ID, target.ID, domainroom.RoleReader)
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected domainroom.ErrOwnerRoleProtected for the current owner, got %v", err)
	}

	got, err := roomRepo.GetMember(ctx, rm.ID, target.ID)
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if got.Role != domainroom.RoleMaster {
		t.Fatalf("expected target's role to remain master after the rejected UpdateMemberRole, got %s", got.Role)
	}
}

// TestConcurrentTransferOwnershipAndUpdateMemberRoleSerializes proves that
// RoomRepository.TransferOwnership and RoomRepository.UpdateMemberRole,
// racing to act on the same target member of the same room (one promoting
// them to owner, the other trying to change their role to something else),
// never leave the room observably inconsistent -- i.e. never with zero or
// two RoleMaster members, and never with a RoleMaster member who isn't
// rooms.owner_id. Both methods lock the rooms row (TransferOwnership via
// its CAS UPDATE, UpdateMemberRole via its `SELECT ... FOR UPDATE`), which
// should serialize the two calls: whichever wins the lock commits (or, for
// UpdateMemberRole, is rejected under the lock) fully before the other
// proceeds. It repeats the race across many fresh rooms because Postgres's
// lock-acquisition order across two concurrently-started transactions is
// not deterministic -- a single iteration could pass by only ever
// exercising one of the two possible interleavings.
func TestConcurrentTransferOwnershipAndUpdateMemberRoleSerializes(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	const iterations = 20
	for i := 0; i < iterations; i++ {
		owner := createTestUser(ctx, t, userRepo, "race-owner")
		target := createTestUser(ctx, t, userRepo, "race-target")

		rm := &domainroom.Room{
			ID: uuid.New().String(), Name: "Race Room", OwnerID: owner.ID,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := roomRepo.Create(ctx, rm); err != nil {
			t.Fatalf("iteration %d: Create failed: %v", i, err)
		}
		if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
			ID: uuid.New().String(), RoomID: rm.ID, UserID: target.ID, Role: domainroom.RoleAdmin, JoinedAt: time.Now(),
		}); err != nil {
			t.Fatalf("iteration %d: AddMember failed: %v", i, err)
		}

		var wg sync.WaitGroup
		var transferErr, roleErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			transferErr = roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, target.ID)
		}()
		go func() {
			defer wg.Done()
			roleErr = roomRepo.UpdateMemberRole(ctx, rm.ID, target.ID, domainroom.RoleReader)
		}()
		wg.Wait()

		if transferErr != nil {
			t.Fatalf("iteration %d: TransferOwnership returned unexpected error: %v", i, transferErr)
		}
		// UpdateMemberRole must either succeed (it committed before
		// TransferOwnership locked the rooms row, i.e. target wasn't yet
		// owner) or be rejected with ErrOwnerRoleProtected (it observed
		// target already promoted to owner under the lock); any other
		// outcome means the recheck failed to close the race.
		if roleErr != nil && !errors.Is(roleErr, domainroom.ErrOwnerRoleProtected) {
			t.Fatalf("iteration %d: UpdateMemberRole returned unexpected error: %v", i, roleErr)
		}

		finalRoom, err := roomRepo.GetByID(ctx, rm.ID)
		if err != nil {
			t.Fatalf("iteration %d: GetByID failed: %v", i, err)
		}
		members, err := roomRepo.ListMembers(ctx, rm.ID)
		if err != nil {
			t.Fatalf("iteration %d: ListMembers failed: %v", i, err)
		}
		masters := 0
		for _, m := range members {
			if m.Role != domainroom.RoleMaster {
				continue
			}
			masters++
			if m.UserID != finalRoom.OwnerID {
				t.Fatalf("iteration %d: member %s holds RoleMaster but is not rooms.owner_id (%s)", i, m.UserID, finalRoom.OwnerID)
			}
		}
		if masters != 1 {
			t.Fatalf("iteration %d: expected exactly one master, found %d", i, masters)
		}
	}
}

// TestRemoveMemberRejectsCurrentOwner is RemoveMember's counterpart to
// TestUpdateMemberRoleRejectsCurrentOwner: it proves the same lock-and-
// recheck (`SELECT owner_id FROM rooms WHERE id = $1 FOR UPDATE`) rejects a
// RemoveMember call against whoever the room's *current* owner is, even
// when that owner only became owner via a TransferOwnership that landed
// after a caller's own stale pre-check.
func TestRemoveMemberRejectsCurrentOwner(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "remove-recheck-owner")
	target := createTestUser(ctx, t, userRepo, "remove-recheck-target")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Remove Recheck Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: uuid.New().String(), RoomID: rm.ID, UserID: target.ID, Role: domainroom.RoleAdmin, JoinedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, target.ID); err != nil {
		t.Fatalf("TransferOwnership failed: %v", err)
	}

	err := roomRepo.RemoveMember(ctx, rm.ID, target.ID)
	if !errors.Is(err, domainroom.ErrOwnerRoleProtected) {
		t.Fatalf("expected domainroom.ErrOwnerRoleProtected for the current owner, got %v", err)
	}

	got, err := roomRepo.GetMember(ctx, rm.ID, target.ID)
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if got.Role != domainroom.RoleMaster {
		t.Fatalf("expected target's membership to remain intact after the rejected RemoveMember, got role %s", got.Role)
	}
}

// TestConcurrentTransferOwnershipAndRemoveMemberSerializes is
// TestConcurrentTransferOwnershipAndUpdateMemberRoleSerializes's counterpart
// for RemoveMember: it races RoomRepository.TransferOwnership (promoting
// target to owner) against RoomRepository.RemoveMember (trying to remove
// target) for the same target member of the same room. Unlike
// UpdateMemberRole (which only ever mutates the member row's role),
// RemoveMember can delete the row outright, so both methods locking the
// rooms row admits two distinct legitimate outcomes rather than one:
//
//   - TransferOwnership commits first (fully, under the rooms row lock):
//     target is promoted to master, and RemoveMember -- unblocked only
//     after that commit -- observes target is now the owner and is
//     rejected with domainroom.ErrOwnerRoleProtected. Target remains a
//     member with exactly one master matching rooms.owner_id.
//   - RemoveMember commits first (target wasn't yet owner when it read
//     rooms.owner_id under the lock): target's room_members row is deleted
//     entirely, so TransferOwnership's own membership-role update -- which
//     runs after RemoveMember's commit releases the rooms row lock --
//     affects zero rows and TransferOwnership rolls back with
//     domain.ErrNotFound, leaving rooms.owner_id unchanged at the original
//     owner and target no longer a member at all.
//
// Any other final state (e.g. zero masters, two masters, or a master not
// matching rooms.owner_id) means the recheck failed to close the race. It
// repeats the race across many fresh rooms for the same non-determinism
// reason TestConcurrentTransferOwnershipAndUpdateMemberRoleSerializes does.
func TestConcurrentTransferOwnershipAndRemoveMemberSerializes(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	const iterations = 20
	for i := 0; i < iterations; i++ {
		owner := createTestUser(ctx, t, userRepo, "remove-race-owner")
		target := createTestUser(ctx, t, userRepo, "remove-race-target")

		rm := &domainroom.Room{
			ID: uuid.New().String(), Name: "Remove Race Room", OwnerID: owner.ID,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := roomRepo.Create(ctx, rm); err != nil {
			t.Fatalf("iteration %d: Create failed: %v", i, err)
		}
		if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
			ID: uuid.New().String(), RoomID: rm.ID, UserID: target.ID, Role: domainroom.RoleAdmin, JoinedAt: time.Now(),
		}); err != nil {
			t.Fatalf("iteration %d: AddMember failed: %v", i, err)
		}

		var wg sync.WaitGroup
		var transferErr, removeErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			transferErr = roomRepo.TransferOwnership(ctx, rm.ID, owner.ID, target.ID)
		}()
		go func() {
			defer wg.Done()
			removeErr = roomRepo.RemoveMember(ctx, rm.ID, target.ID)
		}()
		wg.Wait()

		switch {
		case transferErr == nil && removeErr != nil:
			// TransferOwnership won: RemoveMember must have been rejected
			// specifically because target is now the owner, never for any
			// other reason.
			if !errors.Is(removeErr, domainroom.ErrOwnerRoleProtected) {
				t.Fatalf("iteration %d: TransferOwnership succeeded but RemoveMember returned unexpected error: %v", i, removeErr)
			}
			finalRoom, err := roomRepo.GetByID(ctx, rm.ID)
			if err != nil {
				t.Fatalf("iteration %d: GetByID failed: %v", i, err)
			}
			if finalRoom.OwnerID != target.ID {
				t.Fatalf("iteration %d: expected target to be owner after TransferOwnership won, got owner %s", i, finalRoom.OwnerID)
			}
			got, err := roomRepo.GetMember(ctx, rm.ID, target.ID)
			if err != nil {
				t.Fatalf("iteration %d: GetMember failed: %v", i, err)
			}
			if got.Role != domainroom.RoleMaster {
				t.Fatalf("iteration %d: expected target's role master, got %s", i, got.Role)
			}
		case transferErr != nil && removeErr == nil:
			// RemoveMember won: TransferOwnership must have failed
			// specifically because its membership-role update found target
			// already removed, never for any other reason, and the
			// original owner must remain owner unchanged.
			if !errors.Is(transferErr, domain.ErrNotFound) {
				t.Fatalf("iteration %d: RemoveMember succeeded but TransferOwnership returned unexpected error: %v", i, transferErr)
			}
			finalRoom, err := roomRepo.GetByID(ctx, rm.ID)
			if err != nil {
				t.Fatalf("iteration %d: GetByID failed: %v", i, err)
			}
			if finalRoom.OwnerID != owner.ID {
				t.Fatalf("iteration %d: expected owner unchanged after TransferOwnership rolled back, got %s", i, finalRoom.OwnerID)
			}
			if _, err := roomRepo.GetMember(ctx, rm.ID, target.ID); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("iteration %d: expected target to have been removed, GetMember returned %v", i, err)
			}
		default:
			t.Fatalf("iteration %d: expected exactly one of TransferOwnership/RemoveMember to succeed, got transferErr=%v removeErr=%v", i, transferErr, removeErr)
		}

		// Regardless of which side won, the room must always end up with
		// exactly one RoleMaster member matching rooms.owner_id -- never
		// zero, never two.
		finalRoom, err := roomRepo.GetByID(ctx, rm.ID)
		if err != nil {
			t.Fatalf("iteration %d: GetByID failed: %v", i, err)
		}
		members, err := roomRepo.ListMembers(ctx, rm.ID)
		if err != nil {
			t.Fatalf("iteration %d: ListMembers failed: %v", i, err)
		}
		masters := 0
		for _, m := range members {
			if m.Role != domainroom.RoleMaster {
				continue
			}
			masters++
			if m.UserID != finalRoom.OwnerID {
				t.Fatalf("iteration %d: member %s holds RoleMaster but is not rooms.owner_id (%s)", i, m.UserID, finalRoom.OwnerID)
			}
		}
		if masters != 1 {
			t.Fatalf("iteration %d: expected exactly one master, found %d", i, masters)
		}
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

// TestTransferOwnershipRejectsStaleOwner (Step 27's review fix) proves that
// TransferOwnership's rooms.owner_id update is a genuine compare-and-swap:
// calling it a second time with an oldOwnerID that is no longer the room's
// current owner (because a first, successful transfer already moved
// ownership elsewhere) must fail with domain.ErrNotFound rather than
// silently overwriting owner_id again — the exact race two concurrent
// TransferOwnership calls, each reading a stale owner via their own
// pre-transaction GetByID, could otherwise hit.
func TestTransferOwnershipRejectsStaleOwner(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	original := createTestUser(ctx, t, userRepo, "stale-cas-original")
	first := createTestUser(ctx, t, userRepo, "stale-cas-first")
	second := createTestUser(ctx, t, userRepo, "stale-cas-second")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Stale Owner CAS Room", OwnerID: original.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	for _, u := range []*domainuser.User{first, second} {
		if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
			ID: uuid.New().String(), RoomID: rm.ID, UserID: u.ID, Role: domainroom.RoleMember, JoinedAt: time.Now(),
		}); err != nil {
			t.Fatalf("AddMember(%s) failed: %v", u.Username, err)
		}
	}

	// This transfer succeeds: original really is the current owner.
	if err := roomRepo.TransferOwnership(ctx, rm.ID, original.ID, first.ID); err != nil {
		t.Fatalf("first TransferOwnership failed: %v", err)
	}

	// Simulates the loser of a concurrent transfer race: it still believes
	// original is the current owner (a stale read from before the first
	// transfer committed), so its CAS must fail.
	err := roomRepo.TransferOwnership(ctx, rm.ID, original.ID, second.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for a stale-owner CAS, got %v", err)
	}

	// The first (successful) transfer's effects must be untouched by the
	// second (rejected) attempt.
	got, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.OwnerID != first.ID {
		t.Fatalf("expected owner_id to remain %s after the rejected stale CAS, got %s", first.ID, got.OwnerID)
	}
	secondMember, err := roomRepo.GetMember(ctx, rm.ID, second.ID)
	if err != nil {
		t.Fatalf("GetMember(second) failed: %v", err)
	}
	if secondMember.Role != domainroom.RoleMember {
		t.Fatalf("expected second's role to remain member (untouched), got %s", secondMember.Role)
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
// GetByID and UpdateAISettings (Step 26's narrow replacement for the old
// full-row Update): a freshly created room has both as nil (NULL), and
// after UpdateAISettings sets them to non-nil values, GetByID, ListByUserID,
// and ListByUserIDWithRole all observe the same values.
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
	if err := roomRepo.UpdateAISettings(ctx, rm.ID, true, &provider, true, &model); err != nil {
		t.Fatalf("UpdateAISettings failed: %v", err)
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
	if err := roomRepo.UpdateAISettings(ctx, rm.ID, true, nil, true, nil); err != nil {
		t.Fatalf("UpdateAISettings (clear) failed: %v", err)
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

// TestRoomRepositorySetArchived proves that SetArchived flips is_archived
// via its single dedicated UPDATE (leaving every other column untouched)
// and returns domain.ErrNotFound for a nonexistent room, without ever
// touching forked_from_room_id.
func TestRoomRepositorySetArchived(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "set-archived-owner@example.com",
		Username:     "set-archived-owner",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        "Archivable Room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := roomRepo.SetArchived(ctx, rm.ID, true); err != nil {
		t.Fatalf("SetArchived(true) failed: %v", err)
	}
	got, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !got.IsArchived {
		t.Fatal("expected is_archived true after SetArchived(true)")
	}
	if got.Name != rm.Name {
		t.Fatalf("expected SetArchived to leave name untouched, got %q", got.Name)
	}

	if err := roomRepo.SetArchived(ctx, rm.ID, false); err != nil {
		t.Fatalf("SetArchived(false) failed: %v", err)
	}
	got, err = roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.IsArchived {
		t.Fatal("expected is_archived false after SetArchived(false)")
	}

	if err := roomRepo.SetArchived(ctx, uuid.New().String(), true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a nonexistent room, got %v", err)
	}
}

// forkedFromRoomIDOf locates roomID within rooms and returns its
// ForkedFromRoomID, or nil with found == false if roomID is absent from the
// slice.
func forkedFromRoomIDOf(rooms []*domainroom.Room, roomID string) (id *string, found bool) {
	for _, rm := range rooms {
		if rm.ID == roomID {
			return rm.ForkedFromRoomID, true
		}
	}
	return nil, false
}

// TestRoomRepositoryForkedFromRoomIDRoundTrip proves that
// forked_from_room_id is persisted at Create time, read back by GetByID/
// ListByUserID/ListByUserIDWithRole, and left untouched by Update (it is
// write-once).
func TestRoomRepositoryForkedFromRoomIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "fork-link-owner@example.com",
		Username:     "fork-link-owner",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	source := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        "Source Room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, source); err != nil {
		t.Fatalf("create source room: %v", err)
	}

	fork := &domainroom.Room{
		ID:               uuid.New().String(),
		Name:             "Source Room (Fork)",
		Description:      "",
		OwnerID:          owner.ID,
		ForkedFromRoomID: &source.ID,
		IsArchived:       true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := roomRepo.Create(ctx, fork); err != nil {
		t.Fatalf("create fork room: %v", err)
	}

	got, err := roomRepo.GetByID(ctx, fork.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ForkedFromRoomID == nil || *got.ForkedFromRoomID != source.ID {
		t.Fatalf("expected forked_from_room_id %s, got %v", source.ID, got.ForkedFromRoomID)
	}
	if !got.IsArchived {
		t.Fatal("expected is_archived true")
	}

	// ListByUserID and ListByUserIDWithRole must each surface the same
	// forked_from_room_id GetByID just proved — the doc comment above
	// claims coverage of all three read paths, so all three must actually
	// be exercised here.
	listed, err := roomRepo.ListByUserID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	listedForkedFrom, found := forkedFromRoomIDOf(listed, fork.ID)
	if !found {
		t.Fatalf("expected ListByUserID to include fork room %s", fork.ID)
	}
	if listedForkedFrom == nil || *listedForkedFrom != source.ID {
		t.Fatalf("expected ListByUserID's forked_from_room_id %s, got %v", source.ID, listedForkedFrom)
	}

	listedWithRole, err := roomRepo.ListByUserIDWithRole(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByUserIDWithRole failed: %v", err)
	}
	var listedWithRoleRooms []*domainroom.Room
	for _, rwr := range listedWithRole {
		listedWithRoleRooms = append(listedWithRoleRooms, rwr.Room)
	}
	listedWithRoleForkedFrom, found := forkedFromRoomIDOf(listedWithRoleRooms, fork.ID)
	if !found {
		t.Fatalf("expected ListByUserIDWithRole to include fork room %s", fork.ID)
	}
	if listedWithRoleForkedFrom == nil || *listedWithRoleForkedFrom != source.ID {
		t.Fatalf("expected ListByUserIDWithRole's forked_from_room_id %s, got %v", source.ID, listedWithRoleForkedFrom)
	}

	// UpdateDetails never touches forked_from_room_id (it only ever issues
	// an UPDATE against name/description/updated_at), even if the in-memory
	// struct's field were (incorrectly) cleared before calling it.
	got.Name = "Renamed Fork"
	got.ForkedFromRoomID = nil
	if err := roomRepo.UpdateDetails(ctx, fork.ID, got.Name, got.Description); err != nil {
		t.Fatalf("UpdateDetails failed: %v", err)
	}
	afterUpdate, err := roomRepo.GetByID(ctx, fork.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if afterUpdate.ForkedFromRoomID == nil || *afterUpdate.ForkedFromRoomID != source.ID {
		t.Fatalf("expected forked_from_room_id to remain %s after UpdateDetails, got %v", source.ID, afterUpdate.ForkedFromRoomID)
	}
	if afterUpdate.Name != "Renamed Fork" {
		t.Fatalf("expected name to be updated to %q, got %q", "Renamed Fork", afterUpdate.Name)
	}
}

// TestRoomRepositoryUpdateDetailsAIContextCutoffAndAISettingsDoNotClobber
// (Step 26's regression test) proves the three narrow setters that replaced
// the old full-row Update are each scoped to their own columns: calling
// UpdateAIContextCutoff after UpdateDetails must not revert the name/
// description UpdateDetails just set, and calling UpdateAISettings after
// both must not revert either of the earlier writes -- closing the lost-
// update race the old Update(ctx, *Room) shape risked between
// RoomUsecase.UpdateRoom / UpdateAIContextCutoff / UpdateSettings.
func TestRoomRepositoryUpdateDetailsAIContextCutoffAndAISettingsDoNotClobber(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "narrow-update-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "Original Name", Description: "original desc",
		OwnerID: owner.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := roomRepo.UpdateDetails(ctx, rm.ID, "New Name", "new desc"); err != nil {
		t.Fatalf("UpdateDetails failed: %v", err)
	}

	cutoff := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if err := roomRepo.UpdateAIContextCutoff(ctx, rm.ID, &cutoff); err != nil {
		t.Fatalf("UpdateAIContextCutoff failed: %v", err)
	}

	provider := "openai"
	model := "gpt-5.2"
	if err := roomRepo.UpdateAISettings(ctx, rm.ID, true, &provider, true, &model); err != nil {
		t.Fatalf("UpdateAISettings failed: %v", err)
	}

	got, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Name != "New Name" || got.Description != "new desc" {
		t.Fatalf("expected UpdateDetails's write to survive, got name=%q description=%q", got.Name, got.Description)
	}
	if got.AIContextCutoffAt == nil || !got.AIContextCutoffAt.Equal(cutoff) {
		t.Fatalf("expected UpdateAIContextCutoff's write to survive, got %v", got.AIContextCutoffAt)
	}
	if got.AIProvider == nil || *got.AIProvider != provider || got.AIModel == nil || *got.AIModel != model {
		t.Fatalf("expected UpdateAISettings's write to survive, got provider=%v model=%v", got.AIProvider, got.AIModel)
	}
}

// TestConcurrentUpdateAISettingsPartialFieldsNoLostUpdate is a concurrency
// regression test for UpdateAISettings's CASE-WHEN-gated single UPDATE
// (mirroring TestConcurrentUpdateRoomAndAIContextCutoffNoLostUpdate's
// mock-level counterpart in usecase/room, but exercised here against real
// Postgres): two concurrent calls, each setting only one of AIProvider/
// AIModel and leaving the other field's flag false, must both land. Before
// this UPDATE was made CASE-WHEN-gated, UpdateAISettings always wrote both
// columns from whatever values its caller passed; a caller resolving "leave
// unchanged" by reading the room first and echoing back its current value
// (the old usecase-layer pattern) could have its stale read of the
// sibling's column overwrite that column's concurrently-written value. The
// CASE WHEN UPDATE below closes that race structurally: a false flag never
// touches its column at the database level, regardless of interleaving, so
// there is nothing left to race.
func TestConcurrentUpdateAISettingsPartialFieldsNoLostUpdate(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)

	owner := createTestUser(ctx, t, userRepo, "ai-settings-race-owner")

	rm := &domainroom.Room{
		ID: uuid.New().String(), Name: "AI Settings Race Room", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	provider := "anthropic"
	model := "claude-opus-4"

	var wg sync.WaitGroup
	var providerErr, modelErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		providerErr = roomRepo.UpdateAISettings(ctx, rm.ID, true, &provider, false, nil)
	}()
	go func() {
		defer wg.Done()
		modelErr = roomRepo.UpdateAISettings(ctx, rm.ID, false, nil, true, &model)
	}()
	wg.Wait()

	if providerErr != nil {
		t.Fatalf("UpdateAISettings (provider only) failed: %v", providerErr)
	}
	if modelErr != nil {
		t.Fatalf("UpdateAISettings (model only) failed: %v", modelErr)
	}

	got, err := roomRepo.GetByID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.AIProvider == nil || *got.AIProvider != provider {
		t.Fatalf("expected both concurrent calls' ai_provider change to land (%q), got %v", provider, got.AIProvider)
	}
	if got.AIModel == nil || *got.AIModel != model {
		t.Fatalf("expected both concurrent calls' ai_model change to land (%q), got %v", model, got.AIModel)
	}
}
