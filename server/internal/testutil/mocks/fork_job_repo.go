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

	// Rooms, if set, is the *RoomRepo CompleteAndUnarchive applies its
	// room-side is_archived=false write to, mirroring
	// postgres.RoomForkRepository.CompleteAndUnarchive reaching into both
	// the rooms and room_fork_jobs tables from one method. Tests that never
	// assert on room archival after a fork job completes (e.g. ones that
	// only exercise membership/status-transition logic) may leave it nil,
	// in which case CompleteAndUnarchive skips the room-side write
	// entirely.
	Rooms *RoomRepo
}

func (f *ForkJobRepo) ensureInit() {
	if f.Jobs == nil {
		f.Jobs = make(map[string]*roomfork.Job)
	}
}

// cloneJob returns a shallow copy of job, additionally deep-copying the
// *string ErrorMessage points at (a plain `cp := *job` copy would still
// share the same *string, letting a caller-side or store-side mutation of
// *ErrorMessage bleed across the copy boundary). Used by both Create and
// GetByID so neither hands out — nor stores — the caller's own *roomfork.Job
// pointer: without this, a test goroutine holding the pointer it passed to
// Create (or received from GetByID) would race the background worker
// goroutine's later MarkRunning/UpdateProgress/MarkCompleted/MarkFailed
// mutations of that same struct.
func cloneJob(job *roomfork.Job) *roomfork.Job {
	cp := *job
	if job.ErrorMessage != nil {
		msg := *job.ErrorMessage
		cp.ErrorMessage = &msg
	}
	return &cp
}

// Create persists a new Job, storing a copy rather than the caller's own
// pointer (see cloneJob).
func (f *ForkJobRepo) Create(_ context.Context, job *roomfork.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureInit()

	f.Jobs[job.ID] = cloneJob(job)
	return nil
}

// GetByID retrieves a Job by ID, returning a copy rather than the
// internally-stored pointer (see cloneJob) — otherwise a caller mutating the
// returned *roomfork.Job (or reading it concurrently) would race the
// background worker goroutine's later writes to the same stored Job.
// Returns domain.ErrNotFound if not present.
func (f *ForkJobRepo) GetByID(_ context.Context, id string) (*roomfork.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneJob(job), nil
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

// CompleteAndUnarchive clears newRoomID's IsArchived flag via Rooms (if
// set — see the type doc comment) and transitions jobID to the terminal
// StatusCompleted state, mirroring
// postgres.RoomForkRepository.CompleteAndUnarchive's combined write. Returns
// domain.ErrNotFound if jobID does not exist, or whatever error Rooms.
// SetArchived returns if Rooms is set and newRoomID does not exist there.
func (f *ForkJobRepo) CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error {
	if f.Rooms != nil {
		if err := f.Rooms.SetArchived(ctx, newRoomID, false); err != nil {
			return err
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[jobID]
	if !ok {
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusCompleted
	job.UpdatedAt = time.Now()
	return nil
}
