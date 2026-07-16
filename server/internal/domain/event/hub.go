// Package event defines the messaging event port (MessageHub) consumed by
// MessageUsecase to broadcast room activity, along with its initial
// dependency-free adapter (InProcessHub, see inprocess_hub.go).
//
// Per the project's swap-point table (CLAUDE.md), MessageHub's initial
// implementation is InProcessHub and is expected to be swapped for a
// Redis-backed implementation in a later phase without any change to usecase
// code. Because InProcessHub has zero external dependencies (stdlib
// sync/channels only), it is kept alongside the port it implements rather
// than in a separate infrastructure package; this is a deliberate, minimal
// exception to strict Clean Architecture layering, not a general pattern to
// repeat for adapters with real external dependencies.
package event

import (
	"context"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
)

// EventType identifies the kind of room activity a RoomEvent describes.
type EventType string

const (
	// EventMessageCreated indicates a new message (human or AI) was
	// persisted in a room.
	EventMessageCreated EventType = "message_created"

	// EventMessageUpdated indicates an existing message (e.g. an AI
	// response updated in place by RegenerateAIMessage) was persisted in a
	// room.
	EventMessageUpdated EventType = "message_updated"

	// EventTokenChunk indicates an incremental delta of an in-progress AI
	// streaming response (MessageUsecase.SendAIMessageStream). Unlike
	// EventMessageCreated/EventMessageUpdated, this event type is not tied
	// to a persisted write: it is published once per chunk received from
	// the LLM Gateway's stream, well before the final persisted state is
	// known.
	EventTokenChunk EventType = "token_chunk"
)

// StreamChunkEvent carries an incremental AI streaming delta for an
// EventTokenChunk RoomEvent.
type StreamChunkEvent struct {
	// MessageID is the ID of the AI placeholder message this delta belongs
	// to. The placeholder is already persisted (with
	// message.MessageStatusStreaming) before streaming starts, so
	// subscribers can correlate every delta to the message they are
	// rendering.
	MessageID string

	// Delta is the incremental text produced by this chunk.
	Delta string

	// SummaryUsed reports whether the AI context sent to the LLM for this
	// stream folded older room history into a cached summary rather than
	// sending it verbatim (see the context-assembly step's usedSummary
	// return value). It is the same value on every chunk of a given stream.
	SummaryUsed bool
}

// RoomEvent describes a single piece of room activity to broadcast to
// subscribers. Message and Chunk are mutually exclusive payload fields,
// keyed by Type: Message holds the full domain message (not a re-serialized
// DTO) for EventMessageCreated/EventMessageUpdated, and is nil otherwise;
// Chunk holds an incremental streaming delta for EventTokenChunk, and is nil
// otherwise. domain/event importing domain/message is a domain-to-domain
// dependency and does not violate the Clean Architecture dependency rule,
// which only forbids inner layers importing outer ones.
type RoomEvent struct {
	// Type identifies what kind of event this is.
	Type EventType

	// RoomID is the room the event occurred in.
	RoomID string

	// Message is the message that was created or updated. Set only for
	// EventMessageCreated/EventMessageUpdated; see the RoomEvent doc comment
	// for the Message/Chunk mutual-exclusivity contract.
	Message *message.Message

	// Chunk is the incremental streaming delta this event carries. Set only
	// for EventTokenChunk; see the RoomEvent doc comment for the
	// Message/Chunk mutual-exclusivity contract.
	Chunk *StreamChunkEvent

	// TargetUserIDs restricts delivery to the given user IDs. A nil or
	// empty slice means "all subscribers of the room" — the common case for
	// message_created/message_updated, which every member should see.
	TargetUserIDs []string

	// OccurredAt is when the event occurred.
	OccurredAt time.Time

	// UsedContextSummary reports whether Message (when it is a
	// newly-produced AI message) was generated from a context that included
	// a cached/freshly-computed summary of older room history in place of
	// the raw messages it replaces (see
	// usecase/message.MessageUsecase.assembleAIContext, Step 50). It is
	// always false for a human message or any event that is not the direct
	// result of an AI invocation -- like Message, it is a one-time,
	// request-scoped signal describing how this particular AI response was
	// generated, not a persisted message property.
	UsedContextSummary bool
}

// MessageHub is the port through which MessageUsecase publishes room
// activity and (in later steps, e.g. the WebSocket endpoint) callers
// subscribe to it. Implementations must make Publish best-effort: it must
// never block message persistence and must never cause the write that
// triggered it to fail or roll back.
//
// The initial implementation is InProcessHub (see inprocess_hub.go); a
// Redis-backed implementation providing multi-instance fan-out is expected
// in a later phase (see CLAUDE.md's Interface Swap Points table) and must
// satisfy this same interface without requiring changes to callers.
type MessageHub interface {
	// Publish broadcasts event to every subscriber of event.RoomID (filtered
	// by TargetUserIDs when non-empty). It has no error return: publishing
	// is best-effort, so a delivery failure (e.g. a full subscriber buffer)
	// must never surface as an error to the caller, which has already
	// completed the write the event describes.
	Publish(ctx context.Context, event RoomEvent)

	// Subscribe registers userID as a listener for events in roomID and
	// returns a receive-only channel of RoomEvent plus an unsubscribe
	// function. Callers must invoke the returned function exactly once
	// (though implementations should tolerate repeated calls) when they are
	// done listening, to release the subscription and allow the channel to
	// be closed. The channel is closed when the unsubscribe function runs;
	// callers must stop reading from it at that point.
	Subscribe(ctx context.Context, roomID, userID string) (<-chan RoomEvent, func())

	// Revoke forcibly ends every open Subscribe subscription userID currently
	// holds on roomID, closing each subscription's channel exactly as if its
	// own unsubscribe function had been called. It is used when userID loses
	// access to roomID entirely (removed from the room, or leaving it) so a
	// live WebSocket connection cannot keep observing a room the caller is no
	// longer a member of — a role change alone does not warrant this (any
	// member may still view the room, so ChangeMemberRole never calls
	// Revoke; see usecase/room.RoomUsecase.ChangeMemberRole's doc comment).
	//
	// Revoke has no error return and must never block: like Publish, it is
	// best-effort with respect to callers, though unlike Publish its actual
	// effect (closing matching subscriptions) is synchronous within a single
	// process. It is a no-op if userID has no open subscription on roomID.
	//
	// RedisHub's implementation only closes subscriptions held open on the
	// same process (API server replica) that calls Revoke — see its doc
	// comment for why a WebSocket connection served by a different replica
	// is not affected by this call.
	Revoke(ctx context.Context, roomID, userID string)
}
