package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
)

// MessageHandler handles HTTP requests for message endpoints, including
// sending, listing, AI-assisted messaging, and AI response regeneration.
// It delegates business logic to MessageUsecase.
type MessageHandler struct {
	usecase *msgusecase.MessageUsecase
}

// NewMessageHandler creates a new MessageHandler with the given MessageUsecase.
func NewMessageHandler(usecase *msgusecase.MessageUsecase) *MessageHandler {
	return &MessageHandler{usecase: usecase}
}

// Send handles POST /rooms/:roomId/messages. It sends a user message to the
// specified room. The authenticated user ID is extracted from the Echo context.
// On success it returns HTTP 201 with the created MessageResponse. It returns
// HTTP 400 for invalid input, HTTP 403 if the user lacks permission, HTTP 404
// if the room is not found, and HTTP 500 for unexpected errors.
func (h *MessageHandler) Send(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req SendMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "content is required"})
	}

	msg, err := h.usecase.SendMessage(c.Request().Context(), userID, roomID, req.Content)
	if err != nil {
		return handleMessageError(c, err)
	}

	return c.JSON(http.StatusCreated, toMessageResponse(msg))
}

// List handles GET /rooms/:roomId/messages. It returns a paginated list of
// messages in the specified room. Pagination is controlled by the optional
// "cursor" and "limit" query parameters. The limit is clamped between 1 and
// 100, defaulting to 20. On success it returns HTTP 200 with a
// MessageListResponse containing the messages and an optional next cursor.
func (h *MessageHandler) List(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	cursor := c.QueryParam("cursor")

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	// Clamp limit between 1 and 100
	limit = max(1, min(100, limit))

	page, err := h.usecase.ListMessages(c.Request().Context(), userID, roomID, cursor, limit)
	if err != nil {
		return handleMessageError(c, err)
	}

	messages := make([]MessageResponse, len(page.Messages))
	for i, msg := range page.Messages {
		messages[i] = toMessageResponse(msg)
	}

	return c.JSON(http.StatusOK, MessageListResponse{
		Messages:   messages,
		NextCursor: page.NextCursor,
	})
}

// SendAI handles POST /rooms/:roomId/messages/ai. It sends a user message and
// invokes the LLM Gateway to generate an AI response. The request body may
// optionally specify a model name. On success it returns HTTP 201 with a
// SendAIMessageResponse containing both the user message and the AI message;
// the AI message's used_context_summary reports whether its context included
// a summary of older room history (see MessageResponse.UsedContextSummary and
// usecase/message.MessageUsecase.assembleAIContext, Step 50). Check
// ai_message.status to determine if the LLM call succeeded ("completed") or
// failed ("failed"). It returns HTTP 400 for invalid input, HTTP 403 if the
// user lacks permission, HTTP 404 if the room is not found, and HTTP 502 if the
// AI service encounters an error.
func (h *MessageHandler) SendAI(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req SendAIMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "content is required"})
	}

	result, err := h.usecase.SendAIMessage(c.Request().Context(), userID, roomID, req.Content, req.Model, req.Private)
	if err != nil {
		return handleMessageError(c, err)
	}

	aiResp := toMessageResponse(result.AIMessage)
	aiResp.UsedContextSummary = result.UsedContextSummary

	return c.JSON(http.StatusCreated, SendAIMessageResponse{
		UserMessage: toMessageResponse(result.HumanMessage),
		AIMessage:   aiResp,
	})
}

// StreamAI handles POST /rooms/:roomId/messages/ai/stream. It sends a user
// message and invokes the LLM Gateway's streaming completion endpoint,
// returning immediately with HTTP 202 rather than waiting for the AI
// response to finish generating. The response body is the same
// SendAIMessageResponse shape SendAI returns; ai_message.status will be
// "streaming" on the happy path (or "failed" if the gateway rejected the
// request synchronously -- e.g. an unknown model), never "completed": the
// response is forwarded to WebSocket-connected room members as it arrives,
// via a sequence of "token_chunk" frames followed by a final
// "message_updated" frame once the stream ends (see
// websocket_handler.go and MessageUsecase.SendAIMessageStream).
//
// Private AI mode (SendAIMessageRequest.Private) is not supported by this
// endpoint yet: a request with "private": true is rejected with HTTP 400
// rather than silently broadcasting a private exchange to the whole room.
// It otherwise returns HTTP 400 for empty content, HTTP 403 if the user
// lacks permission, HTTP 404 if the room is not found, and HTTP 402 if the
// room's token balance is exhausted, matching SendAI's error mapping via
// handleMessageError.
func (h *MessageHandler) StreamAI(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req SendAIMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "content is required"})
	}
	if req.Private {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "private mode is not supported for streaming yet"})
	}

	result, err := h.usecase.SendAIMessageStream(c.Request().Context(), userID, roomID, req.Content, req.Model)
	if err != nil {
		return handleMessageError(c, err)
	}

	return c.JSON(http.StatusAccepted, SendAIMessageResponse{
		UserMessage: toMessageResponse(result.HumanMessage),
		AIMessage:   toMessageResponse(result.AIMessage),
	})
}

// RegenerateAI handles POST /rooms/:roomId/messages/:messageId/regenerate.
// It regenerates an AI response for an existing human message. The request body
// may optionally specify a different model. The target message must be of type
// "human"; otherwise HTTP 400 is returned. On success it returns HTTP 200 with
// the new AI MessageResponse, whose used_context_summary reports whether its
// context included a summary of older room history (see
// MessageResponse.UsedContextSummary and
// usecase/message.MessageUsecase.assembleAIContext, Step 50).
func (h *MessageHandler) RegenerateAI(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	messageID := c.Param("messageId")

	var req RegenerateAIMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	msg, usedSummary, err := h.usecase.RegenerateAIMessage(c.Request().Context(), userID, roomID, messageID, req.Model)
	if err != nil {
		return handleMessageError(c, err)
	}

	resp := toMessageResponse(msg)
	resp.UsedContextSummary = usedSummary

	return c.JSON(http.StatusOK, resp)
}

// Delete handles DELETE /rooms/:roomId/messages/:messageId. It soft-deletes
// a message: the caller must either be the message's own sender or hold at
// least admin in the room (see MessageUsecase.DeleteMessage for the exact
// owner-or-admin rule). On success it returns HTTP 204 with no content. It
// returns HTTP 403 if the caller lacks permission and HTTP 404 if the
// message does not exist or does not belong to the room.
func (h *MessageHandler) Delete(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	messageID := c.Param("messageId")

	if err := h.usecase.DeleteMessage(c.Request().Context(), userID, roomID, messageID); err != nil {
		return handleMessageError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// UpdateExclude handles PATCH /rooms/:roomId/messages/:messageId. It toggles
// whether a message is excluded from future AI context assembly. The caller
// must be allowed domainroom.ActionInvokeAI (member or above). On success it
// returns HTTP 200 with the updated MessageResponse. It returns HTTP 400 for
// invalid input, HTTP 403 if the caller lacks permission, and HTTP 404 if
// the message does not exist or does not belong to the room.
func (h *MessageHandler) UpdateExclude(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	messageID := c.Param("messageId")

	var req UpdateMessageExcludeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	msg, err := h.usecase.SetExcludeFromAI(c.Request().Context(), userID, roomID, messageID, req.ExcludeFromAI)
	if err != nil {
		return handleMessageError(c, err)
	}

	return c.JSON(http.StatusOK, toMessageResponse(msg))
}

func toMessageResponse(msg *domainmessage.Message) MessageResponse {
	return MessageResponse{
		ID:                    msg.ID,
		RoomID:                msg.RoomID,
		SenderID:              msg.SenderID,
		Content:               msg.Content,
		Type:                  string(msg.Type),
		Status:                string(msg.Status),
		Sequence:              msg.Sequence,
		InResponseToMessageID: msg.InResponseToMessageID,
		IsDeleted:             msg.IsDeleted,
		ExcludeFromAI:         msg.ExcludeFromAI,
		Visibility:            string(msg.Visibility),
		CreatedAt:             msg.CreatedAt,
		UpdatedAt:             msg.UpdatedAt,
	}
}

func handleMessageError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrForbidden) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
	}
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "not found"})
	}
	if errors.Is(err, domain.ErrLLMGateway) {
		middleware.GetLogger(c).Error("llm gateway error", "error", err)
		return c.JSON(http.StatusBadGateway, ErrorResponse{Message: "ai service error"})
	}
	if errors.Is(err, domain.ErrInvalidMessageType) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "message must be of type human"})
	}
	if errors.Is(err, domain.ErrInsufficientBalance) {
		return c.JSON(http.StatusPaymentRequired, ErrorResponse{Message: "insufficient token balance"})
	}
	if errors.Is(err, domain.ErrArchivedRoom) {
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "room is archived"})
	}
	if errors.Is(err, domain.ErrConflict) {
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "message is still streaming"})
	}
	middleware.GetLogger(c).Error("unhandled message error", "error", err)
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}
