package message

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
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

// TestAssembleAIContextUnderBudgetNeverSummarizes proves an under-budget
// context skips summarization entirely: exactly one Complete call (the
// answer) and no cache write.
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

// TestAssembleAIContextSummarizesOnOverflowAndCaches proves an overflowing
// context triggers summarization (a summarize call followed by the answer
// call), sends the correct older-public bucket as the summarization
// prompt, and caches the result keyed on the newest older-public sequence.
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
	if cached.CoveredUpToSequence != 2 {
		t.Errorf("cached.CoveredUpToSequence = %d, want 2 (the sequence of msg-2, the newest older-public message)", cached.CoveredUpToSequence)
	}
}

// TestAssembleAIContextExcludedMessageInsideTailWindowStillFillsEligibleTail
// is the regression test for the eligibility-before-slicing fix in
// assembleAIContext: the verbatim recent tail must contain the
// summaryRecentTailCount most-recent ELIGIBLE messages, not simply the
// summaryRecentTailCount most-recent raw rows by position.
//
// 12 human messages (seq 1..12) are seeded, then msg-3 -- which falls inside
// the raw positional tail window (the 10 newest raw rows span seq 3..12) --
// is excluded via SetExcludeFromAI. Slicing the raw batch positionally
// before filtering (the bug) would still take the 10 newest raw rows
// (seq 3..12, one of which -- msg-3 -- gets silently dropped by
// ai.ContextBuilder.Build's own filter) as the tail, leaving the raw
// remainder (seq 1..2) as the older-public bucket and boundarySeq=2:
// msg-2, despite being newer than every remaining tail candidate other than
// the excluded msg-3, would wrongly end up summarized instead of verbatim.
// Filtering for eligibility first (the fix) instead pulls msg-2 into the
// tail to keep it at 10 eligible entries, leaving only msg-1 in the
// older-public bucket and boundarySeq=1.
func TestAssembleAIContextExcludedMessageInsideTailWindowStillFillsEligibleTail(t *testing.T) {
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

	var msg3ID string
	for i := 1; i <= 11; i++ {
		seeded, err := uc.SendMessage(ctx, "user-1", "room-1", fmt.Sprintf("msg-%d", i))
		if err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
		if i == 3 {
			msg3ID = seeded.ID
		}
	}
	if _, err := uc.SetExcludeFromAI(ctx, "user-1", "room-1", msg3ID, true); err != nil {
		t.Fatalf("exclude msg-3 from AI context: %v", err)
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

	// Only msg-1 should have been folded into the summary: msg-3 is
	// ineligible (excluded) and does not count against the tail, and msg-2
	// is pulled into the verbatim tail to keep it at 10 eligible entries.
	expectedOlderPublicChat := []ai.ChatMessage{
		{Role: "user", Content: "msg-1"},
	}
	expectedPrompt := ai.BuildSummarizationPrompt(expectedOlderPublicChat, true)
	if !reflect.DeepEqual(callMessages[0], expectedPrompt) {
		t.Fatalf("expected first Complete call's messages to equal BuildSummarizationPrompt's output\ngot:  %+v\nwant: %+v", callMessages[0], expectedPrompt)
	}

	cached, err := summaryRepo.Get(ctx, "room-1")
	if err != nil {
		t.Fatalf("expected a cached summary, got error: %v", err)
	}
	if cached.CoveredUpToSequence != 1 {
		t.Errorf("cached.CoveredUpToSequence = %d, want 1 (only msg-1 remains in the older-public bucket once msg-2 is pulled into the eligible tail)", cached.CoveredUpToSequence)
	}

	// The final answer-generation call's verbatim messages must contain
	// msg-2 (pulled into the tail) and must NOT contain msg-3 (excluded, so
	// never eligible for context regardless of bucket).
	answerMsgs := callMessages[1]
	var sawMsg2, sawMsg3 bool
	for _, m := range answerMsgs {
		switch m.Content {
		case "msg-2":
			sawMsg2 = true
		case "msg-3":
			sawMsg3 = true
		}
	}
	if !sawMsg2 {
		t.Error("expected msg-2 to appear verbatim in the answer-generation context (pulled into the eligible tail)")
	}
	if sawMsg3 {
		t.Error("expected msg-3 to never appear in the answer-generation context (excluded from AI)")
	}
}

// TestAssembleAIContextCacheHitOnMatchingBoundary proves two things about
// the newest-sequence boundary (see context.go's boundarySeq comment and
// ContextSummary.CoveredUpToSequence's doc comment):
//
//  1. A call that re-derives the exact same older-public bucket -- because
//     it fetches the exact same underlying messages, not because no new
//     messages exist in the room -- genuinely hits the cache.
//     RegenerateAIMessage on the same human message is the natural way to
//     trigger this: it fetches context via ListByRoomUpTo(targetMsg.Sequence),
//     bounded to the same messages the original SendAIMessage call saw,
//     so it resolves to the identical boundary sequence.
//  2. A subsequent call that legitimately sees new older-public history
//     (a message aging out of the recent tail because new messages were
//     sent) computes a different, larger boundary sequence and correctly
//     forces a fresh summarization. An oldest-sequence boundary would
//     instead keep hitting the stale cache here and silently drop the
//     newly bucketed message from the AI's context -- the oldest surviving
//     message in a room's older-public bucket never changes once
//     summarized, so keying the cache on it would never invalidate as new
//     messages age into that bucket.
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

	// First overflowing call: seq 1..11 seeded, msg-12 reserves seq 12
	// (human) and seq 13 (AI). The 10 newest (seq 3..12) form the recent
	// tail, leaving seq 1/2 as the older-public bucket -- boundary sequence
	// 2 (seq 2, the newest of the two).
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

	// Regenerating the same human message's AI response fetches context via
	// ListByRoomUpTo(targetMsg.Sequence), bounded to the exact same seq
	// 1..12 messages the first call saw -- the boundary is genuinely
	// unchanged (still 2), so this must hit the cache: exactly one more
	// Complete call (the regenerated answer, no re-summarization) and no
	// additional cache write.
	regenerated, usedSummary, err := uc.RegenerateAIMessage(ctx, "user-1", "room-1", first.HumanMessage.ID, "gpt-5-mini")
	if err != nil {
		t.Fatalf("RegenerateAIMessage failed: %v", err)
	}
	if regenerated == nil {
		t.Fatal("expected a non-nil regenerated AI message")
	}
	if !usedSummary {
		t.Fatal("expected the regenerated call to also report usedSummary=true (served from cache)")
	}
	if gw.CompleteCallCount != 3 {
		t.Fatalf("expected exactly 1 additional Complete call (the answer only, cache hit) after regenerating, got total %d", gw.CompleteCallCount)
	}
	if summaryRepo.UpsertCallCount != upsertsAfterFirst {
		t.Fatalf("expected no additional cache write on a genuine cache hit, Upsert count went from %d to %d", upsertsAfterFirst, summaryRepo.UpsertCallCount)
	}

	// Sending a new message (msg-14) grows the room to seq 1..14: the 10
	// newest (seq 5..14) now form the recent tail, so seq 1/2/3/4 are the
	// older-public bucket -- boundary sequence 4, genuinely different from
	// the cached 2. This must miss the cache and force a fresh
	// summarization, proving the newly bucketed seq-3/seq-4 messages are
	// not silently dropped from the AI's context.
	third, err := uc.SendAIMessage(ctx, "user-1", "room-1", "msg-14", "gpt-5-mini", false)
	if err != nil {
		t.Fatalf("third SendAIMessage failed: %v", err)
	}
	if !third.UsedContextSummary {
		t.Fatal("expected the third call to also use summarization")
	}
	if gw.CompleteCallCount != 5 {
		t.Fatalf("expected 2 additional Complete calls (fresh summarize + answer) after the boundary changed, got total %d", gw.CompleteCallCount)
	}
	if summaryRepo.UpsertCallCount != upsertsAfterFirst+1 {
		t.Fatalf("expected exactly 1 additional cache write once the boundary changed, Upsert count went from %d to %d", upsertsAfterFirst, summaryRepo.UpsertCallCount)
	}

	cached, err := summaryRepo.Get(ctx, "room-1")
	if err != nil {
		t.Fatalf("expected a cached summary after the boundary-changing call, got error: %v", err)
	}
	if cached.CoveredUpToSequence != 4 {
		t.Fatalf("cached.CoveredUpToSequence = %d, want 4 (the sequence of msg-4, the newest older-public message after the third call)", cached.CoveredUpToSequence)
	}
}

// TestAssembleAIContextNeverSummarizesPrivateMessages proves a private
// message and its AI reply are kept verbatim in the recent/older-private
// buckets and never appear in the summarization call's input, even once
// the room's history overflows and forces summarization of the public
// bucket.
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

// TestAssembleAIContextInvalidationForcesFreshSummary proves SetExcludeFromAI
// invalidates a room's cached summary (via the shared
// mocks.MessageRepo.SummaryRepo wiring) so the next overflowing
// SendAIMessage call re-summarizes rather than reusing the now-stale cache.
func TestAssembleAIContextInvalidationForcesFreshSummary(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}
	// SetExcludeFromAI below now invalidates the cache via
	// msgRepo.UpdateExcludeFromAIAndInvalidateSummary rather than a
	// separate summaryRepo call, so msgRepo needs to be wired to the same
	// summaryRepo instance for this test's cache-invalidation assertion to
	// observe the effect (see mocks.MessageRepo.SummaryRepo's doc comment).
	msgRepo.SummaryRepo = summaryRepo

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

// TestAssembleAIContextSkipsStaleUpsertOnConcurrentInvalidation proves the
// revision-fencing mechanism (see ai.ContextSummaryRepository's "Revision
// fencing" doc comment): a DeleteByRoom landing while summarization is in
// flight -- simulated here via CompleteFunc, which runs after
// summaryOrCompute has already captured the pre-delete revision via
// GetRevision but before it calls Upsert -- makes the subsequent Upsert a
// no-op instead of resurrecting a summary that predates the invalidation.
func TestAssembleAIContextSkipsStaleUpsertOnConcurrentInvalidation(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	callIndex := 0
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompleteFunc: func(_ context.Context, _ *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			callIndex++
			if callIndex == 1 {
				// Simulate a concurrent DeleteMessage/SetExcludeFromAI
				// landing on this room while this (the summarization)
				// Complete call is still in flight.
				if err := summaryRepo.DeleteByRoom(context.Background(), "room-1"); err != nil {
					t.Fatalf("simulated concurrent DeleteByRoom failed: %v", err)
				}
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
		t.Fatal("expected UsedContextSummary=true for this call even though the cache write itself was skipped")
	}
	if result.AIMessage.Content != "Final answer." {
		t.Fatalf("expected the answer call to still succeed despite the no-op'd cache write, got %q", result.AIMessage.Content)
	}

	if summaryRepo.UpsertNoopCount != 1 {
		t.Fatalf("expected exactly 1 no-op'd Upsert (stale revision), got %d", summaryRepo.UpsertNoopCount)
	}
	if _, err := summaryRepo.Get(ctx, "room-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected no cached summary after the stale Upsert no-op'd (the concurrent DeleteByRoom's delete must not have been undone), got err=%v", err)
	}
}

// TestAssembleAIContextIncludeImagesResolvedFromModelMetadata proves the
// summarization call's includeImages decision is resolved from the target
// model's SupportsImageInput metadata (via ai.ResolveSupportsImageInput):
// a Vision-capable model receives the real image URL, while a non-Vision
// (or catalog-absent, fallback-table) model instead sees the
// "[image attachment]" text placeholder.
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

// TestAssembleAIContextSummarizationFailureDegradesGracefully proves that
// when the summarization Complete call fails, assembleAIContext's fallback
// to verbatim() actually sends the older-public bucket (msg-1/msg-2, which
// would otherwise have been replaced by a summary) and the recent tail
// (msg-3..msg-12) to the final answer-generating Complete call unchanged --
// not a truncated or partially-summarized context, and critically, without
// injecting a system-role "Summary of earlier conversation:" message, which
// would otherwise silently fabricate summary content that was never
// actually produced.
func TestAssembleAIContextSummarizationFailureDegradesGracefully(t *testing.T) {
	msgRepo := &mocks.MessageRepo{}
	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedRoom("room-1", nil)
	summaryRepo := &mocks.ContextSummaryRepo{}

	callIndex := 0
	var secondCallMessages []ai.ChatMessage
	gw := &mocks.LLMGateway{
		Models:                []ai.ModelInfo{{ID: "gpt-5-mini", ContextWindow: 8000, SupportsImageInput: true}},
		TokenEstimateResponse: &ai.TokenEstimateResponse{EstimatedTokens: 999_999},
		CompleteFunc: func(_ context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
			callIndex++
			if callIndex == 1 {
				return nil, fmt.Errorf("summarization backend unavailable")
			}
			secondCallMessages = req.Messages
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

	// The final answer-generating call must have received the full,
	// un-summarized history: msg-1..msg-11 (the seeded messages -- including
	// msg-1/msg-2, the would-be-summarized older-public bucket) followed by
	// msg-12 (this call's own human message, always part of the verbatim
	// recent tail), in chronological order, none of them dropped or altered.
	for i := 1; i <= 11; i++ {
		want := fmt.Sprintf("msg-%d", i)
		found := false
		for _, m := range secondCallMessages {
			if m.Content == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected the final Complete call's messages to contain seeded message %q verbatim, got %+v", want, secondCallMessages)
		}
	}
	// No summary was ever produced (the one summarization attempt failed),
	// so no system-role message -- which is exclusively how a cached/fresh
	// summary is injected (see assembleAIContext's "Summary of earlier
	// conversation:" system message) -- must appear anywhere in the
	// fallback context.
	for _, m := range secondCallMessages {
		if m.Role == "system" {
			t.Errorf("expected no system-role (summary) message in the degraded-fallback context, got %+v", m)
		}
	}
}

// TestAssembleAIContextEmptySummaryContentDegradesGracefully proves that a
// summarization Complete call which succeeds but returns only whitespace
// content is treated the same as an outright Complete failure: it degrades
// to the un-summarized context (never a system message carrying an empty
// "Summary of earlier conversation:\n" body) and, critically, is never
// cached -- an empty cached summary would otherwise keep silently discarding
// the older-public bucket for every subsequent call that hits it.
func TestAssembleAIContextEmptySummaryContentDegradesGracefully(t *testing.T) {
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
				// A "successful" completion carrying only whitespace --
				// not a Complete error, but not a usable summary either.
				return &ai.CompletionResponse{Content: "   \n\t  "}, nil
			}
			return &ai.CompletionResponse{Content: "Final answer despite empty summary."}, nil
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
		t.Fatalf("expected SendAIMessage to succeed despite the empty summary content, got error: %v", err)
	}
	if result.UsedContextSummary {
		t.Fatal("expected UsedContextSummary=false when the summarization completion returned empty content")
	}
	if result.AIMessage.Content != "Final answer despite empty summary." {
		t.Fatalf("expected the un-summarized context to still produce an answer, got %q", result.AIMessage.Content)
	}
	if callIndex != 2 {
		t.Fatalf("expected exactly one summarization attempt (no retry) plus the final answer call, got %d total Complete calls", callIndex)
	}
	if summaryRepo.UpsertCallCount != 0 {
		t.Fatalf("expected no cache write for an empty summary, got %d Upsert calls", summaryRepo.UpsertCallCount)
	}
	if _, err := summaryRepo.Get(ctx, "room-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected no cached summary to exist for room-1, got err=%v", err)
	}
}
