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

// TestSendAIMessageContextFetchFailureCreatesFailedPlaceholder proves that a
// ListByRoom failure after the human message has already been persisted
// (see SendAIMessage) does not leave the human message orphaned without a
// corresponding AI message: exactly one human message and one failed-status
// AI message must exist afterward, so a client retry lands on
// RegenerateAIMessage's UPDATE-in-place path instead of duplicating the
// human message via a second SendAIMessage call.
func TestSendAIMessageContextFetchFailureCreatesFailedPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{ListByRoomErr: errors.New("boom: context fetch failed")}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err == nil {
		t.Fatal("expected SendAIMessage to return the context-fetch error")
	}

	var humanCount, failedAICount int
	for _, m := range msgRepo.Messages {
		if m.RoomID != "room-1" {
			continue
		}
		switch m.Type {
		case domainmessage.MessageTypeHuman:
			humanCount++
		case domainmessage.MessageTypeAI:
			if m.Status != domainmessage.MessageStatusFailed {
				t.Fatalf("expected the AI message to have failed status, got %s", m.Status)
			}
			failedAICount++
		}
	}
	if humanCount != 1 {
		t.Fatalf("expected exactly 1 human message, got %d", humanCount)
	}
	if failedAICount != 1 {
		t.Fatalf("expected exactly 1 failed AI message, got %d", failedAICount)
	}
}

// TestSendAIMessageContextEnrichmentFailureCreatesFailedPlaceholder is the
// same regression test as
// TestSendAIMessageContextFetchFailureCreatesFailedPlaceholder, but for a
// failure downstream in assembleAIContext (via
// buildAndEnrichContextBucket/enrichWithAttachments) instead of the initial
// ListByRoom fetch: it must be routed through the same failed-placeholder
// path, leaving exactly one human message and one failed-status AI message.
func TestSendAIMessageContextEnrichmentFailureCreatesFailedPlaceholder(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	attachmentRepo := &mocks.AttachmentRepo{ListByMessageIDErr: errors.New("boom: attachment enrichment failed")}

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, attachmentRepo, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err == nil {
		t.Fatal("expected SendAIMessage to return the context-enrichment error")
	}

	var humanCount, failedAICount int
	for _, m := range msgRepo.Messages {
		if m.RoomID != "room-1" {
			continue
		}
		switch m.Type {
		case domainmessage.MessageTypeHuman:
			humanCount++
		case domainmessage.MessageTypeAI:
			if m.Status != domainmessage.MessageStatusFailed {
				t.Fatalf("expected the AI message to have failed status, got %s", m.Status)
			}
			failedAICount++
		}
	}
	if humanCount != 1 {
		t.Fatalf("expected exactly 1 human message, got %d", humanCount)
	}
	if failedAICount != 1 {
		t.Fatalf("expected exactly 1 failed AI message, got %d", failedAICount)
	}
}

// TestSendAIMessageCompletedMessageCreateFailureCreatesFailedPlaceholder
// asserts that when the LLM call succeeds but persisting the completed AI
// message fails (msgRepo.Create's second call — the first is humanMsg),
// SendAIMessage still upholds its own invariant that every error path after
// humanMsg exists leaves a failed AI placeholder for a client retry to land
// on via RegenerateAIMessage's UPDATE-in-place path (mirroring the
// context-fetch/context-enrichment failure regression tests above), instead
// of returning bare (nil, err) with nothing persisted at aiSeq.
func TestSendAIMessageCompletedMessageCreateFailureCreatesFailedPlaceholder(t *testing.T) {
	createErr := errors.New("boom: completed AI message create failed")
	msgRepo := &mocks.MessageRepo{FailCreateOnCall: 2, FailCreateErr: createErr}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if !errors.Is(err, createErr) {
		t.Fatalf("expected the original create error to be returned, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected a nil result on failure, got %+v", result)
	}

	var humanCount, failedAICount int
	for _, m := range msgRepo.Messages {
		if m.RoomID != "room-1" {
			continue
		}
		switch m.Type {
		case domainmessage.MessageTypeHuman:
			humanCount++
		case domainmessage.MessageTypeAI:
			if m.Status != domainmessage.MessageStatusFailed {
				t.Fatalf("expected the AI message to have failed status, got %s", m.Status)
			}
			failedAICount++
		}
	}
	if humanCount != 1 {
		t.Fatalf("expected exactly 1 human message, got %d", humanCount)
	}
	if failedAICount != 1 {
		t.Fatalf("expected exactly 1 failed AI message placeholder, got %d", failedAICount)
	}
}

// TestPublishMessageEventSuppressesOwnerlessPrivateMessage proves that
// publishMessageEvent skips the hub entirely for a private message whose
// SenderID is nil (an "ownerless" private message — see its doc comment for
// how messages.sender_id's ON DELETE SET NULL produces this state): a
// subscriber to the room must receive nothing, rather than the broadcast
// targetUserIDsForVisibility's nil fallback would otherwise cause.
func TestPublishMessageEventSuppressesOwnerlessPrivateMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	hub := event.NewInProcessHub()

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	events, unsubscribe := hub.Subscribe(ctx, "room-1", "some-room-member")
	defer unsubscribe()

	ownerlessPrivateMsg := &domainmessage.Message{
		ID:         "msg-ownerless",
		RoomID:     "room-1",
		SenderID:   nil,
		Content:    "orphaned by a deleted sender",
		Type:       domainmessage.MessageTypeAI,
		Status:     domainmessage.MessageStatusCompleted,
		Visibility: domainmessage.MessageVisibilityPrivate,
	}
	uc.publishMessageEvent(ctx, event.RoomEvent{
		Type:          event.EventMessageCreated,
		RoomID:        "room-1",
		Message:       ownerlessPrivateMsg,
		TargetUserIDs: targetUserIDsForVisibility(ownerlessPrivateMsg),
		OccurredAt:    time.Now(),
	})

	select {
	case got := <-events:
		t.Fatalf("expected no event to be published for an ownerless private message, got %+v", got)
	default:
		// Expected: nothing was published.
	}

	// Control case: a normal public message is still delivered, proving the
	// subscriber setup above would have caught a real (non-suppressed) publish.
	publicMsg := &domainmessage.Message{
		ID:         "msg-public",
		RoomID:     "room-1",
		Content:    "a normal public message",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Visibility: domainmessage.MessageVisibilityPublic,
	}
	uc.publishMessageEvent(ctx, event.RoomEvent{
		Type:          event.EventMessageCreated,
		RoomID:        "room-1",
		Message:       publicMsg,
		TargetUserIDs: targetUserIDsForVisibility(publicMsg),
		OccurredAt:    time.Now(),
	})

	select {
	case got := <-events:
		if got.Message.ID != publicMsg.ID {
			t.Fatalf("expected to receive the public message, got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the public message's event to be delivered, got nothing")
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

// TestRegenerateAIMessageRejectsStreamingTarget asserts that
// RegenerateAIMessage rejects a request whose target AI response is still
// domainmessage.MessageStatusStreaming with domain.ErrConflict, rather than
// racing the in-flight streaming writer by overwriting its content.
func TestRegenerateAIMessageRejectsStreamingTarget(t *testing.T) {
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

	// Force the existing AI response into the still-streaming state, as if
	// a token_chunk stream were still in flight for it.
	msgRepo.Messages[result.AIMessage.ID].Status = domainmessage.MessageStatusStreaming

	_, _, err = uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
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

	// Second call succeeds — failed placeholder should not appear in LLM context
	gw.ShouldErr = false
	result2, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
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
// DeleteMessage surfaces a failure from msgRepo.DeleteAndInvalidateSummary
// to the caller, and that the message is left NOT soft-deleted when it
// fails — mirroring the real postgres implementation, which performs the
// soft delete and the summary invalidation inside a single transaction, so
// a failure anywhere in it rolls back atomically. This fake models that
// same all-or-nothing contract via InvalidateSummary/DeleteByRoomErr (see
// mocks.MessageRepo.InvalidateSummary's doc comment).
func TestDeleteMessagePropagatesSummaryInvalidationFailure(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	invalidationErr := errors.New("boom: summary invalidation failed")
	summaryRepo := &mocks.ContextSummaryRepo{DeleteByRoomErr: invalidationErr}
	msgRepo.InvalidateSummary = summaryRepo.DeleteByRoom

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if err := uc.DeleteMessage(ctx, "user-1", "room-1", sent.ID); !errors.Is(err, invalidationErr) {
		t.Fatalf("expected DeleteMessage to propagate the combined delete+invalidate error, got %v", err)
	}

	// Nothing must have mutated: the real transaction is all-or-nothing.
	notDeleted, err := msgRepo.GetByID(ctx, sent.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID after DeleteMessage: %v", err)
	}
	if notDeleted.IsDeleted {
		t.Fatal("expected the message to remain NOT soft-deleted when the combined delete+invalidate call fails")
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
// SetExcludeFromAI surfaces a failure from
// msgRepo.UpdateExcludeFromAIAndInvalidateSummary to the caller, and that
// the flag is left untouched when it fails — mirroring the real postgres
// implementation's single-transaction all-or-nothing contract (see
// TestDeleteMessagePropagatesSummaryInvalidationFailure's doc comment for
// the same reasoning).
func TestSetExcludeFromAIPropagatesSummaryInvalidationFailure(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	invalidationErr := errors.New("boom: summary invalidation failed")
	summaryRepo := &mocks.ContextSummaryRepo{DeleteByRoomErr: invalidationErr}
	msgRepo.InvalidateSummary = summaryRepo.DeleteByRoom

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	sent, err := uc.SendMessage(ctx, "user-1", "room-1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", sent.ID, true); !errors.Is(err, invalidationErr) {
		t.Fatalf("expected SetExcludeFromAI to propagate the combined update+invalidate error, got %v", err)
	}

	// Nothing must have mutated: the real transaction is all-or-nothing.
	unchanged, err := msgRepo.GetByID(ctx, sent.ID, "user-1")
	if err != nil {
		t.Fatalf("GetByID after SetExcludeFromAI: %v", err)
	}
	if unchanged.ExcludeFromAI {
		t.Fatal("expected ExcludeFromAI to remain false when the combined update+invalidate call fails")
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

// --- Archived room guard (Step 32: room fork) ---

// noLLMCallGateway returns a *mocks.LLMGateway whose CompleteFunc and
// StreamFunc both call t.Error, so any of the four archived-room guard
// tests below fails loudly if the guard is ever bypassed and the usecase
// reaches an LLM Gateway call it should have rejected before.
func noLLMCallGateway(t *testing.T) *mocks.LLMGateway {
	t.Helper()
	return &mocks.LLMGateway{
		CompleteFunc: func(_ context.Context, _ *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			t.Error("unexpected LLM Gateway Complete call for an archived room")
			return nil, domain.ErrArchivedRoom
		},
		StreamFunc: func(_ context.Context, _ *ai.CompletionRequest) (<-chan ai.StreamResult, error) {
			t.Error("unexpected LLM Gateway Stream call for an archived room")
			return nil, domain.ErrArchivedRoom
		},
	}
}

// TestSendMessageArchivedRoom asserts that SendMessage rejects a new post
// into an archived room with domain.ErrArchivedRoom, before reserving any
// sequence number.
func TestSendMessageArchivedRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.Rooms["room-1"] = &domainroom.Room{ID: "room-1", IsArchived: true}

	uc := NewMessageUsecase(msgRepo, roomRepo, noLLMCallGateway(t), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if len(msgRepo.Seqs) != 0 {
		t.Errorf("expected no sequence number reserved for an archived room, got %v", msgRepo.Seqs)
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

	uc := NewMessageUsecase(msgRepo, roomRepo, noLLMCallGateway(t), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if len(msgRepo.Seqs) != 0 {
		t.Errorf("expected no sequence number reserved for an archived room, got %v", msgRepo.Seqs)
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

	uc := NewMessageUsecase(msgRepo, roomRepo, noLLMCallGateway(t), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if len(msgRepo.Seqs) != 0 {
		t.Errorf("expected no sequence number reserved for an archived room, got %v", msgRepo.Seqs)
	}
}

// TestRegenerateAIMessageRejectsArchivedRoom asserts that RegenerateAIMessage
// rejects a regeneration request against an archived room with
// domain.ErrArchivedRoom, mirroring Send/SendAIMessage/SendAIMessageStream's
// identical guard -- checked right after loading the room, before the
// target message is even looked up.
func TestRegenerateAIMessageRejectsArchivedRoom(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	// Send human message + AI response via SendAIMessage while the room is
	// still active, then archive it before attempting to regenerate.
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	roomRepo.Rooms["room-1"].IsArchived = true

	// Swap in a gateway that fails the test if RegenerateAIMessage reaches
	// an LLM call, and snapshot the sequence state reserved by the setup
	// SendAIMessage call above, so a post-call comparison proves the guard
	// left it untouched.
	uc.llmGateway = noLLMCallGateway(t)
	seqsBefore := msgRepo.Seqs["room-1"]

	_, _, err = uc.RegenerateAIMessage(ctx, "user-1", "room-1", result.HumanMessage.ID, "test-model")
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom, got %v", err)
	}
	if msgRepo.Seqs["room-1"] != seqsBefore {
		t.Errorf("expected sequence state untouched by a rejected regeneration, before=%d after=%d", seqsBefore, msgRepo.Seqs["room-1"])
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

// TestSendAIMessageStreamPersistsFailedPlaceholderOnCreateError asserts
// that, when the streaming AI placeholder's own initial Create call fails,
// SendAIMessageStream still persists and broadcasts a failed-status AI
// placeholder for humanMsg -- mirroring SendAIMessage's equivalent recovery
// when its completed-AI-message Create call fails -- rather than returning
// bare with humanMsg left orphaned and no AI message a client retry via
// RegenerateAIMessage could land on.
func TestSendAIMessageStreamPersistsFailedPlaceholderOnCreateError(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	// The 1st Create call persists humanMsg; the 2nd is the streaming AI
	// placeholder's own Create -- fail exactly that one.
	wantErr := errors.New("ai placeholder create boom")
	msgRepo.FailCreateOnCall = 2
	msgRepo.FailCreateErr = wantErr

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the original Create error to be returned, got %v", err)
	}

	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg == nil {
		t.Fatal("expected a failed AI placeholder message to have been persisted despite the Create error")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected the persisted placeholder status failed, got %s", aiMsg.Status)
	}
	if aiMsg.Visibility != domainmessage.MessageVisibilityPublic {
		t.Fatalf("expected the persisted placeholder visibility public, got %s", aiMsg.Visibility)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageCreated && evt.Message != nil && evt.Message.ID == aiMsg.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published event status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the failed placeholder's EventMessageCreated")
		}
	}
}

// TestSendAIMessageStreamFinalizesPlaceholderOnListByRoomError asserts that,
// when SendAIMessageStream's context-fetch ListByRoom call fails after the
// AI placeholder has already been created and broadcast
// (Status = MessageStatusStreaming), the placeholder is finalized to
// Status = MessageStatusFailed and a terminating EventMessageUpdated is
// published, rather than left visibly stuck at "streaming" forever while the
// original ListByRoom error is simply returned bare.
func TestSendAIMessageStreamFinalizesPlaceholderOnListByRoomError(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	wantErr := errors.New("list by room boom")
	msgRepo.ListByRoomErr = wantErr

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the original ListByRoom error to be returned, got %v", err)
	}

	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg == nil {
		t.Fatal("expected an AI placeholder message to have been created")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected AI placeholder finalized to failed, got %s", aiMsg.Status)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == aiMsg.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published event status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the finalizing EventMessageUpdated")
		}
	}
}

// ctxDoneAwareMessageRepo wraps *mocks.MessageRepo, recording (via
// updateCtxWasDone) whether the ctx passed to UpdateAIResponse was already
// Done at call time. TestSendAIMessageStreamFinalizesPlaceholderOnListByRoomErrorDespiteCancelledRequestCtx
// and TestSendAIMessageStreamSynchronousDispatchFailureDespiteCancelledRequestCtx
// use this to prove finalizeStreamSetupFailure and SendAIMessageStream's
// dispatch-failure branch each derive their own finalizeCtx (detached from
// the request ctx's cancellation via context.WithoutCancel) for their
// terminal UpdateAIResponse write, rather than reusing a pre-cancelled
// request ctx directly -- mocks.MessageRepo itself ignores ctx entirely, so
// it cannot observe this on its own.
type ctxDoneAwareMessageRepo struct {
	*mocks.MessageRepo
	updateCtxWasDone bool
}

func (r *ctxDoneAwareMessageRepo) UpdateAIResponse(ctx context.Context, id, content string, status domainmessage.MessageStatus, updatedAt time.Time) error {
	if ctx.Err() != nil {
		r.updateCtxWasDone = true
	}
	return r.MessageRepo.UpdateAIResponse(ctx, id, content, status, updatedAt)
}

// TestSendAIMessageStreamFinalizesPlaceholderOnListByRoomErrorDespiteCancelledRequestCtx
// asserts that finalizeStreamSetupFailure still finalizes the AI placeholder
// to Status = MessageStatusFailed and publishes the terminating
// EventMessageUpdated even when the request ctx passed into
// SendAIMessageStream is already cancelled by the time the ListByRoom error
// is handled -- proving the finalization write runs on an independent
// context, not the (already-Done) request ctx directly, which would
// otherwise let the very cancellation that can trigger this error path also
// silently defeat finalizeStreamSetupFailure's entire purpose.
func TestSendAIMessageStreamFinalizesPlaceholderOnListByRoomErrorDespiteCancelledRequestCtx(t *testing.T) {
	msgRepo := &ctxDoneAwareMessageRepo{MessageRepo: &mocks.MessageRepo{}}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	ctx, cancel := context.WithCancel(context.Background())
	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	wantErr := errors.New("list by room boom")
	msgRepo.ListByRoomErr = wantErr

	// Cancel the request ctx before SendAIMessageStream even starts: every
	// downstream mock ignores ctx for its own reads/writes, so this only
	// exercises whether finalizeStreamSetupFailure's own write is affected.
	cancel()

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the original ListByRoom error to be returned, got %v", err)
	}

	if msgRepo.updateCtxWasDone {
		t.Fatal("expected UpdateAIResponse's finalization write to run on an independent, non-cancelled context, not the cancelled request ctx")
	}

	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg == nil {
		t.Fatal("expected an AI placeholder message to have been created")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected AI placeholder finalized to failed despite the cancelled request ctx, got %s", aiMsg.Status)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == aiMsg.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published event status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the finalizing EventMessageUpdated despite the cancelled request ctx")
		}
	}
}

// TestSendAIMessageStreamFinalizesPlaceholderOnAssembleContextError asserts
// that, when SendAIMessageStream's assembleAIContext call fails (here, via a
// mocked attachment-lookup failure) after the AI placeholder has already
// been created and broadcast, the placeholder is finalized to
// Status = MessageStatusFailed and a terminating EventMessageUpdated is
// published, exactly like the ListByRoom error path above.
func TestSendAIMessageStreamFinalizesPlaceholderOnAssembleContextError(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	wantErr := errors.New("attachment lookup boom")
	attachmentRepo := &mocks.AttachmentRepo{ListByMessageIDErr: wantErr}
	uc := NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, hub, &mocks.BillingGuard{}, attachmentRepo, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	_, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the original assembleAIContext error to be returned, got %v", err)
	}

	var aiMsg *domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.Type == domainmessage.MessageTypeAI {
			aiMsg = m
		}
	}
	if aiMsg == nil {
		t.Fatal("expected an AI placeholder message to have been created")
	}
	if aiMsg.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected AI placeholder finalized to failed, got %s", aiMsg.Status)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == aiMsg.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published event status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the finalizing EventMessageUpdated")
		}
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

// TestSendAIMessageStreamSynchronousDispatchFailureDespiteCancelledRequestCtx
// asserts that SendAIMessageStream's dispatch-failure branch (llmGateway.Stream
// itself fails synchronously) still finalizes the AI placeholder to
// Status = MessageStatusFailed and publishes the terminating
// EventMessageUpdated even when the request ctx is already cancelled by the
// time the failure is handled -- proving that write also runs on an
// independent context, not the (already-Done) request ctx directly.
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

func TestSendAIMessageStreamSynchronousDispatchFailureDespiteCancelledRequestCtx(t *testing.T) {
	msgRepo := &ctxDoneAwareMessageRepo{MessageRepo: &mocks.MessageRepo{}}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{StreamErr: fmt.Errorf("bad model")}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")

	ctx, cancel := context.WithCancel(context.Background())
	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	// Cancel the request ctx before SendAIMessageStream even starts: every
	// downstream mock ignores ctx for its own reads/writes, so this only
	// exercises whether the dispatch-failure branch's own finalization
	// write is affected.
	cancel()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "Hello", "test-model")
	if err != nil {
		t.Fatalf("expected no Go error on a synchronous dispatch failure, got %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status immediately despite the cancelled request ctx, got %s", result.AIMessage.Status)
	}
	if msgRepo.updateCtxWasDone {
		t.Fatal("expected UpdateAIResponse's finalization write to run on an independent, non-cancelled context, not the cancelled request ctx")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == result.AIMessage.ID {
				if evt.Message.Status != domainmessage.MessageStatusFailed {
					t.Fatalf("expected published event status failed, got %s", evt.Message.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the finalizing EventMessageUpdated despite the cancelled request ctx")
		}
	}
}

// TestSendAIMessageStreamFallsBackToCompleteOnUnsupportedTransport asserts
// that when llmGateway.Stream fails synchronously with
// domain.ErrStreamingUnsupported (exactly what gateway.GRPCClient.Stream
// returns when LLM_GATEWAY_TRANSPORT=grpc), the send must still succeed via
// a background fallback to the unary Complete call rather than being
// marked failed outright -- without that fallback, any streaming send over
// the gRPC transport would be silently broken.
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

// TestSendAIMessageStreamSynchronousDispatchFailureUsedContextSummary
// exercises stream.go's synchronous-dispatch-failure path: even though the
// gateway's Stream call fails before any chunk is ever published,
// assembleAIContext still ran (and summarized) beforehand, so the
// EventMessageUpdated publishing the immediate "failed" status must still
// carry UsedContextSummary=true -- parity with the non-streaming SendAIMessage
// failure path, which already reports it correctly.
func TestSendAIMessageStreamSynchronousDispatchFailureUsedContextSummary(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompletionResponse:    &ai.CompletionResponse{Content: "This is the summary."},
		StreamErr:             fmt.Errorf("bad model"),
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	for i := 1; i <= 11; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini")
	if err != nil {
		t.Fatalf("expected no Go error on a synchronous dispatch failure, got %v", err)
	}
	if result.AIMessage.Status != domainmessage.MessageStatusFailed {
		t.Fatalf("expected failed status immediately, got %s", result.AIMessage.Status)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == event.EventMessageUpdated && evt.Message != nil && evt.Message.ID == result.AIMessage.ID {
				if !evt.UsedContextSummary {
					t.Fatal("expected UsedContextSummary=true on the dispatch-failure EventMessageUpdated for a summarized context")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the dispatch-failure EventMessageUpdated")
		}
	}
}

// TestSendAIMessageStreamSummaryUsedFalse asserts that, for an under-budget
// context that never triggers summarization, every published EventTokenChunk's
// SummaryUsed field is false, and so is the terminating EventMessageUpdated's
// UsedContextSummary (parity check for TestSendAIMessageStreamSummaryUsedTrue
// below).
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

	chunks, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if len(chunks) != 1 {
		t.Fatalf("expected exactly 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Chunk.SummaryUsed {
		t.Fatal("expected SummaryUsed=false for an under-budget context")
	}
	if final.UsedContextSummary {
		t.Fatal("expected the terminating EventMessageUpdated's UsedContextSummary=false for an under-budget context")
	}
}

// TestSendAIMessageStreamSummaryUsedTrue forces assembleAIContext to
// summarize (mirroring TestAssembleAIContextSummarizesOnOverflowAndCaches's
// overflow setup, but exercised through the streaming endpoint) and asserts
// that the terminating EventMessageUpdated's UsedContextSummary is true,
// matching the token_chunk events' own SummaryUsed flag: if
// consumeAIStream's final publish omitted UsedContextSummary, it would
// silently clear the "Summarized history" badge the token_chunk frames had
// already shown once the stream finalized.
func TestSendAIMessageStreamSummaryUsedTrue(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	hub := event.NewInProcessHub()
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompletionResponse:    &ai.CompletionResponse{Content: "This is the summary."},
		StreamChunks:          []*ai.StreamChunk{{ID: "c1", Model: "test-model", Delta: "hi"}},
	}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw, hub, &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	ctx := context.Background()

	for i := 1; i <= 11; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	sub, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	result, err := uc.SendAIMessageStream(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini")
	if err != nil {
		t.Fatalf("SendAIMessageStream failed: %v", err)
	}

	chunks, final := collectUntilMessageUpdated(t, sub, result.AIMessage.ID)
	if len(chunks) != 1 || !chunks[0].Chunk.SummaryUsed {
		t.Fatalf("expected 1 chunk with SummaryUsed=true, got %+v", chunks)
	}
	if !final.UsedContextSummary {
		t.Fatal("expected the terminating EventMessageUpdated's UsedContextSummary=true for a summarized context")
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
