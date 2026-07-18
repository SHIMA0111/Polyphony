// Package postgres implements the domain repository ports against PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
)

// MessageRepository implements the message.MessageRepository interface using PostgreSQL.
type MessageRepository struct {
	pool *pgxpool.Pool
}

// NewMessageRepository creates a new MessageRepository backed by the given connection pool.
func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

// Create persists a new message to the database. is_deleted and
// exclude_from_ai always start false for a newly-created message.
// msg.Visibility is persisted as-is (the caller — MessageUsecase — always
// sets it explicitly to MessageVisibilityPublic or MessageVisibilityPrivate;
// the column has a NOT NULL DEFAULT 'public' at the schema level as a
// belt-and-suspenders default for any row inserted outside that path).
func (r *MessageRepository) Create(ctx context.Context, msg *message.Message) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO messages (id, room_id, sender_id, content, type, status, sequence, in_response_to_message_id, is_deleted, exclude_from_ai, visibility, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		msg.ID, msg.RoomID, msg.SenderID, msg.Content, string(msg.Type), string(msg.Status), msg.Sequence, msg.InResponseToMessageID, false, false, string(msg.Visibility), msg.CreatedAt, msg.UpdatedAt,
	)
	return err
}

// scanMessage scans a message row into a Message struct.
func scanMessage(scanner interface{ Scan(dest ...any) error }) (*message.Message, error) {
	var msg message.Message
	var msgType, status, visibility string
	err := scanner.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msgType, &status, &msg.Sequence, &msg.InResponseToMessageID, &msg.IsDeleted, &msg.ExcludeFromAI, &visibility, &msg.CreatedAt, &msg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	msg.Type = message.MessageType(msgType)
	msg.Status = message.MessageStatus(status)
	msg.Visibility = message.MessageVisibility(visibility)
	return &msg, nil
}

const messageColumns = `id, room_id, sender_id, content, type, status, sequence, in_response_to_message_id, is_deleted, exclude_from_ai, visibility, created_at, updated_at`

// visibilityFilter is the SQL predicate applied to every read path
// (GetByID, ListByRoom, ListByRoomUpTo) so a message with
// visibility = 'private' is returned only to its own sender: for every
// other requestingUserID it is excluded exactly as if the row did not
// exist. paramIndex is the 1-based positional parameter number to bind
// requestingUserID to in the enclosing query.
func visibilityFilter(paramIndex int) string {
	return fmt.Sprintf("(visibility = 'public' OR sender_id = $%d)", paramIndex)
}

// GetByID retrieves a message by its unique identifier. It returns
// domain.ErrNotFound if the message does not exist. requestingUserID
// controls visibility: a private message is returned only when
// requestingUserID is its sender; otherwise this returns domain.ErrNotFound,
// identical to a genuinely missing row, so a private message is never
// distinguishable from a nonexistent one to a non-owner.
func (r *MessageRepository) GetByID(ctx context.Context, id string, requestingUserID string) (*message.Message, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE id = $1 AND `+visibilityFilter(2), id, requestingUserID,
	)
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return msg, nil
}

// ListByRoom returns messages for a room using cursor-based pagination, ordered newest first.
// The cursor is a message ID; if empty, fetching starts from the most recent message.
// It returns a CursorPage containing up to limit messages and a next cursor if more pages exist.
// requestingUserID controls visibility: a private message whose sender is
// not requestingUserID is excluded from the page entirely, as if it does
// not exist — it is invisible to every user other than its owner.
func (r *MessageRepository) ListByRoom(ctx context.Context, roomID string, cursor string, limit int, requestingUserID string) (*message.CursorPage, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND is_deleted = false AND `+visibilityFilter(3)+`
			 ORDER BY sequence DESC LIMIT $2`,
			roomID, limit+1, requestingUserID,
		)
	} else {
		// Get cursor message's sequence. The same visibilityFilter as the
		// surrounding list queries applies here so a cursor pointing at
		// another user's private message resolves identically to an
		// unknown cursor, rather than leaking that a private message with
		// that ID exists.
		var cursorSeq int64
		err = r.pool.QueryRow(ctx,
			`SELECT sequence FROM messages WHERE id = $1 AND room_id = $2 AND `+visibilityFilter(3), cursor, roomID, requestingUserID,
		).Scan(&cursorSeq)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("invalid cursor: %w", domain.ErrNotFound)
			}
			return nil, err
		}

		rows, err = r.pool.Query(ctx,
			`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND sequence < $2 AND is_deleted = false AND `+visibilityFilter(4)+`
			 ORDER BY sequence DESC LIMIT $3`,
			roomID, cursorSeq, limit+1, requestingUserID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*message.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	page := &message.CursorPage{}

	if len(messages) > limit {
		// There are more pages
		messages = messages[:limit]
		lastID := messages[len(messages)-1].ID
		page.NextCursor = &lastID
	}

	page.Messages = messages
	return page, nil
}

// ListByRoomUpTo returns up to limit messages with sequence less than or
// equal to maxSequence, ordered newest first. requestingUserID controls
// visibility: a private message whose sender is not requestingUserID is
// excluded, as if it does not exist — this is what keeps another user's
// private exchange out of AI context assembled for a different requester.
func (r *MessageRepository) ListByRoomUpTo(ctx context.Context, roomID string, maxSequence int64, limit int, requestingUserID string) ([]*message.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND sequence <= $2 AND is_deleted = false AND `+visibilityFilter(4)+`
		 ORDER BY sequence DESC LIMIT $3`,
		roomID, maxSequence, limit, requestingUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*message.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

// GetNextInRoom returns the message with the smallest sequence greater than afterSequence.
// It returns domain.ErrNotFound if no subsequent message exists.
func (r *MessageRepository) GetNextInRoom(ctx context.Context, roomID string, afterSequence int64) (*message.Message, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND sequence > $2
		 ORDER BY sequence ASC LIMIT 1`,
		roomID, afterSequence,
	)
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return msg, nil
}

// UpdateAIResponse updates the content, status, and updated_at fields of an AI message.
// It returns domain.ErrNotFound if the message does not exist.
func (r *MessageRepository) UpdateAIResponse(ctx context.Context, id string, content string, status message.MessageStatus, updatedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE messages SET content = $1, status = $2, updated_at = $3 WHERE id = $4`,
		content, string(status), updatedAt, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateExcludeFromAI sets the exclude_from_ai flag and updated_at of a
// message. It returns domain.ErrNotFound if the message does not exist.
func (r *MessageRepository) UpdateExcludeFromAI(ctx context.Context, id string, exclude bool, updatedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE messages SET exclude_from_ai = $1, updated_at = $2 WHERE id = $3`,
		exclude, updatedAt, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete soft-deletes a message by its unique identifier: it sets
// is_deleted = true and updated_at = NOW() rather than physically removing
// the row, so a soft-deleted message remains fetchable via GetByID but is
// excluded from ListByRoom, ListByRoomUpTo, and AI context assembly. It
// returns domain.ErrNotFound if the message does not exist or was already
// deleted (RowsAffected() == 0 covers both cases, so a client double-deleting
// sees a 404 rather than a silent no-op success).
func (r *MessageRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE messages SET is_deleted = true, updated_at = NOW() WHERE id = $1 AND is_deleted = false`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteAndInvalidateSummary atomically soft-deletes messageID and
// invalidates roomID's cached AI context summary in a single transaction —
// see message.MessageRepository.DeleteAndInvalidateSummary's doc comment for
// why this must be one atomic operation rather than a separate
// ContextSummaryRepository.DeleteByRoom call plus this repository's own
// Delete.
//
// The transaction: (1) acquires the same room-scoped
// pg_advisory_xact_lock(hashtext(room_id)) that
// ContextSummaryRepository.Upsert/DeleteByRoom take, so this can never
// interleave with a concurrent summarization's cache write for the same
// room; (2) runs the identical soft-delete UPDATE Delete uses, returning
// domain.ErrNotFound (and rolling back without touching the summary tables
// at all) if it affects zero rows; (3) runs the identical
// DELETE-then-upsert-revision statement
// ContextSummaryRepository.DeleteByRoom uses against
// message_context_summaries/context_summary_revisions. Only once all three
// steps succeed is the transaction committed, so a failure at any point
// leaves every table exactly as it was before this call.
func (r *MessageRepository) DeleteAndInvalidateSummary(ctx context.Context, messageID string, roomID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, roomID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE messages SET is_deleted = true, updated_at = NOW() WHERE id = $1 AND is_deleted = false`, messageID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
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

// UpdateExcludeFromAIAndInvalidateSummary atomically toggles messageID's
// exclude_from_ai flag and invalidates roomID's cached AI context summary in
// a single transaction — see
// message.MessageRepository.UpdateExcludeFromAIAndInvalidateSummary's doc
// comment for why this must be one atomic operation rather than a separate
// ContextSummaryRepository.DeleteByRoom call plus this repository's own
// UpdateExcludeFromAI. Structurally identical to DeleteAndInvalidateSummary
// (see its doc comment for the full three-step transaction breakdown), with
// the exclude_from_ai UPDATE in place of the soft-delete one.
func (r *MessageRepository) UpdateExcludeFromAIAndInvalidateSummary(ctx context.Context, messageID string, roomID string, exclude bool, updatedAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, roomID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE messages SET exclude_from_ai = $1, updated_at = $2 WHERE id = $3`,
		exclude, updatedAt, messageID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
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

// CountAndMaxSequence returns, in a single query, the total number of
// messages in roomID (including soft-deleted and private ones — a
// structural count, not a visibility-filtered read) and the highest
// sequence value currently assigned in roomID (0 if the room has no
// messages) — see message.MessageRepository.CountAndMaxSequence. Computing
// both in one query, rather than as two separate round trips, is what makes
// the returned (total, maxSeq) pair a consistent snapshot: no message
// insert can land between the count and the max being read.
func (r *MessageRepository) CountAndMaxSequence(ctx context.Context, roomID string) (int64, int64, error) {
	var total, maxSeq int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MAX(sequence), 0) FROM messages WHERE room_id = $1`, roomID,
	).Scan(&total, &maxSeq)
	if err != nil {
		return 0, 0, err
	}
	return total, maxSeq, nil
}

// ListByRoomAfter returns up to limit messages in roomID with
// afterSequence < sequence <= maxSequence, ordered ascending by sequence
// (oldest first) — see message.MessageRepository.ListByRoomAfter. Like
// CountAndMaxSequence, it ignores soft-delete/visibility/exclude-from-ai
// flags: a room fork copies the room's entire, unfiltered history up to
// the frozen maxSequence boundary its caller passes.
func (r *MessageRepository) ListByRoomAfter(ctx context.Context, roomID string, afterSequence int64, maxSequence int64, limit int) ([]*message.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND sequence > $2 AND sequence <= $3
		 ORDER BY sequence ASC LIMIT $4`,
		roomID, afterSequence, maxSequence, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*message.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

// CreateBatch persists msgs within a single transaction, executing the same
// INSERT statement Create uses once per message and committing once at the
// end, so a failure partway through leaves no partially-copied batch
// persisted — see message.MessageRepository.CreateBatch.
func (r *MessageRepository) CreateBatch(ctx context.Context, msgs []*message.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	for _, msg := range msgs {
		_, err = tx.Exec(ctx,
			`INSERT INTO messages (id, room_id, sender_id, content, type, status, sequence, in_response_to_message_id, is_deleted, exclude_from_ai, visibility, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			msg.ID, msg.RoomID, msg.SenderID, msg.Content, string(msg.Type), string(msg.Status), msg.Sequence, msg.InResponseToMessageID, msg.IsDeleted, msg.ExcludeFromAI, string(msg.Visibility), msg.CreatedAt, msg.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// ReserveSequenceRange atomically reserves count contiguous sequence numbers
// for a room and returns the first one; the caller owns [first, first+count).
// It uses a single UPDATE ... RETURNING statement, so the read-modify-write
// is performed atomically by PostgreSQL and is safe under concurrent callers
// racing on the same room (see the row-level lock a single-statement UPDATE
// implicitly takes). It returns domain.ErrNotFound if the room has no
// sequence counter entry.
func (r *MessageRepository) ReserveSequenceRange(ctx context.Context, roomID string, count int64) (int64, error) {
	var first int64
	err := r.pool.QueryRow(ctx,
		`UPDATE room_sequences SET next_sequence = next_sequence + $2
		 WHERE room_id = $1
		 RETURNING next_sequence - $2`,
		roomID, count,
	).Scan(&first)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	return first, nil
}
