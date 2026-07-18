// Package message implements the chat message use cases (MessageUsecase):
// sending and listing human messages, invoking the AI to generate and
// regenerate AI messages within a room, and building the AI context from
// prior room history, on top of the domain/message entity and repository
// port.
package message

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
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
//
// Broadcasting is fire-and-forget: after each successful persist, the
// usecase publishes a RoomEvent via hub, but hub.Publish never returns an
// error and is never awaited for delivery, so a broadcast failure (e.g. a
// slow subscriber) can never roll back a write or cause the surrounding HTTP
// request to fail.
type MessageUsecase struct {
	msgRepo        domainmessage.MessageRepository
	roomRepo       room.RoomRepository
	llmGateway     ai.LLMGateway
	hub            event.MessageHub
	contextBuilder ai.ContextBuilder
	billing        BillingGuard
	// defaultAIModel is the deployment-wide fallback model string, sourced
	// from Config.DefaultAIModel by the caller of NewMessageUsecase. It is
	// the lowest-precedence tier consulted by resolveModel, used only when
	// both the request and the room's configured Room.AIModel are empty.
	defaultAIModel string
}

// NewMessageUsecase creates a new MessageUsecase. hub receives a
// message_created/message_updated event after every successful message
// persist; pass event.NewInProcessHub() for the initial in-process
// implementation. contextBuilder is always initialized internally to
// ai.NewDefaultContextBuilder(); it is not a constructor parameter so that
// swapping the implementation (e.g. for history summarization in a later
// phase) does not require touching every call site. billing is consulted
// before every AI invocation (SendAIMessage/RegenerateAIMessage) to reject
// requests once the room owner's token balance is exhausted, and to record
// usage after a successful completion; pass a *billingusecase.BillingUsecase
// (see usecase/billing), which satisfies BillingGuard structurally.
// defaultAIModel is the deployment-wide fallback model string consulted by
// resolveModel (see model_resolution.go) whenever an AI request omits an
// explicit model and the target room has no configured
// domainroom.Room.AIModel; pass cfg.DefaultAIModel from
// internal/infrastructure/config.Config.
func NewMessageUsecase(
	msgRepo domainmessage.MessageRepository,
	roomRepo room.RoomRepository,
	llmGateway ai.LLMGateway,
	hub event.MessageHub,
	billing BillingGuard,
	defaultAIModel string,
) *MessageUsecase {
	return &MessageUsecase{
		msgRepo:        msgRepo,
		roomRepo:       roomRepo,
		llmGateway:     llmGateway,
		hub:            hub,
		contextBuilder: ai.NewDefaultContextBuilder(),
		billing:        billing,
		defaultAIModel: defaultAIModel,
	}
}

// SendMessage creates a human message in a room. The caller must be allowed
// domainroom.ActionSendMessage (guest or above; a reader may not send). It
// reserves a single sequence number, persists the message, and publishes
// EventMessageCreated on the hub after the persist succeeds. Publishing is
// fire-and-forget: its outcome never affects the returned error, and it only
// happens once the write has already succeeded.
func (u *MessageUsecase) SendMessage(ctx context.Context, userID, roomID, content string) (*domainmessage.Message, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionSendMessage) {
		return nil, domain.ErrForbidden
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

// ListMessages returns paginated messages for a room. Any valid member
// (including reader) may list messages; no domainroom.Action check beyond
// membership is applied.
func (u *MessageUsecase) ListMessages(ctx context.Context, userID, roomID, cursor string, limit int) (*domainmessage.CursorPage, error) {
	if _, err := u.getMember(ctx, roomID, userID); err != nil {
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
//
// The caller must be allowed domainroom.ActionInvokeAI (member or above; a
// reader or guest may not invoke AI).
func (u *MessageUsecase) SendAIMessage(ctx context.Context, userID, roomID, content, model string) (*SendAIResult, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionInvokeAI) {
		return nil, domain.ErrForbidden
	}
	if err := u.billing.CheckBalance(ctx, roomID); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	model = resolveModel(model, rm, u.defaultAIModel)

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

	// Build chat messages (reverse to chronological order), filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages.
	chatMsgs := u.contextBuilder.Build(contextPage.Messages, rm.AIContextCutoffAt)

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

	// Fire-and-forget: the AI message is already durably persisted, so a
	// usage-recording failure must never affect the returned result.
	if err := u.billing.RecordUsage(ctx, roomID, aiMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg}, nil
}

// RegenerateAIMessage regenerates the AI response for a specific human message.
// It always updates the existing AI message (created by SendAIMessage) in place,
// preserving sequence order. The AI message is guaranteed to exist because
// SendAIMessage always creates a placeholder even on LLM failure. After the
// update succeeds, it publishes EventMessageUpdated on the hub; publishing is
// fire-and-forget and never affects the returned error.
//
// The caller must be allowed domainroom.ActionInvokeAI (member or above; a
// reader or guest may not invoke AI).
func (u *MessageUsecase) RegenerateAIMessage(ctx context.Context, userID, roomID, messageID, model string) (*domainmessage.Message, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionInvokeAI) {
		return nil, domain.ErrForbidden
	}
	if err := u.billing.CheckBalance(ctx, roomID); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	model = resolveModel(model, rm, u.defaultAIModel)

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

	// Build chat messages (reverse to chronological order), filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages.
	chatMsgs := u.contextBuilder.Build(contextMsgs, rm.AIContextCutoffAt)

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

	// Fire-and-forget: the AI message update is already durably persisted,
	// so a usage-recording failure must never affect the returned result.
	if err := u.billing.RecordUsage(ctx, roomID, nextMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", nextMsg.ID)
	}

	u.hub.Publish(ctx, event.RoomEvent{
		Type:       event.EventMessageUpdated,
		RoomID:     roomID,
		Message:    nextMsg,
		OccurredAt: now,
	})

	return nextMsg, nil
}

// DeleteMessage soft-deletes a message in roomID on behalf of userID. The
// caller must either be the message's own sender (SenderID == userID) or
// hold at least domainroom.RoleAdmin in the room (a moderation delete by an
// admin or master). Any other caller — including a non-sender member below
// admin — gets domain.ErrForbidden. It returns domain.ErrNotFound if the
// message does not exist or does not belong to roomID. On success it
// delegates to msgRepo.Delete, which performs the soft delete (see
// domainmessage.MessageRepository.Delete).
//
// DeleteMessage intentionally does not use domainroom.Action/Allows: the
// Action enum has no message-level delete action, so the owner-or-admin
// rule is expressed directly via member.Role.AtLeast(domainroom.RoleAdmin)
// combined with the sender-equality check.
func (u *MessageUsecase) DeleteMessage(ctx context.Context, userID, roomID, messageID string) error {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return err
	}

	msg, err := u.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if msg.RoomID != roomID {
		return domain.ErrNotFound
	}

	isOwner := msg.SenderID != nil && *msg.SenderID == userID
	isModerator := member.Role.AtLeast(room.RoleAdmin)
	if !isOwner && !isModerator {
		return domain.ErrForbidden
	}

	return u.msgRepo.Delete(ctx, messageID)
}

// SetExcludeFromAI toggles whether a message is excluded from future AI
// context assembly (ai.ContextBuilder.Build), without affecting its
// visibility in normal room message listings. The caller must be allowed
// domainroom.ActionInvokeAI (member or above; a reader or guest may not
// toggle this flag) — the same role gate SendAIMessage/RegenerateAIMessage
// use. It returns domain.ErrNotFound if the message does not exist or does
// not belong to roomID. On success it persists the change via
// msgRepo.UpdateExcludeFromAI and returns the mutated in-memory Message
// (mirroring RegenerateAIMessage's pattern of returning the updated struct
// rather than re-fetching).
func (u *MessageUsecase) SetExcludeFromAI(ctx context.Context, userID, roomID, messageID string, exclude bool) (*domainmessage.Message, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionInvokeAI) {
		return nil, domain.ErrForbidden
	}

	msg, err := u.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg.RoomID != roomID {
		return nil, domain.ErrNotFound
	}

	now := time.Now()
	if err := u.msgRepo.UpdateExcludeFromAI(ctx, messageID, exclude, now); err != nil {
		return nil, err
	}
	msg.ExcludeFromAI = exclude
	msg.UpdatedAt = now

	return msg, nil
}

// getMember loads the caller's membership in roomID, translating a missing
// membership (domain.ErrNotFound) into domain.ErrForbidden so that a
// non-member can never distinguish "room does not exist" from "room exists
// but I'm not a member of it" via the returned error.
func (u *MessageUsecase) getMember(ctx context.Context, roomID, userID string) (*room.RoomMember, error) {
	member, err := u.roomRepo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrForbidden
		}
		return nil, err
	}
	return member, nil
}
