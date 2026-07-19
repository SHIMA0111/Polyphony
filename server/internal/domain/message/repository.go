package message

import (
	"context"
	"time"
)

// MessageRepository defines persistence operations for messages.
type MessageRepository interface {
	// Create persists a new message.
	Create(ctx context.Context, msg *Message) error

	// GetByID retrieves a message by ID. Returns ErrNotFound if not found.
	// requestingUserID controls visibility: a message with
	// Visibility == MessageVisibilityPrivate is returned only when
	// requestingUserID matches its SenderID; for every other caller it is
	// treated exactly as if it does not exist (ErrNotFound), never
	// distinguished from a genuinely missing row.
	GetByID(ctx context.Context, id string, requestingUserID string) (*Message, error)

	// ListByRoom returns messages in a room using cursor-based pagination.
	// Messages are ordered by sequence descending (newest first).
	// If the cursor is empty, starts from the most recent message.
	// requestingUserID controls visibility: a message with
	// Visibility == MessageVisibilityPrivate whose SenderID does not match
	// requestingUserID is excluded from the page entirely, as if it does
	// not exist — it is invisible to every user other than its owner.
	ListByRoom(ctx context.Context, roomID string, cursor string, limit int, requestingUserID string) (*CursorPage, error)

	// ListByRoomUpTo returns up to limit messages in a room with sequence <= maxSequence,
	// ordered by sequence descending (newest first).
	// Used to build AI context up to a specific message. requestingUserID
	// controls visibility: a message with Visibility == MessageVisibilityPrivate
	// whose SenderID does not match requestingUserID is excluded, as if it
	// does not exist — it is invisible to every user other than its owner,
	// including when another user's subsequent AI request assembles context.
	ListByRoomUpTo(ctx context.Context, roomID string, maxSequence int64, limit int, requestingUserID string) ([]*Message, error)

	// GetNextInRoom returns the message with the smallest sequence greater than
	// afterSequence in the given room. Returns ErrNotFound if no such message exists.
	GetNextInRoom(ctx context.Context, roomID string, afterSequence int64) (*Message, error)

	// UpdateAIResponse updates the content, status, and updated_at of an AI message.
	UpdateAIResponse(ctx context.Context, id string, content string, status MessageStatus, updatedAt time.Time) error

	// UpdateExcludeFromAI sets the exclude_from_ai flag and updated_at of a
	// message. It follows the same signature shape as UpdateAIResponse.
	// Returns ErrNotFound if the message does not exist.
	UpdateExcludeFromAI(ctx context.Context, id string, exclude bool, updatedAt time.Time) error

	// UpdateExcludeFromAIAndInvalidateSummary atomically sets messageID's
	// exclude_from_ai flag (per UpdateExcludeFromAI's semantics) and
	// invalidates roomID's cached AI context summary (per
	// ai.ContextSummaryRepository.DeleteByRoom's semantics), as a single
	// all-or-nothing operation, for exactly the same reason
	// DeleteAndInvalidateSummary must be atomic -- see that method's doc
	// comment for the race a one-sided ordering leaves open in either
	// direction, and why only same-transaction atomicity closes it.
	//
	// Returns ErrNotFound if messageID does not exist; on that path nothing
	// is mutated, including the summary invalidation.
	UpdateExcludeFromAIAndInvalidateSummary(ctx context.Context, messageID string, exclude bool, roomID string) error

	// Delete soft-deletes a message by ID: it sets is_deleted = true (and
	// updates updated_at) rather than physically removing the row. A
	// soft-deleted message is excluded from ListByRoom, ListByRoomUpTo, and
	// AI context assembly, but remains fetchable via GetByID. Returns
	// ErrNotFound if the message does not exist or is already deleted.
	Delete(ctx context.Context, id string) error

	// DeleteAndInvalidateSummary atomically soft-deletes messageID (per
	// Delete's semantics) and invalidates roomID's cached AI context summary
	// (per ai.ContextSummaryRepository.DeleteByRoom's semantics: the cached
	// summary is deleted and the room's invalidation revision is bumped),
	// as a single all-or-nothing operation.
	//
	// This atomicity is required, not merely convenient: running the two
	// steps as separate calls leaves a race window open no matter which
	// order they run in. Invalidate-then-mutate (the prior implementation)
	// has a window where a summarization already reading the pre-delete
	// messages can start after the revision bump but finish and Upsert
	// before the message mutation commits, caching a fresh summary that
	// still contains the about-to-be-removed content -- the invalidation
	// happened, but too early to invalidate the summary it was meant to
	// invalidate. Mutate-then-invalidate has the opposite failure: if the
	// mutation commits but the separate invalidation call then fails, a
	// summary computed from the now-stale (pre-mutation) content remains
	// the cached, servable summary indefinitely, with no way to retry only
	// the invalidation half. Only performing both statements inside one
	// database transaction -- as the postgres implementation does, gated by
	// the same room-scoped advisory lock ai.ContextSummaryRepository's
	// Upsert/DeleteByRoom take, so this also serializes against any
	// concurrently-committing Upsert for roomID -- closes both directions:
	// the message mutation and the summary invalidation either both commit
	// or neither does.
	//
	// Returns ErrNotFound if messageID does not exist or is already deleted
	// (mirroring Delete); on that path nothing is mutated, including the
	// summary invalidation.
	DeleteAndInvalidateSummary(ctx context.Context, messageID, roomID string) error

	// ReserveSequenceRange atomically reserves count contiguous sequence
	// numbers for the given room and returns the first one; the caller owns
	// the whole range [first, first+count). This lets a caller that needs to
	// persist multiple related messages (e.g. a human message and its AI
	// response) allocate every sequence number they need in a single atomic
	// step, so no other message can be interleaved between them. Returns
	// domain.ErrNotFound if the room has no sequence counter row, and
	// domain.ErrInvalidArgument if count <= 0 — a non-positive count would
	// either reserve nothing while still consuming a round trip (count == 0)
	// or corrupt the room's sequence counter by moving it backwards (count <
	// 0), so implementations must reject it before ever reaching the
	// underlying storage mutation.
	ReserveSequenceRange(ctx context.Context, roomID string, count int64) (int64, error)

	// CountByRoom returns the total number of messages (including
	// soft-deleted and private ones — this is a structural count, not a
	// visibility-filtered read) in roomID. It drives
	// room_fork_jobs.total_messages, letting a room-fork job report overall
	// progress before it copies a single message.
	CountByRoom(ctx context.Context, roomID string) (int64, error)

	// CountAndMaxSequence returns, in a single atomic read, the total number
	// of messages in roomID (identical in scope to CountByRoom — including
	// soft-deleted and private ones) together with the highest sequence
	// number currently present (0 if the room has no messages). A room fork
	// (usecase/room.RoomUsecase.runForkJob) calls this once, up front,
	// instead of CountByRoom, so total and maxSeq are read together as one
	// consistent snapshot: maxSeq is then passed as every subsequent
	// ListByRoomAfter call's maxSequence bound, freezing the copy's upper
	// boundary at job-start time. Without that frozen boundary, a message
	// posted to the source room mid-copy could either be silently missed or
	// — since ListByRoomAfter's afterSequence cursor only ever advances —
	// cause the copy loop to run indefinitely if messages keep arriving
	// faster than batches are drained.
	CountAndMaxSequence(ctx context.Context, roomID string) (total int64, maxSeq int64, err error)

	// ListByRoomAfter returns up to limit messages in roomID with
	// afterSequence < sequence <= maxSequence, ordered ascending by sequence
	// (oldest first) — the forward-cursor counterpart to ListByRoomUpTo's
	// newest-first pagination. It ignores soft-delete/visibility/
	// exclude-from-ai flags entirely (unlike every other read method on
	// this interface): a room fork (usecase/room.RoomUsecase.runForkJob)
	// copies the full, unfiltered conversation into the destination room,
	// independent of any AI-context filtering rule. maxSequence is the
	// frozen snapshot boundary a caller obtained from
	// CountAndMaxSequence at the start of the copy — passing it on every
	// call (rather than an ever-growing "current max") is what excludes
	// messages posted to the source room after the copy began and
	// guarantees the calling loop terminates.
	ListByRoomAfter(ctx context.Context, roomID string, afterSequence int64, maxSequence int64, limit int) ([]*Message, error)

	// CreateBatch persists msgs atomically: either every message in msgs is
	// committed, or (on any single insert failure) none are. This is what
	// lets a room-fork job's per-batch progress counter
	// (room_fork_jobs.copied_messages) only ever advance by whole,
	// successfully-committed batches, never a partially-copied batch.
	CreateBatch(ctx context.Context, msgs []*Message) error
}
