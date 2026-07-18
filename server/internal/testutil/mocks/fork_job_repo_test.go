package mocks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
)

// newPendingJob returns a fresh StatusPending *roomfork.Job for id, ready to
// be persisted via ForkJobRepo.Create by the caller.
func newPendingJob(id string) *roomfork.Job {
	now := time.Now()
	return &roomfork.Job{
		ID: id, SourceRoomID: "source-room", NewRoomID: "new-room",
		Status: roomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
}

// TestForkJobRepoMarkRunningRejectsNonPending proves MarkRunning's
// `job.Status != roomfork.StatusPending` guard — mirroring
// postgres.RoomForkRepository.MarkRunning's `AND status = 'pending'` —
// rejects a job that is not currently StatusPending, leaving it untouched.
func TestForkJobRepoMarkRunningRejectsNonPending(t *testing.T) {
	repo := &ForkJobRepo{}
	ctx := context.Background()

	job := newPendingJob("job-1")
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.MarkRunning(ctx, job.ID, 10); err != nil {
		t.Fatalf("first MarkRunning failed: %v", err)
	}

	// The job is now StatusRunning, not StatusPending.
	if err := repo.MarkRunning(ctx, job.ID, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for MarkRunning on an already-running job, got %v", err)
	}

	got, err := repo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.TotalMessages != 10 {
		t.Fatalf("expected total_messages to remain 10 after a rejected re-MarkRunning, got %d", got.TotalMessages)
	}
}

// TestForkJobRepoUpdateProgressRejectsNonRunning proves UpdateProgress's
// `job.Status != roomfork.StatusRunning` guard — mirroring
// postgres.RoomForkRepository.UpdateProgress's `AND status = 'running'` —
// rejects a job that is not currently StatusRunning (still pending here),
// leaving it untouched.
func TestForkJobRepoUpdateProgressRejectsNonRunning(t *testing.T) {
	repo := &ForkJobRepo{}
	ctx := context.Background()

	job := newPendingJob("job-1")
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.UpdateProgress(ctx, job.ID, 5); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for UpdateProgress on a still-pending job, got %v", err)
	}

	got, err := repo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.CopiedMessages != 0 {
		t.Fatalf("expected copied_messages to remain 0 after a rejected UpdateProgress, got %d", got.CopiedMessages)
	}
}

// TestForkJobRepoMarkFailedRejectsTerminal proves MarkFailed's
// `job.Status != roomfork.StatusPending && job.Status != roomfork.StatusRunning`
// guard — mirroring postgres.RoomForkRepository.MarkFailed's `AND status IN
// ('pending', 'running')` — rejects a job that has already reached a
// terminal state, without overwriting the previously-recorded error
// message.
func TestForkJobRepoMarkFailedRejectsTerminal(t *testing.T) {
	repo := &ForkJobRepo{}
	ctx := context.Background()

	job := newPendingJob("job-1")
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.MarkFailed(ctx, job.ID, "first failure"); err != nil {
		t.Fatalf("first MarkFailed failed: %v", err)
	}

	// The job is now StatusFailed, a terminal state.
	if err := repo.MarkFailed(ctx, job.ID, "second failure"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for MarkFailed on an already-terminal job, got %v", err)
	}

	got, err := repo.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "first failure" {
		t.Fatalf("expected error_message to remain 'first failure' after a rejected re-MarkFailed, got %v", got.ErrorMessage)
	}
}

// TestForkJobRepoMarkFailedAcceptsPendingAndRunning proves MarkFailed's
// guard accepts both non-terminal source states: a job can fail directly
// from StatusPending (e.g. the initial CountAndMaxSequence call errors
// before MarkRunning ever runs) as well as from StatusRunning.
func TestForkJobRepoMarkFailedAcceptsPendingAndRunning(t *testing.T) {
	repo := &ForkJobRepo{}
	ctx := context.Background()

	pendingJob := newPendingJob("job-pending")
	if err := repo.Create(ctx, pendingJob); err != nil {
		t.Fatalf("Create pendingJob failed: %v", err)
	}
	if err := repo.MarkFailed(ctx, pendingJob.ID, "boom"); err != nil {
		t.Fatalf("MarkFailed on a pending job failed: %v", err)
	}

	runningJob := newPendingJob("job-running")
	if err := repo.Create(ctx, runningJob); err != nil {
		t.Fatalf("Create runningJob failed: %v", err)
	}
	if err := repo.MarkRunning(ctx, runningJob.ID, 3); err != nil {
		t.Fatalf("MarkRunning failed: %v", err)
	}
	if err := repo.MarkFailed(ctx, runningJob.ID, "boom"); err != nil {
		t.Fatalf("MarkFailed on a running job failed: %v", err)
	}
}
