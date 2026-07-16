package message

import (
	"context"
	"log/slog"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// attachmentViewURLExpiry is how long a presigned view URL minted for an
// AI-context image part remains valid. Matches
// usecase/attachment.viewURLExpiry: the URL only needs to survive the single
// LLM Gateway request it is embedded in.
const attachmentViewURLExpiry = time.Hour

// summaryRecentTailCount is the number of newest messages (by Sequence)
// that assembleAIContext always keeps verbatim, never summarized,
// regardless of how large the fetched context window is. Keeping a
// verbatim recent tail means the AI always sees the exact wording of the
// most recent turns, even when older history has been folded into a cached
// summary.
const summaryRecentTailCount = 10

// reservedOutputTokens is subtracted from a model's resolved context window
// (ai.ResolveContextWindow) to compute assembleAIContext's usable input
// token budget, leaving headroom for the model's own reply. It is a fixed
// approximation, not a per-request MaxTokens-aware calculation: Out of
// scope explicitly excludes real tokenizer-accurate limit enforcement for
// this step.
const reservedOutputTokens = 1024

// filterEligibleMessages reproduces ai.ContextBuilder.Build's exclusion filter and
// chronological reordering (via ai.IsEligibleForContext, in the exact same iteration
// order Build uses), returning the parallel []*domainmessage.Message slice that lines
// up 1:1 with ai.ContextBuilder.Build(msgs, cutoff)'s output.
//
// This exists solely to correlate Build's []ai.ChatMessage output back to its source
// messages for enrichWithAttachments: Build's signature is frozen (it returns
// []ai.ChatMessage, not the source messages, so there is no message ID on its output
// to look up attachments by), so this helper rebuilds the same filtered, chronological
// slice independently, using the same exported predicate Build itself calls.
func filterEligibleMessages(msgs []*domainmessage.Message, cutoff *time.Time) []*domainmessage.Message {
	eligible := make([]*domainmessage.Message, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		if ai.IsEligibleForContext(msgs[i], cutoff) {
			eligible = append(eligible, msgs[i])
		}
	}
	return eligible
}

// enrichWithAttachments upgrades chatMsgs entries whose source message carries one or
// more image attachments into multimodal ai.ChatMessage Parts payloads, for both
// SendAIMessage and RegenerateAIMessage's context-assembly path.
//
// chatMsgs must be ai.ContextBuilder.Build(msgs, cutoff)'s output and sourceMsgs must
// be filterEligibleMessages(msgs, cutoff)'s output for that same (msgs, cutoff) pair,
// so the two slices line up 1:1 by index -- see filterEligibleMessages' doc comment
// for why this indirection is needed instead of Build returning message IDs directly.
//
// A message with no image attachments is left untouched (its Parts stays nil/empty,
// so it still serializes via the plain-Content path). A message with one or more
// image attachments has Parts set to: a text part carrying its original Content (only
// if Content is non-empty), followed by one image part per attachment in
// attachmentRepo.ListByMessageID order, each built from a freshly presigned view URL
// (u.objStorage.PresignView -- the same helper usecase/attachment.AttachmentUsecase's
// ListAttachments uses, reused here rather than re-deriving S3 URLs).
//
// It is a no-op (returns chatMsgs unchanged) if this usecase was constructed with a
// nil attachmentRepo or objStorage, so callers/tests that don't care about Vision
// attachments don't need to wire either dependency.
//
// # Errors
// Returns the first error encountered from attachmentRepo.ListByMessageID or
// objStorage.PresignView: a context-assembly call cannot silently omit an attachment
// the sender attached, so any lookup/presign failure aborts the whole enrichment
// rather than falling back to the plain-Content path for that message.
func (u *MessageUsecase) enrichWithAttachments(
	ctx context.Context,
	chatMsgs []ai.ChatMessage,
	sourceMsgs []*domainmessage.Message,
) ([]ai.ChatMessage, error) {
	if u.attachmentRepo == nil || u.objStorage == nil {
		return chatMsgs, nil
	}

	for i := range chatMsgs {
		attachments, err := u.attachmentRepo.ListByMessageID(ctx, sourceMsgs[i].ID)
		if err != nil {
			return nil, err
		}
		if len(attachments) == 0 {
			continue
		}

		parts := make([]ai.ContentPart, 0, len(attachments)+1)
		if chatMsgs[i].Content != "" {
			parts = append(parts, ai.ContentPart{
				Type: ai.ContentPartTypeText,
				Text: chatMsgs[i].Content,
			})
		}
		for _, att := range attachments {
			viewURL, err := u.objStorage.PresignView(ctx, att.S3Key, attachmentViewURLExpiry)
			if err != nil {
				return nil, err
			}
			parts = append(parts, ai.ContentPart{
				Type:     ai.ContentPartTypeImageURL,
				ImageURL: viewURL,
			})
		}
		chatMsgs[i].Parts = parts
	}
	return chatMsgs, nil
}

// assembleAIContext is the single mechanism SendAIMessage/RegenerateAIMessage
// use to turn a raw, sequence-descending batch of fetched room messages
// (msgs, as returned by MessageRepository.ListByRoom/ListByRoomUpTo) into the
// []ai.ChatMessage sent to ai.LLMGateway.Complete, deciding along the way
// whether older history needs to be replaced by a cached/freshly-computed
// AI-generated summary (phases.md Phase 18) because the full context would
// overflow rm's target model's resolved context window.
//
// # Bucketing
//
// msgs is split into three buckets, oldest-message-per-bucket-boundary
// documented inline below:
//   - recentRaw: the first summaryRecentTailCount entries (the newest, by
//     Sequence) -- always kept verbatim, never summarized, so the AI always
//     sees the exact wording of the most recent turns.
//   - olderPrivateRaw: everything older than the recent tail whose
//     Visibility is private. Because msgs was fetched with requestingUserID
//     already applied (MessageRepository's visibility filter), any private
//     message present here already belongs to the caller -- there is no
//     "someone else's private message" to filter out again.
//   - olderPublicRaw: everything older than the recent tail whose
//     Visibility is public -- the only bucket ever eligible for
//     summarization/caching.
//
// olderPrivateRaw is deliberately never summarized or written into the
// shared, room-scoped (not per-user) message_context_summaries cache: since
// the cache has no per-user dimension, caching any private content would
// leak it to every other member's subsequent AI calls in the same room. This
// is why a private message can never end up in a cached summary "by
// construction" (it never enters the summarizable bucket in the first
// place) rather than needing an explicit invalidation trigger when a
// message's visibility or exclusion state changes.
//
// # Budget check and summarization
//
// ai.ContextBuilder.Build is called separately over each raw bucket (it is a
// pure filter/map function and composes fine over sub-slices), each result
// enriched with image attachments exactly as SendAIMessage/RegenerateAIMessage
// did directly before this step (Step 39). If olderPublicRaw is empty, there
// is nothing to summarize and the verbatim buckets are concatenated as-is.
// Otherwise, the estimated token cost of the full verbatim concatenation is
// compared against rm's target model's resolved context window (minus
// reservedOutputTokens); if it fits, the verbatim concatenation is still
// returned unchanged. Only on overflow is the older-public bucket replaced by
// a single system-role summary message -- see the inline comments below for
// the cache-hit/summarize/cache-write sequence.
//
// # Ordering caveat
//
// olderPublicRaw/olderPrivateRaw are split from the same sequence-descending
// remainder, so each preserves its own relative chronological order, but the
// two buckets are interleaved in real time. The final concatenation always
// places olderPrivateChat immediately before recentChat (both "never
// summarized, always verbatim") regardless of their true interleaving with
// the (possibly summarized) older-public bucket; this is an accepted,
// documented approximation of true chronological order, not a bug.
func (u *MessageUsecase) assembleAIContext(
	ctx context.Context,
	rm *room.Room,
	model string,
	msgs []*domainmessage.Message,
) ([]ai.ChatMessage, bool, error) {
	cutoff := rm.AIContextCutoffAt

	tailCount := summaryRecentTailCount
	if tailCount > len(msgs) {
		tailCount = len(msgs)
	}
	recentRaw := msgs[:tailCount]
	remainder := msgs[tailCount:]

	var olderPublicRaw, olderPrivateRaw []*domainmessage.Message
	for _, m := range remainder {
		if m.Visibility == domainmessage.MessageVisibilityPrivate {
			olderPrivateRaw = append(olderPrivateRaw, m)
		} else {
			olderPublicRaw = append(olderPublicRaw, m)
		}
	}

	recentChat, err := u.buildAndEnrichContextBucket(ctx, recentRaw, cutoff)
	if err != nil {
		return nil, false, err
	}
	olderPrivateChat, err := u.buildAndEnrichContextBucket(ctx, olderPrivateRaw, cutoff)
	if err != nil {
		return nil, false, err
	}

	if len(olderPublicRaw) == 0 {
		// Nothing eligible to summarize -- the common case for most
		// rooms/history lengths.
		return append(append([]ai.ChatMessage{}, olderPrivateChat...), recentChat...), false, nil
	}

	olderPublicChat, err := u.buildAndEnrichContextBucket(ctx, olderPublicRaw, cutoff)
	if err != nil {
		return nil, false, err
	}

	verbatim := func() []ai.ChatMessage {
		out := make([]ai.ChatMessage, 0, len(olderPublicChat)+len(olderPrivateChat)+len(recentChat))
		out = append(out, olderPublicChat...)
		out = append(out, olderPrivateChat...)
		out = append(out, recentChat...)
		return out
	}

	// Resolve the target model's context window on a best-effort basis:
	// ListModels failing must not fail the whole AI call, so a nil models
	// slice is passed through to ResolveContextWindow/
	// ResolveSupportsImageInput, which fall back to their hard-coded tables.
	models, listErr := u.llmGateway.ListModels(ctx)
	if listErr != nil {
		models = nil
	}
	budget := ai.ResolveContextWindow(models, model) - reservedOutputTokens

	estimate, estErr := u.llmGateway.EstimateTokens(ctx, &ai.TokenEstimateRequest{
		Model:    model,
		Messages: verbatim(),
	})
	if estErr != nil {
		// Cannot determine whether the context is oversized -- degrade to
		// sending the raw, potentially-oversized context rather than
		// failing the whole AI call (this step never hard-rejects on
		// tokenizer-accuracy grounds; see step50.md's Out of scope note).
		slog.Error("failed to estimate context tokens; skipping summarization check", "error", estErr, "room_id", rm.ID)
		return verbatim(), false, nil
	}
	if estimate.EstimatedTokens <= budget {
		return verbatim(), false, nil
	}

	// Overflow: the oldest message still included in olderPublicRaw (its
	// last element, since olderPublicRaw is itself sequence-descending)
	// marks the boundary a cached summary must match exactly to be reused.
	boundarySeq := olderPublicRaw[len(olderPublicRaw)-1].Sequence

	summaryText, cacheErr := u.summaryOrCompute(ctx, rm.ID, model, boundarySeq, olderPublicChat, models)
	if cacheErr != nil {
		// Best-effort: a summarization or cache-write failure degrades to
		// the un-summarized (potentially oversized) context rather than
		// failing the whole AI call.
		slog.Error("context summarization failed; falling back to unsummarized context", "error", cacheErr, "room_id", rm.ID)
		return verbatim(), false, nil
	}

	result := make([]ai.ChatMessage, 0, 1+len(olderPrivateChat)+len(recentChat))
	result = append(result, ai.ChatMessage{Role: "system", Content: "Summary of earlier conversation:\n" + summaryText})
	result = append(result, olderPrivateChat...)
	result = append(result, recentChat...)
	return result, true, nil
}

// buildAndEnrichContextBucket runs ai.ContextBuilder.Build over a single raw
// message sub-slice (see assembleAIContext's bucketing) and applies the same
// attachment enrichment (Step 39) SendAIMessage/RegenerateAIMessage applied
// directly before this step existed, using filterEligibleMessages to
// reconstruct Build's parallel source-message slice for that same
// (subMsgs, cutoff) pair.
func (u *MessageUsecase) buildAndEnrichContextBucket(
	ctx context.Context,
	subMsgs []*domainmessage.Message,
	cutoff *time.Time,
) ([]ai.ChatMessage, error) {
	chatMsgs := u.contextBuilder.Build(subMsgs, cutoff)
	return u.enrichWithAttachments(ctx, chatMsgs, filterEligibleMessages(subMsgs, cutoff))
}

// summaryOrCompute returns the summary text to prepend to the AI context for
// (roomID, model, boundarySeq): a cached ai.ContextSummary if
// u.summaryRepo.Get returns one whose Model and CoveredUpToSequence match
// exactly, or a freshly computed one otherwise.
//
// On a cache miss, it resolves includeImages from the same models slice
// already fetched by assembleAIContext for the context-window resolution (no
// second ListModels call), builds the summarization prompt over
// olderPublicChat (already attachment-enriched -- so a Vision-capable model
// receives the real image parts when includeImages is true), calls
// ai.LLMGateway.Complete, and -- on success -- caches the result via
// u.summaryRepo.Upsert before returning it. A Complete or EstimateTokens
// failure, or an Upsert failure, is returned to the caller (assembleAIContext
// logs it and degrades to the un-summarized context); a successful Complete
// whose subsequent cache-write fails still returns the freshly computed
// summary text, since the summary itself is still valid for this one call
// even though it won't be reused by a later one.
func (u *MessageUsecase) summaryOrCompute(
	ctx context.Context,
	roomID, model string,
	boundarySeq int64,
	olderPublicChat []ai.ChatMessage,
	models []ai.ModelInfo,
) (string, error) {
	if cached, err := u.summaryRepo.Get(ctx, roomID); err == nil &&
		cached.Model == model && cached.CoveredUpToSequence == boundarySeq {
		return cached.SummaryText, nil
	}

	includeImages := ai.ResolveSupportsImageInput(models, model)
	prompt := ai.BuildSummarizationPrompt(olderPublicChat, includeImages)

	completion, err := u.llmGateway.Complete(ctx, &ai.CompletionRequest{Model: model, Messages: prompt})
	if err != nil {
		return "", err
	}
	summaryText := completion.Content

	tokenCount := 0
	if estimate, err := u.llmGateway.EstimateTokens(ctx, &ai.TokenEstimateRequest{
		Model:    model,
		Messages: []ai.ChatMessage{{Role: "system", Content: summaryText}},
	}); err == nil {
		tokenCount = estimate.EstimatedTokens
	}

	if err := u.summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              roomID,
		Model:               model,
		CoveredUpToSequence: boundarySeq,
		SummaryText:         summaryText,
		TokenCount:          tokenCount,
	}); err != nil {
		slog.Error("failed to cache context summary", "error", err, "room_id", roomID)
	}

	return summaryText, nil
}
