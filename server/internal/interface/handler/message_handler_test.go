package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
)

// mockMsgRepo for handler tests
type mockMsgRepoForHandler struct {
	messages map[string]*domainmessage.Message
	seqs     map[string]int64
}

func newMockMsgRepoForHandler() *mockMsgRepoForHandler {
	return &mockMsgRepoForHandler{
		messages: make(map[string]*domainmessage.Message),
		seqs:     make(map[string]int64),
	}
}

func (m *mockMsgRepoForHandler) Create(_ context.Context, msg *domainmessage.Message) error {
	m.messages[msg.ID] = msg
	return nil
}
func (m *mockMsgRepoForHandler) GetByID(_ context.Context, id string) (*domainmessage.Message, error) {
	msg, ok := m.messages[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return msg, nil
}
func (m *mockMsgRepoForHandler) ListByRoom(_ context.Context, roomID, _ string, limit int) (*domainmessage.CursorPage, error) {
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
func (m *mockMsgRepoForHandler) Delete(_ context.Context, id string) error {
	delete(m.messages, id)
	return nil
}
func (m *mockMsgRepoForHandler) GetNextSequence(_ context.Context, roomID string) (int64, error) {
	seq := m.seqs[roomID]
	m.seqs[roomID] = seq + 1
	return seq, nil
}

// mockRoomRepoForMsg for handler message tests
type mockRoomRepoForMsg struct {
	members map[string]map[string]bool
}

func newMockRoomRepoForMsg() *mockRoomRepoForMsg {
	return &mockRoomRepoForMsg{members: make(map[string]map[string]bool)}
}

func (m *mockRoomRepoForMsg) addMember(roomID, userID string) {
	if m.members[roomID] == nil {
		m.members[roomID] = make(map[string]bool)
	}
	m.members[roomID][userID] = true
}

func (m *mockRoomRepoForMsg) Create(_ context.Context, _ *domainroom.Room) error { return nil }
func (m *mockRoomRepoForMsg) GetByID(_ context.Context, _ string) (*domainroom.Room, error) {
	return nil, domain.ErrNotFound
}
func (m *mockRoomRepoForMsg) ListByUserID(_ context.Context, _ string) ([]*domainroom.Room, error) {
	return nil, nil
}
func (m *mockRoomRepoForMsg) Update(_ context.Context, _ *domainroom.Room) error { return nil }
func (m *mockRoomRepoForMsg) Delete(_ context.Context, _ string) error           { return nil }
func (m *mockRoomRepoForMsg) AddMember(_ context.Context, _ *domainroom.RoomMember) error {
	return nil
}
func (m *mockRoomRepoForMsg) GetMember(_ context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	if m.members[roomID] != nil && m.members[roomID][userID] {
		return &domainroom.RoomMember{RoomID: roomID, UserID: userID}, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockRoomRepoForMsg) ListMembers(_ context.Context, _ string) ([]*domainroom.RoomMember, error) {
	return nil, nil
}
func (m *mockRoomRepoForMsg) RemoveMember(_ context.Context, _, _ string) error { return nil }

type mockLLMForHandler struct {
	shouldErr bool
}

func (m *mockLLMForHandler) Complete(_ context.Context, _ *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	if m.shouldErr {
		return nil, fmt.Errorf("%w: mock", domain.ErrLLMGateway)
	}
	return &ai.CompletionResponse{Content: "AI response", Model: "test"}, nil
}
func (m *mockLLMForHandler) ListModels(_ context.Context) ([]ai.ModelInfo, error) { return nil, nil }

func setupMessageTest(isMember bool) (*echo.Echo, *MessageHandler) {
	msgRepo := newMockMsgRepoForHandler()
	roomRepo := newMockRoomRepoForMsg()
	if isMember {
		roomRepo.addMember("room-1", "user-1")
	}
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mockLLMForHandler{})
	return echo.New(), NewMessageHandler(uc)
}

func TestSendMessageHandler201(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
		strings.NewReader(`{"content":"Hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.Send(c); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestSendMessageHandler400(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
		strings.NewReader(`{"content":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.Send(c); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSendMessageHandler403(t *testing.T) {
	e, h := setupMessageTest(false)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
		strings.NewReader(`{"content":"Hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.Send(c); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestListMessagesHandler200(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/messages", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.List(c); err != nil {
		t.Fatalf("List error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSendAIHandler201(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
		strings.NewReader(`{"content":"What is Go?","model":"test"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.SendAI(c); err != nil {
		t.Fatalf("SendAI error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}
