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
	// CoveredUpToSequence is the Sequence of the oldest message included in
	// the summarized (older-public) bucket at the time this summary was
	// computed. A subsequent call only reuses this summary when its own
	// computed boundary sequence matches exactly (see
	// MessageUsecase.assembleAIContext) -- an exact-match cache, not an
	// incremental one (see step50.md's Out of scope section).
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
type ContextSummaryRepository interface {
	// Get returns the cached ContextSummary for roomID, or
	// domain.ErrNotFound if no summary has been cached for this room yet
	// (or it was invalidated via DeleteByRoom and not yet recomputed).
	Get(ctx context.Context, roomID string) (*ContextSummary, error)

	// Upsert creates or replaces the single cached row for
	// summary.RoomID: a room has at most one cached summary at a time, so
	// this always overwrites any existing row for the same room rather
	// than inserting a second one.
	Upsert(ctx context.Context, summary *ContextSummary) error

	// DeleteByRoom invalidates (deletes) the cached summary for roomID, if
	// any. It must not return an error when no cached row exists for
	// roomID -- callers (MessageUsecase.DeleteMessage/SetExcludeFromAI)
	// invoke this unconditionally on every mutation regardless of whether a
	// summary was ever computed for the room, and treating "nothing to
	// delete" as a failure would turn routine message edits into spurious
	// error paths.
	DeleteByRoom(ctx context.Context, roomID string) error
}
