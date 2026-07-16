package roomfork

import "context"

// ForkJobRepository defines persistence operations for room-fork Jobs. Every
// state-transition method (MarkRunning/MarkCompleted/MarkFailed) also
// updates UpdatedAt; MarkCompleted and MarkFailed are terminal — no further
// state transition is ever applied to a Job past either of them.
type ForkJobRepository interface {
	// Create persists a new Job. The caller (usecase/room.RoomUsecase.ForkRoom)
	// always creates it with Status == StatusPending and
	// TotalMessages == CopiedMessages == 0.
	Create(ctx context.Context, job *Job) error

	// GetByID retrieves a Job by ID. Returns domain.ErrNotFound if it does
	// not exist.
	GetByID(ctx context.Context, id string) (*Job, error)

	// MarkRunning transitions a Job from StatusPending to StatusRunning and
	// records totalMessages (the source room's message count as counted at
	// the start of the copy). Returns domain.ErrNotFound if the job does
	// not exist.
	MarkRunning(ctx context.Context, id string, totalMessages int64) error

	// UpdateProgress sets CopiedMessages to copiedMessages, called once per
	// successfully-persisted batch while Status == StatusRunning. Returns
	// domain.ErrNotFound if the job does not exist.
	UpdateProgress(ctx context.Context, id string, copiedMessages int64) error

	// MarkCompleted transitions a Job to the terminal StatusCompleted
	// state. Returns domain.ErrNotFound if the job does not exist.
	MarkCompleted(ctx context.Context, id string) error

	// MarkFailed transitions a Job to the terminal StatusFailed state and
	// records errMsg. Returns domain.ErrNotFound if the job does not exist.
	MarkFailed(ctx context.Context, id string, errMsg string) error
}
