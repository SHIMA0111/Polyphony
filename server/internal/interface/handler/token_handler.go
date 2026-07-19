package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// TokenHandler handles HTTP requests for token estimation. Unlike
// ModelHandler (which delegates to a ModelUsecase), it talks to ai.LLMGateway
// directly: token estimation has no room/user-scoped business logic to wrap,
// it is a pure proxy to the LLM Gateway's estimation endpoint.
type TokenHandler struct {
	llmGateway ai.LLMGateway
}

// NewTokenHandler creates a new TokenHandler with the given ai.LLMGateway.
func NewTokenHandler(llmGateway ai.LLMGateway) *TokenHandler {
	return &TokenHandler{llmGateway: llmGateway}
}

// EstimateTokens handles POST /tokens/estimate. It forwards the parsed
// request to the LLM Gateway's token estimation endpoint and returns the
// approximate token count. It returns HTTP 400 if the request body cannot be
// bound or Model is empty, HTTP 502 if the LLM Gateway call fails, and HTTP
// 200 with a TokenEstimateResponse on success.
func (h *TokenHandler) EstimateTokens(c echo.Context) error {
	var req TokenEstimateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	if req.Model == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "model is required"})
	}

	messages := make([]ai.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ai.ChatMessage{Role: m.Role, Content: m.Content}
	}

	resp, err := h.llmGateway.EstimateTokens(c.Request().Context(), &ai.TokenEstimateRequest{
		Model:    req.Model,
		Messages: messages,
	})
	if err != nil {
		middleware.GetLogger(c).Error("failed to estimate tokens via llm gateway", "error", err)
		return c.JSON(http.StatusBadGateway, ErrorResponse{Message: "failed to estimate tokens"})
	}

	return c.JSON(http.StatusOK, TokenEstimateResponse{
		Model:           resp.Model,
		EstimatedTokens: resp.EstimatedTokens,
	})
}
