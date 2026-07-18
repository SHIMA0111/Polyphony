package mocks

import (
	"context"
	"errors"
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

	// Rooms is the *RoomRepo CompleteAndUnarchive applies its room-side
	// is_archived=false write to, mirroring
	// postgres.RoomForkRepository.CompleteAndUnarchive reaching into both
	// the rooms and room_fork_jobs tables from one method.
	// CompleteAndUnarchive requires it and fails when unset; nil remains a
	// valid zero value only for tests that never drive a fork to
	// completion.
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
// goroutine's later MarkRunning/UpdateProgress/MarkFailed/
// CompleteAndUnarchive mutations of that same struct.
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

// MarkRunning transitions a Job from StatusPending to StatusRunning and
// records totalMessages, mirroring
// postgres.RoomForkRepository.MarkRunning's `AND status = 'pending'` guard.
// Returns domain.ErrNotFound if the job does not exist or is not currently
// StatusPending, leaving it untouched in either case.
func (f *ForkJobRepo) MarkRunning(_ context.Context, id string, totalMessages int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok || job.Status != roomfork.StatusPending {
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusRunning
	job.TotalMessages = totalMessages
	job.UpdatedAt = time.Now()
	return nil
}

// UpdateProgress sets CopiedMessages on a Job, mirroring
// postgres.RoomForkRepository.UpdateProgress's `AND status = 'running'`
// guard. Returns domain.ErrNotFound if the job does not exist or is not
// currently StatusRunning, leaving it untouched in either case.
func (f *ForkJobRepo) UpdateProgress(_ context.Context, id string, copiedMessages int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok || job.Status != roomfork.StatusRunning {
		return domain.ErrNotFound
	}
	job.CopiedMessages = copiedMessages
	job.UpdatedAt = time.Now()
	return nil
}

// MarkFailed transitions a Job to the terminal StatusFailed state and
// records errMsg, mirroring
// postgres.RoomForkRepository.MarkFailed's `AND status IN ('pending',
// 'running')` guard. Returns domain.ErrNotFound if the job does not exist
// or has already reached a terminal state, leaving it untouched in either
// case.
func (f *ForkJobRepo) MarkFailed(_ context.Context, id string, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok || (job.Status != roomfork.StatusPending && job.Status != roomfork.StatusRunning) {
		return domain.ErrNotFound
	}
	job.Status = roomfork.StatusFailed
	job.ErrorMessage = &errMsg
	job.UpdatedAt = time.Now()
	return nil
}

// CompleteAndUnarchive validates jobID exists, is linked to newRoomID (its
// NewRoomID matches), and is currently StatusRunning — all before mutating
// either store — then transitions it to the terminal StatusCompleted state
// and clears newRoomID's IsArchived flag via Rooms (if set — see the type
// doc comment), mirroring
// postgres.RoomForkRepository.CompleteAndUnarchive's tightened WHERE clause
// and job-before-room ordering. Returns domain.ErrNotFound if jobID does
// not exist, is not linked to newRoomID, or is not StatusRunning — none of
// those cases mutates Jobs or Rooms. f.mu is held across the entire
// operation, including the Rooms.SetArchived call — Rooms guards its own
// state with a separate mutex, so this cannot deadlock, and holding f.mu
// throughout prevents a concurrent GetByID/MarkFailed from observing the
// job as StatusCompleted before the room-side write has actually landed (or
// failed). If Rooms.SetArchived then fails (e.g. newRoomID does not exist
// there), the job-side write is rolled back to its pre-call Status and
// UpdatedAt before returning that error, so a room-side failure never
// leaves the job completed while the room stays archived — approximating
// the postgres implementation's single-transaction atomicity across the
// two separate in-memory stores.
func (f *ForkJobRepo) CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[jobID]
	if !ok || job.NewRoomID != newRoomID || job.Status != roomfork.StatusRunning {
		return domain.ErrNotFound
	}

	// Rooms is mandatory for this method: the real implementation updates
	// rooms.is_archived and room_fork_jobs in one transaction, so a mock
	// that "completed" the job without touching any room would silently
	// break that contract for the test using it. Failing before mutating
	// surfaces the misconfiguration instead.
	if f.Rooms == nil {
		return errors.New("mocks.ForkJobRepo: Rooms must be wired before calling CompleteAndUnarchive")
	}

	prevStatus := job.Status
	prevUpdatedAt := job.UpdatedAt
	job.Status = roomfork.StatusCompleted
	job.UpdatedAt = time.Now()

	if err := f.Rooms.SetArchived(ctx, newRoomID, false); err != nil {
		job.Status = prevStatus
		job.UpdatedAt = prevUpdatedAt
		return err
	}
	return nil
}
