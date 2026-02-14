package message

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

const defaultContextMessages = 50

// SendAIResult holds both the human and AI messages from a SendAIMessage call.
// When AIMessage.Status is "failed", the LLM call failed but both messages were persisted.
type SendAIResult struct {
	HumanMessage *domainmessage.Message
	AIMessage    *domainmessage.Message
}

// MessageUsecase provides message-related business logic.
type MessageUsecase struct {
	msgRepo    domainmessage.MessageRepository
	roomRepo   room.RoomRepository
	llmGateway ai.LLMGateway
}

// NewMessageUsecase creates a new MessageUsecase.
func NewMessageUsecase(
	msgRepo domainmessage.MessageRepository,
	roomRepo room.RoomRepository,
	llmGateway ai.LLMGateway,
) *MessageUsecase {
	return &MessageUsecase{
		msgRepo:    msgRepo,
		roomRepo:   roomRepo,
		llmGateway: llmGateway,
	}
}

// SendMessage creates a human message in a room.
func (u *MessageUsecase) SendMessage(ctx context.Context, userID, roomID, content string) (*domainmessage.Message, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	seq, err := u.msgRepo.GetNextSequence(ctx, roomID)
	if err != nil {
		return nil, err
	}

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

	if err = u.msgRepo.Create(ctx, msg); err != nil {
		return nil, err
	}

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
func (u *MessageUsecase) SendAIMessage(ctx context.Context, userID, roomID, content, model string) (*SendAIResult, error) {
	humanMsg, err := u.SendMessage(ctx, userID, roomID, content)
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

	// Allocate sequence for the AI message regardless of success/failure
	aiSeq, err := u.msgRepo.GetNextSequence(ctx, roomID)
	if err != nil {
		return nil, err
	}

	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:        uuid.New().String(),
		RoomID:    roomID,
		SenderID:  nil,
		Type:      domainmessage.MessageTypeAI,
		Sequence:  aiSeq,
		CreatedAt: aiNow,
		UpdatedAt: aiNow,
	}

	if llmErr != nil {
		// Save failed placeholder so regenerate can update it later
		aiMsg.Content = ""
		aiMsg.Status = domainmessage.MessageStatusFailed
		_ = u.msgRepo.Create(ctx, aiMsg) // best-effort save
		return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg}, nil
	}

	aiMsg.Content = completion.Content
	aiMsg.Status = domainmessage.MessageStatusCompleted

	if err = u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg}, nil
}

// RegenerateAIMessage regenerates the AI response for a specific human message.
// It always updates the existing AI message (created by SendAIMessage) in place,
// preserving sequence order. The AI message is guaranteed to exist because
// SendAIMessage always creates a placeholder even on LLM failure.
func (u *MessageUsecase) RegenerateAIMessage(ctx context.Context, userID, roomID, messageID, model string) (*domainmessage.Message, error) {
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
