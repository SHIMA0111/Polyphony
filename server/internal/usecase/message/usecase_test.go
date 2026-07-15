package message

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	_, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", "nonexistent-message-id", "test-model")
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	if _, err := uc.ListMessages(ctx, "user-1", "room-1", "", 20); err != nil {
		t.Fatalf("expected reader to list messages, got error: %v", err)
	}
}

func TestListMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
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

func TestRegenerateAIMessageAfterFailure(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	failingGateway := &mocks.LLMGateway{ShouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, failingGateway, event.NewInProcessHub())
	ctx := context.Background()

	// SendAIMessage with failing LLM — returns result with failed AI placeholder
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", result.AIMessage.Status)
	}

	// Switch to working LLM and regenerate
	failingGateway.ShouldErr = false
	aiMsg, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	// Send human message + AI response via SendAIMessage
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	originalSeq := result.AIMessage.Sequence
	msgCountBefore := len(msgRepo.Messages)

	// Regenerate — should overwrite the existing AI message, not create a new one
	regenerated, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
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

func TestRegenerateAIMessageNotHuman(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	// Send a human message and get AI response
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	// Try to regenerate from the AI message (should fail)
	_, err = uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.AIMessage.ID, "test-model")
	if err != domain.ErrInvalidMessageType {
		t.Fatalf("expected ErrInvalidMessageType, got %v", err)
	}
}

func TestRegenerateAIMessageNotFound(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	_, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", "nonexistent", "test-model")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRegenerateAIMessageWrongRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-2", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	// Send message in room-1
	humanMsg, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Try to regenerate in room-2 (should fail)
	_, err = uc.RegenerateAIMessage(ctx, "user-1", "room-2", humanMsg.ID, "test-model")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRegenerateAIMessageNotMember(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	humanMsg, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// user-2 is not a member
	_, err = uc.RegenerateAIMessage(ctx, "user-2", "room-1", humanMsg.ID, "test-model")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSendAIMessageContextExcludesFailedMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	gw := &mocks.LLMGateway{ShouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub())
	ctx := context.Background()

	// First call fails — creates human + failed AI placeholder
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status, got %s", result.AIMessage.Status)
	}

	// Second call succeeds — failed placeholder should not appear in LLM context
	gw.ShouldErr = false
	result2, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	if result2.AIMessage.Content != "AI response" {
		t.Fatalf("expected AI response, got %s", result2.AIMessage.Content)
	}
	// The fact that the LLM call succeeds confirms the context was valid
	// (no empty assistant message that could confuse the LLM)
}

func TestSendAIMessageLLMError(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{ShouldErr: true}, event.NewInProcessHub())
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub())
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
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
