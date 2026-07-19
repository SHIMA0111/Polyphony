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

// MarkRunning transitions a Job from StatusPending to StatusRunning and
// records totalMessages. The UPDATE is scoped to status = 'pending', so a
// job that is not currently pending (already running, or terminal) is left
// untouched. It returns domain.ErrNotFound if the job does not exist or is
// not currently StatusPending.
func (r *RoomForkRepository) MarkRunning(ctx context.Context, id string, totalMessages int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, total_messages = $2, updated_at = NOW() WHERE id = $3 AND status = $4`,
		string(roomfork.StatusRunning), totalMessages, id, string(roomfork.StatusPending),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateProgress sets copiedMessages on a Job. The UPDATE is scoped to
// status = 'running', so a job that is not currently running (still
// pending, or already terminal) is left untouched. It returns
// domain.ErrNotFound if the job does not exist or is not currently
// StatusRunning.
func (r *RoomForkRepository) UpdateProgress(ctx context.Context, id string, copiedMessages int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET copied_messages = $1, updated_at = NOW() WHERE id = $2 AND status = $3`,
		copiedMessages, id, string(roomfork.StatusRunning),
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
// records errMsg. The UPDATE is scoped to status IN ('pending', 'running')
// — the only two non-terminal states a Job can fail from — so a job that
// has already reached a terminal state is left untouched. It returns
// domain.ErrNotFound if the job does not exist or has already reached a
// terminal state.
func (r *RoomForkRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3 AND status IN ($4, $5)`,
		string(roomfork.StatusFailed), errMsg, id, string(roomfork.StatusPending), string(roomfork.StatusRunning),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// CompleteAndUnarchive clears newRoomID's is_archived flag and transitions
// jobID to StatusCompleted within a single transaction — see
// roomfork.ForkJobRepository.CompleteAndUnarchive's GoDoc for why the two
// writes must land atomically. Either update affecting zero rows returns
// domain.ErrNotFound and rolls back the transaction, so a nonexistent job or
// room never leaves the other write applied on its own.
//
// The job update is the first write and is scoped by
// `id = $jobID AND new_room_id = $newRoomID AND status = 'running'`: it only
// ever performs the running->completed transition, and only for the job
// actually linked to newRoomID, rather than any job row matching id alone.
// This closes a caller/job mismatch (or a job already completed/failed)
// silently unarchiving a room it has no claim over. Validating/updating the
// job before touching the room also means a validation failure here leaves
// the room's is_archived flag untouched — the room write only runs once the
// job write is known to have succeeded.
func (r *RoomForkRepository) CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE room_fork_jobs SET status = $1, updated_at = NOW()
		 WHERE id = $2 AND new_room_id = $3 AND status = $4`,
		string(roomfork.StatusCompleted), jobID, newRoomID, string(roomfork.StatusRunning),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	tag, err = tx.Exec(ctx,
		`UPDATE rooms SET is_archived = false, updated_at = NOW() WHERE id = $1`,
		newRoomID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return tx.Commit(ctx)
}
