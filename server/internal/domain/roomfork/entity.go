// Package roomfork defines the room-fork job entity and its repository
// port. A Job tracks the progress of an asynchronous, batch copy of one
// room's messages into a newly created room (see phases.md Phase 20 and
// usecase/room.RoomUsecase.ForkRoom/runForkJob).
//
// Like domain/room and domain/message, this package has zero non-stdlib
// imports: it depends only on the Go standard library, so the usecase layer
// (the only place that orchestrates roomfork.Job together with
// domain/room.Room and domain/message.Message) is the sole point where
// these domains meet.
package roomfork

import "time"

// Status is the lifecycle state of a Job.
type Status string

const (
	// StatusPending is a Job's initial state, set when ForkRoom creates the
	// job row, before the background worker (runForkJob) has started
	// reading any messages.
	StatusPending Status = "pending"

	// StatusRunning is set once the worker has counted the source room's
	// messages (populating TotalMessages) and begun copying batches.
	StatusRunning Status = "running"

	// StatusCompleted is a terminal state: every message has been copied
	// and the destination room's IsArchived flag has been cleared. A
	// completed Job is never mutated further.
	StatusCompleted Status = "completed"

	// StatusFailed is a terminal state reached if any step of the copy
	// fails (message counting, sequence reservation, batch persistence).
	// ErrorMessage carries the failure reason; the destination room stays
	// archived and, depending on when the failure occurred, may hold zero
	// or a partial set of copied messages. A failed Job is never retried
	// automatically (see step32.md's Out of scope).
	StatusFailed Status = "failed"
)

// Job tracks a single room-fork background copy operation: how far it has
// progressed (CopiedMessages out of TotalMessages) and its terminal outcome
// (Status/ErrorMessage), so a caller can poll it via
// usecase/room.RoomUsecase.GetForkJobStatus without blocking on the copy
// itself.
type Job struct {
	ID           string
	SourceRoomID string
	NewRoomID    string
	Status       Status
	// TotalMessages is the source room's message count as of the moment the
	// worker started (see ForkJobRepository.MarkRunning); it is 0 while
	// Status == StatusPending.
	TotalMessages int64
	// CopiedMessages is the running count of messages successfully
	// persisted into NewRoomID so far, advanced only in whole-batch
	// increments (see ForkJobRepository.UpdateProgress).
	CopiedMessages int64
	// ErrorMessage is non-nil only when Status == StatusFailed, holding the
	// error that aborted the copy.
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
