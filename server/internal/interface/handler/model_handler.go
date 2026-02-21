package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// ModelHandler handles HTTP requests for LLM model listing.
type ModelHandler struct {
	llmGateway ai.LLMGateway
}

// NewModelHandler creates a new ModelHandler.
func NewModelHandler(llmGateway ai.LLMGateway) *ModelHandler {
	return &ModelHandler{llmGateway: llmGateway}
}

// List handles GET /models. It returns the available LLM models from the gateway.
func (h *ModelHandler) List(c echo.Context) error {
	models, err := h.llmGateway.ListModels(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Message: "failed to fetch models"})
	}

	resp := make([]ModelResponse, len(models))
	for i, m := range models {
		resp[i] = ModelResponse{ID: m.ID, Name: m.Name, Provider: m.Provider}
	}

	return c.JSON(http.StatusOK, ModelListResponse{Models: resp})
}
