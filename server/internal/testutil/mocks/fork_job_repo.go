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
}

func (f *ForkJobRepo) ensureInit() {
	if f.Jobs == nil {
		f.Jobs = make(map[string]*roomfork.Job)
	}
}

// Create persists a new Job.
func (f *ForkJobRepo) Create(_ context.Context, job *roomfork.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureInit()

	f.Jobs[job.ID] = job
	return nil
}

// GetByID retrieves a Job by ID. Returns domain.ErrNotFound if not present.
func (f *ForkJobRepo) GetByID(_ context.Context, id string) (*roomfork.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	job, ok := f.Jobs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return job, nil
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
