// Package message defines the chat message entity, its repository port, and cursor pagination types.
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
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// attachmentViewURLExpiry is how long a presigned view URL minted for an
// AI-context image part remains valid. Matches
// usecase/attachment.viewURLExpiry: the URL only needs to survive the single
// LLM Gateway request it is embedded in.
const attachmentViewURLExpiry = time.Hour

const defaultContextMessages = 50

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
	if err := middleware.Authorize(member.Role, room.ActionSendMessage); err != nil {
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

	u.hub.Publish(ctx, event.RoomEvent{
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
	if err := middleware.Authorize(member.Role, room.ActionInvokeAI); err != nil {
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
	contextPage, err := u.msgRepo.ListByRoom(ctx, roomID, "", defaultContextMessages, userID)
	if err != nil {
		return nil, err
	}

	// Build chat messages (reverse to chronological order, filtering out
	// soft-deleted, exclude_from_ai, failed, and pre-cutoff messages),
	// summarizing older history behind a cache when the estimated context
	// would overflow model's resolved context window (Step 50).
	chatMsgs, usedSummary, err := u.assembleAIContext(ctx, rm, model, contextPage.Messages)
	if err != nil {
		return nil, err
	}

	// Call LLM Gateway
	completion, llmErr := u.llmGateway.Complete(ctx, &ai.CompletionRequest{
		Model:    model,
		Messages: chatMsgs,
	})

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

	if llmErr != nil {
		// Save failed placeholder so regenerate can update it later
		aiMsg.Content = ""
		aiMsg.Status = domainmessage.MessageStatusFailed
		if err := u.msgRepo.Create(ctx, aiMsg); err != nil {
			return nil, err
		}
		u.hub.Publish(ctx, event.RoomEvent{
			Type:               event.EventMessageCreated,
			RoomID:             roomID,
			Message:            aiMsg,
			TargetUserIDs:      targetUserIDsForVisibility(aiMsg),
			OccurredAt:         aiNow,
			UsedContextSummary: usedSummary,
		})
		return &SendAIResult{HumanMessage: humanMsg, AIMessage: aiMsg, UsedContextSummary: usedSummary}, nil
	}

	aiMsg.Content = completion.Content
	aiMsg.Status = domainmessage.MessageStatusCompleted

	if err = u.msgRepo.Create(ctx, aiMsg); err != nil {
		return nil, err
	}
	u.hub.Publish(ctx, event.RoomEvent{
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
	if err := middleware.Authorize(member.Role, room.ActionInvokeAI); err != nil {
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

	u.hub.Publish(ctx, event.RoomEvent{
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
	if err := middleware.Authorize(member.Role, room.ActionInvokeAI); err != nil {
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
