package roomfork

import "context"

// ForkJobRepository defines persistence operations for room-fork Jobs. Every
// state-transition method (MarkRunning/UpdateProgress/MarkFailed/
// CompleteAndUnarchive) also updates UpdatedAt; MarkFailed and
// CompleteAndUnarchive are terminal — no further state transition is ever
// applied to a Job past either of them.
//
// # Source-state guards
//
// MarkRunning, UpdateProgress, and MarkFailed each narrow their update to
// rows whose current Status matches the state transition they represent
// (MarkRunning requires StatusPending; UpdateProgress requires
// StatusRunning; MarkFailed requires StatusPending or StatusRunning),
// mirroring CompleteAndUnarchive's own narrowed match. A Job whose id exists
// but whose current Status does not satisfy the guard is indistinguishable
// from a Job that does not exist at all: both return domain.ErrNotFound,
// so a late or duplicated call from runForkJob's single-goroutine sequence
// (or a retried/replayed one) can never regress a Job backwards out of a
// terminal state or skip StatusRunning's totalMessages bookkeeping.
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
	// not exist or is not currently StatusPending (see "Source-state
	// guards" above).
	MarkRunning(ctx context.Context, id string, totalMessages int64) error

	// UpdateProgress sets CopiedMessages to copiedMessages, called once per
	// successfully-persisted batch while Status == StatusRunning. Returns
	// domain.ErrNotFound if the job does not exist or is not currently
	// StatusRunning (see "Source-state guards" above).
	UpdateProgress(ctx context.Context, id string, copiedMessages int64) error

	// MarkFailed transitions a Job to the terminal StatusFailed state and
	// records errMsg. Returns domain.ErrNotFound if the job does not exist
	// or is not currently StatusPending or StatusRunning (see "Source-state
	// guards" above) — in particular, a Job that already reached
	// StatusCompleted (via CompleteAndUnarchive) can never be marked failed
	// after the fact.
	MarkFailed(ctx context.Context, id string, errMsg string) error

	// CompleteAndUnarchive atomically transitions a Job to the terminal
	// StatusCompleted state and clears newRoomID's rooms.is_archived flag,
	// as a single transaction. This is what a successful room-fork copy
	// (usecase/room.RoomUsecase.runForkJob) must call instead of issuing
	// RoomRepository.SetArchived and a separate job-completion write as two
	// separate calls: with two calls, a failure of the second write after
	// the first (unarchive) write already committed would leave the room
	// live while the job stays stuck in StatusRunning forever. Folding both
	// writes into one transaction means either both land or neither does.
	//
	// StatusCompleted is meant to imply the room accepts posts: a poller
	// that observes a Job with Status == StatusCompleted may immediately
	// rely on newRoomID no longer being archived, without a separate
	// GetByID(newRoomID) round trip to confirm it. The job-side match is
	// narrow — jobID, newRoomID, AND Status == StatusRunning must all agree
	// — so a jobID/newRoomID pair that don't belong to each other, or a job
	// that isn't currently running, is rejected as domain.ErrNotFound
	// before either write lands, rather than transitioning a job or
	// unarchiving a room that doesn't match the caller's expectations.
	CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error
}
