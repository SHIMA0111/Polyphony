package message

import "context"

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

	// Delete removes a message by ID. Returns ErrNotFound if not found.
	Delete(ctx context.Context, id string) error

	// GetNextSequence atomically allocates and returns the next sequence
	// number for the given room.
	GetNextSequence(ctx context.Context, roomID string) (int64, error)
}
