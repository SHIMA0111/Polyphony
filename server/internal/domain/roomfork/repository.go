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
	//
	// Deprecated: a successful room-fork copy should call
	// CompleteAndUnarchive instead, which also clears the destination
	// room's is_archived flag in the same transaction. MarkCompleted alone
	// is kept for callers (and tests) that only need the Job-side state
	// transition, e.g. exercising this repository in isolation from a room.
	MarkCompleted(ctx context.Context, id string) error

	// MarkFailed transitions a Job to the terminal StatusFailed state and
	// records errMsg. Returns domain.ErrNotFound if the job does not exist.
	MarkFailed(ctx context.Context, id string, errMsg string) error

	// CompleteAndUnarchive atomically transitions a Job to the terminal
	// StatusCompleted state and clears newRoomID's rooms.is_archived flag,
	// as a single transaction. This is what a successful room-fork copy
	// (usecase/room.RoomUsecase.runForkJob) must call instead of issuing
	// RoomRepository.SetArchived and MarkCompleted as two separate calls:
	// with two calls, a failure of the second (job-completion) write after
	// the first (unarchive) write already committed would leave the room
	// live while the job stays stuck in StatusRunning forever. Folding both
	// writes into one transaction means either both land or neither does.
	//
	// StatusCompleted is meant to imply the room accepts posts: a poller
	// that observes a Job with Status == StatusCompleted may immediately
	// rely on newRoomID no longer being archived, without a separate
	// GetByID(newRoomID) round trip to confirm it. Returns
	// domain.ErrNotFound if either the job or the room does not exist.
	CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error
}
