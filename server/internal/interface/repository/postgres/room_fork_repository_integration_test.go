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
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// TestRoomForkRepository_CreateAndGetByID proves Create/GetByID round-trip
// a Job's fields, including a nil ErrorMessage for a freshly created
// pending job.
func TestRoomForkRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	forkRepo := NewRoomForkRepository(pool)

	source, dest := seedForkRoomPair(ctx, t, userRepo, roomRepo, "fork-repo-crud")

	now := time.Now()
	job := &roomfork.Job{
		ID:           uuid.New().String(),
		SourceRoomID: source.ID,
		NewRoomID:    dest.ID,
		Status:       roomfork.StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := forkRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != roomfork.StatusPending {
		t.Fatalf("expected status pending, got %s", got.Status)
	}
	if got.SourceRoomID != source.ID || got.NewRoomID != dest.ID {
		t.Fatalf("expected source/new room IDs %s/%s, got %s/%s", source.ID, dest.ID, got.SourceRoomID, got.NewRoomID)
	}
	if got.ErrorMessage != nil {
		t.Fatalf("expected nil error_message, got %v", *got.ErrorMessage)
	}
	if got.TotalMessages != 0 || got.CopiedMessages != 0 {
		t.Fatalf("expected zero total/copied messages, got %d/%d", got.TotalMessages, got.CopiedMessages)
	}

	if _, err := forkRepo.GetByID(ctx, uuid.New().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a nonexistent job, got %v", err)
	}
}

// TestRoomForkRepository_StateTransitions proves the full
// pending -> running -> completed lifecycle round-trips through
// MarkRunning/UpdateProgress/MarkCompleted.
func TestRoomForkRepository_StateTransitions(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	forkRepo := NewRoomForkRepository(pool)

	source, dest := seedForkRoomPair(ctx, t, userRepo, roomRepo, "fork-repo-lifecycle")

	now := time.Now()
	job := &roomfork.Job{
		ID: uuid.New().String(), SourceRoomID: source.ID, NewRoomID: dest.ID,
		Status: roomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := forkRepo.MarkRunning(ctx, job.ID, 42); err != nil {
		t.Fatalf("MarkRunning failed: %v", err)
	}
	got, err := forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != roomfork.StatusRunning || got.TotalMessages != 42 {
		t.Fatalf("expected running/42, got %s/%d", got.Status, got.TotalMessages)
	}

	if err := forkRepo.UpdateProgress(ctx, job.ID, 10); err != nil {
		t.Fatalf("UpdateProgress failed: %v", err)
	}
	got, err = forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.CopiedMessages != 10 {
		t.Fatalf("expected copied_messages 10, got %d", got.CopiedMessages)
	}

	if err := forkRepo.MarkCompleted(ctx, job.ID); err != nil {
		t.Fatalf("MarkCompleted failed: %v", err)
	}
	got, err = forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != roomfork.StatusCompleted {
		t.Fatalf("expected status completed, got %s", got.Status)
	}
}

// TestRoomForkRepository_MarkFailed proves MarkFailed transitions a job to
// StatusFailed and persists the error message.
func TestRoomForkRepository_MarkFailed(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	forkRepo := NewRoomForkRepository(pool)

	source, dest := seedForkRoomPair(ctx, t, userRepo, roomRepo, "fork-repo-failed")

	now := time.Now()
	job := &roomfork.Job{
		ID: uuid.New().String(), SourceRoomID: source.ID, NewRoomID: dest.ID,
		Status: roomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := forkRepo.MarkFailed(ctx, job.ID, "boom"); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}
	got, err := forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != roomfork.StatusFailed {
		t.Fatalf("expected status failed, got %s", got.Status)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "boom" {
		t.Fatalf("expected error_message 'boom', got %v", got.ErrorMessage)
	}

	if err := forkRepo.MarkRunning(ctx, uuid.New().String(), 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a nonexistent job, got %v", err)
	}
}

// seedForkRoomPair creates a user and two rooms owned by that user (a
// "source" and a "new"/destination room), for tests that only need two
// room IDs to satisfy room_fork_jobs' foreign keys without exercising
// RoomUsecase.ForkRoom itself.
func seedForkRoomPair(ctx context.Context, t *testing.T, userRepo *UserRepository, roomRepo *RoomRepository, label string) (source, dest *domainroom.Room) {
	t.Helper()

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        label + "@example.com",
		Username:     label,
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	source = &domainroom.Room{
		ID: uuid.New().String(), Name: label + " source", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, source); err != nil {
		t.Fatalf("create source room: %v", err)
	}

	dest = &domainroom.Room{
		ID: uuid.New().String(), Name: label + " dest", OwnerID: owner.ID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := roomRepo.Create(ctx, dest); err != nil {
		t.Fatalf("create dest room: %v", err)
	}

	return source, dest
}
