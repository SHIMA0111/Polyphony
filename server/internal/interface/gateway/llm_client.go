// Package gateway implements outbound gateway ports, including the REST client for the LLM Gateway service.
package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// LLMClient implements the ai.LLMGateway interface using REST calls to the LLM Gateway service.
type LLMClient struct {
	baseURL    string
	httpClient *http.Client
	// streamClient is used only by Stream. Unlike httpClient, it is built
	// with no http.Client.Timeout: Timeout bounds the entire
	// request-plus-response-body lifetime, which would tear down a
	// legitimately long-running SSE stream partway through. Cancellation for
	// a streaming call instead relies entirely on the context.Context passed
	// to http.NewRequestWithContext by the caller.
	streamClient *http.Client
}

// NewLLMClient creates a new LLMClient configured to call the LLM Gateway at the given base URL.
// The HTTP client used for unary calls (Complete/ListModels/EstimateTokens) is configured with a
// 60-second timeout; the client used for Stream has no timeout (see LLMClient.streamClient).
func NewLLMClient(baseURL string) *LLMClient {
	return &LLMClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		streamClient: &http.Client{},
	}
}

// --- Private request/response DTOs matching LLM Gateway format ---

type completionReqDTO struct {
	Model       string       `json:"model"`
	Messages    []chatMsgDTO `json:"messages"`
	MaxTokens   *int         `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
}

// chatMsgDTO is the wire shape of a single chat message, matching the LLM
// Gateway's REST contract on both the request and response side.
//
// Content is `any` rather than `string` because the gateway's `content`
// field is a `#[serde(untagged)]` union (see
// llm-gateway/src/adapters/inbound/rest/request.rs's ContentDto): a plain
// JSON string for text-only messages, or an array of contentPartDTO objects
// for multimodal (Vision) messages. On the outbound (request) side, build it
// with toContentDTO rather than assigning a raw string/slice directly, so the
// two shapes stay centralized in one place. On the inbound (response) side,
// json.Unmarshal decodes either a bare JSON string into a Go string, or a
// content-parts array into a []interface{} of map[string]interface{}, held
// by this `any` -- see Complete's use of contentDTOToText when reading it
// back out.
type chatMsgDTO struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// contentPartDTO is the wire shape of a single multimodal content part,
// matching the LLM Gateway's ContentPartDto
// (llm-gateway/src/adapters/inbound/rest/request.rs): internally tagged by
// Type ("text" | "image_url" | "image_base64"), with only the fields
// relevant to that Type populated (the others are omitted via `omitempty`).
type contentPartDTO struct {
	Type      string       `json:"type"`
	Text      string       `json:"text,omitempty"`
	ImageURL  *imageURLDTO `json:"image_url,omitempty"`
	MediaType string       `json:"media_type,omitempty"`
	Data      string       `json:"data,omitempty"`
}

// imageURLDTO is the nested `image_url` object of a contentPartDTO whose
// Type is "image_url", matching the gateway's `{"url": "..."}` shape rather
// than a bare string.
type imageURLDTO struct {
	URL string `json:"url"`
}

// toContentDTO builds the wire-format `content` value for a single domain
// ai.ChatMessage: a non-empty Parts takes precedence over Content and
// becomes a []contentPartDTO (via toContentPartDTO); otherwise Content
// becomes a plain string (the common, backward-compatible text-only case).
func toContentDTO(m ai.ChatMessage) any {
	if len(m.Parts) == 0 {
		return m.Content
	}
	parts := make([]contentPartDTO, len(m.Parts))
	for i, p := range m.Parts {
		parts[i] = toContentPartDTO(p)
	}
	return parts
}

// contentDTOToText extracts the plain-text representation of a decoded
// chatMsgDTO.Content value, mirroring the gRPC transport's pbContentToText
// (see grpc_client.go) so both transports agree on what a multimodal
// response flattens to. json.Unmarshal decodes the gateway's untagged
// `content` union into one of two shapes when read back into this `any`
// field: a bare JSON string decodes as a Go string (the common, text-only
// case); an array of content-part objects decodes as []interface{} of
// map[string]interface{}, each keyed by "type" (mirroring contentPartDTO's
// JSON tags). Only "text" parts contribute their "text" field; "image_url"
// and "image_base64" parts (and any unrecognized type) are ignored. Any
// other shape -- including a value that fails these assertions -- yields the
// empty string rather than panicking.
func contentDTOToText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var sb strings.Builder
		for _, part := range c {
			m, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if text, ok := m["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// toContentPartDTO converts a single domain ai.ContentPart to its wire-format
// contentPartDTO, dispatching on p.Type (one of the ai.ContentPartType*
// constants). An unrecognized Type falls back to a text part carrying
// p.Text (defaulting to the empty string).
func toContentPartDTO(p ai.ContentPart) contentPartDTO {
	switch p.Type {
	case ai.ContentPartTypeImageURL:
		return contentPartDTO{Type: "image_url", ImageURL: &imageURLDTO{URL: p.ImageURL}}
	case ai.ContentPartTypeImageBase64:
		var mediaType, data string
		if p.ImageBase64 != nil {
			mediaType, data = p.ImageBase64.MediaType, p.ImageBase64.Data
		}
		return contentPartDTO{Type: "image_base64", MediaType: mediaType, Data: data}
	default:
		return contentPartDTO{Type: "text", Text: p.Text}
	}
}

type completionRespDTO struct {
	Model   string      `json:"model"`
	Choices []choiceDTO `json:"choices"`
	Usage   usageDTO    `json:"usage"`
}

type choiceDTO struct {
	Message chatMsgDTO `json:"message"`
}

type usageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// streamChunkDTO mirrors the LLM Gateway's `CompletionChunkDto` wire shape
// (Step 43), carried as the JSON payload of each unnamed SSE `data:` frame
// from `POST /completions/stream`. Delta and FinishReason are pointers
// because the gateway omits them from the JSON body when not applicable to
// a given chunk, rather than serializing `null`; Stream flattens a nil
// pointer to Go's zero value ("" for both).
type streamChunkDTO struct {
	ID           string    `json:"id"`
	Model        string    `json:"model"`
	Delta        *string   `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
	Usage        *usageDTO `json:"usage"`
}

// modelPricingDTO mirrors the LLM Gateway's nested `ModelPricingDto`
// (`llm-gateway/src/adapters/inbound/rest/response.rs`): per-1M-token USD
// pricing, present only when `modelDTO.Pricing` is non-nil.
type modelPricingDTO struct {
	InputPricePerMillionTokens  float64 `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens float64 `json:"output_price_per_million_tokens"`
	Currency                    string  `json:"currency"`
}

// modelDTO mirrors the LLM Gateway's `ModelInfoDto` wire shape. ContextWindow,
// SupportsImageInput, and Pricing are pointers because the gateway omits them
// from the JSON body entirely when unknown (`#[serde(skip_serializing_if =
// "Option::is_none")]`), rather than serializing `null`; ListModels flattens
// a nil pointer to Go's zero value ("0/false = unknown").
type modelDTO struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Provider           string           `json:"provider"`
	ContextWindow      *int             `json:"context_window"`
	SupportsImageInput *bool            `json:"supports_image_input"`
	Pricing            *modelPricingDTO `json:"pricing"`
}

type modelsRespDTO struct {
	Models []modelDTO `json:"models"`
}

type tokenEstimateReqDTO struct {
	Model    string       `json:"model"`
	Messages []chatMsgDTO `json:"messages"`
}

type tokenEstimateRespDTO struct {
	Model           string `json:"model"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

// Complete sends a chat completion request to the LLM Gateway and returns the response.
// It returns a domain.ErrLLMGateway-wrapped error on request marshalling, HTTP, or decode failures.
func (c *LLMClient) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	msgs := make([]chatMsgDTO, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMsgDTO{Role: m.Role, Content: toContentDTO(m)}
	}

	body := completionReqDTO{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", domain.ErrLLMGateway, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	var result completionRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", domain.ErrLLMGateway, err)
	}

	// contentDTOToText handles both shapes the gateway's untagged `content`
	// union can decode to: a bare JSON string (today's common case, since no
	// provider adapter yet echoes image content back in a response) and a
	// content-parts array (a future multimodal response), mirroring the gRPC
	// transport's pbContentToText so both transports agree on the result.
	content := ""
	if len(result.Choices) > 0 {
		content = contentDTOToText(result.Choices[0].Message.Content)
	}

	return &ai.CompletionResponse{
		Content:      content,
		Model:        result.Model,
		PromptTokens: result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}, nil
}

// sseDoneSentinel is the literal `data:` payload the LLM Gateway sends on an
// unnamed frame to terminate a successful stream (Step 43's SSE contract).
const sseDoneSentinel = "[DONE]"

// sseErrorEventName is the SSE `event:` value the LLM Gateway sends on a
// mid-stream provider failure; its `data:` payload is a plain-text error
// message (domain_err.to_string() on the gateway side), not JSON.
const sseErrorEventName = "error"

// Stream sends a streaming completion request to the LLM Gateway's
// `POST /completions/stream` endpoint and returns a channel of incremental
// ai.StreamResult items.
//
// It builds the same request body Complete builds and POSTs it with
// `Accept: text/event-stream`, using c.streamClient (which has no
// http.Client.Timeout, unlike c.httpClient -- see NewLLMClient). A non-2xx
// initial response is a synchronous dispatch failure: the response body is
// read and closed, and a domain.ErrLLMGateway-wrapped error is returned
// immediately with a nil channel; in this case no goroutine is started and
// the caller must not read from the (nil) channel.
//
// On a 200 OK response, Stream starts a goroutine that owns resp.Body (closing
// it via defer) and parses the SSE body into ai.StreamResult items, framing on
// blank lines exactly like the SSE spec: an unnamed frame whose data decodes as
// streamChunkDTO becomes a Chunk-carrying result; an unnamed frame whose data is
// the literal sseDoneSentinel ends the stream cleanly with no further item; an
// `event: error` frame becomes a single Err-carrying result wrapping
// domain.ErrLLMGateway around its plain-text data; a transport-level read
// error (anything other than a clean EOF at a frame boundary) becomes a single
// Err-carrying result wrapping domain.ErrLLMGateway; and an EOF that is never
// preceded by the sseDoneSentinel frame (the connection closed early --
// dropped connection, gateway crash mid-response, etc.) also becomes a single
// Err-carrying result wrapping domain.ErrLLMGateway, rather than ending
// silently as if the stream had completed normally. In every case the
// goroutine closes the output channel exactly once, as its last action, so
// callers can safely range over it.
func (c *LLMClient) Stream(ctx context.Context, req *ai.CompletionRequest) (<-chan ai.StreamResult, error) {
	msgs := make([]chatMsgDTO, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMsgDTO{Role: m.Role, Content: toContentDTO(m)}
	}

	body := completionReqDTO{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", domain.ErrLLMGateway, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/completions/stream", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	out := make(chan ai.StreamResult)
	go readSSEStream(resp.Body, out)
	return out, nil
}

// sseFrame holds the accumulated "event:" line of a single SSE frame
// (delimited by a blank line), as read by readSSEStream. Its "data:" lines
// are accumulated separately (see readSSEStream's dataLines), since a frame
// may carry more than one and they must be joined with "\n".
type sseFrame struct {
	event string
}

// readSSEStream owns body (closing it on return) and parses it as an SSE
// stream, sending one ai.StreamResult on out per completed frame per the
// contract documented on Stream, and closing out exactly once as its last
// action.
func readSSEStream(body io.ReadCloser, out chan<- ai.StreamResult) {
	defer close(out)
	defer func() { _ = body.Close() }()

	reader := bufio.NewReader(body)
	var frame sseFrame
	var dataLines []string
	// sawDone tracks whether the unnamed frame carrying sseDoneSentinel has
	// been processed by flush. It is checked only on the EOF path below: a
	// transport-level read error or an `event: error` frame already emits
	// its own Err-carrying result and returns immediately, so there is
	// nothing for sawDone to disambiguate in either of those cases.
	var sawDone bool

	flush := func() bool {
		// Continues the loop when true (nothing to flush), stops it (returns
		// false) when a terminal frame was processed.
		defer func() {
			frame = sseFrame{}
			dataLines = nil
		}()

		if frame.event == "" && len(dataLines) == 0 {
			return true
		}
		data := strings.Join(dataLines, "\n")

		if frame.event == sseErrorEventName {
			out <- ai.StreamResult{Err: fmt.Errorf("%w: stream error: %s", domain.ErrLLMGateway, data)}
			return false
		}

		// Default/unnamed frame.
		if data == sseDoneSentinel {
			sawDone = true
			return false
		}

		var chunk streamChunkDTO
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			out <- ai.StreamResult{Err: fmt.Errorf("%w: decode stream chunk: %v", domain.ErrLLMGateway, err)}
			return false
		}

		delta, finishReason := "", ""
		if chunk.Delta != nil {
			delta = *chunk.Delta
		}
		if chunk.FinishReason != nil {
			finishReason = *chunk.FinishReason
		}
		var usage *ai.Usage
		if chunk.Usage != nil {
			usage = &ai.Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
		out <- ai.StreamResult{Chunk: &ai.StreamChunk{
			ID:           chunk.ID,
			Model:        chunk.Model,
			Delta:        delta,
			FinishReason: finishReason,
			Usage:        usage,
		}}
		return true
	}

	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")

		if trimmed == "" && err == nil {
			// Blank line: frame boundary.
			if !flush() {
				return
			}
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "event:"):
			frame.event = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		case strings.HasPrefix(trimmed, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}

		if err != nil {
			if err == io.EOF {
				// Flush any final frame that wasn't terminated by a trailing
				// blank line before ending cleanly.
				flush()
				if !sawDone {
					// The connection closed without ever delivering the
					// [DONE] sentinel: from here this is indistinguishable
					// from a clean end unless flagged explicitly, so a
					// truncated stream (dropped connection, gateway crash
					// mid-response, etc.) would otherwise complete silently
					// with whatever partial content had streamed so far.
					out <- ai.StreamResult{Err: fmt.Errorf("%w: stream ended before [DONE] sentinel", domain.ErrLLMGateway)}
				}
				return
			}
			out <- ai.StreamResult{Err: fmt.Errorf("%w: stream transport error: %v", domain.ErrLLMGateway, err)}
			return
		}
	}
}

// ListModels retrieves the list of available models from the LLM Gateway.
// It returns a domain.ErrLLMGateway-wrapped error on HTTP or decode failures.
func (c *LLMClient) ListModels(ctx context.Context) ([]ai.ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	var result modelsRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", domain.ErrLLMGateway, err)
	}

	models := make([]ai.ModelInfo, len(result.Models))
	for i, m := range result.Models {
		info := ai.ModelInfo{ID: m.ID, Name: m.Name, Provider: m.Provider}
		if m.ContextWindow != nil {
			info.ContextWindow = *m.ContextWindow
		}
		if m.SupportsImageInput != nil {
			info.SupportsImageInput = *m.SupportsImageInput
		}
		if m.Pricing != nil {
			info.InputPricePerMillionTokens = m.Pricing.InputPricePerMillionTokens
			info.OutputPricePerMillionTokens = m.Pricing.OutputPricePerMillionTokens
		}
		models[i] = info
	}
	return models, nil
}

// EstimateTokens sends a token estimation request to the LLM Gateway and returns the
// approximate token count. It returns a domain.ErrLLMGateway-wrapped error on request
// marshalling, HTTP, or decode failures.
func (c *LLMClient) EstimateTokens(ctx context.Context, req *ai.TokenEstimateRequest) (*ai.TokenEstimateResponse, error) {
	msgs := make([]chatMsgDTO, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMsgDTO{Role: m.Role, Content: toContentDTO(m)}
	}

	body := tokenEstimateReqDTO{
		Model:    req.Model,
		Messages: msgs,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", domain.ErrLLMGateway, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tokens/estimate", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", domain.ErrLLMGateway, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", domain.ErrLLMGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrLLMGateway, resp.StatusCode, string(respBody))
	}

	var result tokenEstimateRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", domain.ErrLLMGateway, err)
	}

	return &ai.TokenEstimateResponse{
		Model:           result.Model,
		EstimatedTokens: result.EstimatedTokens,
	}, nil
}
