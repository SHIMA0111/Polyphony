package message

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// streamBackgroundTimeout bounds the background goroutine SendAIMessageStream
// launches to consume the LLM Gateway's stream and forward chunks over the
// hub. It deliberately does not derive from the originating HTTP request's
// context.Context: that context is cancelled by Echo the instant
// SendAIMessageStream returns and the 202 response is flushed, and using it
// for the background work would cut off in-flight generation (and therefore
// delivery to every *other* room member still watching over WebSocket) the
// moment the request that kicked it off completes.
const streamBackgroundTimeout = 5 * time.Minute

// SendAIMessageStream sends a human message and streams the AI response
// token-by-token instead of waiting for the full completion.
//
// It applies the same authorization (domainroom.ActionInvokeAI) and Step
// 42 pre-call balance guard (BillingGuard.CheckBalance) as SendAIMessage,
// then persists the human message and an AI placeholder message
// (Status = MessageStatusStreaming, Content = "") synchronously, publishing
// EventMessageCreated for both exactly as SendAIMessage does. It returns
// immediately after that -- the result's AIMessage.Status is "streaming" on
// the happy path, so callers must not assume completion the way they can
// with SendAIMessage's return value.
//
// If the LLM Gateway's Stream call fails synchronously with
// domain.ErrStreamingUnsupported (the selected ai.LLMGateway implementation
// doesn't support streaming at all -- currently only
// interface/gateway.GRPCClient, when config.Config.LLMGatewayTransport is
// "grpc"), this transparently falls back to the unary Complete call instead
// of failing the send: see completeAIMessageFallback's doc comment. Any
// other synchronous Stream failure (bad model, connection refused, etc.)
// immediately updates the placeholder to Status = MessageStatusFailed and
// publishes EventMessageUpdated; this is not a Go error return, mirroring
// SendAIMessage's existing "both messages always returned, check
// AIMessage.Status" contract.
//
// Otherwise, a background goroutine (see consumeAIStream) ranges over the
// gateway's stream: it publishes an EventTokenChunk RoomEvent for every
// non-empty delta, then -- once the stream ends or fails -- persists the
// accumulated content via the existing MessageRepository.UpdateAIResponse
// (Status = MessageStatusCompleted on success, MessageStatusFailed on a
// mid-stream error) and publishes EventMessageUpdated with the final state.
// A message left Status = MessageStatusFailed by this method (whether from a
// synchronous dispatch failure or a mid-stream one) can be retried via
// RegenerateAIMessage exactly like a non-streaming failure.
//
// A successful stream's final chunk's Usage (if any) is billed through
// BillingGuard.RecordUsage exactly like SendAIMessage's post-completion
// recording -- fire-and-forget, logged on failure, never affecting the
// persisted message. A stream that never delivers a Usage-bearing chunk
// records no usage at all (see this step's documented billing gap).
//
// Private AI mode (SendAIMessage's `private bool` parameter) is not
// supported by this method; callers that need a private streaming send must
// wait for a later step. Both the human message and the AI placeholder are
// always persisted with MessageVisibilityPublic.
//
// It returns domain.ErrArchivedRoom if the room is archived (see
// domainroom.Room.IsArchived / SendMessage's matching guard) — checked right
// after loading rm, before any sequence number is reserved, so an
// in-progress or failed room-fork copy job can never race new streamed
// posts landing in the archived destination room.
func (u *MessageUsecase) SendAIMessageStream(ctx context.Context, userID, roomID, content, model string) (*SendAIResult, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if err := room.Authorize(member.Role, room.ActionInvokeAI); err != nil {
		return nil, err
	}
	if err := u.billing.CheckBalance(ctx, roomID); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if rm.IsArchived {
		return nil, domain.ErrArchivedRoom
	}
	model = resolveModel(model, rm, u.defaultAIModel)

	// Reserve both sequence numbers atomically as one range before creating
	// either row, exactly like SendAIMessage.
	firstSeq, err := u.msgRepo.ReserveSequenceRange(ctx, roomID, 2)
	if err != nil {
		return nil, err
	}
	humanSeq, aiSeq := firstSeq, firstSeq+1

	humanMsg, err := u.createHumanMessage(ctx, userID, roomID, content, humanSeq, domainmessage.MessageVisibilityPublic)
	if err != nil {
		return nil, err
	}

	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:                    uuid.New().String(),
		RoomID:                roomID,
		SenderID:              nil,
		Content:               "",
		Type:                  domainmessage.MessageTypeAI,
		Status:                domainmessage.MessageStatusStreaming,
		Sequence:              aiSeq,
		Visibility:            domainmessage.MessageVisibilityPublic,
		InResponseToMessageID: &humanMsg.ID,
		CreatedAt:             aiNow,
		UpdatedAt:             aiNow,
	}
	if err := u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}
	u.hub.Publish(ctx, event.RoomEvent{
		Type:          event.EventMessageCreated,
		RoomID:        roomID,
		Message:       aiMsg,
		TargetUserIDs: targetUserIDsForVisibility(aiMsg),
		OccurredAt:    aiNow,
	})

	// Fetch context messages, same as SendAIMessage.
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages, userID)
	if err != nil {
		return nil, err
	}

	// Build chat messages (reverse to chronological order, filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages),
	// summarizing older history behind a cache when the estimated context
	// would overflow the model's resolved context window (Step 50), exactly
	// mirroring SendAIMessage's assembleAIContext call.
	chatMsgs, summaryUsed, err := u.assembleAIContext(ctx, rm, model, contextPage.Messages)
	if err != nil {
		return nil, err
	}

	// streamCtx is intentionally derived from context.Background(), not ctx
	// -- see streamBackgroundTimeout's doc comment for why.
	streamCtx, cancel := context.WithTimeout(context.Background(), streamBackgroundTimeout)

	resultCh, err := u.llmGateway.Stream(streamCtx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})
	if err != nil {
		if errors.Is(err, domain.ErrStreamingUnsupported) {
			// The selected ai.LLMGateway transport doesn't support Stream at
			// all (currently: gateway.GRPCClient over gRPC) -- fall back to
			// the unary Complete call in the background instead of failing
			// the send. The placeholder stays Status = MessageStatusStreaming
			// exactly like the normal happy path, so the caller sees no
			// difference from a real stream dispatch succeeding; see
			// completeAIMessageFallback's doc comment for the rest of the
			// contract (single terminating EventMessageUpdated, no
			// EventTokenChunk).
			aiMsgForCaller := *aiMsg
			go u.completeAIMessageFallback(streamCtx, cancel, aiMsgForCaller, roomID, model, chatMsgs, summaryUsed)
			return &SendAIResult{HumanMessage: humanMsg, AIMessage: &aiMsgForCaller}, nil
		}

		cancel()

		failedNow := time.Now()
		if updateErr := u.msgRepo.UpdateAIResponse(ctx, aiMsg.ID, "", domainmessage.MessageStatusFailed, failedNow); updateErr != nil {
			return nil, updateErr
		}
		aiMsg.Status = domainmessage.MessageStatusFailed
		aiMsg.UpdatedAt = failedNow
		u.hub.Publish(ctx, event.RoomEvent{
			Type:               event.EventMessageUpdated,
			RoomID:             roomID,
			Message:            aiMsg,
			TargetUserIDs:      targetUserIDsForVisibility(aiMsg),
			OccurredAt:         failedNow,
			UsedContextSummary: summaryUsed,
		})
		return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg}, nil
	}

	// aiMsgForCaller is an independent copy of aiMsg, taken here (on this
	// goroutine, before the background goroutine starts) rather than
	// returning the aiMsg pointer itself: aiMsg is the same object
	// u.msgRepo holds internally keyed by ID (Create does not copy it), so
	// consumeAIStream's later MessageRepository.UpdateAIResponse call for
	// this same ID can end up mutating that exact object concurrently with
	// whatever the HTTP handler does with the returned AIMessage (e.g.
	// serializing it to JSON). Go's "go f(x)" statement evaluates x (here,
	// the struct copy passed to consumeAIStream) in this goroutine before
	// the new goroutine starts, so both copies below are safely independent
	// of each other and of aiMsg.
	aiMsgForCaller := *aiMsg
	go u.consumeAIStream(streamCtx, cancel, aiMsgForCaller, roomID, model, resultCh, summaryUsed)

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: &aiMsgForCaller}, nil
}

// consumeAIStream is SendAIMessageStream's background completion path. It
// ranges over resultCh (as returned by ai.LLMGateway.Stream) until the
// channel closes, publishing an EventTokenChunk RoomEvent for every chunk
// with a non-empty Delta (accumulating the full response in a
// strings.Builder) and capturing the final chunk's Usage, if any.
//
// Once resultCh closes, it persists the accumulated content via
// MessageRepository.UpdateAIResponse -- Status = MessageStatusCompleted if
// the stream ended without an error item, or MessageStatusFailed if it ended
// with one (persisting whatever partial content was assembled, matching
// RegenerateAIMessage's retry-friendly placeholder pattern) -- and publishes
// EventMessageUpdated with the resulting state. On successful completion
// with a captured Usage, it records usage via BillingGuard.RecordUsage
// fire-and-forget (logged on failure); a stream that never delivered a
// Usage-bearing chunk skips recording and logs at Warn level instead, since
// BillingGuard.RecordUsage would no-op on a zero total anyway and skipping
// avoids a pointless lookup. It always calls cancel() before returning, to
// release streamCtx's resources.
//
// aiMsg is a value copy of the AI placeholder message (see
// SendAIMessageStream's aiMsgForCaller), so mutating its fields here is safe:
// it shares no memory with the aiMsg pointer SendAIMessageStream already
// returned to its caller.
func (u *MessageUsecase) consumeAIStream(
	ctx context.Context,
	cancel context.CancelFunc,
	aiMsg domainmessage.Message,
	roomID, model string,
	resultCh <-chan ai.StreamResult,
	summaryUsed bool,
) {
	defer cancel()

	var content strings.Builder
	var usage *ai.Usage
	var streamErr error

	for res := range resultCh {
		if res.Err != nil {
			streamErr = res.Err
			break
		}
		if res.Chunk == nil {
			continue
		}
		if res.Chunk.Delta != "" {
			content.WriteString(res.Chunk.Delta)
			u.hub.Publish(ctx, event.RoomEvent{
				Type:   event.EventTokenChunk,
				RoomID: roomID,
				Chunk: &event.StreamChunkEvent{
					MessageID:   aiMsg.ID,
					Delta:       res.Chunk.Delta,
					SummaryUsed: summaryUsed,
				},
				OccurredAt: time.Now(),
			})
		}
		if res.Chunk.Usage != nil {
			usage = res.Chunk.Usage
		}
	}

	now := time.Now()
	status := domainmessage.MessageStatusCompleted
	if streamErr != nil {
		status = domainmessage.MessageStatusFailed
	}

	if err := u.msgRepo.UpdateAIResponse(ctx, aiMsg.ID, content.String(), status, now); err != nil {
		slog.Error("failed to persist streamed AI response", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}
	aiMsg.Content = content.String()
	aiMsg.Status = status
	aiMsg.UpdatedAt = now

	u.hub.Publish(ctx, event.RoomEvent{
		Type:               event.EventMessageUpdated,
		RoomID:             roomID,
		Message:            &aiMsg,
		TargetUserIDs:      targetUserIDsForVisibility(&aiMsg),
		OccurredAt:         now,
		UsedContextSummary: summaryUsed,
	})

	if streamErr != nil {
		slog.Error("AI stream ended with error", "error", streamErr, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}

	if usage == nil {
		slog.Warn("AI stream completed with no usage-bearing final chunk; skipping usage recording",
			"room_id", roomID, "message_id", aiMsg.ID)
		return
	}

	// Fire-and-forget: the AI message is already durably persisted, so a
	// usage-recording failure must never affect anything downstream.
	if err := u.billing.RecordUsage(ctx, roomID, aiMsg.ID, model, usage.PromptTokens, usage.CompletionTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}
}

// completeAIMessageFallback is SendAIMessageStream's fallback completion
// path (M2 post-review fix), used only when the LLM Gateway's Stream call
// fails synchronously with domain.ErrStreamingUnsupported. Rather than
// surfacing that as a hard failure -- which used to silently break every
// streaming send whenever LLM_GATEWAY_TRANSPORT=grpc was configured, since
// gateway.GRPCClient never implements Stream -- this transparently completes
// the request through the unary ai.LLMGateway.Complete call instead,
// preserving the streaming endpoint's 202+placeholder contract (the caller
// already received a Status = MessageStatusStreaming placeholder) while
// never publishing an EventTokenChunk, since there is no incremental data to
// forward.
//
// Once Complete resolves (or fails), it persists the result via
// MessageRepository.UpdateAIResponse (Status = MessageStatusCompleted on
// success, MessageStatusFailed on failure, matching consumeAIStream's own
// status mapping) and publishes a single EventMessageUpdated with the
// resulting state -- the same terminating signal consumeAIStream's happy
// path publishes, so the client's merge logic (mergeMessageEvent) needs no
// special case for this fallback. On success it also records usage via
// BillingGuard.RecordUsage fire-and-forget, mirroring SendAIMessage's
// non-streaming usage recording -- unlike a stream's per-chunk Usage (which
// may never arrive), Complete's response always carries a usage total.
//
// aiMsg is a value copy, exactly like consumeAIStream's aiMsg parameter --
// see that method's doc comment for why mutating it here is safe. cancel is
// called unconditionally before returning, to release streamCtx's resources
// (mirroring consumeAIStream's own defer cancel()).
func (u *MessageUsecase) completeAIMessageFallback(
	ctx context.Context,
	cancel context.CancelFunc,
	aiMsg domainmessage.Message,
	roomID, model string,
	chatMsgs []ai.ChatMessage,
	summaryUsed bool,
) {
	defer cancel()

	completion, err := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})

	now := time.Now()
	status := domainmessage.MessageStatusCompleted
	content := ""
	if err != nil {
		status = domainmessage.MessageStatusFailed
	} else {
		content = completion.Content
	}

	if updateErr := u.msgRepo.UpdateAIResponse(ctx, aiMsg.ID, content, status, now); updateErr != nil {
		slog.Error("failed to persist gRPC-transport-fallback AI response", "error", updateErr, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}
	aiMsg.Content = content
	aiMsg.Status = status
	aiMsg.UpdatedAt = now

	u.hub.Publish(ctx, event.RoomEvent{
		Type:               event.EventMessageUpdated,
		RoomID:             roomID,
		Message:            &aiMsg,
		TargetUserIDs:      targetUserIDsForVisibility(&aiMsg),
		OccurredAt:         now,
		UsedContextSummary: summaryUsed,
	})

	if err != nil {
		slog.Error("gRPC-transport-fallback Complete call failed", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}

	// Fire-and-forget: the AI message is already durably persisted, so a
	// usage-recording failure must never affect anything downstream.
	if err := u.billing.RecordUsage(ctx, roomID, aiMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}
}
