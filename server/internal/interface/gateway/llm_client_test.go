package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// TestLLMClientEstimateTokensSuccess exercises EstimateTokens against a
// stub HTTP server standing in for the LLM Gateway: it asserts the request
// is marshalled/sent correctly and the response is decoded into
// *ai.TokenEstimateResponse correctly.
func TestLLMClientEstimateTokensSuccess(t *testing.T) {
	var capturedPath string
	var capturedBody tokenEstimateReqDTO

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokenEstimateRespDTO{
			Model:           "gpt-5.2",
			EstimatedTokens: 6,
		})
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	req := &ai.TokenEstimateRequest{
		Model: "gpt-5.2",
		Messages: []ai.ChatMessage{
			{Role: "user", Content: "hello world"},
		},
	}

	resp, err := client.EstimateTokens(context.Background(), req)
	if err != nil {
		t.Fatalf("EstimateTokens returned error: %v", err)
	}

	if capturedPath != "/tokens/estimate" {
		t.Fatalf("expected request path /tokens/estimate, got %s", capturedPath)
	}
	if capturedBody.Model != "gpt-5.2" {
		t.Fatalf("expected marshalled model gpt-5.2, got %s", capturedBody.Model)
	}
	if len(capturedBody.Messages) != 1 || capturedBody.Messages[0].Content != "hello world" {
		t.Fatalf("expected marshalled messages to include hello world, got %+v", capturedBody.Messages)
	}

	if resp.Model != "gpt-5.2" {
		t.Fatalf("expected decoded model gpt-5.2, got %s", resp.Model)
	}
	if resp.EstimatedTokens != 6 {
		t.Fatalf("expected decoded estimated_tokens 6, got %d", resp.EstimatedTokens)
	}
}

// TestLLMClientEstimateTokensNonOKStatus asserts a non-200 gateway response
// yields an ErrLLMGateway-wrapped error.
func TestLLMClientEstimateTokensNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	req := &ai.TokenEstimateRequest{
		Model: "gpt-5.2",
		Messages: []ai.ChatMessage{
			{Role: "user", Content: "hello"},
		},
	}

	_, err := client.EstimateTokens(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for non-200 gateway response, got nil")
	}
	if !domain.IsLLMGatewayError(err) {
		t.Fatalf("expected ErrLLMGateway-wrapped error, got %v", err)
	}
}

// TestLLMClientListModelsFlattensMetadata exercises ListModels' flattening of
// the gateway's nested, optional wire shape (context_window/supports_image_input
// as JSON `null`-omittable pointers, pricing as a nested object) into Go's flat
// ai.ModelInfo fields, for both a fully-populated model and one with every
// optional field absent (which must flatten to the documented zero values).
func TestLLMClientListModelsFlattensMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"models": [
				{
					"id": "gpt-5.2",
					"name": "GPT-5.2",
					"provider": "openai",
					"context_window": 400000,
					"supports_image_input": true,
					"pricing": {
						"input_price_per_million_tokens": 2.5,
						"output_price_per_million_tokens": 10.0,
						"currency": "USD"
					}
				},
				{
					"id": "claude-opus",
					"name": "Claude Opus",
					"provider": "anthropic"
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	want0 := ai.ModelInfo{
		ID:                          "gpt-5.2",
		Name:                        "GPT-5.2",
		Provider:                    "openai",
		ContextWindow:               400_000,
		InputPricePerMillionTokens:  2.5,
		OutputPricePerMillionTokens: 10.0,
		SupportsImageInput:          true,
	}
	if models[0] != want0 {
		t.Errorf("unexpected model[0]: got %+v, want %+v", models[0], want0)
	}

	want1 := ai.ModelInfo{ID: "claude-opus", Name: "Claude Opus", Provider: "anthropic"}
	if models[1] != want1 {
		t.Errorf("unexpected model[1] (absent optional fields should flatten to zero values): got %+v, want %+v", models[1], want1)
	}
}
