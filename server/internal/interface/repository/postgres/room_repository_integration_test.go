//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

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
