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
//
// CreateFunc, if set, overrides Create entirely, taking priority over the
// default in-memory-store behavior; it is invoked while holding mu, so
// implementations must not call back into MessageRepo. Use it to simulate a
// Create failure (optionally on only a specific call, e.g. by counting
// invocations in the closure) without affecting the fake's other methods.
type MessageRepo struct {
	mu sync.Mutex
	// Messages is the backing store of messages, keyed by message ID;
	// access only while holding mu.
	Messages map[string]*message.Message
	// Seqs is the per-room next-sequence counter, keyed by room ID; access
	// only while holding mu.
	Seqs map[string]int64 // roomID -> next sequence to allocate
	// CreateFunc, if set, overrides Create entirely. See the type doc
	// comment above.
	CreateFunc func(ctx context.Context, msg *message.Message) error
	// ListByRoomErr, if non-nil, is returned by every ListByRoom call
	// instead of its default in-memory-store behavior, simulating a
	// context-fetch failure (e.g. a database error) independent of the
	// fake's other methods.
	ListByRoomErr error
	// DeleteAndInvalidateSummaryErr, if non-nil, makes
	// DeleteAndInvalidateSummary return it instead of succeeding, without
	// soft-deleting the message -- mirroring
	// postgres.MessageRepository.DeleteAndInvalidateSummary's single-
	// transaction all-or-nothing outcome when the summary-invalidation half
	// of it fails (e.g. a simulated connection error), for tests exercising
	// callers' handling of that failure (MessageUsecase.DeleteMessage
	// propagates it; see its doc comment).
	DeleteAndInvalidateSummaryErr error
	// UpdateExcludeFromAIAndInvalidateSummaryErr is
	// DeleteAndInvalidateSummaryErr's counterpart for
	// UpdateExcludeFromAIAndInvalidateSummary (MessageUsecase.
	// SetExcludeFromAI propagates it; see its doc comment).
	UpdateExcludeFromAIAndInvalidateSummaryErr error
	// SummaryRepo, if set, is invoked by DeleteAndInvalidateSummary/
	// UpdateExcludeFromAIAndInvalidateSummary to actually invalidate the
	// room's cached summary, mirroring how postgres.MessageRepository's
	// combined methods reach into the same tables
	// ContextSummaryRepository.DeleteByRoom writes to. This fake otherwise
	// has no cached-summary state of its own to keep in sync, so wiring a
	// *ContextSummaryRepo here (as PaymentRepo wires a *BalanceRepo) is what
	// lets a test observe, through that same summaryRepo instance, that a
	// DeleteMessage/SetExcludeFromAI call actually invalidated the cache --
	// see TestAssembleAIContextInvalidationForcesFreshSummary. If unset
	// (the common case, since most tests don't assert on cache-invalidation
	// side effects), the combined methods only mutate the message. If
	// SummaryRepo.DeleteByRoom returns an error, the combined method rolls
	// back its own message mutation and returns that error, preserving the
	// all-or-nothing contract end to end.
	SummaryRepo *ContextSummaryRepo
}

func (m *MessageRepo) ensureInit() {
	if m.Messages == nil {
		m.Messages = make(map[string]*message.Message)
	}
	if m.Seqs == nil {
		m.Seqs = make(map[string]int64)
	}
}

// Create persists a new message, or delegates to CreateFunc if set (see the
// type doc comment).
func (m *MessageRepo) Create(ctx context.Context, msg *message.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureInit()

	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, msg)
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
// owned by requestingUserID is also excluded (see visibleTo). If
// ListByRoomErr is non-nil, it is returned immediately instead (see the type
// doc comment).
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

// UpdateExcludeFromAIAndInvalidateSummary is UpdateExcludeFromAI's
// all-or-nothing counterpart, mirroring
// postgres.MessageRepository.UpdateExcludeFromAIAndInvalidateSummary.
// Existence is checked before UpdateExcludeFromAIAndInvalidateSummaryErr so
// a not-found message never masks (or is masked by) an injected
// invalidation failure. If UpdateExcludeFromAIAndInvalidateSummaryErr is
// set, it is returned and the flag is left untouched. Otherwise, if
// SummaryRepo is wired (see its doc comment), this calls its DeleteByRoom
// to actually invalidate roomID's cached summary; a failure there rolls
// back the flag toggle and returns that error, so a caller observing this
// fake sees the same all-or-nothing outcome the real transaction gives.
func (m *MessageRepo) UpdateExcludeFromAIAndInvalidateSummary(ctx context.Context, id string, exclude bool, roomID string) error {
	m.mu.Lock()
	msg, ok := m.Messages[id]
	if !ok {
		m.mu.Unlock()
		return domain.ErrNotFound
	}
	if m.UpdateExcludeFromAIAndInvalidateSummaryErr != nil {
		m.mu.Unlock()
		return m.UpdateExcludeFromAIAndInvalidateSummaryErr
	}
	prevExclude, prevUpdatedAt := msg.ExcludeFromAI, msg.UpdatedAt
	msg.ExcludeFromAI = exclude
	msg.UpdatedAt = time.Now()
	summaryRepo := m.SummaryRepo
	m.mu.Unlock()

	if summaryRepo == nil {
		return nil
	}
	if err := summaryRepo.DeleteByRoom(ctx, roomID); err != nil {
		m.mu.Lock()
		msg.ExcludeFromAI, msg.UpdatedAt = prevExclude, prevUpdatedAt
		m.mu.Unlock()
		return err
	}
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

// DeleteAndInvalidateSummary is Delete's all-or-nothing counterpart,
// mirroring postgres.MessageRepository.DeleteAndInvalidateSummary: see
// UpdateExcludeFromAIAndInvalidateSummary's doc comment for the SummaryRepo
// wiring this uses to invalidate roomID's cached summary. Existence is
// checked before DeleteAndInvalidateSummaryErr so a not-found message never
// masks (or is masked by) an injected invalidation failure. If
// DeleteAndInvalidateSummaryErr is set, it is returned and the message is
// left NOT deleted. Otherwise, if SummaryRepo.DeleteByRoom fails, the soft
// delete is rolled back and that error is returned, preserving the
// all-or-nothing contract.
func (m *MessageRepo) DeleteAndInvalidateSummary(ctx context.Context, id string, roomID string) error {
	m.mu.Lock()
	msg, ok := m.Messages[id]
	if !ok || msg.IsDeleted {
		m.mu.Unlock()
		return domain.ErrNotFound
	}
	if m.DeleteAndInvalidateSummaryErr != nil {
		m.mu.Unlock()
		return m.DeleteAndInvalidateSummaryErr
	}
	prevDeleted, prevUpdatedAt := msg.IsDeleted, msg.UpdatedAt
	msg.IsDeleted = true
	msg.UpdatedAt = time.Now()
	summaryRepo := m.SummaryRepo
	m.mu.Unlock()

	if summaryRepo == nil {
		return nil
	}
	if err := summaryRepo.DeleteByRoom(ctx, roomID); err != nil {
		m.mu.Lock()
		msg.IsDeleted, msg.UpdatedAt = prevDeleted, prevUpdatedAt
		m.mu.Unlock()
		return err
	}
	return nil
}

// CountByRoom returns the total number of messages in roomID, ignoring
// soft-delete/visibility.
func (m *MessageRepo) CountByRoom(_ context.Context, roomID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for _, msg := range m.Messages {
		if msg.RoomID == roomID {
			count++
		}
	}
	return count, nil
}

// CountAndMaxSequence returns roomID's total message count and highest
// sequence number (0 if the room has no messages) in one pass, mirroring
// postgres.MessageRepository.CountAndMaxSequence's atomicity guarantee (the
// mock is single-threaded under mu for the duration of the call, so both
// values reflect the same snapshot of Messages).
func (m *MessageRepo) CountAndMaxSequence(_ context.Context, roomID string) (total int64, maxSeq int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, msg := range m.Messages {
		if msg.RoomID == roomID {
			total++
			if msg.Sequence > maxSeq {
				maxSeq = msg.Sequence
			}
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
