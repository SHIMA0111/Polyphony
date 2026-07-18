package message

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// --- Tests ---

func TestSendMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	msg, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if msg.Content != "Hello" {
		t.Fatalf("expected Hello, got %s", msg.Content)
	}
	if msg.Type != domainmessage.MessageTypeHuman {
		t.Fatalf("expected human type, got %s", msg.Type)
	}
}

func TestSendMessageNotMember(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

// TestSendMessageReaderForbidden asserts that a reader — who may view
// messages but not send them — gets domain.ErrForbidden from SendMessage.
func TestSendMessageReaderForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleReader))

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for reader sending a message, got %v", err)
	}
}

// TestSendMessageGuestAllowed asserts that a guest — who may send messages
// but not invoke AI — can successfully call SendMessage.
func TestSendMessageGuestAllowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleGuest))
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	msg, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != nil {
		t.Fatalf("expected guest to send a message, got error: %v", err)
	}
	if msg.Content != "Hello" {
		t.Fatalf("expected Hello, got %s", msg.Content)
	}
}

// TestSendAIMessageGuestForbidden asserts that a guest — who may send
// messages but not invoke AI — gets domain.ErrForbidden from SendAIMessage.
func TestSendAIMessageGuestForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleGuest))

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for guest invoking AI, got %v", err)
	}
}

// TestRegenerateAIMessageGuestForbidden asserts that a guest gets
// domain.ErrForbidden from RegenerateAIMessage, without ever reaching the
// underlying message lookup.
func TestRegenerateAIMessageGuestForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleGuest))

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, _, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", "nonexistent-message-id", "test-model")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for guest regenerating an AI message, got %v", err)
	}
}

// TestListMessagesReaderAllowed asserts that a reader — read-only — can
// still list messages (ListMessages only checks membership, not any Action).
func TestListMessagesReaderAllowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleReader))

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	if _, err := uc.ListMessages(ctx, "user-1", "room-1", "", 20); err != nil {
		t.Fatalf("expected reader to list messages, got error: %v", err)
	}
}

func TestListMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, _ = uc.SendMessage(ctx, "user-1", "room-1", "msg1")
	_, _ = uc.SendMessage(ctx, "user-1", "room-1", "msg2")

	page, err := uc.ListMessages(ctx, "user-1", "room-1", "", 20)
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(page.Messages))
	}
}

func TestSendAIMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	if result.HumanMessage.Content != "What is Go?" {
		t.Fatalf("expected human content 'What is Go?', got %s", result.HumanMessage.Content)
	}
	if result.HumanMessage.Type != domainmessage.MessageTypeHuman {
		t.Fatalf("expected human type, got %s", result.HumanMessage.Type)
	}
	if result.AIMessage.Content != "AI response" {
		t.Fatalf("expected AI response, got %s", result.AIMessage.Content)
	}
	if result.AIMessage.Type != domainmessage.MessageTypeAI {
		t.Fatalf("expected ai type, got %s", result.AIMessage.Type)
	}
	if result.AIMessage.SenderID != nil {
		t.Fatal("expected nil sender for AI message")
	}
	if result.AIMessage.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.AIMessage.Status)
	}
}

// TestSendAIMessageHonorsRoomConfiguredModel asserts that, when the request
// omits a model, SendAIMessage resolves to the room's configured
// Room.AIModel (set via RoomUsecase.UpdateSettings) rather than the
// deployment-wide default — the room tier of resolveModel's precedence.
func TestSendAIMessageHonorsRoomConfiguredModel(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	roomRepo.Rooms["room-1"].AIModel = strPtr("room-configured-model")

	var usedModel string
	gw := &mocks.LLMGateway{
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			usedModel = req.Model
			return &ai.CompletionResponse{Content: "AI response", Model: req.Model, PromptTokens: 1, OutputTokens: 1}, nil
		},
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "global-default-model")
	ctx := context.Background()

	// Request omits model entirely.
	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	if usedModel != "room-configured-model" {
		t.Fatalf("expected room-configured model to be used, got %q", usedModel)
	}
}

func TestRegenerateAIMessageAfterFailure(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	failingGateway := &mocks.LLMGateway{ShouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, failingGateway, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// SendAIMessage with failing LLM — returns result with failed AI placeholder
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", result.AIMessage.Status)
	}

	// Switch to working LLM and regenerate
	failingGateway.ShouldErr = false
	aiMsg, _, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != nil {
		t.Fatalf("RegenerateAIMessage failed: %v", err)
	}
	if aiMsg.Content != "AI response" {
		t.Fatalf("expected AI response, got %s", aiMsg.Content)
	}
	if aiMsg.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status, got %s", aiMsg.Status)
	}
	if aiMsg.ID != result.AIMessage.ID {
		t.Fatal("expected regenerate to update the existing placeholder, not create new")
	}
}

func TestRegenerateAIMessageOverwritesExisting(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// Send human message + AI response via SendAIMessage
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	originalSeq := result.AIMessage.Sequence
	msgCountBefore := len(msgRepo.Messages)

	// Regenerate — should overwrite the existing AI message, not create a new one
	regenerated, _, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != nil {
		t.Fatalf("RegenerateAIMessage failed: %v", err)
	}

	// Should reuse the same message ID and sequence
	if regenerated.ID != result.AIMessage.ID {
		t.Fatalf("expected same message ID %s, got %s", result.AIMessage.ID, regenerated.ID)
	}
	if regenerated.Sequence != originalSeq {
		t.Fatalf("expected sequence %d preserved, got %d", originalSeq, regenerated.Sequence)
	}

	// No new messages should have been created
	if len(msgRepo.Messages) != msgCountBefore {
		t.Fatalf("expected %d messages (no new), got %d", msgCountBefore, len(msgRepo.Messages))
	}
}

// TestRegenerateAIMessageHonorsRoomConfiguredModel asserts that, when the
// request omits a model, RegenerateAIMessage resolves to the room's
// configured Room.AIModel rather than the deployment-wide default.
func TestRegenerateAIMessageHonorsRoomConfiguredModel(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "global-default-model")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	// Configure the room's default model only after the initial send, then
	// regenerate with an omitted model.
	roomRepo.Rooms["room-1"].AIModel = strPtr("room-configured-model")

	var usedModel string
	gw := &mocks.LLMGateway{
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			usedModel = req.Model
			return &ai.CompletionResponse{Content: "regenerated", Model: req.Model, PromptTokens: 1, OutputTokens: 1}, nil
		},
	}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "global-default-model")

	if _, _, err := uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, ""); err != nil {
		t.Fatalf("RegenerateAIMessage failed: %v", err)
	}
	if usedModel != "room-configured-model" {
		t.Fatalf("expected room-configured model to be used, got %q", usedModel)
	}
}

func TestRegenerateAIMessageNotHuman(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// Send a human message and get AI response
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	// Try to regenerate from the AI message (should fail)
	_, _, err = uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.AIMessage.ID, "test-model")
	if err != domain.ErrInvalidMessageType {
		t.Fatalf("expected ErrInvalidMessageType, got %v", err)
	}
}

// TestRegenerateAIMessageNotFound asserts that regenerating a nonexistent
// message ID returns domain.ErrForbidden rather than domain.ErrNotFound: the
// filtered GetByID lookup (see MessageRepository.GetByID) cannot distinguish
// "does not exist" from "exists but is another user's private message", so
// once room membership is already verified, RegenerateAIMessage treats any
// not-found result from that lookup as forbidden (see the RegenerateAIMessage
// doc comment).
func TestRegenerateAIMessageNotFound(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, _, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", "nonexistent", "test-model")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestRegenerateAIMessageWrongRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-2", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	roomRepo.SeedRoom("room-2", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// Send message in room-1
	humanMsg, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Try to regenerate in room-2 (should fail)
	_, _, err = uc.RegenerateAIMessage(ctx, "user-1", "room-2", humanMsg.ID, "test-model")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRegenerateAIMessageNotMember(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	humanMsg, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// user-2 is not a member
	_, _, err = uc.RegenerateAIMessage(ctx, "user-2", "room-1", humanMsg.ID, "test-model")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSendAIMessageContextExcludesFailedMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	gw := &mocks.LLMGateway{ShouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// First call fails — creates human + failed AI placeholder
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", result.AIMessage.Status)
	}

	// Second call succeeds — capture the request sent to the LLM Gateway and
	// verify the failed AI placeholder (an empty-content assistant message)
	// from the first call is not present in its context.
	gw.ShouldErr = false
	var capturedReq *ai.CompletionRequest
	gw.CompleteFunc = func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
		capturedReq = req
		return &ai.CompletionResponse{Content: "AI response", Model: "test-model"}, nil
	}
	result2, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	if result2.AIMessage.Content != "AI response" {
		t.Fatalf("expected AI response, got %s", result2.AIMessage.Content)
	}
	if capturedReq == nil {
		t.Fatal("expected CompleteFunc to have been called")
	}
	for _, m := range capturedReq.Messages {
		if m.Role == "assistant" && m.Content == "" {
			t.Fatalf("context sent to LLM Gateway contains the failed AI placeholder: %+v", capturedReq.Messages)
		}
	}
}

func TestSendAIMessageLLMError(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{ShouldErr: true}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both messages should be returned
	if result.HumanMessage == nil {
		t.Fatal("expected human message in result")
	}
	if result.AIMessage == nil {
		t.Fatal("expected AI message in result")
	}
	// AI message should have failed status
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", result.AIMessage.Status)
	}
	if result.AIMessage.Content != "" {
		t.Fatalf("expected empty content for failed AI message, got %s", result.AIMessage.Content)
	}
}

// TestSendAIMessageSequenceAdjacencyAndResponseLinkage asserts that the
// human message and its AI response are allocated adjacent sequence numbers
// (reserved atomically as a single range) and that the AI message records a
// durable InResponseToMessageID link back to the human message it answers.
func TestSendAIMessageSequenceAdjacencyAndResponseLinkage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	humanMsg, aiMsg := result.HumanMessage, result.AIMessage
	if aiMsg.Sequence != humanMsg.Sequence+1 {
		t.Fatalf("expected aiMsg.Sequence == humanMsg.Sequence + 1, got human=%d ai=%d", humanMsg.Sequence, aiMsg.Sequence)
	}
	if aiMsg.InResponseToMessageID == nil {
		t.Fatal("expected AIMessage.InResponseToMessageID to be set")
	}
	if *aiMsg.InResponseToMessageID != humanMsg.ID {
		t.Fatalf("expected AIMessage.InResponseToMessageID == humanMsg.ID (%s), got %s", humanMsg.ID, *aiMsg.InResponseToMessageID)
	}
	if humanMsg.InResponseToMessageID != nil {
		t.Fatal("expected human message InResponseToMessageID to be nil")
	}
}

// --- Context filtering (Step 23: AI context control) ---

// captureCompletionMessages returns an *mocks.LLMGateway whose CompleteFunc
// records the ChatMessage slice it was called with into *captured, so a
// test can assert on exactly what context was sent to the LLM.
func captureCompletionMessages(captured *[]ai.ChatMessage) *mocks.LLMGateway {
	return &mocks.LLMGateway{
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			*captured = req.Messages
			return &ai.CompletionResponse{Content: "AI response"}, nil
		},
	}
}

func containsContent(msgs []ai.ChatMessage, content string) bool {
	for _, m := range msgs {
		if m.Content == content {
			return true
		}
	}
	return false
}

// TestSendAIMessageContextExcludesSoftDeletedMessage asserts that a message
// soft-deleted via DeleteMessage does not appear in the LLM context built by
// a subsequent SendAIMessage call.
func TestSendAIMessageContextExcludesSoftDeletedMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	toDelete, err := uc.SendMessage(ctx, "user-1", "room-1", "secret message")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if err := uc.DeleteMessage(ctx, "user-1", "room-1", toDelete.ID); err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if containsContent(captured, "secret message") {
		t.Fatal("expected soft-deleted message to be excluded from AI context")
	}
	if !containsContent(captured, "follow up") {
		t.Fatal("expected the new message to be included in AI context")
	}
}

// TestSendAIMessageContextExcludesExcludeFromAIMessage asserts that a
// message toggled exclude_from_ai via SetExcludeFromAI does not appear in
// the LLM context built by a subsequent SendAIMessage call, even though it
// still exists (is not deleted).
func TestSendAIMessageContextExcludesExcludeFromAIMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	toExclude, err := uc.SendMessage(ctx, "user-1", "room-1", "private aside")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", toExclude.ID, true); err != nil {
		t.Fatalf("SetExcludeFromAI failed: %v", err)
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if containsContent(captured, "private aside") {
		t.Fatal("expected exclude_from_ai message to be excluded from AI context")
	}

	// The message should still exist and be listable — exclude_from_ai
	// only affects AI context, not room visibility.
	page, err := uc.ListMessages(ctx, "user-1", "room-1", "", 20)
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	found := false
	for _, m := range page.Messages {
		if m.ID == toExclude.ID {
			found = true
			if !m.ExcludeFromAI {
				t.Fatal("expected ExcludeFromAI to be true on the listed message")
			}
		}
	}
	if !found {
		t.Fatal("expected excluded message to still appear in ListMessages")
	}
}

// TestSendAIMessageContextExcludesPreCutoffMessages asserts that a message
// created before the room's configured AIContextCutoffAt does not appear in
// the LLM context built by SendAIMessage.
func TestSendAIMessageContextExcludesPreCutoffMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	oldMsg, err := uc.SendMessage(ctx, "user-1", "room-1", "ancient history")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	cutoff := time.Now()
	// Rewind the fixture message's CreatedAt to before the cutoff. The mock
	// stores the exact pointer SendMessage returned, so mutating it here
	// mutates the fixture as seen by ListByRoom too.
	oldMsg.CreatedAt = cutoff.Add(-time.Hour)
	roomRepo.SeedRoom("room-1", &cutoff)

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "new message after cutoff", "test-model", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if containsContent(captured, "ancient history") {
		t.Fatal("expected pre-cutoff message to be excluded from AI context")
	}
	if !containsContent(captured, "new message after cutoff") {
		t.Fatal("expected the new message to be included in AI context")
	}
}

// TestSendAIMessageAttachmentEnrichmentNoAttachments asserts that a message
// with zero attachments keeps going through the plain-Content path
// unchanged: its ai.ChatMessage entry in the built LLM context has no Parts.
func TestSendAIMessageAttachmentEnrichmentNoAttachments(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	attachmentRepo := &mocks.AttachmentRepo{}
	objStorage := &mocks.ObjectStorage{}

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, attachmentRepo, objStorage, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	if _, err := uc.SendMessage(ctx, "user-1", "room-1", "plain text message"); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	found := false
	for _, m := range captured {
		if m.Content == "plain text message" {
			found = true
			if len(m.Parts) != 0 {
				t.Fatalf("expected no Parts on a message with no attachments, got %+v", m.Parts)
			}
		}
	}
	if !found {
		t.Fatal("expected to find the no-attachment message in the captured context")
	}
}

// TestSendAIMessageAttachmentEnrichmentSingleImage asserts that a message
// with a single image attachment is upgraded to a two-part Parts payload: a
// text part carrying its original content, followed by an image_url part
// built from a freshly presigned view URL.
func TestSendAIMessageAttachmentEnrichmentSingleImage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	attachmentRepo := &mocks.AttachmentRepo{}
	objStorage := &mocks.ObjectStorage{}

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, attachmentRepo, objStorage, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "check this out")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := attachmentRepo.Create(ctx, &domainattachment.Attachment{
		ID:        "att-1",
		RoomID:    "room-1",
		S3Key:     "attachments/room-1/att-1",
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, "att-1", sent.ID, "room-1"); err != nil {
		t.Fatalf("AttachToMessage failed: %v", err)
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	var found *ai.ChatMessage
	for i := range captured {
		if captured[i].Content == "check this out" {
			found = &captured[i]
		}
	}
	if found == nil {
		t.Fatal("expected to find the attachment-bearing message in the captured context")
	}
	if len(found.Parts) != 2 {
		t.Fatalf("expected 2 parts (1 text + 1 image), got %d: %+v", len(found.Parts), found.Parts)
	}
	if found.Parts[0].Type != ai.ContentPartTypeText || found.Parts[0].Text != "check this out" {
		t.Fatalf("expected first part to be the original text, got %+v", found.Parts[0])
	}
	if found.Parts[1].Type != ai.ContentPartTypeImageURL ||
		found.Parts[1].ImageURL != "https://mock-view/attachments/room-1/att-1" {
		t.Fatalf("expected second part to be a presigned image URL, got %+v", found.Parts[1])
	}
}

// TestSendAIMessageAttachmentEnrichmentMultipleImages asserts that a message
// with multiple image attachments produces one image part per attachment,
// in attachmentRepo.ListByMessageID order (creation-time ascending).
func TestSendAIMessageAttachmentEnrichmentMultipleImages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	attachmentRepo := &mocks.AttachmentRepo{}
	objStorage := &mocks.ObjectStorage{}

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, attachmentRepo, objStorage, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "two photos")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	now := time.Now()
	if err := attachmentRepo.Create(ctx, &domainattachment.Attachment{
		ID: "att-1", RoomID: "room-1", S3Key: "attachments/room-1/att-1", MimeType: "image/png",
		SizeBytes: 1024, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed attachment 1: %v", err)
	}
	if err := attachmentRepo.Create(ctx, &domainattachment.Attachment{
		ID: "att-2", RoomID: "room-1", S3Key: "attachments/room-1/att-2", MimeType: "image/jpeg",
		SizeBytes: 2048, CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("seed attachment 2: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, "att-1", sent.ID, "room-1"); err != nil {
		t.Fatalf("AttachToMessage att-1 failed: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, "att-2", sent.ID, "room-1"); err != nil {
		t.Fatalf("AttachToMessage att-2 failed: %v", err)
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model", false); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	var found *ai.ChatMessage
	for i := range captured {
		if captured[i].Content == "two photos" {
			found = &captured[i]
		}
	}
	if found == nil {
		t.Fatal("expected to find the attachment-bearing message in the captured context")
	}
	if len(found.Parts) != 3 {
		t.Fatalf("expected 3 parts (1 text + 2 images), got %d: %+v", len(found.Parts), found.Parts)
	}
	if found.Parts[1].ImageURL != "https://mock-view/attachments/room-1/att-1" {
		t.Fatalf("expected part[1] to reference att-1, got %+v", found.Parts[1])
	}
	if found.Parts[2].ImageURL != "https://mock-view/attachments/room-1/att-2" {
		t.Fatalf("expected part[2] to reference att-2, got %+v", found.Parts[2])
	}
}

// --- DeleteMessage ---

func TestDeleteMessageSenderCanDeleteOwnMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-1", "room-1", sent.ID); err != nil {
		t.Fatalf("expected sender to delete own message, got error: %v", err)
	}
}

func TestDeleteMessageNonSenderNonAdminForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-1", "user-2", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-2", "room-1", sent.ID); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for non-sender non-admin member, got %v", err)
	}
}

func TestDeleteMessageAdminCanDeleteAnotherMembersMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-1", "user-2", string(domainroom.RoleAdmin))
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-2", "room-1", sent.ID); err != nil {
		t.Fatalf("expected admin to delete another member's message, got error: %v", err)
	}
}

func TestDeleteMessageWrongRoomNotFound(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-2", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-1", "room-2", sent.ID); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for a message from a different room, got %v", err)
	}
}

// TestDeleteMessagePropagatesSummaryInvalidationFailure proves that
// DeleteMessage surfaces a msgRepo.DeleteAndInvalidateSummary failure to the
// caller instead of only logging it, and that -- per that method's
// all-or-nothing transaction contract -- the message has NOT been deleted
// when that happens: nothing is mutated when the combined call fails,
// rather than the soft delete having already gone through un-invalidated.
func TestDeleteMessagePropagatesSummaryInvalidationFailure(t *testing.T) {
	invalidationErr := errors.New("boom: summary invalidation failed")
	msgRepo := &mocks.MessageRepo{DeleteAndInvalidateSummaryErr: invalidationErr}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-1", "room-1", sent.ID); !errors.Is(err, invalidationErr) {
		t.Fatalf("expected DeleteMessage to propagate the summary invalidation error, got %v", err)
	}

	// Nothing must have been mutated: the combined call is all-or-nothing,
	// so a failure means DeleteMessage returned without the soft delete
	// having taken effect.
	notDeleted, err := msgRepo.GetByID(ctx, sent.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID after DeleteMessage: %v", err)
	}
	if notDeleted.IsDeleted {
		t.Fatal("expected the message to remain NOT deleted when the combined delete-and-invalidate call fails")
	}
}

// --- SetExcludeFromAI ---

func TestSetExcludeFromAIMemberAllowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	updated, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", sent.ID, true)
	if err != nil {
		t.Fatalf("expected member to toggle exclude_from_ai, got error: %v", err)
	}
	if !updated.ExcludeFromAI {
		t.Fatal("expected ExcludeFromAI to be true")
	}
}

func TestSetExcludeFromAIAdminAllowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleAdmin))
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", sent.ID, true); err != nil {
		t.Fatalf("expected admin to toggle exclude_from_ai, got error: %v", err)
	}
}

func TestSetExcludeFromAIMasterAllowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleMaster))
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", sent.ID, true); err != nil {
		t.Fatalf("expected master to toggle exclude_from_ai, got error: %v", err)
	}
}

func TestSetExcludeFromAIGuestForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleGuest))

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// Directly seed a message so a guest (who cannot SendMessage-then-target
	// their own, since guests can send but the point is the toggle check) is
	// exercised against an existing message ID.
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", Content: "hello"},
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", "msg-1", true); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for guest, got %v", err)
	}
}

func TestSetExcludeFromAIReaderForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", string(domainroom.RoleReader))

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", Content: "hello"},
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", "msg-1", true); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for reader, got %v", err)
	}
}

// TestSetExcludeFromAIPropagatesSummaryInvalidationFailure proves that
// SetExcludeFromAI surfaces a
// msgRepo.UpdateExcludeFromAIAndInvalidateSummary failure to the caller, and
// that -- per that method's all-or-nothing transaction contract -- the flag
// has NOT been toggled when that happens: nothing is mutated when the
// combined call fails, rather than the toggle having already gone through
// un-invalidated.
func TestSetExcludeFromAIPropagatesSummaryInvalidationFailure(t *testing.T) {
	invalidationErr := errors.New("boom: summary invalidation failed")
	msgRepo := &mocks.MessageRepo{UpdateExcludeFromAIAndInvalidateSummaryErr: invalidationErr}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", sent.ID, true); !errors.Is(err, invalidationErr) {
		t.Fatalf("expected SetExcludeFromAI to propagate the summary invalidation error, got %v", err)
	}

	// Nothing must have been mutated: the combined call is all-or-nothing,
	// so a failure means SetExcludeFromAI returned without the toggle
	// having taken effect.
	notUpdated, err := msgRepo.GetByID(ctx, sent.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID after SetExcludeFromAI: %v", err)
	}
	if notUpdated.ExcludeFromAI {
		t.Fatal("expected ExcludeFromAI to remain false when the combined update-and-invalidate call fails")
	}
}

// --- Billing guard tests (Step 42) ---

// TestSendAIMessageInsufficientBalance asserts that SendAIMessage returns
// domain.ErrInsufficientBalance immediately when the billing guard rejects,
// and that no messages are created (the guard runs before any message is
// persisted).
func TestSendAIMessageInsufficientBalance(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	guard := &mocks.BillingGuard{CheckBalanceErr: domain.ErrInsufficientBalance}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != domain.ErrInsufficientBalance {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
	if len(msgRepo.Messages) != 0 {
		t.Fatalf("expected zero messages created, got %d", len(msgRepo.Messages))
	}
}

// TestSendAIMessageRecordsUsage asserts that on a successful completion,
// RecordUsage is called exactly once with the AI message's ID, the resolved
// model name, and the completion's PromptTokens/OutputTokens.
func TestSendAIMessageRecordsUsage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	guard := &mocks.BillingGuard{}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if len(guard.RecordUsageCalls) != 1 {
		t.Fatalf("expected exactly 1 RecordUsage call, got %d", len(guard.RecordUsageCalls))
	}
	call := guard.RecordUsageCalls[0]
	if call.AIMessageID != result.AIMessage.ID {
		t.Fatalf("expected RecordUsage aiMessageID %s, got %s", result.AIMessage.ID, call.AIMessageID)
	}
	if call.Model != "test-model" {
		t.Fatalf("expected RecordUsage model test-model, got %s", call.Model)
	}
	if call.PromptTokens != 10 || call.OutputTokens != 5 {
		t.Fatalf("expected RecordUsage tokens 10/5 (default mock completion), got %d/%d", call.PromptTokens, call.OutputTokens)
	}
}

// TestSendAIMessageRecordUsageErrorSwallowed asserts that a RecordUsage
// error does not change SendAIMessage's returned result or error — usage
// recording is fire-and-forget.
func TestSendAIMessageRecordUsageErrorSwallowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	guard := &mocks.BillingGuard{RecordUsageErr: fmt.Errorf("db unavailable")}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("expected RecordUsage error to be swallowed, got error: %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status despite RecordUsage error, got %s", result.AIMessage.Status)
	}
}

// TestRegenerateAIMessageInsufficientBalance asserts that
// RegenerateAIMessage returns domain.ErrInsufficientBalance when the billing
// guard rejects, before any context-fetching/LLM call takes place.
func TestRegenerateAIMessageInsufficientBalance(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	guard := &mocks.BillingGuard{CheckBalanceErr: domain.ErrInsufficientBalance}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	_, _, err = uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != domain.ErrInsufficientBalance {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
}

// TestRegenerateAIMessageRecordsUsage asserts that on a successful
// regeneration, RecordUsage is called exactly once with nextMsg.ID (the
// existing AI message's ID, preserved across regeneration).
func TestRegenerateAIMessageRecordsUsage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	setupGuard := &mocks.BillingGuard{}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), setupGuard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	guard := &mocks.BillingGuard{}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	regenerated, _, err := uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "regen-model")
	if err != nil {
		t.Fatalf("RegenerateAIMessage failed: %v", err)
	}

	if len(guard.RecordUsageCalls) != 1 {
		t.Fatalf("expected exactly 1 RecordUsage call, got %d", len(guard.RecordUsageCalls))
	}
	call := guard.RecordUsageCalls[0]
	if call.AIMessageID != regenerated.ID {
		t.Fatalf("expected RecordUsage aiMessageID %s, got %s", regenerated.ID, call.AIMessageID)
	}
	if call.Model != "regen-model" {
		t.Fatalf("expected RecordUsage model regen-model, got %s", call.Model)
	}
}

// TestRegenerateAIMessageRecordUsageErrorSwallowed asserts that a
// RecordUsage error during regeneration does not change the returned
// message or error.
func TestRegenerateAIMessageRecordUsageErrorSwallowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	setupGuard := &mocks.BillingGuard{}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), setupGuard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	guard := &mocks.BillingGuard{RecordUsageErr: fmt.Errorf("db unavailable")}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	regenerated, _, err := uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != nil {
		t.Fatalf("expected RecordUsage error to be swallowed, got error: %v", err)
	}
	if regenerated.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status despite RecordUsage error, got %s", regenerated.Status)
	}
}

// --- Private AI mode (Step 41) ---

// TestSendAIMessagePrivateSetsVisibilityAndAISenderID asserts that
// SendAIMessage(..., private=true) marks both the human message and the AI
// message MessageVisibilityPrivate, and — per the documented deviation from
// the usual "AI messages have nil SenderID" convention — sets the AI
// message's SenderID to the requesting user, so the single visibility
// predicate can filter both rows uniformly.
func TestSendAIMessagePrivateSetsVisibilityAndAISenderID(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "secret question", "test-model", true)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if result.HumanMessage.Visibility != domainmessage.MessageVisibilityPrivate {
		t.Fatalf("expected human message visibility private, got %s", result.HumanMessage.Visibility)
	}
	if result.AIMessage.Visibility != domainmessage.MessageVisibilityPrivate {
		t.Fatalf("expected AI message visibility private, got %s", result.AIMessage.Visibility)
	}
	if result.AIMessage.SenderID == nil || *result.AIMessage.SenderID != "user-1" {
		t.Fatalf("expected private AI message SenderID to be the requester, got %v", result.AIMessage.SenderID)
	}
}

// TestSendAIMessagePublicDefaultsVisibility asserts that a non-private
// SendAIMessage call still explicitly persists MessageVisibilityPublic on
// both messages (not the zero value), and leaves the AI message's SenderID
// nil (the pre-existing convention for public AI messages).
func TestSendAIMessagePublicDefaultsVisibility(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if result.HumanMessage.Visibility != domainmessage.MessageVisibilityPublic {
		t.Fatalf("expected human message visibility public, got %s", result.HumanMessage.Visibility)
	}
	if result.AIMessage.Visibility != domainmessage.MessageVisibilityPublic {
		t.Fatalf("expected AI message visibility public, got %s", result.AIMessage.Visibility)
	}
	if result.AIMessage.SenderID != nil {
		t.Fatal("expected nil SenderID for a public AI message")
	}
}

// TestListMessagesExcludesOtherUsersPrivateExchange asserts that a private
// human+AI exchange created by user-1 is invisible to user-2's ListMessages
// call on the same room, but fully visible to user-1's own call.
func TestListMessagesExcludesOtherUsersPrivateExchange(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-1", "user-2", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "secret question", "test-model", true)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	// user-2 (a different room member) must not see either message of the
	// private exchange.
	otherPage, err := uc.ListMessages(ctx, "user-2", "room-1", "", 20)
	if err != nil {
		t.Fatalf("ListMessages (user-2) failed: %v", err)
	}
	for _, m := range otherPage.Messages {
		if m.ID == result.HumanMessage.ID || m.ID == result.AIMessage.ID {
			t.Fatalf("expected user-2's ListMessages to exclude private message %s", m.ID)
		}
	}

	// user-1 (the owner) must see both.
	ownPage, err := uc.ListMessages(ctx, "user-1", "room-1", "", 20)
	if err != nil {
		t.Fatalf("ListMessages (user-1) failed: %v", err)
	}
	foundHuman, foundAI := false, false
	for _, m := range ownPage.Messages {
		if m.ID == result.HumanMessage.ID {
			foundHuman = true
		}
		if m.ID == result.AIMessage.ID {
			foundAI = true
		}
	}
	if !foundHuman || !foundAI {
		t.Fatalf("expected user-1's ListMessages to include both private messages, got human=%v ai=%v", foundHuman, foundAI)
	}
}

// TestRegenerateAIMessageNonOwnerPrivateForbidden asserts that a non-owner
// room member attempting to regenerate another user's private exchange gets
// domain.ErrForbidden, even though they pass the room-membership check.
func TestRegenerateAIMessageNonOwnerPrivateForbidden(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-1", "user-2", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "secret question", "test-model", true)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	_, _, err = uc.RegenerateAIMessage(ctx, "user-2", "room-1", result.HumanMessage.ID, "test-model")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for non-owner regenerating a private exchange, got %v", err)
	}

	// The owner can still regenerate their own private exchange.
	if _, _, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model"); err != nil {
		t.Fatalf("expected owner to regenerate their own private exchange, got error: %v", err)
	}
}

// TestSendAIMessageContextExcludesOtherUsersPrivateMessage asserts that
// user-2's own SendAIMessage call does not see user-1's earlier private
// message in the AI context, whether user-2's own request is public or
// private.
func TestSendAIMessageContextExcludesOtherUsersPrivateMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-1", "user-2", "member")
	roomRepo.SeedRoom("room-1", nil)

	var captured []ai.ChatMessage
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "user-1's secret", "test-model", true); err != nil {
		t.Fatalf("SendAIMessage (private, user-1) failed: %v", err)
	}

	t.Run("public request", func(t *testing.T) {
		captured = nil
		if _, err := uc.SendAIMessage(ctx, "user-2", "room-1", "user-2's public question", "test-model", false); err != nil {
			t.Fatalf("SendAIMessage (public, user-2) failed: %v", err)
		}
		if containsContent(captured, "user-1's secret") {
			t.Fatal("expected user-1's private message to be excluded from user-2's context")
		}
	})

	t.Run("private request", func(t *testing.T) {
		captured = nil
		if _, err := uc.SendAIMessage(ctx, "user-2", "room-1", "user-2's private question", "test-model", true); err != nil {
			t.Fatalf("SendAIMessage (private, user-2) failed: %v", err)
		}
		if containsContent(captured, "user-1's secret") {
			t.Fatal("expected user-1's private message to be excluded from user-2's own private context")
		}
	})
}

// TestSendAIMessagePrivateWSDeliveryTargetsOnlySender asserts that the
// message_created events published for a private SendAIMessage call carry
// TargetUserIDs restricted to the requester, whereas a public call publishes
// with a nil/empty TargetUserIDs (room broadcast).
func TestSendAIMessagePrivateWSDeliveryTargetsOnlySender(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "secret question", "test-model", true)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case evt := <-sub:
			if evt.TargetUserIDs == nil || len(evt.TargetUserIDs) != 1 || evt.TargetUserIDs[0] != "user-1" {
				t.Fatalf("expected TargetUserIDs [user-1] for a private event, got %v", evt.TargetUserIDs)
			}
			seen[evt.Message.ID] = true
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for private message events")
		}
	}
	if !seen[result.HumanMessage.ID] || !seen[result.AIMessage.ID] {
		t.Fatal("expected events for both the human and AI private messages")
	}
}

// client's existing RegenerateAIMessage retry path applies instead.
//
// enrichWithAttachments is exercised here (rather than the ListByRoom
// context-fetch step) because it is the failure the test doubles in this
// package can trigger deterministically; both steps share the exact same
// error-handling code path in SendAIMessage (see its doc comment), so this
// covers that shared path.
func TestSendAIMessageEnrichmentFailureSavesFailedPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	attachmentRepo := &mocks.AttachmentRepo{}
	objStorage := &mocks.ObjectStorage{}

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, attachmentRepo, objStorage, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// Seed an earlier message carrying an image attachment, so that a later
	// SendAIMessage call's context-assembly enrichment step has something to
	// fail on.
	attachmentBearing, err := uc.SendMessage(ctx, "user-1", "room-1", "check this out")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if err := attachmentRepo.Create(ctx, &domainattachment.Attachment{
		ID:        "att-1",
		RoomID:    "room-1",
		S3Key:     "attachments/room-1/att-1",
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, "att-1", attachmentBearing.ID, "room-1"); err != nil {
		t.Fatalf("AttachToMessage failed: %v", err)
	}

	// Force PresignView to fail for the enrichment step triggered by the
	// SendAIMessage call under test.
	objStorage.ShouldErr = true

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is it?", "test-model", false)
	if err == nil {
		t.Fatal("expected SendAIMessage to return the enrichment error")
	}
	if result != nil {
		t.Fatalf("expected a nil result on error, got %+v", result)
	}

	// Exactly one new human message (the "What is it?" content passed to
	// the failing call) and exactly one failed AI message (its placeholder)
	// should have been created; the pre-existing attachment-bearing message
	// must not have been duplicated or altered.
	var newHuman *domainmessage.Message
	var humanCount, aiCount int
	for _, m := range msgRepo.Messages {
		switch m.Type {
		case domainmessage.MessageTypeHuman:
			humanCount++
			if m.Content == "What is it?" {
				newHuman = m
			}
		case domainmessage.MessageTypeAI:
			aiCount++
		}
	}
	if humanCount != 2 {
		t.Fatalf("expected exactly 2 human messages (1 seeded + 1 new), got %d", humanCount)
	}
	if newHuman == nil {
		t.Fatal("expected to find the new human message created by the failing SendAIMessage call")
	}
	if aiCount != 1 {
		t.Fatalf("expected exactly 1 AI message (the failed placeholder), got %d", aiCount)
	}

	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the AI placeholder status to be failed, got %s", aiMsg.Status)
	}
	if aiMsg.Content != "" {
		t.Fatalf("expected empty content on the failed AI placeholder, got %q", aiMsg.Content)
	}
	if aiMsg.InResponseToMessageID == nil || *aiMsg.InResponseToMessageID != newHuman.ID {
		t.Fatalf("expected the failed AI placeholder to link back to the new human message %s, got %v", newHuman.ID, aiMsg.InResponseToMessageID)
	}

	// The retry path this placeholder exists for: RegenerateAIMessage on the
	// new human message must now succeed once the underlying failure clears.
	objStorage.ShouldErr = false
	regenerated, _, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", newHuman.ID, "test-model")
	if err != nil {
		t.Fatalf("expected RegenerateAIMessage to recover the failed placeholder, got error: %v", err)
	}
	if regenerated.ID != aiMsg.ID {
		t.Fatalf("expected RegenerateAIMessage to update the existing placeholder %s, got a different message %s", aiMsg.ID, regenerated.ID)
	}
	if regenerated.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected regenerated status completed, got %s", regenerated.Status)
	}
}

// TestSendAIMessageCompletedCreateFailureSavesFailedPlaceholder asserts that
// when the LLM call succeeds but msgRepo.Create for the resulting completed
// AI message itself fails, SendAIMessage still saves a status=failed AI
// placeholder linked to the human message (via
// saveFailedAIPlaceholderOnError) before returning the original error --
// exactly like the context-fetch, attachment-enrichment, and LLM-call
// failure paths -- instead of returning the bare error with no placeholder,
// which would leave a client retry unable to tell "still unanswered" from
// "never asked" and would resubmit and duplicate the human message.
func TestSendAIMessageCompletedCreateFailureSavesFailedPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	createErr := fmt.Errorf("simulated create failure")
	var createCalls int
	msgRepo.CreateFunc = func(_ context.Context, msg *domainmessage.Message) error {
		createCalls++
		// The 1st Create call persists the human message and the 3rd
		// persists the failed AI placeholder saved on this test's error
		// path; only the 2nd -- the completed AI message SendAIMessage
		// builds after a successful LLM call -- is made to fail.
		if createCalls == 2 {
			return createErr
		}
		msgRepo.Messages[msg.ID] = msg
		return nil
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model", false)
	if err != createErr {
		t.Fatalf("expected SendAIMessage to return the original create error, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected a nil result on error, got %+v", result)
	}
	if createCalls != 3 {
		t.Fatalf("expected 3 msgRepo.Create calls (human, failed completed AI, failed placeholder), got %d", createCalls)
	}

	var humanMsg, aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		switch m.Type {
		case domainmessage.MessageTypeHuman:
			humanMsg = m
		case domainmessage.MessageTypeAI:
			aiMsg = m
		}
	}
	if humanMsg == nil {
		t.Fatal("expected the human message to have been persisted despite the AI Create failure")
	}
	if aiMsg == nil {
		t.Fatal("expected a failed AI placeholder to have been persisted")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the AI placeholder status to be failed, got %s", aiMsg.Status)
	}
	if aiMsg.Content != "" {
		t.Fatalf("expected empty content on the failed AI placeholder, got %q", aiMsg.Content)
	}
	if aiMsg.InResponseToMessageID == nil || *aiMsg.InResponseToMessageID != humanMsg.ID {
		t.Fatalf("expected the failed AI placeholder to link back to the human message %s, got %v", humanMsg.ID, aiMsg.InResponseToMessageID)
	}
}

// --- Ownerless private message publish suppression ---

// TestPublishMessageEventSuppressesOwnerlessPrivateMessage asserts that
// publishMessageEvent does not call hub.Publish for a private message whose
// SenderID is nil (reachable in production because sender_id is
// ON DELETE SET NULL — see publishMessageEvent's doc comment): broadcasting
// it would be wrong (event.RoomEvent.TargetUserIDs' nil/empty meaning is
// "everyone"), and there is no TargetUserIDs value that means "nobody", so
// the only safe behavior is to drop the event rather than publish it either
// way.
func TestPublishMessageEventSuppressesOwnerlessPrivateMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	hub := event.NewInProcessHub()

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	orphaned := &domainmessage.Message{
		ID:         "msg-orphaned",
		RoomID:     "room-1",
		SenderID:   nil,
		Content:    "a private message whose sender row was later deleted",
		Type:       domainmessage.MessageTypeHuman,
		Visibility: domainmessage.MessageVisibilityPrivate,
	}

	// Call the unexported publish path directly (this test file is in
	// `package message`): every SendAIMessage/RegenerateAIMessage call site
	// goes through publishMessageEvent, so exercising it directly is the
	// most targeted way to assert the suppression itself, independent of
	// how an ownerless private message could arise upstream.
	uc.publishMessageEvent(ctx, event.EventMessageCreated, "room-1", orphaned, time.Now(), false)

	select {
	case evt := <-sub:
		t.Fatalf("expected no event to be published for an ownerless private message, got %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Expected: nothing published.
	}
}

// TestSendAIMessagePublishesNormallyForOwnedPrivateMessage is the control
// for TestPublishMessageEventSuppressesOwnerlessPrivateMessage: a private
// message that does have a SenderID (the ordinary case) must still be
// published (targeted at its owner), so the suppression added for the
// ownerless case does not regress normal private-message delivery.
func TestSendAIMessagePublishesNormallyForOwnedPrivateMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "secret question", "test-model", true); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	select {
	case evt := <-sub:
		if evt.Message.Visibility != domainmessage.MessageVisibilityPrivate {
			t.Fatalf("expected a private message event, got visibility %s", evt.Message.Visibility)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the owned private message's event")
	}
}

// --- Archived room guard (Step 32: room fork) ---

// noCallLLMGateway returns a *mocks.LLMGateway whose CompleteFunc/StreamFunc
// both call t.Error if ever invoked -- used by the archived-room guard tests
// below to prove the rejection happens before any LLM Gateway call, not just
// before a successful one.
func noCallLLMGateway(t *testing.T) *mocks.LLMGateway {
	t.Helper()
	return &mocks.LLMGateway{
		CompleteFunc: func(_ context.Context, _ *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			t.Error("Complete must not be called for a rejected archived-room request")
			return nil, errors.New("unexpected Complete call")
		},
		StreamFunc: func(_ context.Context, _ *ai.CompletionRequest) (<-chan ai.StreamResult, error) {
			t.Error("Stream must not be called for a rejected archived-room request")
			return nil, errors.New("unexpected Stream call")
		},
	}
}

// TestSendMessageArchivedRoom asserts that SendMessage rejects a new post
// into an archived room with domain.ErrArchivedRoom, before reserving any
// sequence number or calling the LLM Gateway.
func TestSendMessageArchivedRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.Rooms["room-1"] = &domainroom.Room{ID: "room-1", IsArchived: true}

	uc := NewMessageUsecase(msgRepo, roomRepo, noCallLLMGateway(t), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if seq := msgRepo.Seqs["room-1"]; seq != 0 {
		t.Fatalf("expected no sequence number to have been reserved, got next-sequence counter %d", seq)
	}
}

// TestSendAIMessageArchivedRoom asserts that SendAIMessage rejects a new
// post into an archived room with domain.ErrArchivedRoom, before reserving
// any sequence number or invoking the LLM Gateway.
func TestSendAIMessageArchivedRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.Rooms["room-1"] = &domainroom.Room{ID: "room-1", IsArchived: true}

	uc := NewMessageUsecase(msgRepo, roomRepo, noCallLLMGateway(t), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if seq := msgRepo.Seqs["room-1"]; seq != 0 {
		t.Fatalf("expected no sequence number to have been reserved, got next-sequence counter %d", seq)
	}
}

// TestSendAIMessageStreamArchivedRoom asserts that SendAIMessageStream
// rejects a new post into an archived room with domain.ErrArchivedRoom,
// before reserving any sequence number or invoking the LLM Gateway's
// streaming endpoint, mirroring SendMessage/SendAIMessage's matching guard.
func TestSendAIMessageStreamArchivedRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.Rooms["room-1"] = &domainroom.Room{ID: "room-1", IsArchived: true}

	uc := NewMessageUsecase(msgRepo, roomRepo, noCallLLMGateway(t), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if seq := msgRepo.Seqs["room-1"]; seq != 0 {
		t.Fatalf("expected no sequence number to have been reserved, got next-sequence counter %d", seq)
	}
}

// TestRegenerateAIMessageRejectsArchivedRoom asserts that RegenerateAIMessage
// rejects a regeneration request against an archived room with
// domain.ErrArchivedRoom, mirroring Send/SendAIMessage/SendAIMessageStream's
// identical guard. The human/AI pair is created before the room is marked
// archived, since SendAIMessage itself would already reject a new send into
// an archived room.
func TestRegenerateAIMessageRejectsArchivedRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("seed SendAIMessage failed: %v", err)
	}
	seqBefore := msgRepo.Seqs["room-1"]

	roomRepo.Rooms["room-1"].IsArchived = true

	// Swap in a gateway that fails the test if RegenerateAIMessage's
	// rejection somehow still reaches the LLM Gateway.
	uc.llmGateway = noCallLLMGateway(t)

	_, _, err = uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if seq := msgRepo.Seqs["room-1"]; seq != seqBefore {
		t.Fatalf("expected next-sequence counter to remain %d after a rejected regenerate, got %d", seqBefore, seq)
	}
}

// --- SendAIMessageStream tests (Step 51) ---

// collectUntilMessageUpdated drains sub, collecting every EventTokenChunk
// event addressed to aiMessageID, until an EventMessageUpdated event for
// that same message ID arrives, which it returns alongside the collected
// chunks. It fails the test if that takes longer than 3 seconds.
func collectUntilMessageUpdated(t *testing.T, sub <-chan event.RoomEvent, aiMessageID string) ([]event.RoomEvent, event.RoomEvent) {
	t.Helper()
	var chunks []event.RoomEvent
	deadline := time.After(3 * time.Second)
	for {
		select {
		case evt := <-sub:
			switch {
			case evt.Type == event.EventTokenChunk && evt.Chunk != nil && evt.Chunk.MessageID == aiMessageID:
				chunks = append(chunks, evt)
			case evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == aiMessageID:
				return chunks, evt
			}
		case <-deadline:
			t.Fatal("timed out waiting for stream completion event")
			return nil, event.RoomEvent{}
		}
	}
}

// waitForCondition polls cond every 5ms until it returns true, failing the
// test if that takes longer than 2 seconds. Used to synchronize on
// fire-and-forget background work (e.g. BillingGuard.RecordUsage) that has
// no event of its own to wait on.
func waitForCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// TestSendAIMessageStreamSuccess asserts the happy path: the returned
// AIMessage is immediately "streaming" with empty content, exactly one
// EventTokenChunk is published per chunk with a non-empty Delta (carrying
// the right MessageID), and a final EventMessageUpdated carries
// Status == "completed" with Content equal to the concatenation of every
// chunk's Delta.
func TestSendAIMessageStreamSuccess(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{
			{ID: "c1", Model: "test-model", Delta: "Go"},
			{ID: "c1", Model: "test-model", Delta: " is a language"},
			{ID: "c1", Model: "test-model", FinishReason: "stop", Usage: &ai.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}},
		},
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusStreaming {
		t.Fatalf("expected streaming status immediately, got %s", result.AIMessage.Status)
	}
	if result.AIMessage.Content != "" {
		t.Fatalf("expected empty content immediately, got %q", result.AIMessage.Content)
	}

	chunks, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 token chunk events (non-empty deltas only), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Chunk.Delta != "Go" || chunks[1].Chunk.Delta != " is a language" {
		t.Fatalf("unexpected chunk deltas: %q, %q", chunks[0].Chunk.Delta, chunks[1].Chunk.Delta)
	}
	for _, c := range chunks {
		if c.Chunk.MessageID != result.AIMessage.ID {
			t.Fatalf("expected chunk MessageID %s, got %s", result.AIMessage.ID, c.Chunk.MessageID)
		}
	}

	if final.Message.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status, got %s", final.Message.Status)
	}
	if final.Message.Content != "Go is a language" {
		t.Fatalf("expected concatenated content %q, got %q", "Go is a language", final.Message.Content)
	}

	// The persisted repo state should match what the events showed.
	persisted, err := msgRepo.GetByID(ctx, result.AIMessage.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if persisted.Content != "Go is a language" || persisted.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected persisted message to match final event, got %+v", persisted)
	}
}

// TestSendAIMessageStreamMidStreamFailureRetryable asserts that a mid-stream
// provider failure publishes token-chunk events for the chunks seen before
// the failure, then one EventMessageUpdated with Status == "failed" and
// Content equal to the partial concatenation, and that the resulting
// placeholder can be retried via RegenerateAIMessage exactly like a
// non-streaming failure.
func TestSendAIMessageStreamMidStreamFailureRetryable(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{
			{ID: "c1", Model: "test-model", Delta: "partial "},
		},
		StreamMidErr: fmt.Errorf("provider exploded"),
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}

	chunks, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if len(chunks) != 1 || chunks[0].Chunk.Delta != "partial " {
		t.Fatalf("expected 1 token chunk before failure, got %+v", chunks)
	}
	if final.Message.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", final.Message.Status)
	}
	if final.Message.Content != "partial " {
		t.Fatalf("expected partial content persisted, got %q", final.Message.Content)
	}

	// Retry via RegenerateAIMessage, switching to a working gateway.
	gw2 := &mocks.LLMGateway{
		CompletionResponse: &ai.CompletionResponse{Content: "recovered", Model: "test-model", PromptTokens: 3, OutputTokens: 2},
	}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, gw2, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	regenerated, _, err := uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != nil {
		t.Fatalf("RegenerateAIMessage after stream failure failed: %v", err)
	}
	if regenerated.Content != "recovered" || regenerated.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected successful regenerate, got %+v", regenerated)
	}
	if regenerated.ID != result.AIMessage.ID {
		t.Fatal("expected regenerate to update the existing streamed placeholder, not create a new one")
	}
}

// TestSendAIMessageStreamSynchronousDispatchFailure asserts that when
// llmGateway.Stream itself fails synchronously (bad model, connection
// refused, etc.), SendAIMessageStream returns no Go error, the returned
// AIMessage.Status is immediately "failed", and no EventTokenChunk is ever
// published.
func TestSendAIMessageStreamSynchronousDispatchFailure(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{StreamErr: fmt.Errorf("bad model")}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("expected no Go error on a synchronous dispatch failure, got %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status immediately, got %s", result.AIMessage.Status)
	}

	// Drain whatever events were already published (human/AI created, AI
	// updated-to-failed) and assert no EventTokenChunk ever appears.
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventTokenChunk {
				t.Fatal("expected no EventTokenChunk for a synchronous dispatch failure")
			}
		case <-deadline:
			return
		}
	}
}

// TestSendAIMessageStreamFallsBackToCompleteOnUnsupportedTransport is a
// forward-ported regression test (originally added alongside the
// domain.ErrStreamingUnsupported mechanism in gateway.GRPCClient.Stream):
// when llmGateway.Stream fails synchronously with
// domain.ErrStreamingUnsupported (exactly what gateway.GRPCClient.Stream
// returns when LLM_GATEWAY_TRANSPORT=grpc), the send must still succeed via
// a background fallback to the unary Complete call rather than being marked
// failed outright -- without this fallback, any streaming send over the
// gRPC transport is silently broken.
func TestSendAIMessageStreamFallsBackToCompleteOnUnsupportedTransport(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{
		StreamErr:          fmt.Errorf("%w: streaming not supported over grpc transport", domain.ErrStreamingUnsupported),
		CompletionResponse: &ai.CompletionResponse{Content: "fallback answer", Model: "test-model", PromptTokens: 7, OutputTokens: 3},
	}
	billing := &mocks.BillingGuard{}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, billing, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("expected no Go error on a fallback dispatch, got %v", err)
	}
	// The caller-visible contract is unchanged from the real-streaming happy
	// path: an immediate "streaming" placeholder, not "failed".
	if result.AIMessage.Status != domainmessage.MessageStatusStreaming {
		t.Fatalf("expected streaming status immediately (fallback is transparent to the caller), got %s", result.AIMessage.Status)
	}

	var final event.RoomEvent
	deadline := time.After(3 * time.Second)
loop:
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventTokenChunk {
				t.Fatal("expected no EventTokenChunk for the unary Complete fallback")
			}
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == result.AIMessage.ID {
				final = evt
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for the fallback's EventMessageUpdated")
		}
	}
	if final.Message.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status, got %s", final.Message.Status)
	}
	if final.Message.Content != "fallback answer" {
		t.Fatalf("expected fallback completion content, got %q", final.Message.Content)
	}

	persistedMsg, err := msgRepo.GetByID(ctx, result.AIMessage.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if persistedMsg.Content != "fallback answer" || persistedMsg.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected persisted message to match the fallback completion, got %+v", persistedMsg)
	}

	// RecordUsage is fire-and-forget, called just after the completion event
	// is published; poll LastRecordUsageCall (which locks internally, unlike
	// reading RecordUsageCalls directly) rather than racing on it.
	waitForCondition(t, func() bool {
		_, ok := billing.LastRecordUsageCall()
		return ok
	})
	call, _ := billing.LastRecordUsageCall()
	if call.PromptTokens != 7 || call.OutputTokens != 3 {
		t.Fatalf("expected RecordUsage called with the Complete response's token counts, got %+v", call)
	}
}

// cancelSensitiveMessageRepo wraps *mocks.MessageRepo, making
// UpdateAIResponse return ctx.Err() immediately if ctx is already Done() --
// unlike the embedded mock's own UpdateAIResponse (like every other
// in-memory fake method here), which discards its context parameter
// entirely and always succeeds regardless of cancellation. This gives
// TestSendAIMessageStreamDispatchFailureFinalizesDespitePreCancelledRequestCtx
// something to observe: a real postgres.MessageRepository call made against
// an already-cancelled context fails immediately (pgx checks ctx before
// issuing the query), a behavior the plain mock otherwise never simulates.
type cancelSensitiveMessageRepo struct {
	*mocks.MessageRepo
}

func (r *cancelSensitiveMessageRepo) UpdateAIResponse(ctx context.Context, id string, content string, status domainmessage.MessageStatus, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.MessageRepo.UpdateAIResponse(ctx, id, content, status, updatedAt)
}

// TestSendAIMessageStreamDispatchFailureFinalizesDespitePreCancelledRequestCtx
// is a regression test proving the synchronous LLM Gateway dispatch-failure
// branch derives its own detached finalizeCtx (context.WithoutCancel(ctx)
// plus a fresh streamFinalizeTimeout deadline) for its terminal
// UpdateAIResponse/publish calls, rather than using SendAIMessageStream's
// own request-scoped ctx directly. ctx is pre-cancelled before the call, as
// Echo would already have done by the time this cleanup work runs if it
// raced the HTTP handler's own return; msgRepo is wrapped (see
// cancelSensitiveMessageRepo) so UpdateAIResponse actually fails fast on an
// already-cancelled context, since the plain mock otherwise ignores
// cancellation entirely. Before the fix, this pre-cancelled ctx reached
// UpdateAIResponse directly, UpdateAIResponse failed, and the placeholder
// was left stuck at Status = MessageStatusStreaming forever (with
// SendAIMessageStream itself returning a bare context.Canceled error);
// after the fix, the placeholder still finalizes to
// Status = MessageStatusFailed.
func TestSendAIMessageStreamDispatchFailureFinalizesDespitePreCancelledRequestCtx(t *testing.T) {
	msgRepo := &cancelSensitiveMessageRepo{MessageRepo: &mocks.MessageRepo{}}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	gw := &mocks.LLMGateway{StreamErr: fmt.Errorf("bad model")}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancelled, as if the HTTP request had already completed.

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("expected no Go error on a synchronous dispatch failure despite the pre-cancelled request ctx, got %v", err)
	}
	if result == nil {
		t.Fatal("expected a non-nil result")
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the placeholder to finalize as failed despite the pre-cancelled request ctx, got %s", result.AIMessage.Status)
	}

	stored, err := msgRepo.GetByID(context.Background(), result.AIMessage.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the persisted placeholder status to be failed, got %s", stored.Status)
	}
}

// failingUpdateAIResponseRepo wraps mocks.MessageRepo to make every
// UpdateAIResponse call fail, for exercising the dispatch-failure branch
// where even the terminal placeholder update cannot be persisted.
type failingUpdateAIResponseRepo struct {
	*mocks.MessageRepo
}

func (r *failingUpdateAIResponseRepo) UpdateAIResponse(ctx context.Context, id string, content string, status domainmessage.MessageStatus, updatedAt time.Time) error {
	return fmt.Errorf("update unavailable")
}

// TestSendAIMessageStreamDispatchFailureWithFailedFinalizeStillReturnsResult
// asserts the dispatch-failure contract when the terminal placeholder
// update itself fails: both messages are already durable and the creation
// event was published, so the call must NOT surface a request error (which
// would invite a duplicate-creating client retry) — it returns the
// committed IDs with their current durable (still streaming) status.
func TestSendAIMessageStreamDispatchFailureWithFailedFinalizeStillReturnsResult(t *testing.T) {
	msgRepo := &failingUpdateAIResponseRepo{MessageRepo: &mocks.MessageRepo{}}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	gw := &mocks.LLMGateway{StreamErr: fmt.Errorf("bad model")}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	result, err := uc.SendAIMessageStream(context.Background(), "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("expected no Go error when the terminal update fails after dispatch failure, got %v", err)
	}
	if result == nil || result.HumanMessage == nil || result.AIMessage == nil {
		t.Fatalf("expected both committed messages in the result, got %+v", result)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusStreaming {
		t.Fatalf("expected the AI message to report its current durable (streaming) status, got %s", result.AIMessage.Status)
	}
}

// TestSendAIMessageStreamSummaryUsedFalse asserts that, in this step's
// standalone state (before Step 50's assembleAIContext lands), every
// published EventTokenChunk's SummaryUsed field is false.
func TestSendAIMessageStreamSummaryUsedFalse(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{{ID: "c1", Model: "test-model", Delta: "hi"}},
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}

	chunks, _ := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if len(chunks) != 1 {
		t.Fatalf("expected exactly 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Chunk.SummaryUsed {
		t.Fatal("expected SummaryUsed=false in this step's standalone state")
	}
}

// --- SendAIMessageStream billing guard tests (Step 42 parity) ---

// TestSendAIMessageStreamInsufficientBalance asserts that SendAIMessageStream
// returns domain.ErrInsufficientBalance immediately when the billing guard
// rejects, and that no messages are created (the guard runs before any
// message is persisted).
func TestSendAIMessageStreamInsufficientBalance(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	guard := &mocks.BillingGuard{CheckBalanceErr: domain.ErrInsufficientBalance}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != domain.ErrInsufficientBalance {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
	if len(msgRepo.Messages) != 0 {
		t.Fatalf("expected zero messages created, got %d", len(msgRepo.Messages))
	}
}

// TestSendAIMessageStreamRecordsUsage asserts that on a successful stream
// with a Usage-bearing final chunk, RecordUsage is called exactly once with
// the AI message's ID, the resolved model, and the final chunk's token
// counts.
func TestSendAIMessageStreamRecordsUsage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	guard := &mocks.BillingGuard{}
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{
			{ID: "c1", Model: "test-model", Delta: "hi"},
			{ID: "c1", Model: "test-model", FinishReason: "stop", Usage: &ai.Usage{PromptTokens: 4, CompletionTokens: 6, TotalTokens: 10}},
		},
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}
	collectUntilMessageUpdated(t, sub, result.AIMessage.ID)

	// RecordUsage is fire-and-forget, called just after the completion
	// event is published; poll LastRecordUsageCall (which locks internally,
	// unlike reading RecordUsageCalls directly) rather than racing on it.
	waitForCondition(t, func() bool {
		_, ok := guard.LastRecordUsageCall()
		return ok
	})

	call, _ := guard.LastRecordUsageCall()
	if call.AIMessageID != result.AIMessage.ID {
		t.Fatalf("expected RecordUsage aiMessageID %s, got %s", result.AIMessage.ID, call.AIMessageID)
	}
	if call.Model != "test-model" {
		t.Fatalf("expected RecordUsage model test-model, got %s", call.Model)
	}
	if call.PromptTokens != 4 || call.OutputTokens != 6 {
		t.Fatalf("expected RecordUsage tokens 4/6, got %d/%d", call.PromptTokens, call.OutputTokens)
	}
}

// TestSendAIMessageStreamRecordUsageErrorSwallowed asserts that a
// RecordUsage error does not affect the persisted/broadcast result of a
// successful stream — usage recording is fire-and-forget.
func TestSendAIMessageStreamRecordUsageErrorSwallowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	guard := &mocks.BillingGuard{RecordUsageErr: fmt.Errorf("db unavailable")}
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{
			{ID: "c1", Model: "test-model", Delta: "hi"},
			{ID: "c1", Model: "test-model", FinishReason: "stop", Usage: &ai.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}},
		},
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}

	_, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if final.Message.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status despite RecordUsage error, got %s", final.Message.Status)
	}
}

// TestSendAIMessageStreamNoUsageChunkSkipsRecordUsage asserts that a stream
// which ends without ever delivering a Usage-bearing chunk never calls
// RecordUsage.
func TestSendAIMessageStreamNoUsageChunkSkipsRecordUsage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	guard := &mocks.BillingGuard{}
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{{ID: "c1", Model: "test-model", Delta: "hi", FinishReason: "stop"}}, // no Usage
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}
	_, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if final.Message.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status, got %s", final.Message.Status)
	}

	time.Sleep(50 * time.Millisecond)
	if _, ok := guard.LastRecordUsageCall(); ok {
		t.Fatal("expected RecordUsage never called when the stream ends with no usage-bearing chunk")
	}
}

// TestSendAIMessageStreamMidStreamFailureNeverRecordsUsage asserts that a
// mid-stream failure never calls RecordUsage, even when chunks were
// delivered before the failure.
func TestSendAIMessageStreamMidStreamFailureNeverRecordsUsage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	guard := &mocks.BillingGuard{}
	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{{ID: "c1", Model: "test-model", Delta: "partial"}},
		StreamMidErr: fmt.Errorf("boom"),
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}
	_, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if final.Message.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", final.Message.Status)
	}

	time.Sleep(50 * time.Millisecond)
	if _, ok := guard.LastRecordUsageCall(); ok {
		t.Fatal("expected RecordUsage never called on a mid-stream failure")
	}
}

// TestSendAIMessageStreamListByRoomFailureFinalizesPlaceholder asserts that
// when the post-placeholder ListByRoom context-fetch call fails,
// SendAIMessageStream (a) returns the original error, (b) finalizes the
// streaming placeholder to Status = MessageStatusFailed instead of leaving
// it stuck at "streaming" forever, and (c) publishes a matching
// EventMessageUpdated -- regression test for that placeholder previously
// being abandoned on this error path.
// TestSendAIMessageStreamPlaceholderCreateFailureSavesFailedPlaceholder
// asserts that when msgRepo.Create for the initial streaming AI placeholder
// itself fails, SendAIMessageStream still saves a status=failed AI
// placeholder linked to the human message (via saveFailedAIPlaceholderOnError)
// before returning the original error -- mirroring
// TestSendAIMessageCompletedCreateFailureSavesFailedPlaceholder's identical
// fix for SendAIMessage's completed-AI-message Create failure -- instead of
// leaving the human message with no AI row at all, which would give
// RegenerateAIMessage's GetNextInRoom-based retry path nothing to act on.
func TestSendAIMessageStreamPlaceholderCreateFailureSavesFailedPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	createErr := fmt.Errorf("simulated create failure")
	var createCalls int
	msgRepo.CreateFunc = func(_ context.Context, msg *domainmessage.Message) error {
		createCalls++
		// The 1st Create call persists the human message and the 3rd
		// persists the failed AI placeholder saved on this test's error
		// path; only the 2nd -- the initial streaming placeholder
		// SendAIMessageStream builds before ever calling the LLM Gateway --
		// is made to fail.
		if createCalls == 2 {
			return createErr
		}
		msgRepo.Messages[msg.ID] = msg
		return nil
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != createErr {
		t.Fatalf("expected SendAIMessageStream to return the original create error, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected a nil result on error, got %+v", result)
	}
	if createCalls != 3 {
		t.Fatalf("expected 3 msgRepo.Create calls (human, failed streaming placeholder, failed placeholder), got %d", createCalls)
	}

	var humanMsg, aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		switch m.Type {
		case domainmessage.MessageTypeHuman:
			humanMsg = m
		case domainmessage.MessageTypeAI:
			aiMsg = m
		}
	}
	if humanMsg == nil {
		t.Fatal("expected the human message to have been persisted despite the AI placeholder Create failure")
	}
	if aiMsg == nil {
		t.Fatal("expected a failed AI placeholder to have been persisted")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the AI placeholder status to be failed, got %s", aiMsg.Status)
	}
	if aiMsg.Content != "" {
		t.Fatalf("expected empty content on the failed AI placeholder, got %q", aiMsg.Content)
	}
	if aiMsg.InResponseToMessageID == nil || *aiMsg.InResponseToMessageID != humanMsg.ID {
		t.Fatalf("expected the failed AI placeholder to link back to the human message %s, got %v", humanMsg.ID, aiMsg.InResponseToMessageID)
	}
}

func TestSendAIMessageStreamListByRoomFailureFinalizesPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	listErr := fmt.Errorf("boom: database unavailable")
	msgRepo.ListByRoomErr = listErr

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err == nil {
		t.Fatal("expected SendAIMessageStream to return the ListByRoom error")
	}
	if !errors.Is(err, listErr) {
		t.Fatalf("expected the original ListByRoom error, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected a nil result on error, got %+v", result)
	}

	// Find the streaming placeholder that was created and published before
	// the ListByRoom call failed.
	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg == nil {
		t.Fatal("expected an AI placeholder message to have been created before the failure")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the placeholder to be finalized as failed, got %s", aiMsg.Status)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == aiMsg.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for EventMessageUpdated after ListByRoom failure")
		}
	}
}

// TestSendAIMessageStreamContextAssemblyFailureFinalizesPlaceholder asserts
// that when assembleAIContext's attachment-enrichment step fails (forced via
// a PresignView error on an attachment linked to a message already in the
// fetched context), SendAIMessageStream (a) returns the original error,
// (b) finalizes the streaming placeholder to Status = MessageStatusFailed,
// and (c) publishes a matching EventMessageUpdated -- mirroring
// TestSendAIMessageStreamListByRoomFailureFinalizesPlaceholder for the later
// of the two pre-dispatch failure points.
func TestSendAIMessageStreamContextAssemblyFailureFinalizesPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	attachmentRepo := &mocks.AttachmentRepo{}
	objStorage := &mocks.ObjectStorage{}

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, attachmentRepo, objStorage, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	// Seed an earlier message carrying an image attachment, so the streaming
	// call's context-assembly enrichment step has something to fail on.
	attachmentBearing, err := uc.SendMessage(ctx, "user-1", "room-1", "check this out")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if err := attachmentRepo.Create(ctx, &domainattachment.Attachment{
		ID:        "att-1",
		RoomID:    "room-1",
		S3Key:     "attachments/room-1/att-1",
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, "att-1", attachmentBearing.ID, "room-1"); err != nil {
		t.Fatalf("AttachToMessage failed: %v", err)
	}

	// Force PresignView to fail for the enrichment step triggered by the
	// SendAIMessageStream call under test.
	objStorage.ShouldErr = true

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is it?", "test-model")
	if err == nil {
		t.Fatal("expected SendAIMessageStream to return the context-assembly error")
	}
	if result != nil {
		t.Fatalf("expected a nil result on error, got %+v", result)
	}

	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg == nil {
		t.Fatal("expected an AI placeholder message to have been created before the failure")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the placeholder to be finalized as failed, got %s", aiMsg.Status)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == aiMsg.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for EventMessageUpdated after context-assembly failure")
		}
	}
}
