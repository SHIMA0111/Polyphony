package postgres

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// ContextSummaryRepository implements the ai.ContextSummaryRepository
// interface using PostgreSQL. It stores at most one cached summary row per
// room in message_context_summaries (room_id is that table's PRIMARY KEY),
// following the same "load/mutate, RowsAffected()==0 -> ErrNotFound"
// conventions used by MessageRepository (see message_repository.go).
type ContextSummaryRepository struct {
	pool *pgxpool.Pool
}

// NewContextSummaryRepository creates a new ContextSummaryRepository backed
// by the given connection pool.
func NewContextSummaryRepository(pool *pgxpool.Pool) *ContextSummaryRepository {
	return &ContextSummaryRepository{pool: pool}
}

// Get retrieves the cached ContextSummary for roomID. It returns
// domain.ErrNotFound if no summary has been cached for this room (or it was
// invalidated via DeleteByRoom and not yet recomputed).
func (r *ContextSummaryRepository) Get(ctx context.Context, roomID string) (*ai.ContextSummary, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT room_id, model, covered_up_to_sequence, summary_text, token_count, created_at, updated_at
		 FROM message_context_summaries WHERE room_id = $1`,
		roomID,
	)

	var s ai.ContextSummary
	err := row.Scan(&s.RoomID, &s.Model, &s.CoveredUpToSequence, &s.SummaryText, &s.TokenCount, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

// Upsert creates or replaces the single cached row for summary.RoomID, via
// INSERT ... ON CONFLICT (room_id) DO UPDATE, but only when roomID's current
// invalidation revision (context_summary_revisions, defaulting to 0 when no
// row exists yet) still equals expectedRevision -- see
// ai.ContextSummaryRepository's "Revision fencing" doc comment. The INSERT's
// source is a SELECT gated on that revision match, so a mismatch makes the
// whole statement affect zero rows (ON CONFLICT never triggers, since
// nothing was proposed for insertion) rather than writing a stale summary.
// A zero-rows-affected outcome is logged at Warn (an expected outcome of a
// detected race, not a failure) and reported to the caller as a nil error,
// per the port's no-op contract; only a genuine query failure returns an
// error.
//
// Invariant: an Upsert can never commit a summary predating a committed
// revision bump. Reading the revision and writing the summary are two
// separate statements' worth of work folded into one SQL statement here,
// but that single-statement atomicity alone does not prevent this Upsert
// and a concurrent DeleteByRoom (for the same room) from interleaving: this
// statement's revision check could read a snapshot taken *before*
// DeleteByRoom's DELETE + revision bump commits, and this statement could
// then go on to commit its own INSERT *after* that DELETE already ran --
// resurrecting exactly the stale summary DeleteByRoom was meant to remove,
// with no subsequent write left to clean it up. pg_advisory_xact_lock,
// acquired on the same room-scoped key DeleteByRoom acquires below, closes
// that window: whichever of the two transactions acquires the lock first
// runs to completion (and releases it) before the other is even allowed to
// evaluate its revision check, so the two can never interleave.
func (r *ContextSummaryRepository) Upsert(ctx context.Context, summary *ai.ContextSummary, expectedRevision int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, summary.RoomID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO message_context_summaries (room_id, model, covered_up_to_sequence, summary_text, token_count, created_at, updated_at)
		 SELECT $1, $2, $3, $4, $5, NOW(), NOW()
		 WHERE COALESCE((SELECT revision FROM context_summary_revisions WHERE room_id = $1), 0) = $6
		 ON CONFLICT (room_id) DO UPDATE
		 SET model = $2, covered_up_to_sequence = $3, summary_text = $4, token_count = $5, updated_at = NOW()`,
		summary.RoomID, summary.Model, summary.CoveredUpToSequence, summary.SummaryText, summary.TokenCount, expectedRevision,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		slog.Warn("context summary upsert skipped: room's invalidation revision moved during summarization",
			"room_id", summary.RoomID, "expected_revision", expectedRevision)
	}
	return tx.Commit(ctx)
}

// GetRevision returns roomID's current context-summary invalidation
// revision, or 0 if DeleteByRoom has never been called for this room (no
// row exists yet in context_summary_revisions).
func (r *ContextSummaryRepository) GetRevision(ctx context.Context, roomID string) (int64, error) {
	var revision int64
	err := r.pool.QueryRow(ctx,
		`SELECT revision FROM context_summary_revisions WHERE room_id = $1`, roomID,
	).Scan(&revision)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return revision, nil
}

// DeleteByRoom invalidates (deletes) the cached summary for roomID, if any,
// and atomically increments roomID's invalidation revision in the same
// statement: the DELETE runs as a data-modifying CTE feeding the following
// INSERT ... ON CONFLICT, which Postgres executes as a single atomic
// operation, so no concurrent Upsert can observe the DELETE without also
// observing the revision bump (or vice versa). It treats zero rows deleted
// (no cached summary existed) as success, not an error -- the revision is
// still bumped unconditionally -- since
// MessageUsecase.DeleteMessage/SetExcludeFromAI call this unconditionally on
// every mutation regardless of whether a summary was ever computed for the
// room.
//
// This acquires the same room-scoped pg_advisory_xact_lock as Upsert,
// before running the statement above, for the same reason documented on
// Upsert: without it, a concurrent Upsert whose revision check was
// evaluated just before this DELETE commits could still land its INSERT
// just after, re-creating the very row this call is meant to invalidate.
// The lock is transaction-scoped (released automatically on commit/
// rollback, not requiring an explicit unlock call) and keyed by
// hashtext(roomID), so it only ever contends with another call (Upsert or
// DeleteByRoom) for this same room.
func (r *ContextSummaryRepository) DeleteByRoom(ctx context.Context, roomID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, roomID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`WITH deleted AS (
		     DELETE FROM message_context_summaries WHERE room_id = $1
		 )
		 INSERT INTO context_summary_revisions (room_id, revision) VALUES ($1, 1)
		 ON CONFLICT (room_id) DO UPDATE SET revision = context_summary_revisions.revision + 1`,
		roomID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
