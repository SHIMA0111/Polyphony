package mocks

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
)

// MessageRepo is an in-memory, map-backed fake implementing
// message.MessageRepository, with a per-room sequence counter mirroring the
// atomic allocation behavior of
// postgres.MessageRepository.ReserveSequenceRange. The zero value
// (mocks.MessageRepo{}) is ready to use; all maps are initialized lazily on
// first write.
//
// MessageRepo is safe for concurrent use.
type MessageRepo struct {
	mu       sync.Mutex
	Messages map[string]*message.Message
	Seqs     map[string]int64 // roomID -> next sequence to allocate
}

func (m *MessageRepo) ensureInit() {
	if m.Messages == nil {
		m.Messages = make(map[string]*message.Message)
	}
	if m.Seqs == nil {
		m.Seqs = make(map[string]int64)
	}
}

// Create persists a new message.
func (m *MessageRepo) Create(_ context.Context, msg *message.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureInit()

	m.Messages[msg.ID] = msg
	return nil
}

// GetByID retrieves a message by ID. Returns domain.ErrNotFound if not
// present.
func (m *MessageRepo) GetByID(_ context.Context, id string) (*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg, ok := m.Messages[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return msg, nil
}

// ListByRoom returns messages in a room, ignoring the cursor (this fake does
// not implement true cursor-based pagination), truncated to limit entries.
// Soft-deleted messages (IsDeleted == true) are excluded, mirroring the
// postgres.MessageRepository behavior.
func (m *MessageRepo) ListByRoom(_ context.Context, roomID, _ string, limit int) (*message.CursorPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var msgs []*message.Message
	for _, msg := range m.Messages {
		if msg.RoomID == roomID && !msg.IsDeleted {
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return &message.CursorPage{Messages: msgs}, nil
}

// ListByRoomUpTo returns up to limit messages in a room with sequence
// <= maxSequence. Soft-deleted messages (IsDeleted == true) are excluded,
// mirroring the postgres.MessageRepository behavior.
func (m *MessageRepo) ListByRoomUpTo(_ context.Context, roomID string, maxSequence int64, limit int) ([]*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var msgs []*message.Message
	for _, msg := range m.Messages {
		if msg.RoomID == roomID && msg.Sequence <= maxSequence && !msg.IsDeleted {
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

// GetNextInRoom returns the message with the smallest sequence greater than
// afterSequence in the given room. Returns domain.ErrNotFound if no such
// message exists.
func (m *MessageRepo) GetNextInRoom(_ context.Context, roomID string, afterSequence int64) (*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var candidates []*message.Message
	for _, msg := range m.Messages {
		if msg.RoomID == roomID && msg.Sequence > afterSequence {
			candidates = append(candidates, msg)
		}
	}
	if len(candidates) == 0 {
		return nil, domain.ErrNotFound
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Sequence < candidates[j].Sequence
	})
	return candidates[0], nil
}

// UpdateAIResponse updates the content, status, and updated_at of an AI
// message. Returns domain.ErrNotFound if the message does not exist.
func (m *MessageRepo) UpdateAIResponse(_ context.Context, id string, content string, status message.MessageStatus, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg, ok := m.Messages[id]
	if !ok {
		return domain.ErrNotFound
	}
	msg.Content = content
	msg.Status = status
	msg.UpdatedAt = updatedAt
	return nil
}

// UpdateExcludeFromAI sets the ExcludeFromAI flag and UpdatedAt of a
// message. Returns domain.ErrNotFound if the message does not exist.
func (m *MessageRepo) UpdateExcludeFromAI(_ context.Context, id string, exclude bool, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg, ok := m.Messages[id]
	if !ok {
		return domain.ErrNotFound
	}
	msg.ExcludeFromAI = exclude
	msg.UpdatedAt = updatedAt
	return nil
}

// Delete soft-deletes a message by ID, setting IsDeleted rather than
// removing it from Messages, mirroring postgres.MessageRepository.Delete.
// Returns domain.ErrNotFound if the message does not exist or is already
// deleted.
func (m *MessageRepo) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg, ok := m.Messages[id]
	if !ok || msg.IsDeleted {
		return domain.ErrNotFound
	}
	msg.IsDeleted = true
	msg.UpdatedAt = time.Now()
	return nil
}

// ReserveSequenceRange atomically reserves count contiguous sequence numbers
// for the given room, starting at 1, and returns the first one; the caller
// owns [first, first+count).
func (m *MessageRepo) ReserveSequenceRange(_ context.Context, roomID string, count int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureInit()

	first := m.Seqs[roomID]
	if first == 0 {
		first = 1
	}
	m.Seqs[roomID] = first + count
	return first, nil
}
