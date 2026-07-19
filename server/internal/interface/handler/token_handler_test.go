package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func TestTokenHandlerEstimateTokens200(t *testing.T) {
	gw := &mocks.LLMGateway{
		TokenEstimateResponse: &ai.TokenEstimateResponse{
			Model:           "gpt-5.2",
			EstimatedTokens: 6,
		},
	}
	h := NewTokenHandler(gw)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/tokens/estimate",
		strings.NewReader(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello world"}]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.EstimateTokens(c); err != nil {
		t.Fatalf("EstimateTokens error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"estimated_tokens":6`) {
		t.Fatalf("expected estimated_tokens 6 in body, got %s", rec.Body.String())
	}
}

func TestTokenHandlerEstimateTokensMissingModel400(t *testing.T) {
	gw := &mocks.LLMGateway{}
	h := NewTokenHandler(gw)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/tokens/estimate",
		strings.NewReader(`{"messages":[{"role":"user","content":"hello world"}]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.EstimateTokens(c); err != nil {
		t.Fatalf("EstimateTokens error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTokenHandlerEstimateTokensGatewayError502(t *testing.T) {
	gw := &mocks.LLMGateway{ShouldErr: true}
	h := NewTokenHandler(gw)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/tokens/estimate",
		strings.NewReader(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello world"}]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.EstimateTokens(c); err != nil {
		t.Fatalf("EstimateTokens error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
