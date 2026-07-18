package handler

import (
	"context"
	"encoding/json"
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

// TestModelHandlerListSerializesMetadataFields is a Step 34 regression test:
// GET /models must serialize context_window/pricing/supports_image_input as
// a pure passthrough of ai.ModelInfo's equivalent fields, unchanged.
func TestModelHandlerListSerializesMetadataFields(t *testing.T) {
	gw := &mocks.LLMGateway{
		Models: []ai.ModelInfo{
			{
				ID:                          "gpt-5",
				Name:                        "GPT-5",
				Provider:                    "openai",
				ContextWindow:               272_000,
				InputPricePerMillionTokens:  1.25,
				OutputPricePerMillionTokens: 10.0,
				SupportsImageInput:          true,
			},
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

	var body ModelListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if len(body.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(body.Models))
	}

	got := body.Models[0]
	want := ModelResponse{
		ID:                          "gpt-5",
		Name:                        "GPT-5",
		Provider:                    "openai",
		ContextWindow:               272_000,
		InputPricePerMillionTokens:  1.25,
		OutputPricePerMillionTokens: 10.0,
		SupportsImageInput:          true,
	}
	if got != want {
		t.Errorf("unexpected model response: got %+v, want %+v", got, want)
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
