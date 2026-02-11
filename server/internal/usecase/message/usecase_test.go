package message

import (
	"context"
	"fmt"
	"testing"

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

	aiMsg, err := uc.SendAIMessage(ctx, "user-1", "room-1", "What is Go?", "test-model")
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}
	if aiMsg.Content != "AI response" {
		t.Fatalf("expected AI response, got %s", aiMsg.Content)
	}
	if aiMsg.Type != domainmessage.MessageTypeAI {
		t.Fatalf("expected ai type, got %s", aiMsg.Type)
	}
	if aiMsg.SenderID != nil {
		t.Fatal("expected nil sender for AI message")
	}
}

func TestSendAIMessageLLMError(t *testing.T) {
	msgRepo := newMockMsgRepo()
	roomRepo := newMockRoomRepo()
	roomRepo.addMember("room-1", "user-1")

	uc := NewMessageUsecase(msgRepo, roomRepo, &mockLLMGateway{shouldErr: true})
	ctx := context.Background()

	_, err := uc.SendAIMessage(ctx, "user-1", "room-1", "Hello", "test-model")
	if err == nil {
		t.Fatal("expected error from LLM gateway")
	}
	if !domain.IsLLMGatewayError(err) {
		t.Fatalf("expected LLM gateway error, got %v", err)
	}
}
