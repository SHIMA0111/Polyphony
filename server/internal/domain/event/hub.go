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
)

// RoomEvent describes a single piece of room activity to broadcast to
// subscribers. Message holds the full domain message (not a re-serialized
// DTO); domain/event importing domain/message is a domain-to-domain
// dependency and does not violate the Clean Architecture dependency rule,
// which only forbids inner layers importing outer ones.
type RoomEvent struct {
	// Type identifies what kind of event this is.
	Type EventType

	// RoomID is the room the event occurred in.
	RoomID string

	// Message is the message that was created or updated.
	Message *message.Message

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
}
