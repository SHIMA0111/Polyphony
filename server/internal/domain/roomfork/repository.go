package roomfork

import "context"

// ForkJobRepository defines persistence operations for room-fork Jobs. Every
// state-transition method (MarkRunning/MarkCompleted/MarkFailed/
// CompleteAndUnarchive) also updates UpdatedAt; MarkCompleted, MarkFailed,
// and CompleteAndUnarchive are terminal — no further state transition is
// ever applied to a Job past any of them.
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

	// CompleteAndUnarchive atomically clears newRoomID's is_archived flag
	// and transitions jobID to the terminal StatusCompleted state within a
	// single database transaction, so the two updates either both land or
	// neither does. This replaces the previous two-call sequence
	// (room.RoomRepository.SetArchived(ctx, newRoomID, false) followed by
	// MarkCompleted) that a caller (usecase/room.RoomUsecase.runForkJob)
	// used to perform: if the second of those two calls failed, the room
	// would be left permanently unarchived (accepting posts) while its Job
	// stayed stuck in StatusRunning forever, with no way for a poller to
	// learn the copy had, in fact, finished. Implementations must therefore
	// reach into both the rooms and room_fork_jobs tables from within this
	// one method, rather than delegating to two independently-committing
	// repository calls.
	//
	// A caller observing StatusCompleted via GetByID may rely on the
	// implication holding in both directions: the new room's is_archived is
	// false if and only if its Job has reached StatusCompleted (barring a
	// separate, later archival of the room for unrelated reasons).
	//
	// The job-side write only ever performs the StatusRunning ->
	// StatusCompleted transition, and only for the Job whose NewRoomID
	// equals newRoomID — a mismatched newRoomID or a job not currently
	// StatusRunning (already completed/failed, or never started) leaves
	// both rows untouched and returns domain.ErrNotFound, the same as a
	// nonexistent jobID. The job write is validated/applied before the
	// room write, so a job-side failure never reaches (or archives-flips)
	// the room.
	//
	// Returns domain.ErrNotFound if jobID does not exist, is not
	// StatusRunning, is not linked to newRoomID, or newRoomID itself does
	// not exist — every case rolls back the whole transaction, leaving both
	// rows exactly as they were.
	CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error
}
