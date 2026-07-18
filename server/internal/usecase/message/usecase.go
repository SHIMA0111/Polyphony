// Package message implements the chat message use cases: sending and
// streaming human/AI messages, regenerating an AI reply, listing message
// history with cursor pagination, assembling AI context (with older-history
// summarization when it doesn't fit the model's token limit -- context.go),
// resolving which model to invoke (model_resolution.go), and guarding calls
// against the room's token balance (billing_guard.go).
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
	if err := room.Authorize(member.Role, room.ActionSendMessage); err != nil {
		return nil, err
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

	u.publishMessageEvent(ctx, event.RoomEvent{
		Type:          event.EventMessageCreated,
		RoomID:        roomID,
		Message:       msg,
		TargetUserIDs: targetUserIDsForVisibility(msg),
		OccurredAt:    now,
	})

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
func targetUserIDsForVisibility(msg *domainmessage.Message) []string {
	if msg.Visibility != domainmessage.MessageVisibilityPrivate || msg.SenderID == nil {
		return nil
	}
	return []string{*msg.SenderID}
}

// publishMessageEvent publishes evt via u.hub, except when evt.Message is a
// private message with a nil SenderID — an "ownerless" private message,
// which can occur because messages.sender_id is ON DELETE SET NULL (see
// schema.sql): if the user who sent a private message is later deleted, the
// row survives with SenderID reset to nil, but its Visibility stays
// private.
//
// For that combination, targetUserIDsForVisibility falls through to its nil
// return (its documented "broadcast to every subscriber of the room"
// sentinel), because it has no other value to return once SenderID is nil.
// That would broadcast a private exchange to the whole room the instant its
// owner's account is deleted — exactly the leak private messages exist to
// prevent. There is no TargetUserIDs value that means "deliver to nobody"
// (see event.RoomEvent.TargetUserIDs's doc comment: nil and empty both mean
// "everyone"), so the only correct app-layer response is to suppress the
// publish entirely rather than attempt to express it through TargetUserIDs.
// This is deliberately not enforced with a DB CHECK constraint: sender_id's
// ON DELETE SET NULL is what keeps user deletion from being blocked by (or
// cascading through) their message history, and a CHECK forbidding
// private+null together would conflict with that by construction.
//
// The message itself is unaffected by this suppression — it was already
// durably persisted before evt was built, and every read path
// (MessageRepository.GetByID/ListByRoom/ListByRoomUpTo's visibilityFilter)
// already excludes a private message from every requestingUserID including
// its own former sender, so it simply becomes unreadable by anyone from
// that point on. Only the live WebSocket delivery is skipped. The
// suppression is logged at Error level, since it reflects a real (if rare)
// data state worth surfacing rather than a routine, expected condition.
func (u *MessageUsecase) publishMessageEvent(ctx context.Context, evt event.RoomEvent) {
	if evt.Message != nil && evt.Message.Visibility == domainmessage.MessageVisibilityPrivate && evt.Message.SenderID == nil {
		slog.Error("suppressing publish for an ownerless private message: no TargetUserIDs value can express delivery to nobody",
			"room_id", evt.RoomID, "message_id", evt.Message.ID, "event_type", evt.Type)
		return
	}
	u.hub.Publish(ctx, evt)
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
// RegenerateAIMessage can retry later via UPDATE only.
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
	// From here on, humanMsg has already been durably persisted and
	// broadcast (see createHumanMessage above): every remaining error path
	// in this function must go through persistFailedAIPlaceholder before
	// returning, so that a client retry lands on RegenerateAIMessage's
	// UPDATE-in-place path instead of calling SendAIMessage again and
	// duplicating the human message.
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages, userID)
	if err != nil {
		if _, phErr := u.persistFailedAIPlaceholder(ctx, roomID, userID, humanMsg, aiSeq, visibility, private, false); phErr != nil {
			slog.Error("failed to persist failed AI message placeholder after context fetch error",
				"error", phErr, "room_id", roomID, "human_message_id", humanMsg.ID)
		}
		return nil, err
	}

	// Build chat messages (reverse to chronological order, filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages),
	// summarizing older history behind a cache when the estimated context
	// would overflow model's resolved context window (Step 50).
	chatMsgs, usedSummary, err := u.assembleAIContext(ctx, rm, model, contextPage.Messages)
	if err != nil {
		if _, phErr := u.persistFailedAIPlaceholder(ctx, roomID, userID, humanMsg, aiSeq, visibility, private, usedSummary); phErr != nil {
			slog.Error("failed to persist failed AI message placeholder after context assembly error",
				"error", phErr, "room_id", roomID, "human_message_id", humanMsg.ID)
		}
		return nil, err
	}

	// Call LLM Gateway
	completion, llmErr := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})

	if llmErr != nil {
		aiMsg, err := u.persistFailedAIPlaceholder(ctx, roomID, userID, humanMsg, aiSeq, visibility, private, usedSummary)
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
		Type:                  domainmessage.MessageTypeAI,
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

	aiMsg.Content = completion.Content
	aiMsg.Status = domainmessage.MessageStatusCompleted

	if err = u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}
	u.publishMessageEvent(ctx, event.RoomEvent{
		Type:               event.EventMessageCreated,
		RoomID:             roomID,
		Message:            aiMsg,
		TargetUserIDs:      targetUserIDsForVisibility(aiMsg),
		OccurredAt:         aiNow,
		UsedContextSummary: usedSummary,
	})

	// Fire-and-forget: the AI message is already durably persisted, so a
	// usage-recording failure must never affect the returned result.
	if err := u.billing.RecordUsage(ctx, roomID, aiMsg.ID, model, completion.PromptTokens, completion.OutputTokens); err != nil {
		slog.Error("failed to record token usage", "error", err, "room_id", roomID, "message_id", aiMsg.ID)
	}

	return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg, UsedContextSummary: usedSummary}, nil
}

// persistFailedAIPlaceholder creates and publishes a failed-status AI message
// standing in for the response SendAIMessage was unable to produce, whatever
// the cause — the context fetch (ListByRoom), context assembly
// (assembleAIContext), or the LLM call itself. It is the single choke point
// every SendAIMessage error path takes once humanMsg has already been
// durably persisted and broadcast, so that a client retry after any such
// error finds an existing failed AI message for humanMsg and lands on
// RegenerateAIMessage's UPDATE-in-place path (see its GoDoc) instead of
// calling SendAIMessage again and duplicating the human message.
//
// A failure to persist or publish the placeholder itself is returned to the
// caller rather than swallowed, since the two current call sites need
// different treatment: SendAIMessage's context-fetch/context-assembly error
// paths only log it (their own, more specific error is what gets returned
// to the client), while its LLM-failure path treats it the same as any
// other message-persistence failure and returns it directly.
func (u *MessageUsecase) persistFailedAIPlaceholder(ctx context.Context, roomID, userID string, humanMsg *domainmessage.Message, aiSeq int64, visibility domainmessage.MessageVisibility, private, usedSummary bool) (*domainmessage.Message, error) {
	aiNow := time.Now()
	aiMsg := &domainmessage.Message{
		ID:                    uuid.New().String(),
		RoomID:                roomID,
		SenderID:              nil,
		Content:               "",
		Type:                  domainmessage.MessageTypeAI,
		Status:                domainmessage.MessageStatusFailed,
		Sequence:              aiSeq,
		Visibility:            visibility,
		InResponseToMessageID: &humanMsg.ID,
		CreatedAt:             aiNow,
		UpdatedAt:             aiNow,
	}
	if private {
		// See the matching comment in SendAIMessage's successful-completion
		// path for why a private AI message records userID as its SenderID.
		aiMsg.SenderID = &userID
	}

	if err := u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}
	u.publishMessageEvent(ctx, event.RoomEvent{
		Type:               event.EventMessageCreated,
		RoomID:             roomID,
		Message:            aiMsg,
		TargetUserIDs:      targetUserIDsForVisibility(aiMsg),
		OccurredAt:         aiNow,
		UsedContextSummary: usedSummary,
	})
	return aiMsg, nil
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
	// Reject regeneration while the existing AI response is still
	// mid-stream (Step 54): its content is a partial, still-growing
	// accumulation of token_chunk deltas, so overwriting it now would race
	// the streaming writer and could leave the message in a corrupted,
	// half-overwritten state. The caller should wait for the stream to
	// finalize (status moves to completed/failed) before retrying.
	if nextMsg.Status == domainmessage.MessageStatusStreaming {
		return nil, false, domain.ErrConflict
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

	u.publishMessageEvent(ctx, event.RoomEvent{
		Type:               event.EventMessageUpdated,
		RoomID:             roomID,
		Message:            nextMsg,
		TargetUserIDs:      targetUserIDsForVisibility(nextMsg),
		OccurredAt:         now,
		UsedContextSummary: usedSummary,
	})

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
	if err := room.Authorize(member.Role, room.ActionInvokeAI); err != nil {
		return nil, err
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
// but I'm not a member of it" via the returned error. See
// room.GetMemberOrForbidden (shared with usecase/room and usecase/invitation,
// which each keep this same thin wrapper).
func (u *MessageUsecase) getMember(ctx context.Context, roomID, userID string) (*room.RoomMember, error) {
	return room.GetMemberOrForbidden(ctx, u.roomRepo, roomID, userID)
}
