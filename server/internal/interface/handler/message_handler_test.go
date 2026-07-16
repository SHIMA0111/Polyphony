package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
)

func setupMessageTest(isMember bool) (*echo.Echo, *MessageHandler) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	if isMember {
		roomRepo.SeedMember("room-1", "user-1", "member")
		roomRepo.SeedRoom("room-1", nil)
	}
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
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

// TestSendAIHandlerPrivate201 asserts that SendAI with "private": true
// returns HTTP 201 with visibility: "private" on both the user_message and
// ai_message in the response body.
func TestSendAIHandlerPrivate201(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
		strings.NewReader(`{"content":"secret question","model":"test","private":true}`))
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

	var resp SendAIMessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.UserMessage.Visibility != "private" {
		t.Fatalf("expected user_message visibility private, got %s", resp.UserMessage.Visibility)
	}
	if resp.AIMessage.Visibility != "private" {
		t.Fatalf("expected ai_message visibility private, got %s", resp.AIMessage.Visibility)
	}
}

// TestListHandlerExcludesOtherUsersPrivateMessage asserts that a private
// exchange created by user-1 via SendAI does not appear in another member's
// (user-2's) List call on the same room.
func TestListHandlerExcludesOtherUsersPrivateMessage(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-1", "user-2", "member")
	roomRepo.SeedRoom("room-1", nil)
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
	e := echo.New()
	h := NewMessageHandler(uc)

	sendReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
		strings.NewReader(`{"content":"secret question","model":"test","private":true}`))
	sendReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	sendRec := httptest.NewRecorder()
	sendCtx := e.NewContext(sendReq, sendRec)
	sendCtx.SetParamNames("roomId")
	sendCtx.SetParamValues("room-1")
	sendCtx.Set("user_id", "user-1")
	if err := h.SendAI(sendCtx); err != nil {
		t.Fatalf("SendAI error: %v", err)
	}
	if sendRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", sendRec.Code)
	}
	var sent SendAIMessageResponse
	if err := json.Unmarshal(sendRec.Body.Bytes(), &sent); err != nil {
		t.Fatalf("failed to unmarshal SendAI response: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/rooms/room-1/messages", nil)
	listRec := httptest.NewRecorder()
	listCtx := e.NewContext(listReq, listRec)
	listCtx.SetParamNames("roomId")
	listCtx.SetParamValues("room-1")
	listCtx.Set("user_id", "user-2")

	if err := h.List(listCtx); err != nil {
		t.Fatalf("List error: %v", err)
	}
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var list MessageListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to unmarshal List response: %v", err)
	}
	for _, m := range list.Messages {
		if m.ID == sent.UserMessage.ID || m.ID == sent.AIMessage.ID {
			t.Fatalf("expected user-2's message list to exclude private message %s", m.ID)
		}
	}
}

func TestSendAIHandlerLLMFailure201(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{ShouldErr: true}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
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

// TestMessageHandlerDelete covers DELETE /rooms/:roomId/messages/:messageId:
// 204 on success (sender deleting their own message), 403 for a non-sender
// non-admin member, and 404 for a message that does not belong to the room.
func TestMessageHandlerDelete(t *testing.T) {
	t.Run("204 sender deletes own message", func(t *testing.T) {
		e, h := setupMessageTest(true)

		sendReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
			strings.NewReader(`{"content":"hello"}`))
		sendReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		sendRec := httptest.NewRecorder()
		sendCtx := e.NewContext(sendReq, sendRec)
		sendCtx.SetParamNames("roomId")
		sendCtx.SetParamValues("room-1")
		sendCtx.Set("user_id", "user-1")
		if err := h.Send(sendCtx); err != nil {
			t.Fatalf("Send error: %v", err)
		}
		var sent MessageResponse
		if err := json.Unmarshal(sendRec.Body.Bytes(), &sent); err != nil {
			t.Fatalf("failed to unmarshal sent message: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/rooms/room-1/messages/"+sent.ID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-1", sent.ID)
		c.Set("user_id", "user-1")

		if err := h.Delete(c); err != nil {
			t.Fatalf("Delete error: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
	})

	t.Run("403 non-sender non-admin", func(t *testing.T) {
		msgRepo := &mocks.MessageRepo{}
		roomRepo := &mocks.RoomRepo{}
		roomRepo.SeedMember("room-1", "user-1", "member")
		roomRepo.SeedMember("room-1", "user-2", "member")
		roomRepo.SeedRoom("room-1", nil)
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
		e := echo.New()
		h := NewMessageHandler(uc)

		sent, err := uc.SendMessage(context.Background(), "user-1", "room-1", "hello")
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/rooms/room-1/messages/"+sent.ID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-1", sent.ID)
		c.Set("user_id", "user-2")

		if err := h.Delete(c); err != nil {
			t.Fatalf("Delete error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("404 message not in room", func(t *testing.T) {
		msgRepo := &mocks.MessageRepo{}
		roomRepo := &mocks.RoomRepo{}
		roomRepo.SeedMember("room-1", "user-1", "member")
		roomRepo.SeedMember("room-2", "user-1", "member")
		roomRepo.SeedRoom("room-1", nil)
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
		e := echo.New()
		h := NewMessageHandler(uc)

		sent, err := uc.SendMessage(context.Background(), "user-1", "room-1", "hello")
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/rooms/room-2/messages/"+sent.ID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-2", sent.ID)
		c.Set("user_id", "user-1")

		if err := h.Delete(c); err != nil {
			t.Fatalf("Delete error: %v", err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

// TestMessageHandlerUpdateExclude covers
// PATCH /rooms/:roomId/messages/:messageId: 200 with the updated body on
// success, and 400 for a malformed JSON request body.
func TestMessageHandlerUpdateExclude(t *testing.T) {
	t.Run("200 updates exclude_from_ai", func(t *testing.T) {
		msgRepo := &mocks.MessageRepo{}
		roomRepo := &mocks.RoomRepo{}
		roomRepo.SeedMember("room-1", "user-1", "member")
		roomRepo.SeedRoom("room-1", nil)
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
		e := echo.New()
		h := NewMessageHandler(uc)

		sent, err := uc.SendMessage(context.Background(), "user-1", "room-1", "hello")
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPatch, "/rooms/room-1/messages/"+sent.ID,
			strings.NewReader(`{"exclude_from_ai":true}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-1", sent.ID)
		c.Set("user_id", "user-1")

		if err := h.UpdateExclude(c); err != nil {
			t.Fatalf("UpdateExclude error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp MessageResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if !resp.ExcludeFromAI {
			t.Fatal("expected exclude_from_ai true in response")
		}
	})

	t.Run("400 invalid body", func(t *testing.T) {
		e, h := setupMessageTest(true)

		req := httptest.NewRequest(http.MethodPatch, "/rooms/room-1/messages/msg-1",
			strings.NewReader(`{invalid`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-1", "msg-1")
		c.Set("user_id", "user-1")

		if err := h.UpdateExclude(c); err != nil {
			t.Fatalf("UpdateExclude error: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

// TestSendAIHandlerInsufficientBalance402 asserts that SendAI returns HTTP
// 402 with body {"message":"insufficient token balance"} when the usecase
// returns domain.ErrInsufficientBalance (Step 42).
func TestSendAIHandlerInsufficientBalance402(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	guard := &mocks.BillingGuard{CheckBalanceErr: domain.ErrInsufficientBalance}
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, "gpt-5-mini")
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
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `{"message":"insufficient token balance"}`) {
		t.Fatalf("expected insufficient token balance body, got %s", rec.Body.String())
	}
}
