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
