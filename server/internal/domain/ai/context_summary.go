package ai

import (
	"context"
	"time"
)

// ContextSummary is a cached, AI-generated summary of a room's older
// message history, produced by MessageUsecase.assembleAIContext (Phase 18
// history summarization) whenever the Step-23-filtered context for a given
// model would overflow that model's token budget. It replaces the older
// public-visibility portion of a room's history with a single, cheaper
// system-role message the next time the same (room, model,
// CoveredUpToSequence) triple is requested.
//
// One row exists per room (see ContextSummaryRepository.Upsert): a room has
// at most one cached summary at a time, keyed only by RoomID, so a new
// summary always replaces the previous one rather than accumulating a
// history of past summaries.
type ContextSummary struct {
	// RoomID is the room this summary was computed for.
	RoomID string
	// Model is the model ID the summary was generated with. A cache lookup
	// only counts as a hit when the requested model matches this exactly:
	// different models may tokenize/summarize differently, and reusing one
	// model's summary for another would silently change the AI's effective
	// context for no cache-invalidation-visible reason.
	Model string
	// CoveredUpToSequence is the Sequence of the newest message included in
	// the summarized (older-public) bucket at the time this summary was
	// computed -- i.e. the boundary between the summarized older-public
	// history and the verbatim recent tail/still-unsummarized messages that
	// follow it. A subsequent call only reuses this summary when its own
	// computed boundary sequence matches exactly (see
	// MessageUsecase.assembleAIContext) -- an exact-match cache, not an
	// incremental one (see step50.md's Out of scope section).
	//
	// This deliberately tracks the newest, not the oldest, summarized
	// sequence: the oldest message in a room's older-public bucket never
	// changes once it has been summarized once, so keying the cache on it
	// would never invalidate as new messages age out of the recent tail
	// into the older-public bucket -- the cached summary would silently go
	// stale and newly bucketed messages would never reach the AI. Tracking
	// the newest summarized sequence instead means the boundary advances
	// (and the cache correctly misses) every time there is new older-public
	// history to fold in.
	CoveredUpToSequence int64
	// SummaryText is the model's plain-text summarization output. It is
	// never a raw transcript, URL, or base64 image blob -- see
	// BuildSummarizationPrompt's doc comment for why only the model's text
	// output is ever cached.
	SummaryText string
	// TokenCount is the estimated token count of SummaryText (via
	// LLMGateway.EstimateTokens), cached alongside the text so a later
	// budget calculation does not need to re-estimate it.
	TokenCount int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ContextSummaryRepository is the port through which
// MessageUsecase.assembleAIContext reads and writes a room's cached context
// summary. The initial (and, as of this step, only) implementation is
// postgres.ContextSummaryRepository (interface/repository/postgres).
//
// # Revision fencing
//
// Upsert/GetRevision/DeleteByRoom together fence a race that would otherwise
// let a stale summary resurrect itself: MessageUsecase.summaryOrCompute calls
// ai.LLMGateway.Complete to produce a new summary, which can take long enough
// for a concurrent message delete or exclude_from_ai toggle landing on the
// same room mid-summarization to invalidate the cache before that Complete
// call returns. Without a fencing mechanism, the in-flight summarization's
// own Upsert would land after that invalidation and silently put the stale,
// pre-invalidation summary right back -- as if it had never happened.
// GetRevision/DeleteByRoom's monotonic per-room counter closes this: a
// caller captures GetRevision before starting summarization and passes it
// back to Upsert as expectedRevision, so an Upsert that lands after a
// concurrent invalidation (which always advances the revision) can detect
// the mismatch and no-op instead of overwriting.
//
// A message delete/exclude_from_ai toggle no longer invalidates by calling
// this interface's DeleteByRoom directly: message.MessageRepository's
// DeleteAndInvalidateSummary/UpdateExcludeFromAIAndInvalidateSummary apply
// the identical DELETE-then-revision-bump statement against the same
// message_context_summaries/context_summary_revisions tables, in the same
// transaction as the message mutation itself and under the same room-scoped
// advisory lock (see those methods' doc comments for why). The revision
// this interface's GetRevision reads is the same counter either write path
// advances, so the fencing described above holds regardless of which one
// performed the invalidation.
type ContextSummaryRepository interface {
	// Get returns the cached ContextSummary for roomID, or
	// domain.ErrNotFound if no summary has been cached for this room yet
	// (or it was invalidated via DeleteByRoom and not yet recomputed).
	Get(ctx context.Context, roomID string) (*ContextSummary, error)

	// Upsert creates or replaces the single cached row for summary.RoomID,
	// but only if roomID's current invalidation revision (see GetRevision)
	// still equals expectedRevision, which the caller must have captured
	// via GetRevision before starting the summarization work that produced
	// summary (see MessageUsecase.summaryOrCompute). A room has at most one
	// cached summary at a time, so a matching-revision Upsert always
	// overwrites any existing row for the same room rather than inserting
	// a second one.
	//
	// If the revision has since moved -- a concurrent DeleteByRoom landed
	// while summary was being computed -- Upsert no-ops: it does not write
	// summary and does not return an error for the mismatch itself (only a
	// genuine underlying failure, e.g. a connection error, is returned), and
	// logs a warning. See the package doc comment's "Revision fencing"
	// section for why this must be silent-but-observable rather than an
	// error: the mismatch is an expected, race-free outcome of the fencing
	// design, not a bug in the caller.
	Upsert(ctx context.Context, summary *ContextSummary, expectedRevision int64) error

	// GetRevision returns roomID's current context-summary invalidation
	// revision: a monotonic counter that starts at 0 for a room that has
	// never had a DeleteByRoom call, and is incremented by every
	// DeleteByRoom call for that room thereafter (whether or not a cached
	// summary actually existed to delete). Callers capture this before
	// starting summarization and pass it back to Upsert as
	// expectedRevision -- see the package doc comment's "Revision fencing"
	// section.
	GetRevision(ctx context.Context, roomID string) (int64, error)

	// DeleteByRoom invalidates (deletes) the cached summary for roomID, if
	// any, and atomically increments roomID's invalidation revision (see
	// GetRevision) in the same operation, so any summarization already in
	// flight when this call lands is guaranteed to observe a moved revision
	// when it later calls Upsert. It must not return an error when no
	// cached row exists for roomID -- message.MessageRepository's
	// DeleteAndInvalidateSummary/UpdateExcludeFromAIAndInvalidateSummary
	// apply this same zero-rows-tolerant DELETE unconditionally on every
	// message mutation regardless of whether a summary was ever computed
	// for the room (see those methods' doc comments), and treating "nothing
	// to delete" as a failure would turn routine message edits into
	// spurious error paths.
	//
	// A genuine failure (e.g. a connection error) must still be returned:
	// the revision bump is the sole mechanism preventing a stale summary
	// from being resurrected by a concurrent in-flight Upsert, and silently
	// continuing past a failed invalidation would leave that summary
	// reachable indefinitely.
	DeleteByRoom(ctx context.Context, roomID string) error
}
