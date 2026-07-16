package postgres

import (
	"context"
	"errors"

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
// INSERT ... ON CONFLICT (room_id) DO UPDATE, so a room's previous summary
// (if any) is always fully replaced rather than accumulating extra rows.
func (r *ContextSummaryRepository) Upsert(ctx context.Context, summary *ai.ContextSummary) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO message_context_summaries (room_id, model, covered_up_to_sequence, summary_text, token_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (room_id) DO UPDATE
		 SET model = $2, covered_up_to_sequence = $3, summary_text = $4, token_count = $5, updated_at = NOW()`,
		summary.RoomID, summary.Model, summary.CoveredUpToSequence, summary.SummaryText, summary.TokenCount,
	)
	return err
}

// DeleteByRoom invalidates (deletes) the cached summary for roomID, if any.
// It treats zero rows affected (no cached summary existed) as success, not
// an error, since MessageUsecase.DeleteMessage/SetExcludeFromAI call this
// unconditionally on every mutation regardless of whether a summary was
// ever computed for the room.
func (r *ContextSummaryRepository) DeleteByRoom(ctx context.Context, roomID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM message_context_summaries WHERE room_id = $1`, roomID)
	return err
}
