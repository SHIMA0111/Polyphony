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
// The revision check and the INSERT/UPDATE it gates run inside a
// transaction that first takes a room-scoped pg_advisory_xact_lock, the
// same lock DeleteByRoom takes before its own delete-and-bump statement.
// Without this, the two statements' READ COMMITTED snapshots could
// interleave under plain autocommit: this method's revision check could
// read a still-current revision, then a concurrent DeleteByRoom could
// delete-and-bump and commit, and then this method's INSERT could still
// land afterward -- a summary computed against a since-superseded revision,
// committed after the very invalidation that was supposed to reject it.
// Serializing both methods on the same lock closes that window: whichever
// call acquires the lock first fully commits (or rolls back) before the
// other's statement can even begin, so the invariant holds -- an Upsert can
// never commit a summary that predates a committed revision bump.
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
// Runs inside a transaction that first takes the same room-scoped
// pg_advisory_xact_lock Upsert takes, serializing this delete-and-bump
// against a concurrently-committing Upsert for the same room -- see
// Upsert's doc comment for the interleaving this closes and the invariant
// it establishes.
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
