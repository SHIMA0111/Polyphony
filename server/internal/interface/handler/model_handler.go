package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	modelusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/model"
)

// ModelHandler handles HTTP requests for LLM model listing. It delegates
// business logic to ModelUsecase rather than depending on ai.LLMGateway
// directly, per Clean Architecture.
type ModelHandler struct {
	usecase *modelusecase.ModelUsecase
}

// NewModelHandler creates a new ModelHandler with the given ModelUsecase.
func NewModelHandler(usecase *modelusecase.ModelUsecase) *ModelHandler {
	return &ModelHandler{usecase: usecase}
}

// List handles GET /models. It returns the available LLM models from the gateway.
func (h *ModelHandler) List(c echo.Context) error {
	models, err := h.usecase.ListModels(c.Request().Context())
	if err != nil {
		middleware.GetLogger(c).Error("failed to fetch models from llm gateway", "error", err)
		return c.JSON(http.StatusBadGateway, ErrorResponse{Message: "failed to fetch models"})
	}

	resp := make([]ModelResponse, len(models))
	for i, m := range models {
		resp[i] = ModelResponse{
			ID:                          m.ID,
			Name:                        m.Name,
			Provider:                    m.Provider,
			ContextWindow:               m.ContextWindow,
			InputPricePerMillionTokens:  m.InputPricePerMillionTokens,
			OutputPricePerMillionTokens: m.OutputPricePerMillionTokens,
			SupportsImageInput:          m.SupportsImageInput,
		}
	}

	return c.JSON(http.StatusOK, ModelListResponse{Models: resp})
}
