package postgres

import (
	"context"
	"errors"
	"fmt"

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
		`INSERT INTO messages (id, room_id, sender_id, content, type, sequence, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		msg.ID, msg.RoomID, msg.SenderID, msg.Content, string(msg.Type), msg.Sequence, msg.CreatedAt, msg.UpdatedAt,
	)
	return err
}

// GetByID retrieves a message by ID.
func (r *MessageRepository) GetByID(ctx context.Context, id string) (*message.Message, error) {
	var msg message.Message
	var msgType string
	err := r.pool.QueryRow(ctx,
		`SELECT id, room_id, sender_id, content, type, sequence, created_at, updated_at
		 FROM messages WHERE id = $1`, id,
	).Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msgType, &msg.Sequence, &msg.CreatedAt, &msg.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	msg.Type = message.MessageType(msgType)
	return &msg, nil
}

// ListByRoom returns messages using cursor-based pagination (newest first).
// Cursor is a message ID; if empty, starts from the most recent.
func (r *MessageRepository) ListByRoom(ctx context.Context, roomID string, cursor string, limit int) (*message.CursorPage, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id, room_id, sender_id, content, type, sequence, created_at, updated_at
			 FROM messages WHERE room_id = $1
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
			`SELECT id, room_id, sender_id, content, type, sequence, created_at, updated_at
			 FROM messages WHERE room_id = $1 AND sequence < $2
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
		var msg message.Message
		var msgType string
		if err := rows.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.Content, &msgType, &msg.Sequence, &msg.CreatedAt, &msg.UpdatedAt); err != nil {
			return nil, err
		}
		msg.Type = message.MessageType(msgType)
		messages = append(messages, &msg)
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
