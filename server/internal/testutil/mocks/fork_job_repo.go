package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
)

// ForkJobRepo is an in-memory, map-backed fake implementing
// roomfork.ForkJobRepository. The zero value (mocks.ForkJobRepo{}) is ready
// to use; its map is initialized lazily on first write.
//
// ForkJobRepo is safe for concurrent use.
type ForkJobRepo struct {
	mu   sync.Mutex
	Jobs map[string]*roomfork.Job

	// Rooms, if set, is the RoomRepo fake CompleteAndUnarchive unarchives
	// alongside marking a Job completed, modeling
	// postgres.RoomForkRepository.CompleteAndUnarchive's single-transaction
	// behavior against a real rooms table for tests that observe the
	// destination room's IsArchived flag after a fork job completes. Nil is
	// a valid zero value for tests that never look at the room side of that
	// transition.
	Rooms *RoomRepo
}

func (f *ForkJobRepo) ensureInit() {
	if f.Jobs == nil {
		f.Jobs = make(map[string]*roomfork.Job)
	}
}

// cloneForkJob returns a shallow copy of job, deep-copying the *string
// ErrorMessage field so the copy shares no mutable state with job. Create
// and GetByID both use this rather than storing/returning the caller's own
// *roomfork.Job pointer directly: without it, a caller holding onto a Job
// it passed to Create (or received from GetByID) would race with this
// fake's own background-goroutine-driven mutations (MarkRunning,
// UpdateProgress, MarkCompleted, MarkFailed, CompleteAndUnarchive all write
// through the map entry under mu) on the very same struct, with no lock
// protecting the caller's read.
func cloneForkJob(job *roomfork.Job) *roomfork.Job {
	cp := *job
	if job.ErrorMessage != nil {
		errMsg := *job.ErrorMessage
		cp.ErrorMessage = &errMsg
	}
	return &cp
}

// Create persists a shallow copy of job (see cloneForkJob), so the caller's
// own *roomfork.Job is never mutated by this fake's later state
// transitions.
func (f *ForkJobRepo) Create(_ context.Context, job *roomfork.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureInit()

	f.Jobs[job.ID] = cloneForkJob(job)
	return nil
}

// GetByID retrieves a shallow copy of the stored Job (see cloneForkJob), so
// the caller can read it without racing this fake's later, lock-protected
// mutations of its own internal state. Returns domain.ErrNotFound if not
// present.
func (f *ForkJobRepo) GetByID(_ context.Context, id string) (*roomfork.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneForkJob(job), nil
}

// MarkRunning transitions a Job to StatusRunning and records
// totalMessages. Returns domain.ErrNotFound if the job does not exist.
func (f *ForkJobRepo) MarkRunning(_ context.Context, id string, totalMessages int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusRunning
	job.TotalMessages = totalMessages
	job.UpdatedAt = time.Now()
	return nil
}

// UpdateProgress sets CopiedMessages on a Job. Returns domain.ErrNotFound
// if the job does not exist.
func (f *ForkJobRepo) UpdateProgress(_ context.Context, id string, copiedMessages int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return domain.ErrNotFound
	}
	job.CopiedMessages = copiedMessages
	job.UpdatedAt = time.Now()
	return nil
}

// MarkCompleted transitions a Job to the terminal StatusCompleted state.
// Returns domain.ErrNotFound if the job does not exist.
func (f *ForkJobRepo) MarkCompleted(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusCompleted
	job.UpdatedAt = time.Now()
	return nil
}

// MarkFailed transitions a Job to the terminal StatusFailed state and
// records errMsg. Returns domain.ErrNotFound if the job does not exist.
func (f *ForkJobRepo) MarkFailed(_ context.Context, id string, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusFailed
	job.ErrorMessage = &errMsg
	job.UpdatedAt = time.Now()
	return nil
}

// CompleteAndUnarchive transitions a Job to the terminal StatusCompleted
// state and, if Rooms is set, clears newRoomID's IsArchived flag —
// modeling postgres.RoomForkRepository.CompleteAndUnarchive's single-
// transaction semantics for tests, including its validation-first ordering:
// id, newRoomID, and StatusRunning must all match before anything is
// mutated (mirroring the real repository's narrowed
// "id AND new_room_id AND status = 'running'" UPDATE), so a mismatched
// newRoomID or a job that isn't currently running leaves both the job and
// the room untouched. Returns domain.ErrNotFound if the job does not exist
// or fails that validation.
func (f *ForkJobRepo) CompleteAndUnarchive(ctx context.Context, id, newRoomID string) error {
	f.mu.Lock()
	job, ok := f.Jobs[id]
	if !ok || job.NewRoomID != newRoomID || job.Status != roomfork.StatusRunning {
		f.mu.Unlock()
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusCompleted
	job.UpdatedAt = time.Now()
	f.mu.Unlock()

	if f.Rooms != nil {
		return f.Rooms.SetArchived(ctx, newRoomID, false)
	}
	return nil
}
