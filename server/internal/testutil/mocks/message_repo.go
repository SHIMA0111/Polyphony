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

	// ListByRoomErr, if non-nil, makes ListByRoom return it instead of a
	// page — for tests exercising the caller's handling of a context-fetch
	// failure (e.g. MessageUsecase.SendAIMessage's failed-AI-placeholder
	// path), without needing a real error condition inside this fake.
	ListByRoomErr error

	// CreateCallCount counts every Create invocation (1-indexed by the time
	// FailCreateOnCall is compared against it). FailCreateOnCall, if
	// positive, makes the Create call whose ordinal equals it return
	// FailCreateErr instead of persisting, leaving every other call
	// (before and after) to succeed normally — for tests exercising a
	// failure on one specific Create within a multi-Create flow (e.g.
	// MessageUsecase.SendAIMessage's completed-AI-message Create, which is
	// the second Create after the human message) without making every
	// Create fail.
	CreateCallCount  int
	FailCreateOnCall int
	FailCreateErr    error

	// InvalidateSummary, if set, is called by DeleteAndInvalidateSummary and
	// UpdateExcludeFromAIAndInvalidateSummary with the room ID, mirroring
	// the message_context_summaries/context_summary_revisions write
	// postgres.MessageRepository's combined methods perform directly via
	// SQL. Real code never wires one repository into another this way —
	// postgres.MessageRepository has no dependency on
	// ai.ContextSummaryRepository — this hook exists purely so a test can
	// point this fake and a mocks.ContextSummaryRepo at the same room (e.g.
	// `msgRepo.InvalidateSummary = summaryRepo.DeleteByRoom`) and observe
	// one call invalidate the other's cache, the way a single PostgreSQL
	// transaction would. It is nil by default, so tests that don't care
	// about summary invalidation are unaffected.
	InvalidateSummary func(ctx context.Context, roomID string) error
}

func (m *MessageRepo) ensureInit() {
	if m.Messages == nil {
		m.Messages = make(map[string]*message.Message)
	}
	if m.Seqs == nil {
		m.Seqs = make(map[string]int64)
	}
}

// Create persists a new message, unless this call's ordinal matches
// FailCreateOnCall, in which case it returns FailCreateErr without
// persisting (see FailCreateOnCall's doc comment).
func (m *MessageRepo) Create(_ context.Context, msg *message.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureInit()

	m.CreateCallCount++
	if m.FailCreateOnCall != 0 && m.CreateCallCount == m.FailCreateOnCall {
		return m.FailCreateErr
	}

	m.Messages[msg.ID] = msg
	return nil
}

// GetByID retrieves a message by ID. Returns domain.ErrNotFound if not
// present, or if present but invisible to requestingUserID (see
// visibleTo).
func (m *MessageRepo) GetByID(_ context.Context, id string, requestingUserID string) (*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg, ok := m.Messages[id]
	if !ok || !visibleTo(msg, requestingUserID) {
		return nil, domain.ErrNotFound
	}
	return msg, nil
}

// visibleTo reports whether msg is visible to requestingUserID: true for
// any message that is not MessageVisibilityPrivate (this also treats the
// zero value of Visibility — used by test fixtures that construct a
// *message.Message directly without setting it — as visible, mirroring
// production's NOT NULL DEFAULT 'public'), and for a private message, true
// only when requestingUserID matches SenderID.
func visibleTo(msg *message.Message, requestingUserID string) bool {
	if msg.Visibility != message.MessageVisibilityPrivate {
		return true
	}
	return msg.SenderID != nil && *msg.SenderID == requestingUserID
}

// ListByRoom returns messages in a room, ignoring the cursor (this fake does
// not implement true cursor-based pagination), ordered sequence-descending
// (newest first, mirroring postgres.MessageRepository.ListByRoom's
// ORDER BY sequence DESC -- callers such as
// MessageUsecase.assembleAIContext rely on this ordering to bucket the
// newest N entries as the "recent, always verbatim" tail), truncated to
// limit entries. Soft-deleted messages (IsDeleted == true) are excluded,
// mirroring the postgres.MessageRepository behavior. A private message not
// owned by requestingUserID is also excluded (see visibleTo).
func (m *MessageRepo) ListByRoom(_ context.Context, roomID, _ string, limit int, requestingUserID string) (*message.CursorPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ListByRoomErr != nil {
		return nil, m.ListByRoomErr
	}

	var msgs []*message.Message
	for _, msg := range m.Messages {
		if msg.RoomID == roomID && !msg.IsDeleted && visibleTo(msg, requestingUserID) {
			msgs = append(msgs, msg)
		}
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Sequence > msgs[j].Sequence
	})
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return &message.CursorPage{Messages: msgs}, nil
}

// ListByRoomUpTo returns up to limit messages in a room with sequence
// <= maxSequence, ordered sequence-descending (newest first, mirroring
// postgres.MessageRepository.ListByRoomUpTo -- see ListByRoom's doc comment
// for why this ordering matters to callers). Soft-deleted messages
// (IsDeleted == true) are excluded, mirroring the postgres.MessageRepository
// behavior. A private message not owned by requestingUserID is also
// excluded (see visibleTo).
func (m *MessageRepo) ListByRoomUpTo(_ context.Context, roomID string, maxSequence int64, limit int, requestingUserID string) ([]*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var msgs []*message.Message
	for _, msg := range m.Messages {
		if msg.RoomID == roomID && msg.Sequence <= maxSequence && !msg.IsDeleted && visibleTo(msg, requestingUserID) {
			msgs = append(msgs, msg)
		}
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Sequence > msgs[j].Sequence
	})
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

// DeleteAndInvalidateSummary soft-deletes a message by ID and invokes
// InvalidateSummary (if set) for roomID, mirroring the atomic
// postgres.MessageRepository implementation's all-or-nothing contract:
// returns domain.ErrNotFound without calling InvalidateSummary at all if the
// message does not exist or is already deleted (mirroring the real
// transaction's mutation statement, checked before the summary step ever
// runs), and — if InvalidateSummary is set and returns an error — returns
// that error with the message left NOT deleted (mirroring the real
// transaction rolling back both steps together).
func (m *MessageRepo) DeleteAndInvalidateSummary(ctx context.Context, messageID string, roomID string) error {
	m.mu.Lock()
	msg, ok := m.Messages[messageID]
	if !ok || msg.IsDeleted {
		m.mu.Unlock()
		return domain.ErrNotFound
	}
	m.mu.Unlock()

	if m.InvalidateSummary != nil {
		if err := m.InvalidateSummary(ctx, roomID); err != nil {
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	msg.IsDeleted = true
	msg.UpdatedAt = time.Now()
	return nil
}

// UpdateExcludeFromAIAndInvalidateSummary sets the ExcludeFromAI flag and
// invokes InvalidateSummary (if set) for roomID, mirroring the atomic
// postgres.MessageRepository implementation's all-or-nothing contract — see
// DeleteAndInvalidateSummary's doc comment for the exact ordering/failure
// semantics this mirrors.
func (m *MessageRepo) UpdateExcludeFromAIAndInvalidateSummary(ctx context.Context, messageID string, roomID string, exclude bool, updatedAt time.Time) error {
	m.mu.Lock()
	msg, ok := m.Messages[messageID]
	m.mu.Unlock()
	if !ok {
		return domain.ErrNotFound
	}

	if m.InvalidateSummary != nil {
		if err := m.InvalidateSummary(ctx, roomID); err != nil {
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ExcludeFromAI = exclude
	msg.UpdatedAt = updatedAt
	return nil
}

// CountAndMaxSequence returns the total number of messages in roomID and
// the highest sequence value currently assigned (0 if none), both read
// under the same mutex hold — mirroring postgres.MessageRepository's
// single-query atomicity.
func (m *MessageRepo) CountAndMaxSequence(_ context.Context, roomID string) (int64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total, maxSeq int64
	for _, msg := range m.Messages {
		if msg.RoomID != roomID {
			continue
		}
		total++
		if msg.Sequence > maxSeq {
			maxSeq = msg.Sequence
		}
	}
	return total, maxSeq, nil
}

// ListByRoomAfter returns up to limit messages in roomID with
// afterSequence < sequence <= maxSequence, ordered ascending by sequence,
// ignoring soft-delete/visibility/exclude-from-ai flags (mirroring the
// postgres.MessageRepository behavior this fake models).
func (m *MessageRepo) ListByRoomAfter(_ context.Context, roomID string, afterSequence int64, maxSequence int64, limit int) ([]*message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var msgs []*message.Message
	for _, msg := range m.Messages {
		if msg.RoomID == roomID && msg.Sequence > afterSequence && msg.Sequence <= maxSequence {
			msgs = append(msgs, msg)
		}
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Sequence < msgs[j].Sequence
	})
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

// CreateBatch persists every message in msgs. This fake does not model
// transactional rollback (it has no partial-failure mode to test against),
// mirroring the always-succeeds nature of the other mock write methods.
func (m *MessageRepo) CreateBatch(_ context.Context, msgs []*message.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureInit()

	for _, msg := range msgs {
		m.Messages[msg.ID] = msg
	}
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
