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
	// requestingUserID controls visibility: a message with
	// Visibility == MessageVisibilityPrivate is returned only when
	// requestingUserID matches its SenderID; for every other caller it is
	// treated exactly as if it does not exist (ErrNotFound), never
	// distinguished from a genuinely missing row.
	GetByID(ctx context.Context, id string, requestingUserID string) (*Message, error)

	// ListByRoom returns messages in a room using cursor-based pagination.
	// Messages are ordered by sequence descending (newest first).
	// If the cursor is empty, starts from the most recent message.
	// requestingUserID controls visibility: a message with
	// Visibility == MessageVisibilityPrivate whose SenderID does not match
	// requestingUserID is excluded from the page entirely, as if it does
	// not exist — it is invisible to every user other than its owner.
	ListByRoom(ctx context.Context, roomID string, cursor string, limit int, requestingUserID string) (*CursorPage, error)

	// ListByRoomUpTo returns up to limit messages in a room with sequence <= maxSequence,
	// ordered by sequence descending (newest first).
	// Used to build AI context up to a specific message. requestingUserID
	// controls visibility: a message with Visibility == MessageVisibilityPrivate
	// whose SenderID does not match requestingUserID is excluded, as if it
	// does not exist — it is invisible to every user other than its owner,
	// including when another user's subsequent AI request assembles context.
	ListByRoomUpTo(ctx context.Context, roomID string, maxSequence int64, limit int, requestingUserID string) ([]*Message, error)

	// GetNextInRoom returns the message with the smallest sequence greater than
	// afterSequence in the given room. Returns ErrNotFound if no such message exists.
	GetNextInRoom(ctx context.Context, roomID string, afterSequence int64) (*Message, error)

	// UpdateAIResponse updates the content, status, and updated_at of an AI message.
	UpdateAIResponse(ctx context.Context, id string, content string, status MessageStatus, updatedAt time.Time) error

	// UpdateExcludeFromAI sets the exclude_from_ai flag and updated_at of a
	// message. It follows the same signature shape as UpdateAIResponse.
	// Returns ErrNotFound if the message does not exist.
	UpdateExcludeFromAI(ctx context.Context, id string, exclude bool, updatedAt time.Time) error

	// Delete soft-deletes a message by ID: it sets is_deleted = true (and
	// updates updated_at) rather than physically removing the row. A
	// soft-deleted message is excluded from ListByRoom, ListByRoomUpTo, and
	// AI context assembly, but remains fetchable via GetByID. Returns
	// ErrNotFound if the message does not exist or is already deleted.
	Delete(ctx context.Context, id string) error

	// ReserveSequenceRange atomically reserves count contiguous sequence
	// numbers for the given room and returns the first one; the caller owns
	// the whole range [first, first+count). This lets a caller that needs to
	// persist multiple related messages (e.g. a human message and its AI
	// response) allocate every sequence number they need in a single atomic
	// step, so no other message can be interleaved between them. Returns
	// ErrNotFound if the room has no sequence counter row.
	ReserveSequenceRange(ctx context.Context, roomID string, count int64) (int64, error)
}
