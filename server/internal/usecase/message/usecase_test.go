package message

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
	ctx := context.Background()

	if _, err := uc.ListMessages(ctx, "user-1", "room-1", "", 20); err != nil {
		t.Fatalf("expected reader to list messages, got error: %v", err)
	}
}

func TestListMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	failingGateway := &mocks.LLMGateway{ShouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, failingGateway, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)
	roomRepo.SeedRoom("room-2", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	gw := &mocks.LLMGateway{ShouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{ShouldErr: true}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{})
	ctx := context.Background()

	toDelete, err := uc.SendMessage(ctx, "user-1", "room-1", "secret message")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if err := uc.DeleteMessage(ctx, "user-1", "room-1", toDelete.ID); err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model"); err != nil {
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
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{})
	ctx := context.Background()

	toExclude, err := uc.SendMessage(ctx, "user-1", "room-1", "private aside")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", toExclude.ID, true); err != nil {
		t.Fatalf("SetExcludeFromAI failed: %v", err)
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "follow up", "test-model"); err != nil {
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
	uc := NewMessageUsecase(msgRepo, roomRepo, captureCompletionMessages(&captured), event.NewInProcessHub(), &mocks.BillingGuard{})
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

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "new message after cutoff", "test-model"); err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if containsContent(captured, "ancient history") {
		t.Fatal("expected pre-cutoff message to be excluded from AI context")
	}
	if !containsContent(captured, "new message after cutoff") {
		t.Fatal("expected the new message to be included in AI context")
	}
}

// --- DeleteMessage ---

func TestDeleteMessageSenderCanDeleteOwnMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-1", "room-2", sent.ID); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for a message from a different room, got %v", err)
	}
}

// --- SetExcludeFromAI ---

func TestSetExcludeFromAIMemberAllowed(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
	ctx := context.Background()

	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", Content: "hello"},
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", "msg-1", true); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for reader, got %v", err)
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
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard)
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
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
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard)
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
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
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard)
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
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

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{})
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	guard := &mocks.BillingGuard{CheckBalanceErr: domain.ErrInsufficientBalance}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard)

	_, err = uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
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
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), setupGuard)
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	guard := &mocks.BillingGuard{}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard)

	regenerated, err := uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "regen-model")
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
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), setupGuard)
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	guard := &mocks.BillingGuard{RecordUsageErr: fmt.Errorf("db unavailable")}
	uc2 := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard)

	regenerated, err := uc2.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != nil {
		t.Fatalf("expected RecordUsage error to be swallowed, got error: %v", err)
	}
	if regenerated.Status != domainmessage.MessageStatusCompleted {
		t.Fatalf("expected completed status despite RecordUsage error, got %s", regenerated.Status)
	}
}
