package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
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
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{ShouldErr: true}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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

// TestSendAIHandlerUsedContextSummaryFalse asserts that an ordinary,
// under-budget SendAI call reports used_context_summary: false on the
// returned ai_message (the common case).
func TestSendAIHandlerUsedContextSummaryFalse(t *testing.T) {
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

	var resp SendAIMessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.AIMessage.UsedContextSummary {
		t.Fatal("expected ai_message.used_context_summary to be false for an under-budget context")
	}
}

// TestSendAIHandlerUsedContextSummaryTrue asserts that a SendAI call whose
// context overflows the model's resolved context window (forced here via a
// mocked, always-oversized TokenEstimateResponse) reports
// used_context_summary: true on the returned ai_message.
func TestSendAIHandlerUsedContextSummaryTrue(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	e := echo.New()
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, newOverflowGateway(), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	h := NewMessageHandler(uc)

	for i := 1; i <= 11; i++ {
		sendReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
			strings.NewReader(`{"content":"seed message"}`))
		sendReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		sendRec := httptest.NewRecorder()
		sendCtx := e.NewContext(sendReq, sendRec)
		sendCtx.SetParamNames("roomId")
		sendCtx.SetParamValues("room-1")
		sendCtx.Set("user_id", "user-1")
		if err := h.Send(sendCtx); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
		strings.NewReader(`{"content":"trigger overflow","model":"gpt-5-mini"}`))
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
	if !resp.AIMessage.UsedContextSummary {
		t.Fatal("expected ai_message.used_context_summary to be true for an overflowing context")
	}
}

// TestRegenerateAIHandler asserts that POST
// /rooms/:roomId/messages/:messageId/regenerate returns 200 with
// used_context_summary reflecting the mocked usecase's summarization
// decision, for both the false (under-budget) and true (overflowing) cases.
func TestRegenerateAIHandler(t *testing.T) {
	t.Run("used_context_summary false for an under-budget context", func(t *testing.T) {
		e, h := setupMessageTest(true)

		sendReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
			strings.NewReader(`{"content":"What is Go?","model":"test"}`))
		sendReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		sendRec := httptest.NewRecorder()
		sendCtx := e.NewContext(sendReq, sendRec)
		sendCtx.SetParamNames("roomId")
		sendCtx.SetParamValues("room-1")
		sendCtx.Set("user_id", "user-1")
		if err := h.SendAI(sendCtx); err != nil {
			t.Fatalf("seed SendAI error: %v", err)
		}
		var sendResp SendAIMessageResponse
		if err := json.Unmarshal(sendRec.Body.Bytes(), &sendResp); err != nil {
			t.Fatalf("failed to unmarshal seed response: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost,
			"/rooms/room-1/messages/"+sendResp.UserMessage.ID+"/regenerate",
			strings.NewReader(`{"model":"test"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-1", sendResp.UserMessage.ID)
		c.Set("user_id", "user-1")

		if err := h.RegenerateAI(c); err != nil {
			t.Fatalf("RegenerateAI error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp MessageResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.UsedContextSummary {
			t.Fatal("expected used_context_summary to be false for an under-budget context")
		}
	})

	t.Run("used_context_summary true for an overflowing context", func(t *testing.T) {
		msgRepo := &mocks.MessageRepo{}
		roomRepo := &mocks.RoomRepo{}
		roomRepo.SeedMember("room-1", "user-1", "member")
		roomRepo.SeedRoom("room-1", nil)
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, newOverflowGateway(), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
		e := echo.New()
		h := NewMessageHandler(uc)

		for i := 1; i <= 11; i++ {
			sendReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
				strings.NewReader(`{"content":"seed message"}`))
			sendReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			sendRec := httptest.NewRecorder()
			sendCtx := e.NewContext(sendReq, sendRec)
			sendCtx.SetParamNames("roomId")
			sendCtx.SetParamValues("room-1")
			sendCtx.Set("user_id", "user-1")
			if err := h.Send(sendCtx); err != nil {
				t.Fatalf("seed message %d: %v", i, err)
			}
		}

		aiReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai",
			strings.NewReader(`{"content":"trigger overflow","model":"gpt-5-mini"}`))
		aiReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		aiRec := httptest.NewRecorder()
		aiCtx := e.NewContext(aiReq, aiRec)
		aiCtx.SetParamNames("roomId")
		aiCtx.SetParamValues("room-1")
		aiCtx.Set("user_id", "user-1")
		if err := h.SendAI(aiCtx); err != nil {
			t.Fatalf("seed SendAI error: %v", err)
		}
		var aiResp SendAIMessageResponse
		if err := json.Unmarshal(aiRec.Body.Bytes(), &aiResp); err != nil {
			t.Fatalf("failed to unmarshal seed response: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost,
			"/rooms/room-1/messages/"+aiResp.UserMessage.ID+"/regenerate",
			strings.NewReader(`{"model":"gpt-5-mini"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "messageId")
		c.SetParamValues("room-1", aiResp.UserMessage.ID)
		c.Set("user_id", "user-1")

		if err := h.RegenerateAI(c); err != nil {
			t.Fatalf("RegenerateAI error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp MessageResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if !resp.UsedContextSummary {
			t.Fatal("expected used_context_summary to be true for an overflowing context")
		}
	})
}

// newOverflowGateway returns a mocks.LLMGateway configured so that any
// context estimate always exceeds the resolved budget, deterministically
// forcing MessageUsecase.assembleAIContext's overflow/summarization branch
// in handler-level tests.
func newOverflowGateway() *mocks.LLMGateway {
	return &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
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
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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
		uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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

	// An omitted exclude_from_ai field must not be silently treated as
	// `false` (which would un-exclude a message the caller never asked to
	// un-exclude) -- it must be rejected with 400.
	t.Run("400 missing exclude_from_ai field", func(t *testing.T) {
		e, h := setupMessageTest(true)

		req := httptest.NewRequest(http.MethodPatch, "/rooms/room-1/messages/msg-1",
			strings.NewReader(`{}`))
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
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
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

// TestMessageHandlerWhitespaceOnlyContent400 asserts that Send, SendAI, and
// StreamAI all reject whitespace-only content (spaces, tabs, newlines, or a
// mix) with HTTP 400, exactly like genuinely empty content -- otherwise a
// message that renders visually empty would still persist, and for the two
// AI endpoints would still invoke the LLM Gateway over nothing.
func TestMessageHandlerWhitespaceOnlyContent400(t *testing.T) {
	whitespaceContents := []string{" ", "   ", "\t", "\n", " \t\n "}

	for _, content := range whitespaceContents {
		bodyBytes, err := json.Marshal(map[string]string{"content": content})
		if err != nil {
			t.Fatalf("marshal request body for %q: %v", content, err)
		}
		body := string(bodyBytes)

		t.Run(fmt.Sprintf("Send/%q", content), func(t *testing.T) {
			e, h := setupMessageTest(true)
			req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages", strings.NewReader(body))
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
				t.Fatalf("expected 400 for content %q, got %d", content, rec.Code)
			}
		})

		t.Run(fmt.Sprintf("SendAI/%q", content), func(t *testing.T) {
			e, h := setupMessageTest(true)
			req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("roomId")
			c.SetParamValues("room-1")
			c.Set("user_id", "user-1")

			if err := h.SendAI(c); err != nil {
				t.Fatalf("SendAI error: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for content %q, got %d", content, rec.Code)
			}
		})

		t.Run(fmt.Sprintf("StreamAI/%q", content), func(t *testing.T) {
			e, h := setupMessageTest(true)
			req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai/stream", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("roomId")
			c.SetParamValues("room-1")
			c.Set("user_id", "user-1")

			if err := h.StreamAI(c); err != nil {
				t.Fatalf("StreamAI error: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for content %q, got %d", content, rec.Code)
			}
		})
	}
}

// --- StreamAI tests (Step 51) ---

// TestMessageHandlerStreamAI202 asserts StreamAI returns HTTP 202 with
// ai_message.status == "streaming" in the JSON body on success.
func TestMessageHandlerStreamAI202(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	gw := &mocks.LLMGateway{
		StreamChunks: []*ai.StreamChunk{
			{ID: "c1", Model: "test-model", Delta: "Hi"},
			{ID: "c1", Model: "test-model", FinishReason: "stop", Usage: &ai.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}},
		},
	}
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	e := echo.New()
	h := NewMessageHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai/stream",
		strings.NewReader(`{"content":"What is Go?","model":"test-model"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.StreamAI(c); err != nil {
		t.Fatalf("StreamAI error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var resp SendAIMessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.AIMessage.Status != "streaming" {
		t.Fatalf("expected ai_message.status streaming, got %s", resp.AIMessage.Status)
	}
}

// TestMessageHandlerStreamAIUsedContextSummaryTrue asserts that a StreamAI
// call whose context overflows the model's resolved context window (forced
// here via newOverflowGateway, exactly like
// TestSendAIHandlerUsedContextSummaryTrue) reports used_context_summary:
// true on the immediately-returned ai_message -- assembleAIContext runs
// synchronously before the background stream goroutine is started, so this
// does not race that goroutine.
func TestMessageHandlerStreamAIUsedContextSummaryTrue(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)

	e := echo.New()
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, newOverflowGateway(), event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	h := NewMessageHandler(uc)

	for i := 1; i <= 11; i++ {
		sendReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages",
			strings.NewReader(`{"content":"seed message"}`))
		sendReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		sendRec := httptest.NewRecorder()
		sendCtx := e.NewContext(sendReq, sendRec)
		sendCtx.SetParamNames("roomId")
		sendCtx.SetParamValues("room-1")
		sendCtx.Set("user_id", "user-1")
		if err := h.Send(sendCtx); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai/stream",
		strings.NewReader(`{"content":"trigger overflow","model":"gpt-5-mini"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.StreamAI(c); err != nil {
		t.Fatalf("StreamAI error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var resp SendAIMessageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !resp.AIMessage.UsedContextSummary {
		t.Fatal("expected ai_message.used_context_summary to be true for an overflowing context")
	}
}

// TestMessageHandlerStreamAI400EmptyContent asserts StreamAI returns HTTP
// 400 for empty content, matching SendAI's validation.
func TestMessageHandlerStreamAI400EmptyContent(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai/stream",
		strings.NewReader(`{"content":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.StreamAI(c); err != nil {
		t.Fatalf("StreamAI error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestMessageHandlerStreamAI400Private asserts StreamAI rejects a
// "private": true request with HTTP 400, since private streaming is not yet
// supported.
func TestMessageHandlerStreamAI400Private(t *testing.T) {
	e, h := setupMessageTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai/stream",
		strings.NewReader(`{"content":"secret","private":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.StreamAI(c); err != nil {
		t.Fatalf("StreamAI error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for private stream request, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "private mode is not supported for streaming yet") {
		t.Fatalf("expected private-mode-unsupported message, got %s", rec.Body.String())
	}
}

// TestMessageHandlerStreamAI402InsufficientBalance asserts StreamAI returns
// HTTP 402 with the same body SendAI returns, when the usecase returns
// domain.ErrInsufficientBalance.
func TestMessageHandlerStreamAI402InsufficientBalance(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	guard := &mocks.BillingGuard{CheckBalanceErr: domain.ErrInsufficientBalance}
	uc := msgusecase.NewMessageUsecase(msgRepo, roomRepo, &mocks.LLMGateway{}, event.NewInProcessHub(), guard, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, &mocks.ContextSummaryRepo{}, "gpt-5-mini")
	e := echo.New()
	h := NewMessageHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/ai/stream",
		strings.NewReader(`{"content":"Hello","model":"test"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.StreamAI(c); err != nil {
		t.Fatalf("StreamAI error: %v", err)
	}
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `{"message":"insufficient token balance"}`) {
		t.Fatalf("expected insufficient token balance body, got %s", rec.Body.String())
	}
}
