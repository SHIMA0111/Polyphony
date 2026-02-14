package message

import (
	"context"
	"time"
)

// MessageRepository defines persistence operations for messages.
type MessageRepository interface {
	// Create persists a new message.
	Create(ctx context.Context, msg *Message) error

	// GetByID retrieves a message by ID. Returns ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*Message, error)

	// ListByRoom returns messages in a room using cursor-based pagination.
	// Messages are ordered by sequence descending (newest first).
	// If the cursor is empty, starts from the most recent message.
	ListByRoom(ctx context.Context, roomID string, cursor string, limit int) (*CursorPage, error)

	// ListByRoomUpTo returns up to limit messages in a room with sequence <= maxSequence,
	// ordered by sequence descending (newest first).
	// Used to build AI context up to a specific message.
	ListByRoomUpTo(ctx context.Context, roomID string, maxSequence int64, limit int) ([]*Message, error)

	// GetNextInRoom returns the message with the smallest sequence greater than
	// afterSequence in the given room. Returns ErrNotFound if no such message exists.
	GetNextInRoom(ctx context.Context, roomID string, afterSequence int64) (*Message, error)

	// UpdateAIResponse updates the content, status, and updated_at of an AI message.
	UpdateAIResponse(ctx context.Context, id string, content string, status MessageStatus, updatedAt time.Time) error

	// Delete removes a message by ID. Returns ErrNotFound if not found.
	Delete(ctx context.Context, id string) error

	// GetNextSequence atomically allocates and returns the next sequence
	// number for the given room.
	GetNextSequence(ctx context.Context, roomID string) (int64, error)
}
