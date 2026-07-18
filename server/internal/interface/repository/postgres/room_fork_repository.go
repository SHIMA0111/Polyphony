package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
)

// RoomForkRepository implements the roomfork.ForkJobRepository interface
// using PostgreSQL, persisting Job rows in room_fork_jobs.
type RoomForkRepository struct {
	pool *pgxpool.Pool
}

// NewRoomForkRepository creates a new RoomForkRepository backed by the
// given connection pool.
func NewRoomForkRepository(pool *pgxpool.Pool) *RoomForkRepository {
	return &RoomForkRepository{pool: pool}
}

// roomForkJobColumns lists room_fork_jobs' columns in the fixed order every
// SELECT/scan in this file relies on.
const roomForkJobColumns = `id, source_room_id, new_room_id, status, total_messages, copied_messages, error_message, created_at, updated_at`

// scanForkJob scans a room_fork_jobs row into a roomfork.Job struct.
func scanForkJob(scanner interface{ Scan(dest ...any) error }) (*roomfork.Job, error) {
	var job roomfork.Job
	var status string
	err := scanner.Scan(&job.ID, &job.SourceRoomID, &job.NewRoomID, &status, &job.TotalMessages, &job.CopiedMessages, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	job.Status = roomfork.Status(status)
	return &job, nil
}

// Create persists a new Job row.
func (r *RoomForkRepository) Create(ctx context.Context, job *roomfork.Job) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO room_fork_jobs (id, source_room_id, new_room_id, status, total_messages, copied_messages, error_message, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		job.ID, job.SourceRoomID, job.NewRoomID, string(job.Status), job.TotalMessages, job.CopiedMessages, job.ErrorMessage, job.CreatedAt, job.UpdatedAt,
	)
	return err
}

// GetByID retrieves a Job by its unique identifier. It returns
// domain.ErrNotFound if it does not exist.
func (r *RoomForkRepository) GetByID(ctx context.Context, id string) (*roomfork.Job, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+roomForkJobColumns+` FROM room_fork_jobs WHERE id = $1`, id)
	job, err := scanForkJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return job, nil
}

// MarkRunning transitions a Job to StatusRunning and records
// totalMessages. It returns domain.ErrNotFound if the job does not exist.
func (r *RoomForkRepository) MarkRunning(ctx context.Context, id string, totalMessages int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, total_messages = $2, updated_at = NOW() WHERE id = $3`,
		string(roomfork.StatusRunning), totalMessages, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateProgress sets copiedMessages on a Job. It returns
// domain.ErrNotFound if the job does not exist.
func (r *RoomForkRepository) UpdateProgress(ctx context.Context, id string, copiedMessages int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET copied_messages = $1, updated_at = NOW() WHERE id = $2`,
		copiedMessages, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkCompleted transitions a Job to the terminal StatusCompleted state.
// It returns domain.ErrNotFound if the job does not exist.
func (r *RoomForkRepository) MarkCompleted(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, updated_at = NOW() WHERE id = $2`,
		string(roomfork.StatusCompleted), id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkFailed transitions a Job to the terminal StatusFailed state and
// records errMsg. It returns domain.ErrNotFound if the job does not exist.
func (r *RoomForkRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
		string(roomfork.StatusFailed), errMsg, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// CompleteAndUnarchive transitions a Job to the terminal StatusCompleted
// state and clears newRoomID's rooms.is_archived flag within a single pgx
// transaction — see roomfork.ForkJobRepository.CompleteAndUnarchive. Both
// statements run against the same *pgxpool.Pool this repository already
// holds (RoomForkRepository and RoomRepository share one underlying
// database, even though they sit behind separate domain repository
// interfaces), so no cross-repository coordination is needed to make the
// two writes atomic. It returns domain.ErrNotFound if either the rooms
// UPDATE or the room_fork_jobs UPDATE affects zero rows, rolling back
// whichever of the two (if any) had already been applied in this
// transaction.
func (r *RoomForkRepository) CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	roomTag, err := tx.Exec(ctx,
		`UPDATE rooms SET is_archived = false, updated_at = NOW() WHERE id = $1`, newRoomID,
	)
	if err != nil {
		return err
	}
	if roomTag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	jobTag, err := tx.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, updated_at = NOW() WHERE id = $2`,
		string(roomfork.StatusCompleted), jobID,
	)
	if err != nil {
		return err
	}
	if jobTag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return tx.Commit(ctx)
}
