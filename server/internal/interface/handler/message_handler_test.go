package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
)

func setupMessageTest(isMember bool) (*echo.Echo, *MessageHandler) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	if isMember {
		roomRepo.SeedMember("room-1", "user-1", "member")
	}
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{})
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

	// Verify response contains both user_message and ai_message
	body := rec.Body.String()
	if !strings.Contains(body, `"user_message"`) {
		t.Fatal("response should contain user_message")
	}
	if !strings.Contains(body, `"ai_message"`) {
		t.Fatal("response should contain ai_message")
	}
	if !strings.Contains(body, `"updated_at"`) {
		t.Fatal("response should contain updated_at")
	}
}

func TestSendAIHandlerLLMFailure201(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{ShouldErr: true})
	e := echo.New()
	h := NewMessageHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
		strings.NewReader(`{"content":"Hello","model":"test"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.SendAI(c); err != nil {
		t.Fatalf("SendAI error: %v", err)
	}
	// Should still return 201 — both messages were created
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"failed"`) {
		t.Fatal("response should contain failed status for AI message")
	}
	if !strings.Contains(body, `"user_message"`) {
		t.Fatal("response should contain user_message")
	}
}
