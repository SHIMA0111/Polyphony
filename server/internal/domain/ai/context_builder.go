package ai

import (
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
)

// ContextBuilder is the single place all AI-context filtering rules live. It
// converts a batch of persisted room messages into the ordered chat history
// sent to the LLM Gateway.
//
// Build takes msgs ordered sequence-descending — the order
// message.MessageRepository.ListByRoom and ListByRoomUpTo return — and
// returns a chronological (oldest first) slice of ChatMessage, ready to be
// assigned to CompletionRequest.Messages.
//
// A message is excluded from the output if any of the following hold:
//   - m.IsDeleted is true (the message was soft-deleted)
//   - m.ExcludeFromAI is true (the sender or an authorized member opted the
//     message out of AI context)
//   - m.Status == message.MessageStatusFailed (an AI placeholder from a
//     failed completion; sending it back would confuse the LLM with an
//     empty assistant turn)
//   - cutoff != nil && m.CreatedAt.Before(*cutoff) (the room's AI context
//     cutoff excludes anything created before it)
//
// A nil cutoff excludes nothing on cutoff grounds.
//
// This interface is the frozen contract later context-assembly steps
// (history summarization in Phase 5/18, streaming) extend or wrap — its
// method signature must not change; a later implementation may decorate or
// replace DefaultContextBuilder (e.g. to inject a cached summary in place of
// truncated history) without callers needing to change how they invoke
// Build.
type ContextBuilder interface {
	// Build filters and reorders msgs into the chronological chat history
	// eligible for AI context, applying the exclusion rules documented on
	// ContextBuilder.
	Build(msgs []*message.Message, cutoff *time.Time) []ChatMessage
}

// DefaultContextBuilder is the initial ContextBuilder implementation: a
// plain filter/map function with no summarization or token-budget logic.
// Later steps (Phase 5/18 history summarization) may wrap or replace it
// behind the same ContextBuilder interface.
type DefaultContextBuilder struct{}

// NewDefaultContextBuilder creates a new DefaultContextBuilder. The zero
// value is also ready to use; this constructor exists for symmetry with
// other domain/usecase constructors and to allow future fields to be added
// without changing call sites.
func NewDefaultContextBuilder() *DefaultContextBuilder {
	return &DefaultContextBuilder{}
}

// Build implements ContextBuilder. See the ContextBuilder doc comment for
// the exact filter list and ordering guarantee.
func (b *DefaultContextBuilder) Build(msgs []*message.Message, cutoff *time.Time) []ChatMessage {
	chatMsgs := make([]ChatMessage, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.IsDeleted {
			continue
		}
		if m.ExcludeFromAI {
			continue
		}
		if m.Status == message.MessageStatusFailed {
			continue
		}
		if cutoff != nil && m.CreatedAt.Before(*cutoff) {
			continue
		}
		role := "user"
		if m.Type == message.MessageTypeAI {
			role = "assistant"
		}
		chatMsgs = append(chatMsgs, ChatMessage{Role: role, Content: m.Content})
	}
	return chatMsgs
}
