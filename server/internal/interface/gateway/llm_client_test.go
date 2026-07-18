package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestLLMClientCompleteTextOnlyMessageMarshalsPlainStringContent asserts that
// a text-only ai.ChatMessage (no Parts) still marshals `content` as a bare
// JSON string, matching the LLM Gateway's backward-compatible REST contract
// (this is a regression check: a naive migration to a multimodal-capable
// content shape could accidentally start wrapping every message in a
// single-element array).
func TestLLMClientCompleteTextOnlyMessageMarshalsPlainStringContent(t *testing.T) {
	var capturedBody completionReqDTO
	// handlerErr records a failure/mismatch observed inside the httptest
	// handler. The handler runs on its own goroutine, so calling t.Fatalf
	// there would invoke runtime.Goexit mid-handler rather than failing the
	// test from the main goroutine; instead it is recorded here and the main
	// goroutine asserts on it after Complete returns.
	var handlerErr error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErr = fmt.Errorf("failed to read request body: %w", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Decode into a generic map first so the test can assert the raw JSON
		// shape of `content` (string, not array) before also decoding into
		// completionReqDTO for field-level assertions.
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			handlerErr = fmt.Errorf("failed to decode raw request body: %w", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		messages, _ := raw["messages"].([]any)
		if len(messages) != 1 {
			handlerErr = fmt.Errorf("expected 1 message in raw body, got %+v", raw["messages"])
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		msg, _ := messages[0].(map[string]any)
		if _, ok := msg["content"].(string); !ok {
			handlerErr = fmt.Errorf("expected content to be a plain JSON string, got %+v (%T)", msg["content"], msg["content"])
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := json.Unmarshal(body, &capturedBody); err != nil {
			handlerErr = fmt.Errorf("failed to decode request body: %w", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(completionRespDTO{
			Model: "gpt-5.2",
			Choices: []choiceDTO{
				{Message: chatMsgDTO{Role: "assistant", Content: "hi there"}},
			},
			Usage: usageDTO{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		})
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	req := &ai.CompletionRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hello world"}},
	}

	resp, err := client.Complete(context.Background(), req)
	if handlerErr != nil {
		t.Fatalf("handler observed a failure: %v", handlerErr)
	}
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if len(capturedBody.Messages) != 1 || capturedBody.Messages[0].Content != "hello world" {
		t.Fatalf("expected marshalled content to be the plain string hello world, got %+v", capturedBody.Messages)
	}
	if resp.Content != "hi there" {
		t.Fatalf("expected decoded content %q, got %q", "hi there", resp.Content)
	}
}

// TestLLMClientCompleteImagePartsMessageMarshalsContentPartsArray asserts
// that an ai.ChatMessage with Parts marshals `content` as an array of typed
// content-part objects matching the LLM Gateway's ContentPartDto shape
// (llm-gateway/src/adapters/inbound/rest/request.rs): a text part, an
// image_url part, and an image_base64 part.
func TestLLMClientCompleteImagePartsMessageMarshalsContentPartsArray(t *testing.T) {
	var capturedRaw map[string]any
	// handlerErr records a failure observed inside the httptest handler. The
	// handler runs on its own goroutine, so calling t.Fatalf there would
	// invoke runtime.Goexit mid-handler rather than failing the test from the
	// main goroutine; instead it is recorded here and the main goroutine
	// asserts on it after Complete returns.
	var handlerErr error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErr = fmt.Errorf("failed to read request body: %w", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &capturedRaw); err != nil {
			handlerErr = fmt.Errorf("failed to decode raw request body: %w", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(completionRespDTO{
			Model: "gpt-5.2",
			Choices: []choiceDTO{
				{Message: chatMsgDTO{Role: "assistant", Content: "it's a cat"}},
			},
			Usage: usageDTO{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
		})
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	req := &ai.CompletionRequest{
		Model: "gpt-5.2",
		Messages: []ai.ChatMessage{
			{
				Role: "user",
				Parts: []ai.ContentPart{
					{Type: ai.ContentPartTypeText, Text: "what is this?"},
					{Type: ai.ContentPartTypeImageURL, ImageURL: "https://example.com/cat.png"},
					{
						Type: ai.ContentPartTypeImageBase64,
						ImageBase64: &ai.ImageBase64Data{
							MediaType: "image/png",
							Data:      "abcd",
						},
					},
				},
			},
		},
	}

	resp, err := client.Complete(context.Background(), req)
	if handlerErr != nil {
		t.Fatalf("handler observed a failure: %v", handlerErr)
	}
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp.Content != "it's a cat" {
		t.Fatalf("expected decoded content %q, got %q", "it's a cat", resp.Content)
	}

	messages, _ := capturedRaw["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message in raw body, got %+v", capturedRaw["messages"])
	}
	msg, _ := messages[0].(map[string]any)
	parts, ok := msg["content"].([]any)
	if !ok || len(parts) != 3 {
		t.Fatalf("expected content to be a 3-element array, got %+v (%T)", msg["content"], msg["content"])
	}

	textPart, _ := parts[0].(map[string]any)
	if textPart["type"] != "text" || textPart["text"] != "what is this?" {
		t.Fatalf("unexpected text part: %+v", textPart)
	}

	imageURLPart, _ := parts[1].(map[string]any)
	if imageURLPart["type"] != "image_url" {
		t.Fatalf("unexpected image_url part: %+v", imageURLPart)
	}
	nestedImageURL, _ := imageURLPart["image_url"].(map[string]any)
	if nestedImageURL["url"] != "https://example.com/cat.png" {
		t.Fatalf("unexpected nested image_url object: %+v", nestedImageURL)
	}

	imageBase64Part, _ := parts[2].(map[string]any)
	if imageBase64Part["type"] != "image_base64" ||
		imageBase64Part["media_type"] != "image/png" ||
		imageBase64Part["data"] != "abcd" {
		t.Fatalf("unexpected image_base64 part: %+v", imageBase64Part)
	}
}

// TestLLMClientCompleteResponseContentPartsArrayConcatenatesText asserts
// that a completion response whose message content decodes as a
// content-parts array (rather than the common bare-string shape) is
// flattened via contentDTOToText: text parts are concatenated in order and
// image parts contribute nothing, mirroring the gRPC transport's
// pbContentToText instead of silently returning empty (the pre-fix
// behavior, which only asserted content as a string).
func TestLLMClientCompleteResponseContentPartsArrayConcatenatesText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "gpt-5.2",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": [
							{"type": "text", "text": "it's a "},
							{"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}},
							{"type": "text", "text": "cat"}
						]
					}
				}
			],
			"usage": {"prompt_tokens": 10, "completion_tokens": 3, "total_tokens": 13}
		}`))
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	req := &ai.CompletionRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "what is this?"}},
	}

	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if resp.Content != "it's a cat" {
		t.Fatalf("expected decoded content to concatenate text parts and skip image parts, got %q", resp.Content)
	}
}

// drainStream collects every ai.StreamResult from ch until it closes,
// failing the test if that takes longer than 2 seconds (guards against a
// hung goroutine leaving the test to time out at the suite level instead).
func drainStream(t *testing.T, ch <-chan ai.StreamResult) []ai.StreamResult {
	t.Helper()
	var results []ai.StreamResult
	timeout := time.After(2 * time.Second)
	for {
		select {
		case res, ok := <-ch:
			if !ok {
				return results
			}
			results = append(results, res)
		case <-timeout:
			t.Fatal("timed out draining stream channel")
			return results
		}
	}
}

// TestLLMClient_StreamSuccess exercises the happy path: several chunk
// frames with incrementing deltas, a final chunk frame carrying
// finish_reason/usage, then the [DONE] sentinel.
func TestLLMClient_StreamSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completions/stream" {
			t.Errorf("expected path /completions/stream, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		frames := []string{
			`{"id":"c1","model":"gpt-5.2","delta":"Hel"}`,
			`{"id":"c1","model":"gpt-5.2","delta":"lo"}`,
			`{"id":"c1","model":"gpt-5.2","delta":"","finish_reason":"stop","usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		}
		for _, f := range frames {
			_, _ = w.Write([]byte("data: " + f + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	ch, err := client.Stream(context.Background(), &ai.CompletionRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	results := drainStream(t, ch)
	if len(results) != 3 {
		t.Fatalf("expected 3 stream results, got %d: %+v", len(results), results)
	}
	for i, res := range results {
		if res.Err != nil {
			t.Fatalf("result[%d]: unexpected error %v", i, res.Err)
		}
		if res.Chunk == nil {
			t.Fatalf("result[%d]: expected non-nil chunk", i)
		}
	}
	if results[0].Chunk.Delta != "Hel" || results[1].Chunk.Delta != "lo" {
		t.Fatalf("unexpected deltas: %q, %q", results[0].Chunk.Delta, results[1].Chunk.Delta)
	}
	last := results[2].Chunk
	if last.FinishReason != "stop" {
		t.Fatalf("expected finish_reason stop, got %q", last.FinishReason)
	}
	if last.Usage == nil || last.Usage.PromptTokens != 5 || last.Usage.CompletionTokens != 2 || last.Usage.TotalTokens != 7 {
		t.Fatalf("unexpected usage: %+v", last.Usage)
	}
}

// TestLLMClient_StreamMidStreamError asserts a mid-stream `event: error`
// frame (after some chunk frames) yields the preceding chunks followed by
// exactly one Err-carrying StreamResult wrapping domain.ErrLLMGateway, then
// the channel closes.
func TestLLMClient_StreamMidStreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		_, _ = w.Write([]byte(`data: {"id":"c1","model":"gpt-5.2","delta":"partial"}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("event: error\ndata: provider exploded\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	ch, err := client.Stream(context.Background(), &ai.CompletionRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	results := drainStream(t, ch)
	if len(results) != 2 {
		t.Fatalf("expected 2 stream results (1 chunk + 1 error), got %d: %+v", len(results), results)
	}
	if results[0].Err != nil || results[0].Chunk == nil || results[0].Chunk.Delta != "partial" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].Chunk != nil {
		t.Fatalf("expected second result to carry no chunk, got %+v", results[1].Chunk)
	}
	if results[1].Err == nil || !domain.IsLLMGatewayError(results[1].Err) {
		t.Fatalf("expected ErrLLMGateway-wrapped error, got %v", results[1].Err)
	}
}

// TestLLMClient_StreamNonOKStatusSynchronousError asserts a non-2xx status
// with no SSE body at all yields a non-nil synchronous error and a nil
// channel, with no goroutine started (verified implicitly by the test
// completing promptly rather than hanging).
func TestLLMClient_StreamNonOKStatusSynchronousError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unknown model"}`))
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	ch, err := client.Stream(context.Background(), &ai.CompletionRequest{
		Model:    "bogus-model",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected non-nil error for non-2xx response")
	}
	if !domain.IsLLMGatewayError(err) {
		t.Fatalf("expected ErrLLMGateway-wrapped error, got %v", err)
	}
	if ch != nil {
		t.Fatal("expected nil channel on synchronous dispatch error")
	}
}

// TestLLMClient_StreamSplitFrameAcrossWrites asserts a single SSE frame
// split across two chunked HTTP writes (flushed mid-frame) is correctly
// re-buffered into one chunk, with no truncation or duplication.
func TestLLMClient_StreamSplitFrameAcrossWrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// Split a single frame's "data:" line across two writes, flushing in
		// between, then complete the frame with the trailing blank line and
		// the [DONE] sentinel.
		_, _ = w.Write([]byte(`data: {"id":"c1","mod`))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(`el":"gpt-5.2","delta":"split-safe"}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	ch, err := client.Stream(context.Background(), &ai.CompletionRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	results := drainStream(t, ch)
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 stream result, got %d: %+v", len(results), results)
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].Chunk == nil || results[0].Chunk.Delta != "split-safe" {
		t.Fatalf("expected delta %q, got %+v", "split-safe", results[0].Chunk)
	}
}

// TestLLMClient_StreamEOFBeforeDoneSentinel asserts that a connection which
// closes after delivering one legitimate chunk frame but before the
// [DONE] sentinel yields that chunk followed by an Err-carrying StreamResult
// wrapping domain.ErrLLMGateway -- regression test for a truncated stream
// (dropped connection, gateway crash mid-response) previously being
// indistinguishable from a clean end and completing silently with only its
// partial content.
func TestLLMClient_StreamEOFBeforeDoneSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		_, _ = w.Write([]byte(`data: {"id":"c1","model":"gpt-5.2","delta":"partial"}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		// No [DONE] sentinel: the handler returns here, closing the
		// connection as if the gateway crashed or the connection dropped
		// mid-stream.
	}))
	defer server.Close()

	client := NewLLMClient(server.URL)
	ch, err := client.Stream(context.Background(), &ai.CompletionRequest{
		Model:    "gpt-5.2",
		Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	results := drainStream(t, ch)
	if len(results) != 2 {
		t.Fatalf("expected 2 stream results (1 chunk + 1 truncation error), got %d: %+v", len(results), results)
	}
	if results[0].Err != nil || results[0].Chunk == nil || results[0].Chunk.Delta != "partial" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].Chunk != nil {
		t.Fatalf("expected second result to carry no chunk, got %+v", results[1].Chunk)
	}
	if results[1].Err == nil || !domain.IsLLMGatewayError(results[1].Err) {
		t.Fatalf("expected ErrLLMGateway-wrapped error, got %v", results[1].Err)
	}
}
