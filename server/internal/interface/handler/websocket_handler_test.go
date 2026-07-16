package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/wsticket"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// setupWebSocketTest builds a WebSocketHandler over a fresh RoomRepo mock, a
// fresh InProcessHub, and a ticket issuer with a 1-minute TTL, along with a
// bare echo.Echo for constructing request contexts.
func setupWebSocketTest() (*echo.Echo, *WebSocketHandler, *mocks.RoomRepo, *event.InProcessHub, *wsticket.Issuer) {
	repo := &mocks.RoomRepo{}
	roomUC := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	hub := event.NewInProcessHub()
	issuer := wsticket.NewIssuer([]byte("test-secret"), time.Minute)
	h := NewWebSocketHandler(roomUC, hub, issuer, nil)
	e := echo.New()
	return e, h, repo, hub, issuer
}

func TestIssueTicket200(t *testing.T) {
	e, h, _, _, issuer := setupWebSocketTest()

	req := httptest.NewRequest(http.MethodPost, "/ws/ticket", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.IssueTicket(c); err != nil {
		t.Fatalf("IssueTicket handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp wsTicketResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ticket == "" {
		t.Fatal("expected non-empty ticket")
	}

	userID, err := issuer.Validate(resp.Ticket)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("expected user-1, got %q", userID)
	}
}

func TestHandleWS_MissingTicket401(t *testing.T) {
	e, h, repo, _, _ := setupWebSocketTest()
	repo.SeedMember("room-1", "user-1", "member")

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/ws", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleWS_InvalidTicket401(t *testing.T) {
	e, h, repo, _, _ := setupWebSocketTest()
	repo.SeedMember("room-1", "user-1", "member")

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/ws?ticket=not-a-real-ticket", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleWS_NonMember403(t *testing.T) {
	e, h, repo, _, issuer := setupWebSocketTest()
	// Seed a room and a different member, but not "user-1".
	repo.SeedMember("room-1", "someone-else", "member")

	ticket, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/ws?ticket="+ticket, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleWS_RoomNotFound404(t *testing.T) {
	e, h, repo, _, issuer := setupWebSocketTest()
	// Seed membership without seeding the room itself: checkMembership
	// succeeds (the member row exists) but the subsequent roomRepo.GetByID
	// lookup fails with domain.ErrNotFound, which is what this test exercises.
	repo.SeedMember("room-1", "user-1", "member")

	ticket, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/ws?ticket="+ticket, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestHandleWS_TwoClientsBroadcastAndTargeted is an integration-style test
// (real HTTP server, two real WebSocket clients, one room) proving both
// broadcast delivery (nil TargetUserIDs) and per-user-targeted delivery
// (TargetUserIDs set) work end-to-end through the shared event.MessageHub.
func TestHandleWS_TwoClientsBroadcastAndTargeted(t *testing.T) {
	repo := &mocks.RoomRepo{}
	roomID := "room-1"
	repo.SeedMember(roomID, "user-1", "member")
	repo.SeedMember(roomID, "user-2", "member")
	repo.Rooms = map[string]*domainroom.Room{
		roomID: {ID: roomID, Name: "Test Room", OwnerID: "user-1"},
	}
	roomUC := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)

	hub := event.NewInProcessHub()
	issuer := wsticket.NewIssuer([]byte("test-secret"), time.Minute)
	h := NewWebSocketHandler(roomUC, hub, issuer, nil)

	e := echo.New()
	e.GET("/rooms/:roomId/ws", h.Handle)
	srv := httptest.NewServer(e)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	ticket1, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue user-1 ticket: %v", err)
	}
	ticket2, err := issuer.Issue("user-2")
	if err != nil {
		t.Fatalf("Issue user-2 ticket: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn1, _, err := websocket.Dial(ctx, wsURL+"/rooms/"+roomID+"/ws?ticket="+ticket1, nil)
	if err != nil {
		t.Fatalf("dial client 1: %v", err)
	}
	defer func() { _ = conn1.Close(websocket.StatusNormalClosure, "") }()

	conn2, _, err := websocket.Dial(ctx, wsURL+"/rooms/"+roomID+"/ws?ticket="+ticket2, nil)
	if err != nil {
		t.Fatalf("dial client 2: %v", err)
	}
	defer func() { _ = conn2.Close(websocket.StatusNormalClosure, "") }()

	// Give the server a moment to register both subscriptions before publishing.
	time.Sleep(100 * time.Millisecond)

	msg1 := &domainmessage.Message{ID: "msg-1", RoomID: roomID, Content: "hello", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted}
	hub.Publish(ctx, event.RoomEvent{Type: event.EventMessageCreated, RoomID: roomID, Message: msg1, OccurredAt: time.Now()})

	var frame1a, frame2a wsEventFrame
	if err := wsjson.Read(ctx, conn1, &frame1a); err != nil {
		t.Fatalf("client 1 read broadcast: %v", err)
	}
	if err := wsjson.Read(ctx, conn2, &frame2a); err != nil {
		t.Fatalf("client 2 read broadcast: %v", err)
	}
	if frame1a.Type != string(event.EventMessageCreated) || frame1a.Message.ID != msg1.ID {
		t.Fatalf("client 1 got unexpected frame: %+v", frame1a)
	}
	if frame2a.Type != string(event.EventMessageCreated) || frame2a.Message.ID != msg1.ID {
		t.Fatalf("client 2 got unexpected frame: %+v", frame2a)
	}

	msg2 := &domainmessage.Message{ID: "msg-2", RoomID: roomID, Content: "targeted", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted}
	sentinel := &domainmessage.Message{ID: "sentinel", RoomID: roomID, Content: "sentinel", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted}

	hub.Publish(ctx, event.RoomEvent{Type: event.EventMessageCreated, RoomID: roomID, Message: msg2, TargetUserIDs: []string{"user-1"}, OccurredAt: time.Now()})
	hub.Publish(ctx, event.RoomEvent{Type: event.EventMessageCreated, RoomID: roomID, Message: sentinel, OccurredAt: time.Now()})

	// Client 1 should see the targeted event, then the broadcast sentinel.
	var frame1b, frame1c wsEventFrame
	if err := wsjson.Read(ctx, conn1, &frame1b); err != nil {
		t.Fatalf("client 1 read targeted: %v", err)
	}
	if frame1b.Message.ID != msg2.ID {
		t.Fatalf("client 1 expected targeted msg-2, got %+v", frame1b)
	}
	if err := wsjson.Read(ctx, conn1, &frame1c); err != nil {
		t.Fatalf("client 1 read sentinel: %v", err)
	}
	if frame1c.Message.ID != sentinel.ID {
		t.Fatalf("client 1 expected sentinel, got %+v", frame1c)
	}

	// Client 2's first read after the broadcast must be the sentinel,
	// proving the targeted event (msg-2) was never delivered to it.
	var frame2b wsEventFrame
	if err := wsjson.Read(ctx, conn2, &frame2b); err != nil {
		t.Fatalf("client 2 read: %v", err)
	}
	if frame2b.Message.ID != sentinel.ID {
		t.Fatalf("client 2 expected sentinel (targeted event must not be delivered), got %+v", frame2b)
	}
}
