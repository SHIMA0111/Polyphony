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

// streamFinalizeTimeout bounds the terminal persistence/publish/billing
// calls consumeAIStream makes once resultCh closes (MessageRepository.
// UpdateAIResponse, the final EventMessageUpdated publish, and
// BillingGuard.RecordUsage). These calls deliberately run against their own
// context derived via context.WithoutCancel(streamCtx) rather than streamCtx
// itself: streamCtx carries streamBackgroundTimeout's 5-minute deadline
// starting from when the stream began, so a response that runs close to
// that ceiling would otherwise leave little or no time -- possibly a
// deadline already in the past -- for finalization to complete, silently
// losing a fully-generated response's content at the last step. See
// consumeAIStream's use of this constant.
const streamFinalizeTimeout = 10 * time.Second

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
// If persisting that initial placeholder itself fails, this is a genuine Go
// error return (there is no earlier-created AI row to update), but a failed
// placeholder is saved on humanMsg's behalf before returning -- mirroring
// SendAIMessage's identical fix for its own completed-AI-message Create
// failure -- so the human message is never left without any AI row at all,
// which RegenerateAIMessage's retry path depends on existing.
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
		// The human message is already durably persisted, so this failure
		// must also get a failed placeholder saved before returning --
		// mirroring SendAIMessage's identical fix for its own completed-AI-
		// message Create failure -- otherwise the human message would be
		// left with no AI row at all (not even a failed one), and a client
		// retry via RegenerateAIMessage would have nothing to regenerate
		// against, since RegenerateAIMessage's GetNextInRoom lookup depends
		// on that row existing.
		u.saveFailedAIPlaceholderOnError(ctx, roomID, userID, humanMsg.ID, aiSeq, domainmessage.MessageVisibilityPublic, false, false, "streaming AI placeholder create", err)
		return nil, err
	}
	// summaryUsed is not yet known at this point -- context assembly runs
	// below -- so this first publish (mirroring the placeholder-created
	// event of every other path) reports false; the final EventMessageUpdated
	// publishes below carry the real value once it is known.
	u.publishMessageEvent(ctx, event.EventMessageCreated, roomID, aiMsg, aiNow, false)

	// Fetch context messages, same as SendAIMessage.
	//
	// From this point on, the placeholder AI message is already durably
	// persisted and its EventMessageCreated already published, so any error
	// path below must finalize it as failed before returning -- see
	// finalizeFailedStreamPlaceholder -- otherwise it would be stuck at
	// Status = MessageStatusStreaming forever, visible to every
	// WebSocket-connected room member as a response that never finishes.
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages, userID)
	if err != nil {
		u.finalizeFailedStreamPlaceholder(ctx, roomID, aiMsg, false, "context fetch", err)
		return nil, err
	}

	// Build chat messages (reverse to chronological order, filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages),
	// summarizing older history behind a cache when the estimated context
	// would overflow the model's resolved context window (Step 50), exactly
	// mirroring SendAIMessage's assembleAIContext call.
	chatMsgs, summaryUsed, err := u.assembleAIContext(ctx, rm, model, contextPage.Messages)
	if err != nil {
		u.finalizeFailedStreamPlaceholder(ctx, roomID, aiMsg, summaryUsed, "context assembly", err)
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
			// EventTokenChunk). aiMsgForCaller is an independent copy for the
			// same reason as the happy path below (see its comment there).
			aiMsgForCaller := *aiMsg
			go u.completeAIMessageFallback(streamCtx, cancel, aiMsgForCaller, roomID, model, chatMsgs, summaryUsed)
			return &SendAIResult{HumanMessage: humanMsg, AIMessage: &aiMsgForCaller, UsedContextSummary: summaryUsed}, nil
		}

		cancel()

		// Detached from ctx exactly like finalizeFailedStreamPlaceholder
		// above (see its doc comment): ctx is the request-scoped context
		// Echo cancels the instant this function returns, and this
		// finalization work must not race that cancellation.
		finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), streamFinalizeTimeout)
		defer finalizeCancel()

		failedNow := time.Now()
		if updateErr := u.msgRepo.UpdateAIResponse(finalizeCtx, aiMsg.ID, "", domainmessage.MessageStatusFailed, failedNow); updateErr != nil {
			// Both messages are already durable and the creation event has
			// been published; surfacing this as a request failure would
			// invite a client retry that duplicates the human turn. Log the
			// stuck-at-streaming placeholder and honor the dispatch-failure
			// contract by returning the committed IDs with their current
			// durable status.
			slog.Error("failed to finalize AI placeholder after stream dispatch failure",
				"error", updateErr, "room_id", roomID, "message_id", aiMsg.ID)
			return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg, UsedContextSummary: summaryUsed}, nil
		}
		aiMsg.Status = domainmessage.MessageStatusFailed
		aiMsg.UpdatedAt = failedNow
		u.publishMessageEvent(finalizeCtx, event.EventMessageUpdated, roomID, aiMsg, failedNow, summaryUsed)
		return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg, UsedContextSummary: summaryUsed}, nil
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

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: &aiMsgForCaller, UsedContextSummary: summaryUsed}, nil
}

// finalizeFailedStreamPlaceholder marks aiMsg -- the streaming placeholder
// SendAIMessageStream already created and announced via EventMessageCreated
// -- as failed, in response to a synchronous error that strikes after the
// placeholder exists but before the LLM Gateway's Stream call is even
// attempted (a context-fetch failure via ListByRoom, or a context-assembly
// failure via assembleAIContext). Without this, such a failure would leave
// the placeholder at Status = MessageStatusStreaming forever: nothing else
// ever transitions it out of "streaming", so every WebSocket-connected room
// member would see a response that never finishes. usedSummary is published
// verbatim on the resulting EventMessageUpdated (the caller passes false for
// the context-fetch failure, since assembleAIContext never ran to produce a
// real value).
//
// Unlike the synchronous LLM Gateway dispatch failure branch earlier in
// SendAIMessageStream (which returns its own UpdateAIResponse failure as
// SendAIMessageStream's error, since no earlier step failed to report), a
// failure of the UpdateAIResponse call here is only logged: the caller
// already has origErr to return, and finalizing the visible row is a
// best-effort cleanup layered on top of that, not the primary failure being
// reported.
//
// ctx here is SendAIMessageStream's own request-scoped context, which Echo
// cancels the instant the HTTP handler returns -- and this method's whole
// purpose is to run cleanup work *while* SendAIMessageStream is in the
// process of returning. Using ctx directly would race that cancellation:
// depending on exact timing, UpdateAIResponse/publishMessageEvent below
// could be cut off before completing, leaving the placeholder stuck at
// Status = MessageStatusStreaming forever -- precisely the outcome this
// method exists to prevent. It therefore derives its own detached
// finalizeCtx, exactly like consumeAIStream's identical finalizeCtx (see
// streamFinalizeTimeout's doc comment).
func (u *MessageUsecase) finalizeFailedStreamPlaceholder(ctx context.Context, roomID string, aiMsg *domainmessage.Message, usedSummary bool, step string, origErr error) {
	finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), streamFinalizeTimeout)
	defer finalizeCancel()

	failedNow := time.Now()
	if err := u.msgRepo.UpdateAIResponse(finalizeCtx, aiMsg.ID, "", domainmessage.MessageStatusFailed, failedNow); err != nil {
		slog.Error("failed to finalize streaming placeholder after a pre-dispatch error",
			"step", step, "original_error", origErr, "update_error", err, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}
	aiMsg.Status = domainmessage.MessageStatusFailed
	aiMsg.UpdatedAt = failedNow
	u.publishMessageEvent(finalizeCtx, event.EventMessageUpdated, roomID, aiMsg, failedNow, usedSummary)
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
// release streamCtx's resources. The terminal UpdateAIResponse/publish/
// RecordUsage calls run against finalizeCtx (see streamFinalizeTimeout), not
// ctx itself, so they are not starved by however little of streamCtx's own
// deadline remains by the time the stream ends.
//
// aiMsg is a value copy of the AI placeholder message (see
// SendAIMessageStream's aiMsgForCaller), so mutating its fields here is safe:
// it shares no memory with the aiMsg pointer SendAIMessageStream already
// returned to its caller. summaryUsed is threaded through from
// SendAIMessageStream's own assembleAIContext call and published verbatim
// on the final EventMessageUpdated (Step 50), mirroring SendAIMessage/
// RegenerateAIMessage's UsedContextSummary reporting.
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

	// finalizeCtx gives the terminal calls below their own short deadline,
	// detached from streamCtx's via context.WithoutCancel -- see
	// streamFinalizeTimeout's doc comment for why streamCtx itself (whose
	// remaining budget could be seconds or negative for a near-ceiling
	// stream) must not also govern finalization.
	finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), streamFinalizeTimeout)
	defer finalizeCancel()

	now := time.Now()
	status := domainmessage.MessageStatusCompleted
	if streamErr != nil {
		status = domainmessage.MessageStatusFailed
	}

	if err := u.msgRepo.UpdateAIResponse(finalizeCtx, aiMsg.ID, content.String(), status, now); err != nil {
		slog.Error("failed to persist streamed AI response", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}
	aiMsg.Content = content.String()
	aiMsg.Status = status
	aiMsg.UpdatedAt = now

	u.publishMessageEvent(finalizeCtx, event.EventMessageUpdated, roomID, &aiMsg, now, summaryUsed)

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
	if err := u.billing.RecordUsage(finalizeCtx, roomID, aiMsg.ID, model, usage.PromptTokens, usage.CompletionTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}
}

// completeAIMessageFallback is SendAIMessageStream's fallback completion
// path, used only when the LLM Gateway's Stream call fails synchronously
// with domain.ErrStreamingUnsupported. Rather than surfacing that as a hard
// failure -- which would otherwise silently break every streaming send
// whenever LLM_GATEWAY_TRANSPORT=grpc is configured, since gateway.GRPCClient
// never implements Stream -- this transparently completes the request
// through the unary ai.LLMGateway.Complete call instead, preserving the
// streaming endpoint's 202+placeholder contract (the caller already received
// a Status = MessageStatusStreaming placeholder) while never publishing an
// EventTokenChunk, since there is no incremental data to forward.
//
// Structurally this mirrors consumeAIStream (same dispatch point, same
// streamCtx/cancel/aiMsg-value-copy contract -- see its doc comment for why
// mutating aiMsg here is safe): ctx is streamCtx, carrying
// streamBackgroundTimeout's full budget for the Complete call itself, and
// cancel is called unconditionally before returning to release streamCtx's
// resources. Once Complete resolves (or fails), the terminal
// UpdateAIResponse/publish/RecordUsage calls run against their own detached
// finalizeCtx (see streamFinalizeTimeout and consumeAIStream's identical
// pattern), not ctx itself, so they are not starved by however little of
// streamCtx's own deadline remains by the time Complete returns.
//
// It persists the result via MessageRepository.UpdateAIResponse
// (Status = MessageStatusCompleted on success, MessageStatusFailed on
// failure, matching consumeAIStream's own status mapping) and publishes a
// single EventMessageUpdated with the resulting state -- the same
// terminating signal consumeAIStream's happy path publishes, so callers need
// no special case for this fallback. On success it also records usage via
// BillingGuard.RecordUsage fire-and-forget, mirroring SendAIMessage's
// non-streaming usage recording -- unlike a stream's per-chunk Usage (which
// may never arrive), Complete's response always carries a usage total.
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

	// Detached from ctx (streamCtx) exactly like consumeAIStream's own
	// finalizeCtx -- see this method's doc comment for why.
	finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), streamFinalizeTimeout)
	defer finalizeCancel()

	now := time.Now()
	status := domainmessage.MessageStatusCompleted
	content := ""
	if err != nil {
		status = domainmessage.MessageStatusFailed
	} else {
		content = completion.Content
	}

	if updateErr := u.msgRepo.UpdateAIResponse(finalizeCtx, aiMsg.ID, content, status, now); updateErr != nil {
		slog.Error("failed to persist gRPC-transport-fallback AI response", "error", updateErr, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}
	aiMsg.Content = content
	aiMsg.Status = status
	aiMsg.UpdatedAt = now

	u.publishMessageEvent(finalizeCtx, event.EventMessageUpdated, roomID, &aiMsg, now, summaryUsed)

	if err != nil {
		slog.Error("gRPC-transport-fallback Complete call failed", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
		return
	}

	// Fire-and-forget: the AI message is already durably persisted, so a
	// usage-recording failure must never affect anything downstream.
	if err := u.billing.RecordUsage(finalizeCtx, roomID, aiMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}
}
