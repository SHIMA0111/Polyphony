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
// MarkRunning/UpdateProgress/CompleteAndUnarchive, and that MarkRunning and
// UpdateProgress each reject a job that isn't currently in the source
// status their guard requires (StatusPending and StatusRunning,
// respectively) without mutating it.
func TestRoomForkRepository_StateTransitions(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	forkRepo := NewRoomForkRepository(pool)

	source, dest := seedForkRoomPair(ctx, t, userRepo, roomRepo, "fork-repo-lifecycle")
	if err := roomRepo.SetArchived(ctx, dest.ID, true); err != nil {
		t.Fatalf("archive dest room: %v", err)
	}

	now := time.Now()
	job := &roomfork.Job{
		ID: uuid.New().String(), SourceRoomID: source.ID, NewRoomID: dest.ID,
		Status: roomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// UpdateProgress before MarkRunning: the job is still StatusPending, not
	// StatusRunning, so the guarded UPDATE must affect zero rows.
	if err := forkRepo.UpdateProgress(ctx, job.ID, 10); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for UpdateProgress on a still-pending job, got %v", err)
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

	// MarkRunning again: the job is now StatusRunning, not StatusPending, so
	// the guarded UPDATE must affect zero rows and leave it untouched.
	if err := forkRepo.MarkRunning(ctx, job.ID, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for MarkRunning on an already-running job, got %v", err)
	}
	got, err = forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.TotalMessages != 42 {
		t.Fatalf("expected total_messages to remain 42 after a rejected re-MarkRunning, got %d", got.TotalMessages)
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

	if err := forkRepo.CompleteAndUnarchive(ctx, job.ID, dest.ID); err != nil {
		t.Fatalf("CompleteAndUnarchive failed: %v", err)
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
// StatusFailed and persists the error message, and that a second MarkFailed
// call against the now-terminal job is rejected without overwriting the
// already-recorded error message.
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

	// The job is now StatusFailed, a terminal state — a second MarkFailed
	// must be rejected and must not overwrite the recorded error message.
	if err := forkRepo.MarkFailed(ctx, job.ID, "second boom"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for MarkFailed on an already-terminal job, got %v", err)
	}
	got, err = forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "boom" {
		t.Fatalf("expected error_message to remain 'boom' after a rejected re-MarkFailed, got %v", got.ErrorMessage)
	}

	if err := forkRepo.MarkRunning(ctx, uuid.New().String(), 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a nonexistent job, got %v", err)
	}
}

// TestRoomForkRepository_CompleteAndUnarchive proves CompleteAndUnarchive
// performs the running->completed job transition together with clearing
// the room's is_archived flag on success, and that both a mismatched
// newRoomID and a non-running job status are rejected with
// domain.ErrNotFound without mutating either row — the tightened WHERE
// clause on the job update in room_fork_repository.go.
func TestRoomForkRepository_CompleteAndUnarchive(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	forkRepo := NewRoomForkRepository(pool)

	source, dest := seedForkRoomPair(ctx, t, userRepo, roomRepo, "fork-repo-complete")
	if err := roomRepo.SetArchived(ctx, dest.ID, true); err != nil {
		t.Fatalf("archive dest room: %v", err)
	}
	if err := roomRepo.SetArchived(ctx, source.ID, true); err != nil {
		t.Fatalf("archive source room: %v", err)
	}

	now := time.Now()
	job := &roomfork.Job{
		ID: uuid.New().String(), SourceRoomID: source.ID, NewRoomID: dest.ID,
		Status: roomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := forkRepo.MarkRunning(ctx, job.ID, 5); err != nil {
		t.Fatalf("MarkRunning failed: %v", err)
	}

	// Wrong newRoomID: source.ID is a genuine room, just not this job's
	// NewRoomID — must fail without completing the job or unarchiving
	// source (which the pre-fix `WHERE id = $2`-only job update would not
	// have caught, since it never checked new_room_id at all).
	if err := forkRepo.CompleteAndUnarchive(ctx, job.ID, source.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a mismatched newRoomID, got %v", err)
	}
	gotJob, err := forkRepo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if gotJob.Status != roomfork.StatusRunning {
		t.Fatalf("expected job to remain running after a mismatched newRoomID, got %s", gotJob.Status)
	}
	gotSource, err := roomRepo.GetByID(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetByID source failed: %v", err)
	}
	if !gotSource.IsArchived {
		t.Fatal("expected source room to remain archived after a mismatched newRoomID")
	}

	// Non-running status: fail the job via a different terminal transition,
	// then retry CompleteAndUnarchive with its own (correct) newRoomID —
	// must still fail, since the job is no longer running, without
	// unarchiving dest.
	if err := forkRepo.MarkFailed(ctx, job.ID, "simulated failure"); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}
	if err := forkRepo.CompleteAndUnarchive(ctx, job.ID, dest.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a non-running job status, got %v", err)
	}
	gotDest, err := roomRepo.GetByID(ctx, dest.ID)
	if err != nil {
		t.Fatalf("GetByID dest failed: %v", err)
	}
	if !gotDest.IsArchived {
		t.Fatal("expected dest room to remain archived while the job is not running")
	}

	// Success path, on a fresh running job/dest pair (the job above is now
	// StatusFailed, a terminal state).
	source2, dest2 := seedForkRoomPair(ctx, t, userRepo, roomRepo, "fork-repo-complete-2")
	if err := roomRepo.SetArchived(ctx, dest2.ID, true); err != nil {
		t.Fatalf("archive dest2 room: %v", err)
	}
	job2 := &roomfork.Job{
		ID: uuid.New().String(), SourceRoomID: source2.ID, NewRoomID: dest2.ID,
		Status: roomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkRepo.Create(ctx, job2); err != nil {
		t.Fatalf("Create job2 failed: %v", err)
	}
	if err := forkRepo.MarkRunning(ctx, job2.ID, 5); err != nil {
		t.Fatalf("MarkRunning job2 failed: %v", err)
	}
	if err := forkRepo.CompleteAndUnarchive(ctx, job2.ID, dest2.ID); err != nil {
		t.Fatalf("CompleteAndUnarchive failed: %v", err)
	}
	gotJob2, err := forkRepo.GetByID(ctx, job2.ID)
	if err != nil {
		t.Fatalf("GetByID job2 failed: %v", err)
	}
	if gotJob2.Status != roomfork.StatusCompleted {
		t.Fatalf("expected job2 status completed, got %s", gotJob2.Status)
	}
	gotDest2, err := roomRepo.GetByID(ctx, dest2.ID)
	if err != nil {
		t.Fatalf("GetByID dest2 failed: %v", err)
	}
	if gotDest2.IsArchived {
		t.Fatal("expected dest2 room to be unarchived after CompleteAndUnarchive")
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
