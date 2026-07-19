package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/wsticket"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// wsWriteTimeout bounds how long a single outbound event write (Handle's
// wsjson.Write call) may block. Without a deadline, a slow or stalled peer
// (e.g. a client that stopped reading but never closed the TCP connection)
// would let a write hang indefinitely, tying up the goroutine and the
// underlying event.MessageHub subscription for that connection.
const wsWriteTimeout = 5 * time.Second

// wsEventFrame is the JSON wire frame forwarded to a connected WebSocket
// client for every event.RoomEvent delivered to it. The top-level "type" and
// "room_id" fields are the stable contract; Message and Chunk are mutually
// exclusive payload fields, keyed by Type: Message is populated (as a
// pointer, so it is omitted entirely rather than serialized as a zero-value
// object) for "message_created"/"message_updated" frames, and Chunk is
// populated for "token_chunk" frames (Step 51's AI streaming chunk
// forwarding).
type wsEventFrame struct {
	Type    string           `json:"type"`
	RoomID  string           `json:"room_id"`
	Message *MessageResponse `json:"message,omitempty"`
	Chunk   *ChunkResponse   `json:"chunk,omitempty"`
}

// ChunkResponse is the JSON payload of a "token_chunk" wsEventFrame,
// mirroring event.StreamChunkEvent.
type ChunkResponse struct {
	MessageID   string `json:"message_id"`
	Delta       string `json:"delta"`
	SummaryUsed bool   `json:"summary_used"`
}

// WebSocketHandler handles the WebSocket ticket-issuance and connection
// endpoints that let clients receive real-time room events
// (event.RoomEvent) pushed by MessageUsecase, rather than only discovering
// new messages by polling GET /rooms/:roomId/messages.
type WebSocketHandler struct {
	roomUsecase  *roomusecase.RoomUsecase
	hub          event.MessageHub
	ticketIssuer *wsticket.Issuer
	// originPatterns lists the host patterns (derived from the server's
	// configured CORS origins) that Handle authorizes for the WebSocket
	// handshake's origin check.
	originPatterns []string
}

// NewWebSocketHandler creates a WebSocketHandler. originPatterns are the
// host patterns (e.g. "localhost:3000") authorized to open cross-origin
// WebSocket connections; see websocket.AcceptOptions.OriginPatterns.
func NewWebSocketHandler(roomUsecase *roomusecase.RoomUsecase, hub event.MessageHub, ticketIssuer *wsticket.Issuer, originPatterns []string) *WebSocketHandler {
	return &WebSocketHandler{
		roomUsecase:    roomUsecase,
		hub:            hub,
		ticketIssuer:   ticketIssuer,
		originPatterns: originPatterns,
	}
}

// IssueTicket handles POST /ws/ticket. It is mounted behind the existing
// authenticated JWTAuth group. It mints a short-lived WebSocket ticket for
// the caller (identified via middleware.GetUserID) and returns it as
// {"ticket": "...", "expires_in": 60}. The client then passes this ticket as
// a query parameter when opening the WebSocket connection, since browsers
// cannot attach an Authorization header to the handshake request. Returns
// HTTP 500 if ticket issuance fails.
func (h *WebSocketHandler) IssueTicket(c echo.Context) error {
	userID := middleware.GetUserID(c)

	ticket, err := h.ticketIssuer.Issue(userID)
	if err != nil {
		middleware.GetLogger(c).Error("issue ws ticket failed", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.JSON(http.StatusOK, wsTicketResponse{Ticket: ticket, ExpiresIn: wsTicketExpiresInSeconds})
}

// wsTicketExpiresInSeconds is the ticket lifetime reported in IssueTicket's
// response body. It documents the lifetime configured on the ticket issuer
// wired up in the DI container (60 seconds); it is not itself enforced here
// — actual expiry is enforced by wsticket.Issuer.Validate.
const wsTicketExpiresInSeconds = 60

// wsTicketResponse is the response body for POST /ws/ticket.
type wsTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
}

// Handle handles GET /rooms/:roomId/ws. It is mounted as a public route:
// browsers cannot send the ticket as an Authorization header on a WebSocket
// handshake, so authentication is performed manually inside this method
// (via the ticket query parameter) rather than through the JWTAuth
// middleware. On success it upgrades the connection and streams every
// event.RoomEvent published for the room (filtered by the hub according to
// TargetUserIDs) to the client as JSON frames until the client disconnects,
// at which point it unsubscribes from the hub.
//
// Before the upgrade, it responds with HTTP 401 if the ticket is missing or
// invalid/expired, HTTP 403 if the authenticated user is not a member of the
// room, HTTP 404 if the room does not exist, and HTTP 500 for any other
// error. After the upgrade succeeds, no further JSON error body can be
// written (the connection has switched protocols), so failures at that
// point only close the WebSocket connection.
func (h *WebSocketHandler) Handle(c echo.Context) error {
	ticket := c.QueryParam("ticket")
	if ticket == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "missing ticket"})
	}

	userID, err := h.ticketIssuer.Validate(ticket)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid or expired ticket"})
	}

	roomID := c.Param("roomId")

	ctx := c.Request().Context()
	// The room value itself is intentionally discarded: GetRoom is called
	// only for its membership-check error return, so this handler keeps
	// compiling regardless of whether GetRoom's return type changes to a
	// role-aware wrapper as part of the RBAC model landing in the same wave.
	if _, err := h.roomUsecase.GetRoom(ctx, userID, roomID); err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
		}
		if errors.Is(err, domain.ErrNotFound) {
			return c.JSON(http.StatusNotFound, ErrorResponse{Message: "not found"})
		}
		middleware.GetLogger(c).Error("ws handshake room lookup failed", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	conn, err := websocket.Accept(c.Response(), c.Request(), &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		// Accept has already written an error response to the ResponseWriter.
		return nil
	}
	defer func() { _ = conn.CloseNow() }()

	ch, unsubscribe := h.hub.Subscribe(ctx, roomID, userID)
	defer unsubscribe()

	// CloseRead starts a background reader that discards any client-sent
	// frames, answers ping/pong and close control frames, and cancels the
	// returned context when the peer disconnects or the connection errors.
	// This connection is server-push-only, so the event-forwarding loop
	// below is driven off this derived context rather than a hand-rolled
	// read loop.
	readCtx := conn.CloseRead(ctx)

	for {
		select {
		case <-readCtx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}

			frame := wsEventFrame{
				Type:   string(ev.Type),
				RoomID: ev.RoomID,
			}
			if ev.Chunk != nil {
				frame.Chunk = &ChunkResponse{
					MessageID:   ev.Chunk.MessageID,
					Delta:       ev.Chunk.Delta,
					SummaryUsed: ev.Chunk.SummaryUsed,
				}
			} else {
				msg := toMessageResponse(ev.Message)
				// UsedContextSummary is a one-time, request-scoped signal
				// (see event.RoomEvent.UsedContextSummary/handler.
				// MessageResponse.UsedContextSummary's doc comments): it is
				// never persisted on the domain message itself, so
				// toMessageResponse always defaults it to false and it must
				// be copied across separately from the originating
				// RoomEvent.
				msg.UsedContextSummary = ev.UsedContextSummary
				frame.Message = &msg
			}

			writeCtx, cancel := context.WithTimeout(readCtx, wsWriteTimeout)
			err := wsjson.Write(writeCtx, conn, frame)
			cancel()
			if err != nil {
				return nil
			}
		}
	}
}
