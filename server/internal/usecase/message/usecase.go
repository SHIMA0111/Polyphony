// Package message implements the chat message use cases (MessageUsecase):
// sending and listing human messages, invoking the AI to generate and
// regenerate AI messages within a room, and building the AI context from
// prior room history, on top of the domain/message entity and repository
// port.
package message

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/storage"
)

// defaultContextMessages is the default number of recent room messages
// fetched as raw input to assembleAIContext (see context.go) by
// SendAIMessage, SendAIMessageStream (stream.go), and RegenerateAIMessage.
const defaultContextMessages = 50

// SendAIResult holds both the human and AI messages from a SendAIMessage call.
// When AIMessage.Status is "failed", the LLM call failed but both messages were persisted.
type SendAIResult struct {
	HumanMessage *domainmessage.Message
	AIMessage    *domainmessage.Message
	// UsedContextSummary reports whether the context assembled for this
	// call (see assembleAIContext) included a cached/freshly-computed
	// summary of older history in place of the raw messages it replaces.
	// It is a one-time, request-scoped signal describing how AIMessage was
	// generated -- it is never persisted on the message itself (see
	// handler.MessageResponse.UsedContextSummary's doc comment).
	UsedContextSummary bool
}

// MessageUsecase provides message-related business logic.
//
// Broadcasting is fire-and-forget: after each successful persist, the
// usecase publishes a RoomEvent via hub, but hub.Publish never returns an
// error and is never awaited for delivery, so a broadcast failure (e.g. a
// slow subscriber) can never roll back a write or cause the surrounding HTTP
// request to fail.
type MessageUsecase struct {
	msgRepo        domainmessage.MessageRepository
	roomRepo       room.RoomRepository
	llmGateway     ai.LLMGateway
	hub            event.MessageHub
	contextBuilder ai.ContextBuilder
	billing        BillingGuard
	attachmentRepo domainattachment.AttachmentRepository
	objStorage     storage.ObjectStorage
	// summaryRepo caches and invalidates per-room context summaries (Step
	// 50, phases.md Phase 18). See assembleAIContext for how it is
	// consulted/written, and DeleteMessage/SetExcludeFromAI for
	// invalidation.
	summaryRepo ai.ContextSummaryRepository
	// defaultAIModel is the deployment-wide fallback model string, sourced
	// from Config.DefaultAIModel by the caller of NewMessageUsecase. It is
	// the lowest-precedence tier consulted by resolveModel, used only when
	// both the request and the room's configured Room.AIModel are empty.
	defaultAIModel string
}

// NewMessageUsecase creates a new MessageUsecase. hub receives a
// message_created/message_updated event after every successful message
// persist; pass event.NewInProcessHub() for the initial in-process
// implementation. contextBuilder is always initialized internally to
// ai.NewDefaultContextBuilder(); it is not a constructor parameter so that
// swapping the implementation (e.g. for history summarization in a later
// phase) does not require touching every call site. billing is consulted
// before every AI invocation (SendAIMessage/RegenerateAIMessage) to reject
// requests once the room owner's token balance is exhausted, and to record
// usage after a successful completion; pass a *billingusecase.BillingUsecase
// (see usecase/billing), which satisfies BillingGuard structurally.
// attachmentRepo and objStorage are used only to enrich AI context with
// image attachments (see enrichWithAttachments): attachmentRepo looks up a
// message's attachments (see domain/attachment, landed in Step 12) and
// objStorage mints a fresh presigned view URL for each one, reusing the same
// storage.ObjectStorage.PresignView helper usecase/attachment's
// ListAttachments uses rather than re-deriving S3 URLs here. summaryRepo
// caches and invalidates per-room AI context summaries (see
// assembleAIContext); pass a postgres.ContextSummaryRepository (see
// interface/repository/postgres). defaultAIModel is the deployment-wide
// fallback model string consulted by resolveModel (see
// model_resolution.go) whenever an AI request omits an explicit model
// and the target room has no configured domainroom.Room.AIModel; pass
// cfg.DefaultAIModel from internal/infrastructure/config.Config.
func NewMessageUsecase(
	msgRepo domainmessage.MessageRepository,
	roomRepo room.RoomRepository,
	llmGateway ai.LLMGateway,
	hub event.MessageHub,
	billing BillingGuard,
	attachmentRepo domainattachment.AttachmentRepository,
	objStorage storage.ObjectStorage,
	summaryRepo ai.ContextSummaryRepository,
	defaultAIModel string,
) *MessageUsecase {
	return &MessageUsecase{
		msgRepo:        msgRepo,
		roomRepo:       roomRepo,
		llmGateway:     llmGateway,
		hub:            hub,
		contextBuilder: ai.NewDefaultContextBuilder(),
		billing:        billing,
		attachmentRepo: attachmentRepo,
		objStorage:     objStorage,
		summaryRepo:    summaryRepo,
		defaultAIModel: defaultAIModel,
	}
}

// SendMessage creates a human message in a room. The caller must be allowed
// domainroom.ActionSendMessage (guest or above; a reader may not send). It
// reserves a single sequence number, persists the message, and publishes
// EventMessageCreated on the hub after the persist succeeds. Publishing is
// fire-and-forget: its outcome never affects the returned error, and it only
// happens once the write has already succeeded.
//
// It returns domain.ErrArchivedRoom if the room is archived (see
// domainroom.Room.IsArchived, set while a room-fork copy job is in
// progress — usecase/room.RoomUsecase.ForkRoom/runForkJob) — new posts into
// an archived room are rejected before any sequence number is reserved.
func (u *MessageUsecase) SendMessage(ctx context.Context, userID, roomID, content string) (*domainmessage.Message, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionSendMessage) {
		return nil, domain.ErrForbidden
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if rm.IsArchived {
		return nil, domain.ErrArchivedRoom
	}

	seq, err := u.msgRepo.ReserveSequenceRange(ctx, roomID, 1)
	if err != nil {
		return nil, err
	}

	return u.createHumanMessage(ctx, userID, roomID, content, seq, domainmessage.MessageVisibilityPublic)
}

// createHumanMessage builds a human message for the given (already reserved)
// sequence number, persists it, and publishes EventMessageCreated after the
// persist succeeds. It is shared by SendMessage (which always passes
// MessageVisibilityPublic and reserves a single sequence) and SendAIMessage
// (which passes MessageVisibilityPrivate when the caller opted into private
// AI mode, and reserves a paired range up front) so both paths construct and
// persist the human message identically apart from visibility. Publishing
// targets only the sender's connections for a private message and the whole
// room for a public one (see targetUserIDsForVisibility).
func (u *MessageUsecase) createHumanMessage(ctx context.Context, userID, roomID, content string, seq int64, visibility domainmessage.MessageVisibility) (*domainmessage.Message, error) {
	now := time.Now()
	msg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     roomID,
		SenderID:   &userID,
		Content:    content,
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   seq,
		Visibility: visibility,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := u.msgRepo.Create(ctx, msg); err != nil {
		return nil, err
	}

	u.publishMessageEvent(ctx, event.EventMessageCreated, roomID, msg, now, false)

	return msg, nil
}

// targetUserIDsForVisibility returns the WebSocket delivery target for msg:
// nil for a public message, meaning "broadcast to every subscriber of the
// room" (event.RoomEvent.TargetUserIDs's documented zero-value behavior); or
// a single-element slice containing msg.SenderID for a private message, so
// it is delivered only to its owner's connections and never reaches any
// other room member. It relies on the invariant that every private message
// — human or AI — has SenderID set to the requesting user's ID (see the
// SenderID deviation comment in SendAIMessage for why this holds for AI
// messages too, which otherwise always have a nil SenderID).
//
// That invariant can be violated: sender_id is
// ON DELETE SET NULL (schema.sql), so a private message can end up with a
// nil SenderID if its sender's user row is later deleted. targetUserIDsForVisibility
// itself has no way to signal that case — it returns nil, which by
// event.RoomEvent.TargetUserIDs's contract means "broadcast to everyone",
// the opposite of what a private message requires. Callers must not publish
// in that case; see publishMessageEvent, which is why every publish call
// site in this package goes through it rather than calling hub.Publish
// directly.
func targetUserIDsForVisibility(msg *domainmessage.Message) []string {
	if msg.Visibility != domainmessage.MessageVisibilityPrivate || msg.SenderID == nil {
		return nil
	}
	return []string{*msg.SenderID}
}

// publishMessageEvent publishes a RoomEvent describing msg on the hub,
// unless msg is an ownerless private message (Visibility ==
// MessageVisibilityPrivate && SenderID == nil) — a state reachable in
// production because sender_id is ON DELETE SET NULL (schema.sql) rather
// than protected by a DB-level CHECK constraint, so the app layer is the
// only place left to catch it. In that state,
// targetUserIDsForVisibility(msg) returns nil, and per
// event.RoomEvent.TargetUserIDs's contract nil means "broadcast to every
// subscriber of the room" — exactly what a private message must never do.
// Since TargetUserIDs has no way to express "deliver to nobody", the only
// safe action is to drop the event instead of publishing it; this is
// therefore logged as an error rather than silently skipped, since it
// always indicates the message's owning user row was deleted out from
// under it.
//
// usedContextSummary is forwarded verbatim onto the published
// event.RoomEvent.UsedContextSummary (Step 50): true only for the AI
// message events produced by a SendAIMessage/RegenerateAIMessage call whose
// context assembly folded older history into a cached summary. Callers with
// nothing to report (e.g. SendMessage's plain human-message publish) pass
// false.
func (u *MessageUsecase) publishMessageEvent(ctx context.Context, eventType event.EventType, roomID string, msg *domainmessage.Message, occurredAt time.Time, usedContextSummary bool) {
	if msg.Visibility == domainmessage.MessageVisibilityPrivate && msg.SenderID == nil {
		slog.Error("suppressing publish of an ownerless private message: TargetUserIDs cannot express \"nobody\"",
			"room_id", roomID, "message_id", msg.ID)
		return
	}
	u.hub.Publish(ctx, event.RoomEvent{
		Type:               eventType,
		RoomID:             roomID,
		Message:            msg,
		TargetUserIDs:      targetUserIDsForVisibility(msg),
		OccurredAt:         occurredAt,
		UsedContextSummary: usedContextSummary,
	})
}

// ListMessages returns paginated messages for a room. Any valid member
// (including reader) may list messages; no domainroom.Action check beyond
// membership is applied. Another user's private messages are excluded from
// the page (see MessageRepository.ListByRoom).
func (u *MessageUsecase) ListMessages(ctx context.Context, userID, roomID, cursor string, limit int) (*domainmessage.CursorPage, error) {
	if _, err := u.getMember(ctx, roomID, userID); err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	return u.msgRepo.ListByRoom(ctx, roomID, cursor, limit, userID)
}

// SendAIMessage sends a human message and gets an AI response.
// On LLM failure, a placeholder AI message with status=failed is saved so that
// RegenerateAIMessage can retry later via UPDATE only. The same placeholder is
// saved — and this function then returns the underlying error — if a failure
// occurs anywhere after the human message is durably persisted but before the
// AI message is (context-fetch via ListByRoom, attachment enrichment, or the
// completed AI message's own msgRepo.Create call failing): without it, a
// client that retries after such an error would resubmit the
// same content and duplicate the human message, because nothing on the
// server records that this human message is still unanswered. With the
// placeholder saved, the exchange looks exactly like an LLM-call failure, so
// the client's existing retry path (RegenerateAIMessage on the human message
// ID) applies uniformly regardless of which step failed. See
// createFailedAIPlaceholder.
// The result always contains both the human and AI messages; check AIMessage.Status
// to determine whether the LLM call succeeded.
//
// The human and AI sequence numbers are reserved together as a single
// contiguous range (via ReserveSequenceRange(ctx, roomID, 2)) before either
// row is written, so no concurrent request can allocate a sequence number
// that lands between them — this is what guarantees the adjacency that
// RegenerateAIMessage's GetNextInRoom lookup depends on. The AI message
// records InResponseToMessageID pointing at the human message, and each
// persisted message publishes EventMessageCreated on the hub after its
// Create call succeeds; publishing never affects the returned error.
//
// The caller must be allowed domainroom.ActionInvokeAI (member or above; a
// reader or guest may not invoke AI).
//
// When private is true (private AI mode, phases.md Phase 14), both the
// human message and the AI response are persisted with
// Visibility = MessageVisibilityPrivate: neither is ever returned by
// ListByRoom/GetByID/ListByRoomUpTo to any user other than userID, neither
// is included in AI context assembled for another user's request, and both
// are delivered over WebSocket only to userID's own connections instead of
// being broadcast to the room (see targetUserIDsForVisibility).
//
// It returns domain.ErrArchivedRoom if the room is archived (see
// domainroom.Room.IsArchived / SendMessage's matching guard) — checked
// right after loading rm, before any sequence number is reserved.
func (u *MessageUsecase) SendAIMessage(ctx context.Context, userID, roomID, content, model string, private bool) (*SendAIResult, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionInvokeAI) {
		return nil, domain.ErrForbidden
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

	visibility := domainmessage.MessageVisibilityPublic
	if private {
		visibility = domainmessage.MessageVisibilityPrivate
	}

	// Reserve both sequence numbers atomically as one range before creating
	// either row, so nothing else can be interleaved between the human
	// message and its AI response.
	firstSeq, err := u.msgRepo.ReserveSequenceRange(ctx, roomID, 2)
	if err != nil {
		return nil, err
	}
	humanSeq, aiSeq := firstSeq, firstSeq+1

	humanMsg, err := u.createHumanMessage(ctx, userID, roomID, content, humanSeq, visibility)
	if err != nil {
		return nil, err
	}

	// Fetch context messages. Passing userID as requestingUserID excludes
	// any other user's private messages from the context this AI call sees.
	//
	// From this point on, the human message is already durably persisted, so
	// any error path below must save a failed AI placeholder before
	// returning — see createFailedAIPlaceholder and the doc comment above.
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages, userID)
	if err != nil {
		// Context assembly has not run yet, so there is no summary usage to
		// report.
		u.saveFailedAIPlaceholderOnError(ctx, roomID, userID, humanMsg.ID, aiSeq, visibility, private, false, "context fetch", err)
		return nil, err
	}

	// Build chat messages (reverse to chronological order, filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages),
	// summarizing older history behind a cache when the estimated context
	// would overflow model's resolved context window (Step 50).
	chatMsgs, usedSummary, err := u.assembleAIContext(ctx, rm, model, contextPage.Messages)
	if err != nil {
		u.saveFailedAIPlaceholderOnError(ctx, roomID, userID, humanMsg.ID, aiSeq, visibility, private, usedSummary, "attachment enrichment", err)
		return nil, err
	}

	// Call LLM Gateway
	completion, llmErr := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})
	if llmErr != nil {
		aiMsg, err := u.createFailedAIPlaceholder(ctx, roomID, userID, humanMsg.ID, aiSeq, visibility, private, usedSummary)
		if err != nil {
			return nil, err
		}
		return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg, UsedContextSummary: usedSummary}, nil
	}

	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:                    uuid.New().String(),
		RoomID:                roomID,
		SenderID:              nil,
		Content:               completion.Content,
		Type:                  domainmessage.MessageTypeAI,
		Status:                domainmessage.MessageStatusCompleted,
		Sequence:              aiSeq,
		Visibility:            visibility,
		InResponseToMessageID: &humanMsg.ID,
		CreatedAt:             aiNow,
		UpdatedAt:             aiNow,
	}
	if private {
		// Deviation from the usual "AI messages have a nil SenderID"
		// convention: a private AI message records userID as its SenderID
		// so the single `visibility = 'public' OR sender_id = $requestingUserID`
		// filter (MessageRepository.GetByID/ListByRoom/ListByRoomUpTo) works
		// uniformly for both the human and AI rows of a private exchange,
		// without introducing a second "owner" column just for AI messages.
		aiMsg.SenderID = &userID
	}

	if err = u.msgRepo.Create(ctx, aiMsg); err != nil {
		// The human message is already durably persisted (see the doc
		// comment above), so this failure must also get a failed placeholder
		// saved before returning, exactly like the context-fetch and
		// attachment-enrichment failure paths above -- otherwise a client
		// retry would resubmit and duplicate the human message.
		u.saveFailedAIPlaceholderOnError(ctx, roomID, userID, humanMsg.ID, aiSeq, visibility, private, usedSummary, "completed AI message create", err)
		return nil, err
	}
	u.publishMessageEvent(ctx, event.EventMessageCreated, roomID, aiMsg, aiNow, usedSummary)

	// Fire-and-forget: the AI message is already durably persisted, so a
	// usage-recording failure must never affect the returned result.
	if err := u.billing.RecordUsage(ctx, roomID, aiMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg, UsedContextSummary: usedSummary}, nil
}

// createFailedAIPlaceholder persists and publishes a status=failed AI
// message in response to humanMsgID, at the already-reserved aiSeq. It is
// the single implementation shared by every SendAIMessage failure path that
// occurs after the human message has been durably persisted (LLM call
// failure, context-fetch failure, attachment-enrichment failure — see
// SendAIMessage's doc comment for why they must all behave identically):
// without a placeholder, a client retry has no way to distinguish "still
// unanswered" from "never asked" and would resubmit the same content,
// duplicating the human message. Once the placeholder exists, the normal
// RegenerateAIMessage(ctx, userID, roomID, humanMsgID, model) retry path
// applies uniformly regardless of which step failed.
//
// aiMsg.SenderID mirrors the successful path's rule for a private exchange
// (see the SenderID deviation comment in SendAIMessage). usedSummary is
// forwarded verbatim to the published event's UsedContextSummary (Step 50);
// callers that failed before context assembly ran at all (e.g. the
// context-fetch failure path) must pass false.
func (u *MessageUsecase) createFailedAIPlaceholder(ctx context.Context, roomID, userID, humanMsgID string, aiSeq int64, visibility domainmessage.MessageVisibility, private bool, usedSummary bool) (*domainmessage.Message, error) {
	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:                    uuid.New().String(),
		RoomID:                roomID,
		SenderID:              nil,
		Status:                domainmessage.MessageStatusFailed,
		Type:                  domainmessage.MessageTypeAI,
		Sequence:              aiSeq,
		Visibility:            visibility,
		InResponseToMessageID: &humanMsgID,
		CreatedAt:             aiNow,
		UpdatedAt:             aiNow,
	}
	if private {
		aiMsg.SenderID = &userID
	}
	if err := u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}
	u.publishMessageEvent(ctx, event.EventMessageCreated, roomID, aiMsg, aiNow, usedSummary)
	return aiMsg, nil
}

// saveFailedAIPlaceholderOnError calls createFailedAIPlaceholder on behalf
// of a SendAIMessage error path that is about to return origErr (a failure
// distinct from the LLM call itself, e.g. context fetch or attachment
// enrichment — see SendAIMessage's doc comment). Unlike the LLM-failure
// path, origErr is always what SendAIMessage returns to its caller
// regardless of whether the placeholder could be saved, so a secondary
// failure to create the placeholder is only logged here (tagged with step)
// rather than replacing or being combined with origErr. usedSummary is
// forwarded to createFailedAIPlaceholder unchanged.
func (u *MessageUsecase) saveFailedAIPlaceholderOnError(ctx context.Context, roomID, userID, humanMsgID string, aiSeq int64, visibility domainmessage.MessageVisibility, private bool, usedSummary bool, step string, origErr error) {
	if _, err := u.createFailedAIPlaceholder(ctx, roomID, userID, humanMsgID, aiSeq, visibility, private, usedSummary); err != nil {
		slog.Error("failed to save failed AI placeholder after SendAIMessage error",
			"step", step, "original_error", origErr, "placeholder_error", err, "room_id", roomID, "human_message_id", humanMsgID)
	}
}

// RegenerateAIMessage regenerates the AI response for a specific human message.
// It always updates the existing AI message (created by SendAIMessage) in place,
// preserving sequence order. The AI message is guaranteed to exist because
// SendAIMessage always creates a placeholder even on LLM failure. After the
// update succeeds, it publishes EventMessageUpdated on the hub; publishing is
// fire-and-forget and never affects the returned error.
//
// The caller must be allowed domainroom.ActionInvokeAI (member or above; a
// reader or guest may not invoke AI).
//
// The target human message is fetched with userID as requestingUserID, so a
// private exchange belonging to another user is invisible to this lookup;
// since room membership has already been verified above, a resulting
// domain.ErrNotFound (which is indistinguishable from a genuinely missing
// message — see MessageRepository.GetByID) is surfaced as domain.ErrForbidden
// rather than domain.ErrNotFound, because the only way a member can fail to
// see an otherwise-existing message is that it is private and belongs to
// someone else.
//
// The second return value reports whether the regenerated response's
// context included a summary of older history (see assembleAIContext); it
// is a one-time, request-scoped signal, not a persisted message property.
func (u *MessageUsecase) RegenerateAIMessage(ctx context.Context, userID, roomID, messageID, model string) (*domainmessage.Message, bool, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, false, err
	}
	if err := room.Authorize(member.Role, room.ActionInvokeAI); err != nil {
		return nil, false, err
	}
	if err := u.billing.CheckBalance(ctx, roomID); err != nil {
		return nil, false, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, false, err
	}
	model = resolveModel(model, rm, u.defaultAIModel)

	// Verify target message exists, is visible to userID, and belongs to
	// the room.
	targetMsg, err := u.msgRepo.GetByID(ctx, messageID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, false, domain.ErrForbidden
		}
		return nil, false, err
	}
	if targetMsg.RoomID != roomID {
		return nil, false, domain.ErrNotFound
	}
	if targetMsg.Type != domainmessage.MessageTypeHuman {
		return nil, false, domain.ErrInvalidMessageType
	}

	// Find the AI response that follows the target message
	nextMsg, err := u.msgRepo.GetNextInRoom(ctx, roomID, targetMsg.Sequence)
	if err != nil {
		return nil, false, err
	}
	if nextMsg.Type != domainmessage.MessageTypeAI {
		return nil, false, domain.ErrNotFound
	}

	// Fetch context up to the target message (inclusive), excluding any
	// other user's private messages from what this regeneration call sees.
	contextMsgs, err := u.msgRepo.ListByRoomUpTo(ctx, roomID, targetMsg.Sequence, defaultContextMessages, userID)
	if err != nil {
		return nil, false, err
	}

	// Build chat messages (reverse to chronological order, filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages),
	// summarizing older history behind a cache when the estimated context
	// would overflow the model's resolved context window (Step 50).
	chatMsgs, usedSummary, err := u.assembleAIContext(ctx, rm, model, contextMsgs)
	if err != nil {
		return nil, false, err
	}

	// Call LLM Gateway
	completion, err := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})
	if err != nil {
		return nil, false, err
	}

	// Update existing AI response in place (preserve sequence and created_at)
	now := time.Now()
	if err = u.msgRepo.UpdateAIResponse(ctx, nextMsg.ID, completion.Content, domainmessage.MessageStatusCompleted, now); err != nil {
		return nil, false, err
	}
	nextMsg.Content = completion.Content
	nextMsg.Status = domainmessage.MessageStatusCompleted
	nextMsg.UpdatedAt = now

	// Fire-and-forget: the AI message update is already durably persisted,
	// so a usage-recording failure must never affect the returned result.
	if err := u.billing.RecordUsage(ctx, roomID, nextMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", nextMsg.ID)
	}

	u.publishMessageEvent(ctx, event.EventMessageUpdated, roomID, nextMsg, now, usedSummary)

	return nextMsg, usedSummary, nil
}

// DeleteMessage soft-deletes a message in roomID on behalf of userID. The
// caller must either be the message's own sender (SenderID == userID) or
// hold at least domainroom.RoleAdmin in the room (a moderation delete by an
// admin or master). Any other caller — including a non-sender member below
// admin — gets domain.ErrForbidden. It returns domain.ErrNotFound if the
// message does not exist or does not belong to roomID. On success it
// delegates to msgRepo.Delete, which performs the soft delete (see
// domainmessage.MessageRepository.Delete).
//
// DeleteMessage intentionally does not use domainroom.Action/Allows: the
// Action enum has no message-level delete action, so the owner-or-admin
// rule is expressed directly via member.Role.AtLeast(domainroom.RoleAdmin)
// combined with the sender-equality check.
func (u *MessageUsecase) DeleteMessage(ctx context.Context, userID, roomID, messageID string) error {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return err
	}

	msg, err := u.msgRepo.GetByID(ctx, messageID, userID)
	if err != nil {
		return err
	}
	if msg.RoomID != roomID {
		return domain.ErrNotFound
	}

	isOwner := msg.SenderID != nil && *msg.SenderID == userID
	isModerator := member.Role.AtLeast(room.RoleAdmin)
	if !isOwner && !isModerator {
		return domain.ErrForbidden
	}

	if err := u.msgRepo.Delete(ctx, messageID); err != nil {
		return err
	}

	// Invalidate the room's cached context summary (Step 50): a deleted
	// message may fall within the previously-summarized range, and coarsely
	// wiping the whole room's cache on every delete (rather than checking
	// whether it actually does) trades a possibly-unnecessary
	// re-summarization for guaranteed correctness. Best-effort: a cache
	// invalidation failure must never fail the delete that already
	// succeeded.
	if err := u.summaryRepo.DeleteByRoom(ctx, roomID); err != nil {
		slog.Error("failed to invalidate cached context summary", "error", err, "room_id", roomID)
	}

	return nil
}

// SetExcludeFromAI toggles whether a message is excluded from future AI
// context assembly (ai.ContextBuilder.Build), without affecting its
// visibility in normal room message listings. The caller must be allowed
// domainroom.ActionInvokeAI (member or above; a reader or guest may not
// toggle this flag) — the same role gate SendAIMessage/RegenerateAIMessage
// use. It returns domain.ErrNotFound if the message does not exist or does
// not belong to roomID. On success it persists the change via
// msgRepo.UpdateExcludeFromAI and returns the mutated in-memory Message
// (mirroring RegenerateAIMessage's pattern of returning the updated struct
// rather than re-fetching).
func (u *MessageUsecase) SetExcludeFromAI(ctx context.Context, userID, roomID, messageID string, exclude bool) (*domainmessage.Message, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !member.Role.Allows(room.ActionInvokeAI) {
		return nil, domain.ErrForbidden
	}

	msg, err := u.msgRepo.GetByID(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	if msg.RoomID != roomID {
		return nil, domain.ErrNotFound
	}

	now := time.Now()
	if err := u.msgRepo.UpdateExcludeFromAI(ctx, messageID, exclude, now); err != nil {
		return nil, err
	}
	msg.ExcludeFromAI = exclude
	msg.UpdatedAt = now

	// Invalidate the room's cached context summary (Step 50): see
	// DeleteMessage's identical invalidation call for why this is
	// deliberately coarse (whole-room, not range-checked) and best-effort.
	if err := u.summaryRepo.DeleteByRoom(ctx, roomID); err != nil {
		slog.Error("failed to invalidate cached context summary", "error", err, "room_id", roomID)
	}

	return msg, nil
}

// getMember loads the caller's membership in roomID, translating a missing
// membership (domain.ErrNotFound) into domain.ErrForbidden so that a
// non-member can never distinguish "room does not exist" from "room exists
// but I'm not a member of it" via the returned error.
func (u *MessageUsecase) getMember(ctx context.Context, roomID, userID string) (*room.RoomMember, error) {
	member, err := u.roomRepo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrForbidden
		}
		return nil, err
	}
	return member, nil
}
