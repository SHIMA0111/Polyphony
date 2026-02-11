package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	msgusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/message"
)

// MessageHandler handles message HTTP requests.
type MessageHandler struct {
	usecase *msgusecase.MessageUsecase
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(usecase *msgusecase.MessageUsecase) *MessageHandler {
	return &MessageHandler{usecase: usecase}
}

// Send handles POST /rooms/:roomId/messages.
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

	return c.JSON(http.StatusCreated, MessageResponse{
		ID:        msg.ID,
		RoomID:    msg.RoomID,
		SenderID:  msg.SenderID,
		Content:   msg.Content,
		Type:      string(msg.Type),
		Sequence:  msg.Sequence,
		CreatedAt: msg.CreatedAt,
	})
}

// List handles GET /rooms/:roomId/messages.
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

	page, err := h.usecase.ListMessages(c.Request().Context(), userID, roomID, cursor, limit)
	if err != nil {
		return handleMessageError(c, err)
	}

	messages := make([]MessageResponse, len(page.Messages))
	for i, msg := range page.Messages {
		messages[i] = MessageResponse{
			ID:        msg.ID,
			RoomID:    msg.RoomID,
			SenderID:  msg.SenderID,
			Content:   msg.Content,
			Type:      string(msg.Type),
			Sequence:  msg.Sequence,
			CreatedAt: msg.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, MessageListResponse{
		Messages:   messages,
		NextCursor: page.NextCursor,
	})
}

// SendAI handles POST /rooms/:roomId/messages/ai.
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

	msg, err := h.usecase.SendAIMessage(c.Request().Context(), userID, roomID, req.Content, req.Model)
	if err != nil {
		return handleMessageError(c, err)
	}

	return c.JSON(http.StatusCreated, MessageResponse{
		ID:        msg.ID,
		RoomID:    msg.RoomID,
		SenderID:  msg.SenderID,
		Content:   msg.Content,
		Type:      string(msg.Type),
		Sequence:  msg.Sequence,
		CreatedAt: msg.CreatedAt,
	})
}

func handleMessageError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrForbidden) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
	}
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "not found"})
	}
	if errors.Is(err, domain.ErrLLMGateway) {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Message: "ai service error"})
	}
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}
