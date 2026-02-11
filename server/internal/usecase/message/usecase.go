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
// Flow: save human msg → fetch context → call LLM → save AI msg → return AI msg.
func (u *MessageUsecase) SendAIMessage(ctx context.Context, userID, roomID, content, model string) (*domainmessage.Message, error) {
	_, err := u.SendMessage(ctx, userID, roomID, content)
	if err != nil {
		return nil, err
	}

	// Fetch context messages
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages)
	if err != nil {
		return nil, err
	}

	// Build chat messages (reverse to chronological order)
	chatMsgs := make([]ai.ChatMessage, 0, len(contextPage.Messages))
	for i := len(contextPage.Messages) - 1; i >= 0; i-- {
		m := contextPage.Messages[i]
		role := "user"
		if m.Type == domainmessage.MessageTypeAI {
			role = "assistant"
		}
		chatMsgs = append(chatMsgs, ai.ChatMessage{Role: role, Content: m.Content})
	}

	// Call LLM Gateway
	completion, err := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})
	if err != nil {
		return nil, err
	}

	// Save AI message
	aiSeq, err := u.msgRepo.GetNextSequence(ctx, roomID)
	if err != nil {
		return nil, err
	}

	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:        uuid.New().String(),
		RoomID:    roomID,
		SenderID:  nil,
		Content:   completion.Content,
		Type:      domainmessage.MessageTypeAI,
		Sequence:  aiSeq,
		CreatedAt: aiNow,
		UpdatedAt: aiNow,
	}

	if err = u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}

	return aiMsg, nil
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
