package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	modelusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/model"
)

func TestModelHandlerList(t *testing.T) {
	gw := &mocks.LLMGateway{
		Models: []ai.ModelInfo{
			{ID: "gpt-5", Name: "GPT-5", Provider: "openai"},
			{ID: "claude-opus", Name: "Claude Opus", Provider: "anthropic"},
		},
	}
	uc := modelusecase.NewModelUsecase(gw)
	h := NewModelHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.List(c); err != nil {
		t.Fatalf("List handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestModelHandlerListGatewayError(t *testing.T) {
	gw := &mocks.LLMGateway{
		ListModelsFunc: func(_ context.Context) ([]ai.ModelInfo, error) {
			return nil, errors.New("gateway unavailable")
		},
	}
	uc := modelusecase.NewModelUsecase(gw)
	h := NewModelHandler(uc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.List(c); err != nil {
		t.Fatalf("List handler error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
