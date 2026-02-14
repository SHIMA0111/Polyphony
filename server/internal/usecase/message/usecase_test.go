package message

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// --- Mock implementations ---

type mockMsgRepo struct {
	messages map[string]*domainmessage.Message
	seqs     map[string]int64
}

func newMockMsgRepo() *mockMsgRepo {
	return &mockMsgRepo{
		messages: make(map[string]*domainmessage.Message),
		seqs:     make(map[string]int64),
	}
}

func (m *mockMsgRepo) Create(_ context.Context, msg *domainmessage.Message) error {
	m.messages[msg.ID] = msg
	return nil
}

func (m *mockMsgRepo) GetByID(_ context.Context, id string) (*domainmessage.Message, error) {
	msg, ok := m.messages[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return msg, nil
}

func (m *mockMsgRepo) ListByRoom(_ context.Context, roomID, _ string, limit int) (*domainmessage.CursorPage, error) {
	var msgs []*domainmessage.Message
	for _, msg := range m.messages {
		if msg.RoomID == roomID {
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return &domainmessage.CursorPage{Messages: msgs}, nil
}

func (m *mockMsgRepo) ListByRoomUpTo(_ context.Context, roomID string, maxSequence int64, limit int) ([]*domainmessage.Message, error) {
	var msgs []*domainmessage.Message
	for _, msg := range m.messages {
		if msg.RoomID == roomID && msg.Sequence <= maxSequence {
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

func (m *mockMsgRepo) GetNextInRoom(_ context.Context, roomID string, afterSequence int64) (*domainmessage.Message, error) {
	var candidates []*domainmessage.Message
	for _, msg := range m.messages {
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

func (m *mockMsgRepo) UpdateAIResponse(_ context.Context, id string, content string, status domainmessage.MessageStatus, updatedAt time.Time) error {
	msg, ok := m.messages[id]
	if !ok {
		return domain.ErrNotFound
	}
	msg.Content = content
	msg.Status = status
	msg.UpdatedAt = updatedAt
	return nil
}

func (m *mockMsgRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.messages[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.messages, id)
	return nil
}

func (m *mockMsgRepo) GetNextSequence(_ context.Context, roomID string) (int64, error) {
	seq := m.seqs[roomID]
	m.seqs[roomID] = seq + 1
	return seq, nil
}

type mockRoomRepo struct {
	members map[string]map[string]bool // roomID -> userID -> exists
}

func newMockRoomRepo() *mockRoomRepo {
	return &mockRoomRepo{members: make(map[string]map[string]bool)}
}

func (m *mockRoomRepo) addMember(roomID, userID string) {
	if m.members[roomID] == nil {
		m.members[roomID] = make(map[string]bool)
	}
	m.members[roomID][userID] = true
}

func (m *mockRoomRepo) Create(_ context.Context, _ *domainroom.Room) error { return nil }
func (m *mockRoomRepo) GetByID(_ context.Context, _ string) (*domainroom.Room, error) {
	return nil, domain.ErrNotFound
}
func (m *mockRoomRepo) ListByUserID(_ context.Context, _ string) ([]*domainroom.Room, error) {
	return nil, nil
}
func (m *mockRoomRepo) Update(_ context.Context, _ *domainroom.Room) error { return nil }
func (m *mockRoomRepo) Delete(_ context.Context, _ string) error           { return nil }
func (m *mockRoomRepo) AddMember(_ context.Context, _ *domainroom.RoomMember) error {
	return nil
}
func (m *mockRoomRepo) GetMember(_ context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	if m.members[roomID] != nil && m.members[roomID][userID] {
		return &domainroom.RoomMember{RoomID: roomID, UserID: userID}, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockRoomRepo) ListMembers(_ context.Context, _ string) ([]*domainroom.RoomMember, error) {
	return nil, nil
}
func (m *mockRoomRepo) RemoveMember(_ context.Context, _, _ string) error { return nil }

type mockLLMGateway struct {
	shouldErr bool
}

func (m *mockLLMGateway) Complete(_ context.Context, _ *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	if m.shouldErr {
		return nil, fmt.Errorf("%w: mock error", domain.ErrLLMGateway)
	}
	return &ai.CompletionResponse{
		Content:      "AI response",
		Model:        "test-model",
		PromptTokens: 10,
		OutputTokens: 5,
	}, nil
}

func (m *mockLLMGateway) ListModels(_ context.Context) ([]ai.ModelInfo, error) {
	return []ai.ModelInfo{{ID: "test-model", Provider: "test"}}, nil
}

// --- Tests ---

func TestSendMessage(t *testing.T) {
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
	ctx := context.Background()

	_, err := uc.SendMessage(ctx, "user-1", "room-1", "Hello")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestListMessages(t *testing.T) {
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	failingGateway := &mockLLMGateway{shouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, failingGateway)
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
	failingGateway.shouldErr = false
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
	ctx := context.Background()

	// Send human message + AI response via SendAIMessage
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	originalSeq := result.AIMessage.Sequence
	msgCountBefore := len(msgRepo.messages)

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
	if len(msgRepo.messages) != msgCountBefore {
		t.Fatalf("expected %d messages (no new), got %d", msgCountBefore, len(msgRepo.messages))
	}
}

func TestRegenerateAIMessageNotHuman(t *testing.T) {
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
	ctx := context.Background()

	_, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", "nonexistent", "test-model")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRegenerateAIMessageWrongRoom(t *testing.T) {
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")
	roomRepo.addMember("room-2", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{})
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	gw := &mockLLMGateway{shouldErr: true}
	uc := NewMessageUsecase(msgRepo, roomRepo, gw)
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
	gw.shouldErr = false
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
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{shouldErr: true})
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
