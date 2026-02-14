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

// MessageRepository implements message.MessageRepository using PostgreSQL.
type MessageRepository struct {
	pool *pgxpool.Pool
}

// NewMessageRepository creates a new MessageRepository.
func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

// Create persists a new message.
func (r *MessageRepository) Create(ctx context.Context, msg *message.Message) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO messages (id, room_id, sender_id, content, type, status, sequence, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		msg.ID, msg.RoomID, msg.SenderID, msg.Content, string(msg.Type), string(msg.Status), msg.Sequence, msg.CreatedAt, msg.UpdatedAt,
	)
	return err
}

// scanMessage scans a message row into a Message struct.
func scanMessage(scanner interface{ Scan(dest ...any) error }) (*message.Message, error) {
	var msg message.Message
	var msgType, status string
	err := scanner.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msgType, &status, &msg.Sequence, &msg.CreatedAt, &msg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	msg.Type = message.MessageType(msgType)
	msg.Status = message.MessageStatus(status)
	return &msg, nil
}

const messageColumns = `id, room_id, sender_id, content, type, status, sequence, created_at, updated_at`

// GetByID retrieves a message by ID.
func (r *MessageRepository) GetByID(ctx context.Context, id string) (*message.Message, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE id = $1`, id,
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

// ListByRoom returns messages using cursor-based pagination (newest first).
// Cursor is a message ID; if empty, starts from the most recent.
func (r *MessageRepository) ListByRoom(ctx context.Context, roomID string, cursor string, limit int) (*message.CursorPage, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+messageColumns+` FROM messages WHERE room_id = $1
			 ORDER BY sequence DESC LIMIT $2`,
			roomID, limit+1,
		)
	} else {
		// Get cursor message's sequence
		var cursorSeq int64
		err = r.pool.QueryRow(ctx,
			`SELECT sequence FROM messages WHERE id = $1 AND room_id = $2`, cursor, roomID,
		).Scan(&cursorSeq)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("invalid cursor: %w", domain.ErrNotFound)
			}
			return nil, err
		}

		rows, err = r.pool.Query(ctx,
			`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND sequence < $2
			 ORDER BY sequence DESC LIMIT $3`,
			roomID, cursorSeq, limit+1,
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

// ListByRoomUpTo returns up to limit messages with sequence <= maxSequence.
func (r *MessageRepository) ListByRoomUpTo(ctx context.Context, roomID string, maxSequence int64, limit int) ([]*message.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE room_id = $1 AND sequence <= $2
		 ORDER BY sequence DESC LIMIT $3`,
		roomID, maxSequence, limit,
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

// UpdateAIResponse updates the content, status, and updated_at of an AI message.
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

// Delete removes a message by ID.
func (r *MessageRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetNextSequence atomically allocates the next sequence number for a room.
func (r *MessageRepository) GetNextSequence(ctx context.Context, roomID string) (int64, error) {
	var seq int64
	err := r.pool.QueryRow(ctx,
		`UPDATE room_sequences SET next_sequence = next_sequence + 1
		 WHERE room_id = $1
		 RETURNING next_sequence - 1`,
		roomID,
	).Scan(&seq)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	return seq, nil
}
