package message

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// --- assembleAIContext / context summarization (Step 50) ---
//
// These tests exercise MessageUsecase.assembleAIContext indirectly through
// SendAIMessage, since assembleAIContext itself is unexported. Every test
// seeds enough messages that summaryRecentTailCount (10) leaves a non-empty
// older-public bucket, and controls overflow deterministically via
// mocks.LLMGateway.TokenEstimateResponse (a fixed EstimatedTokens value
// regardless of the actual request) rather than relying on real token
// counting.

func TestAssembleAIContextUnderBudgetNeverSummarizes(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	gw := &mocks.LLMGateway{
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 100},
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	for i := 1; i <= 11; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if result.UsedContextSummary {
		t.Fatal("expected UsedContextSummary=false for an under-budget context")
	}
	if gw.CompleteCallCount != 1 {
		t.Fatalf("expected exactly 1 Complete call (the answer only), got %d", gw.CompleteCallCount)
	}
	if summaryRepo.UpsertCallCount != 0 {
		t.Fatalf("expected no cache write for an under-budget context, got %d Upsert calls", summaryRepo.UpsertCallCount)
	}
}

func TestAssembleAIContextSummarizesOnOverflowAndCaches(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	var callMessages [][]ai.ChatMessage
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			callMessages = append(callMessages, req.Messages)
			if len(callMessages) == 1 {
				return &ai.CompletionResponse{Content: "This is the summary."}, nil
			}
			return &ai.CompletionResponse{Content: "Final answer."}, nil
		},
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	for i := 1; i <= 11; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("SendAIMessage failed: %v", err)
	}

	if !result.UsedContextSummary {
		t.Fatal("expected UsedContextSummary=true for an overflowing context")
	}
	if gw.CompleteCallCount != 2 {
		t.Fatalf("expected exactly 2 Complete calls (summarize + answer), got %d", gw.CompleteCallCount)
	}

	// Bucketing: 12 messages total (seq 1..12); the 10 newest (seq 3..12)
	// form the verbatim recent tail, leaving seq 1 and 2 as the
	// older-public bucket eligible for summarization.
	expectedOlderPublicChat := []ai.ChatMessage{
		{Role: "user", Content: "msg-1"},
		{Role: "user", Content: "msg-2"},
	}
	expectedPrompt := ai.BuildSummarizationPrompt(expectedOlderPublicChat, true)
	if !reflect.DeepEqual(callMessages[0], expectedPrompt) {
		t.Fatalf("expected first Complete call's messages to equal BuildSummarizationPrompt's output\ngot:  %+v\nwant: %+v", callMessages[0], expectedPrompt)
	}

	cached, err := summaryRepo.Get(ctx, "room-1")
	if err != nil {
		t.Fatalf("expected a cached summary, got error: %v", err)
	}
	if cached.Model != "gpt-5-mini" {
		t.Errorf("cached.Model = %q, want %q", cached.Model, "gpt-5-mini")
	}
	if cached.SummaryText != "This is the summary." {
		t.Errorf("cached.SummaryText = %q, want %q", cached.SummaryText, "This is the summary.")
	}
	if cached.CoveredUpToSequence != 1 {
		t.Errorf("cached.CoveredUpToSequence = %d, want 1 (the sequence of msg-1)", cached.CoveredUpToSequence)
	}
}

func TestAssembleAIContextCacheHitOnMatchingBoundary(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	for i := 1; i <= 11; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	// First overflowing call: seq 1 is (and remains) the oldest surviving
	// message, so this and the next call both resolve to
	// CoveredUpToSequence == 1 -- the second call must hit the cache.
	first, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("first SendAIMessage failed: %v", err)
	}
	if !first.UsedContextSummary {
		t.Fatal("expected first call to use summarization")
	}
	if gw.CompleteCallCount != 2 {
		t.Fatalf("expected 2 Complete calls after the first overflowing send, got %d", gw.CompleteCallCount)
	}
	upsertsAfterFirst := summaryRepo.UpsertCallCount

	second, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-13", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("second SendAIMessage failed: %v", err)
	}
	if !second.UsedContextSummary {
		t.Fatal("expected second call to also report UsedContextSummary=true (served from cache)")
	}
	if gw.CompleteCallCount != 3 {
		t.Fatalf("expected exactly 1 additional Complete call (the answer only, cache hit) after the second send, got total %d", gw.CompleteCallCount)
	}
	if summaryRepo.UpsertCallCount != upsertsAfterFirst {
		t.Fatalf("expected no additional cache write on a cache hit, Upsert count went from %d to %d", upsertsAfterFirst, summaryRepo.UpsertCallCount)
	}
}

func TestAssembleAIContextNeverSummarizesPrivateMessages(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	callIndex := 0
	var allCalls [][]ai.ChatMessage
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			callIndex++
			allCalls = append(allCalls, req.Messages)
			return &ai.CompletionResponse{Content: fmt.Sprintf("response-%d", callIndex)}, nil
		},
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	// seq 1 (human, private) + seq 2 (AI, private): only two messages total
	// so far, well under summaryRecentTailCount -- no overflow logic runs
	// for this call (call 1).
	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "private-question", "gpt-5-mini", true); err != nil {
		t.Fatalf("private SendAIMessage failed: %v", err)
	}

	// seq 3..14: twelve public messages.
	for i := 3; i <= 14; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	// seq 15 (human) + seq 16 (AI): 15 messages fetched (seq 1..15); the 10
	// newest (seq 6..15) form the recent tail, leaving seq 1..5 older:
	// seq 1/2 private (belonging to user-1, the requester) and seq 3/4/5
	// public -- large enough to overflow given TokenEstimateResponse.
	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-15", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("second SendAIMessage failed: %v", err)
	}
	if !result.UsedContextSummary {
		t.Fatal("expected the second call to trigger summarization")
	}
	if len(allCalls) != 3 {
		t.Fatalf("expected 3 total Complete calls (private answer + summarize + answer), got %d", len(allCalls))
	}

	summarizeCall := allCalls[1]
	finalCall := allCalls[2]

	for _, m := range summarizeCall {
		if strings.Contains(m.Content, "private-question") {
			t.Fatalf("private message content leaked into the summarization call: %+v", m)
		}
		if strings.Contains(m.Content, "response-1") {
			t.Fatalf("private AI response content leaked into the summarization call: %+v", m)
		}
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "private-question") {
				t.Fatalf("private message content leaked into a summarization Part: %+v", p)
			}
		}
	}

	foundPrivateQuestion := false
	for _, m := range finalCall {
		if strings.Contains(m.Content, "private-question") {
			foundPrivateQuestion = true
		}
	}
	if !foundPrivateQuestion {
		t.Fatal("expected the private message to still be present verbatim in the final (answer-generating) call")
	}
}

func TestAssembleAIContextInvalidationForcesFreshSummary(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	var firstMsg *messageResult
	for i := 1; i <= 11; i++ {
		sent, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i))
		if err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
		if i == 1 {
			firstMsg = &messageResult{id: sent.ID}
		}
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini", false); err != nil {
		t.Fatalf("first SendAIMessage failed: %v", err)
	}
	if gw.CompleteCallCount != 2 {
		t.Fatalf("expected 2 Complete calls after the first overflowing send, got %d", gw.CompleteCallCount)
	}

	// Toggling exclude_from_ai on the oldest message invalidates the cached
	// summary. The message stays in ListByRoom's results (it is not
	// deleted), so it remains part of the older-public bucket and the
	// boundary sequence this test relies on staying stable is preserved --
	// only the cache entry itself is wiped.
	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", firstMsg.id, true); err != nil {
		t.Fatalf("SetExcludeFromAI failed: %v", err)
	}
	if _, err := summaryRepo.Get(ctx, "room-1"); err == nil {
		t.Fatal("expected the cached summary to be invalidated after SetExcludeFromAI")
	}

	if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-13", "gpt-5-mini", false); err != nil {
		t.Fatalf("second SendAIMessage failed: %v", err)
	}
	if gw.CompleteCallCount != 4 {
		t.Fatalf("expected 2 additional Complete calls (fresh summarize + answer) after invalidation, total got %d", gw.CompleteCallCount)
	}
}

// messageResult is a tiny local struct avoiding an import cycle/extra
// dependency just to carry a message ID between seeding and a later
// assertion in TestAssembleAIContextInvalidationForcesFreshSummary.
type messageResult struct {
	id string
}

func TestAssembleAIContextIncludeImagesResolvedFromModelMetadata(t *testing.T) {
	runCase := func(t *testing.T, models []ai.ModelInfo, model string, expectImagePassthrough bool) {
		msgRepo := &mocks.MessageRepo{}
		roomRepo := &mocks.RoomRepo{}
		roomRepo.SeedMember("room-1", "user-1", "member")
		roomRepo.SeedRoom("room-1", nil)
		summaryRepo := &mocks.ContextSummaryRepo{}
		attachmentRepo := &mocks.AttachmentRepo{}
		objStorage := &mocks.ObjectStorage{}

		var summarizeMessages []ai.ChatMessage
		callIndex := 0
		gw := &mocks.LLMGateway{
			Models:                models,
			TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
			CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
				callIndex++
				if callIndex == 1 {
					summarizeMessages = req.Messages
				}
				return &ai.CompletionResponse{Content: fmt.Sprintf("response-%d", callIndex)}, nil
			},
		}

		uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, attachmentRepo, objStorage, summaryRepo, model)
		ctx := context.Background()

		firstMsg, err := uc.SendMessage(ctx, "user-1", "room-1", "msg-1")
		if err != nil {
			t.Fatalf("seed message 1: %v", err)
		}
		if err := attachmentRepo.Create(ctx, &domainattachment.Attachment{
			ID:       "att-1",
			RoomID:   "room-1",
			S3Key:    "attachments/room-1/att-1",
			MimeType: "image/png",
		}); err != nil {
			t.Fatalf("seed attachment: %v", err)
		}
		if _, err := attachmentRepo.AttachToMessage(ctx, "att-1", firstMsg.ID, "room-1"); err != nil {
			t.Fatalf("AttachToMessage failed: %v", err)
		}
		for i := 2; i <= 11; i++ {
			if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
				t.Fatalf("seed message %d: %v", i, err)
			}
		}

		if _, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-12", model, false); err != nil {
			t.Fatalf("SendAIMessage failed: %v", err)
		}

		hasRawImageURL := false
		hasPlaceholder := false
		for _, m := range summarizeMessages {
			if strings.Contains(m.Content, "[image attachment]") {
				hasPlaceholder = true
			}
			for _, p := range m.Parts {
				if p.Type == ai.ContentPartTypeImageURL {
					hasRawImageURL = true
				}
				if p.Text == "[image attachment]" {
					hasPlaceholder = true
				}
			}
		}

		if expectImagePassthrough {
			if !hasRawImageURL {
				t.Error("expected the real image URL to be passed through to the summarization call")
			}
		} else {
			if hasRawImageURL {
				t.Error("expected no raw image URL anywhere in the summarization call")
			}
			if !hasPlaceholder {
				t.Error("expected the [image attachment] placeholder in the summarization call")
			}
		}
	}

	t.Run("SupportsImageInput true passes the real image through", func(t *testing.T) {
		runCase(t, []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}}, "gpt-5-mini", true)
	})

	t.Run("SupportsImageInput false renders a placeholder", func(t *testing.T) {
		runCase(t, []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: false}}, "gpt-5-mini", false)
	})

	t.Run("model absent from the catalog and the fallback table renders a placeholder", func(t *testing.T) {
		// "totally-unknown-model" is neither in models (nil here) nor in
		// ai's fallbackSupportsImageInput table, so ResolveSupportsImageInput
		// must fall all the way through to its conservative false default.
		runCase(t, nil, "totally-unknown-model", false)
	})
}

func TestAssembleAIContextSummarizationFailureDegradesGracefully(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	callIndex := 0
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			callIndex++
			if callIndex == 1 {
				return nil, fmt.Errorf("summarization backend unavailable")
			}
			return &ai.CompletionResponse{Content: "Final answer despite summarization failure."}, nil
		},
	}

	uc := NewMessageUsecase(msgRepo, roomRepo, gw, event.NewInProcessHub(), &mocks.BillingGuard{}, &mocks.AttachmentRepo{}, &mocks.ObjectStorage{}, summaryRepo, "gpt-5-mini")
	ctx := context.Background()

	for i := 1; i <= 11; i++ {
		if _, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	result, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-12", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("expected SendAIMessage to succeed despite the summarization failure, got error: %v", err)
	}
	if result.UsedContextSummary {
		t.Fatal("expected UsedContextSummary=false when summarization itself failed")
	}
	if result.AIMessage.Content != "Final answer despite summarization failure." {
		t.Fatalf("expected the un-summarized context to still produce an answer, got %q", result.AIMessage.Content)
	}
	if callIndex != 2 {
		t.Fatalf("expected exactly one summarization attempt (no retry) plus the final answer call, got %d total Complete calls", callIndex)
	}
	if summaryRepo.UpsertCallCount != 0 {
		t.Fatalf("expected no cache write after a summarization failure, got %d Upsert calls", summaryRepo.UpsertCallCount)
	}
}
