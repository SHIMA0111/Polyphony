// Package message implements the chat message use cases (MessageUsecase):
// sending and listing human messages, invoking the AI to generate and
// regenerate AI messages within a room, and building the AI context from
// prior room history, on top of the domain/message entity and repository
// port.
package message

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

const defaultContextMessages = 50
const defaultModel = "gpt-5-mini"

// SendAIResult holds both the human and AI messages from a SendAIMessage call.
// When AIMessage.Status is "failed", the LLM call failed but both messages were persisted.
type SendAIResult struct {
	HumanMessage *domainmessage.Message
	AIMessage    *domainmessage.Message
}

// MessageUsecase provides message-related business logic.
//
// Broadcasting is fire-and-forget: after each successful persist, the
// usecase publishes a RoomEvent via hub, but hub.Publish never returns an
// error and is never awaited for delivery, so a broadcast failure (e.g. a
// slow subscriber) can never roll back a write or cause the surrounding HTTP
// request to fail.
type MessageUsecase struct {
	msgRepo    domainmessage.MessageRepository
	roomRepo   room.RoomRepository
	llmGateway ai.LLMGateway
	hub        event.MessageHub
}

// NewMessageUsecase creates a new MessageUsecase. hub receives a
// message_created/message_updated event after every successful message
// persist; pass event.NewInProcessHub() for the initial in-process
// implementation.
func NewMessageUsecase(
	msgRepo domainmessage.MessageRepository,
	roomRepo room.RoomRepository,
	llmGateway ai.LLMGateway,
	hub event.MessageHub,
) *MessageUsecase {
	return &MessageUsecase{
		msgRepo:    msgRepo,
		roomRepo:   roomRepo,
		llmGateway: llmGateway,
		hub:        hub,
	}
}

// SendMessage creates a human message in a room. It reserves a single
// sequence number, persists the message, and publishes EventMessageCreated
// on the hub after the persist succeeds. Publishing is fire-and-forget: its
// outcome never affects the returned error, and it only happens once the
// write has already succeeded.
func (u *MessageUsecase) SendMessage(ctx context.Context, userID, roomID, content string) (*domainmessage.Message, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	seq, err := u.msgRepo.ReserveSequenceRange(ctx, roomID, 1)
	if err != nil {
		return nil, err
	}

	return u.createHumanMessage(ctx, userID, roomID, content, seq)
}

// createHumanMessage builds a human message for the given (already reserved)
// sequence number, persists it, and publishes EventMessageCreated after the
// persist succeeds. It is shared by SendMessage (which reserves a single
// sequence) and SendAIMessage (which reserves a paired range up front) so
// both paths construct and persist the human message identically.
func (u *MessageUsecase) createHumanMessage(ctx context.Context, userID, roomID, content string, seq int64) (*domainmessage.Message, error) {
	now := time.Now()
	msg := &domainmessage.Message{
		ID:        uuid.New().String(),
		RoomID:    roomID,
		SenderID:  &userID,
		Content:   content,
		Type:      domainmessage.MessageTypeHuman,
		Status:    domainmessage.MessageStatusCompleted,
		Sequence:  seq,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := u.msgRepo.Create(ctx, msg); err != nil {
		return nil, err
	}

	u.hub.Publish(ctx, event.RoomEvent{
		Type:       event.EventMessageCreated,
		RoomID:     roomID,
		Message:    msg,
		OccurredAt: now,
	})

	return msg, nil
}

// ListMessages returns paginated messages for a room.
func (u *MessageUsecase) ListMessages(ctx context.Context, userID, roomID, cursor string, limit int) (*domainmessage.CursorPage, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	return u.msgRepo.ListByRoom(ctx, roomID, cursor, limit)
}

// SendAIMessage sends a human message and gets an AI response.
// On LLM failure, a placeholder AI message with status=failed is saved so that
// RegenerateAIMessage can retry later via UPDATE only.
// The result always contains both the human and AI messages; check AIMessage.Status
// to determine whether the LLM call succeeded.
//
// The human and AI sequence numbers are reserved together as a single
// contiguous range (via ReserveSequenceRange(ctx, roomID, 2)) before either
// row is written, so no concurrent request can allocate a sequence number
// that lands between them — this is what guarantees the adjacency that
// RegenerateAIMessage's GetNextInRoom lookup depends on. The AI message
// records InResponseToMessageID pointing at the human message, and each
// persisted message publishes EventMessageCreated on the hub after its
// Create call succeeds; publishing never affects the returned error.
func (u *MessageUsecase) SendAIMessage(ctx context.Context, userID, roomID, content, model string) (*SendAIResult, error) {
	if model == "" {
		model = defaultModel
	}

	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	// Reserve both sequence numbers atomically as one range before creating
	// either row, so nothing else can be interleaved between the human
	// message and its AI response.
	firstSeq, err := u.msgRepo.ReserveSequenceRange(ctx, roomID, 2)
	if err != nil {
		return nil, err
	}
	humanSeq, aiSeq := firstSeq, firstSeq+1

	humanMsg, err := u.createHumanMessage(ctx, userID, roomID, content, humanSeq)
	if err != nil {
		return nil, err
	}

	// Fetch context messages
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages)
	if err != nil {
		return nil, err
	}

	// Build chat messages (reverse to chronological order)
	chatMsgs := u.buildChatMessages(contextPage.Messages)

	// Call LLM Gateway
	completion, llmErr := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})

	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:                    uuid.New().String(),
		RoomID:                roomID,
		SenderID:              nil,
		Type:                  domainmessage.MessageTypeAI,
		Sequence:              aiSeq,
		InResponseToMessageID: &humanMsg.ID,
		CreatedAt:             aiNow,
		UpdatedAt:             aiNow,
	}

	if llmErr != nil {
		// Save failed placeholder so regenerate can update it later
		aiMsg.Content = ""
		aiMsg.Status = domainmessage.MessageStatusFailed
		if err := u.msgRepo.Create(ctx, aiMsg); err != nil {
			return nil, err
		}
		u.hub.Publish(ctx, event.RoomEvent{
			Type:       event.EventMessageCreated,
			RoomID:     roomID,
			Message:    aiMsg,
			OccurredAt: aiNow,
		})
		return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg}, nil
	}

	aiMsg.Content = completion.Content
	aiMsg.Status = domainmessage.MessageStatusCompleted

	if err = u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}
	u.hub.Publish(ctx, event.RoomEvent{
		Type:       event.EventMessageCreated,
		RoomID:     roomID,
		Message:    aiMsg,
		OccurredAt: aiNow,
	})

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg}, nil
}

// RegenerateAIMessage regenerates the AI response for a specific human message.
// It always updates the existing AI message (created by SendAIMessage) in place,
// preserving sequence order. The AI message is guaranteed to exist because
// SendAIMessage always creates a placeholder even on LLM failure. After the
// update succeeds, it publishes EventMessageUpdated on the hub; publishing is
// fire-and-forget and never affects the returned error.
func (u *MessageUsecase) RegenerateAIMessage(ctx context.Context, userID, roomID, messageID, model string) (*domainmessage.Message, error) {
	if model == "" {
		model = defaultModel
	}

	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	// Verify target message exists and belongs to the room
	targetMsg, err := u.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if targetMsg.RoomID != roomID {
		return nil, domain.ErrNotFound
	}
	if targetMsg.Type != domainmessage.MessageTypeHuman {
		return nil, domain.ErrInvalidMessageType
	}

	// Find the AI response that follows the target message
	nextMsg, err := u.msgRepo.GetNextInRoom(ctx, roomID, targetMsg.Sequence)
	if err != nil {
		return nil, err
	}
	if nextMsg.Type != domainmessage.MessageTypeAI {
		return nil, domain.ErrNotFound
	}

	// Fetch context up to the target message (inclusive)
	contextMsgs, err := u.msgRepo.ListByRoomUpTo(ctx, roomID, targetMsg.Sequence, defaultContextMessages)
	if err != nil {
		return nil, err
	}

	// Build chat messages (reverse to chronological order)
	chatMsgs := u.buildChatMessages(contextMsgs)

	// Call LLM Gateway
	completion, err := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})
	if err != nil {
		return nil, err
	}

	// Update existing AI response in place (preserve sequence and created_at)
	now := time.Now()
	if err = u.msgRepo.UpdateAIResponse(ctx, nextMsg.ID, completion.Content, domainmessage.MessageStatusCompleted, now); err != nil {
		return nil, err
	}
	nextMsg.Content = completion.Content
	nextMsg.Status = domainmessage.MessageStatusCompleted
	nextMsg.UpdatedAt = now

	u.hub.Publish(ctx, event.RoomEvent{
		Type:       event.EventMessageUpdated,
		RoomID:     roomID,
		Message:    nextMsg,
		OccurredAt: now,
	})

	return nextMsg, nil
}

// buildChatMessages converts domain messages (sequence descending) to chronological chat messages.
// Messages with status=failed are skipped to avoid sending empty placeholders to the LLM.
func (u *MessageUsecase) buildChatMessages(msgs []*domainmessage.Message) []ai.ChatMessage {
	chatMsgs := make([]ai.ChatMessage, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Status == domainmessage.MessageStatusFailed {
			continue
		}
		role := "user"
		if m.Type == domainmessage.MessageTypeAI {
			role = "assistant"
		}
		chatMsgs = append(chatMsgs, ai.ChatMessage{Role: role, Content: m.Content})
	}
	return chatMsgs
}

func (u *MessageUsecase) checkMembership(ctx context.Context, roomID, userID string) error {
	_, err := u.roomRepo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden
		}
		return err
	}
	return nil
}
