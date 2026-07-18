// Package event provides infrastructure-layer implementations of the
// domain/event.MessageHub port that have external dependencies and therefore
// cannot live alongside the port itself (see domain/event's InProcessHub,
// which is dependency-free and stays in the domain package as a deliberate,
// narrow exception).
//
// RedisHub (this file) is the Phase 10 swap-point implementation: a
// Redis Pub/Sub-backed MessageHub that fans events out to every API server
// process subscribed to a room's channel, rather than only to subscribers
// held in the local process's memory. Per CLAUDE.md's Interface Swap Points
// table, it is selected by the MESSAGE_HUB_DRIVER=redis config value and must
// satisfy domain/event.MessageHub without requiring any change to
// MessageUsecase, the WebSocket handler, or any other caller.
package event

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
)

// roomChannel returns the Redis Pub/Sub channel name used for a given room.
// Every RedisHub instance (i.e. every API server replica) publishes to and
// subscribes on this same channel name for a given roomID, which is what
// makes cross-instance delivery work: Redis, not any single process, is the
// fan-out point.
func roomChannel(roomID string) string {
	return "room:" + roomID
}

// RedisHub is a Redis Pub/Sub-backed implementation of domain/event.MessageHub.
// It preserves InProcessHub's exact per-room and per-user delivery semantics
// (RoomEvent.TargetUserIDs filtering) but fans events out through Redis
// instead of an in-memory registry, so a message published by one API server
// replica is delivered to a WebSocket connection held open by a different
// replica.
//
// Because Redis Pub/Sub has no server-side per-consumer filtering, every
// subscribed replica receives every event published on a room's channel;
// RedisHub applies the TargetUserIDs filter client-side, inside the
// goroutine each Subscribe call spawns, exactly mirroring how InProcessHub
// filters before delivering to each in-memory subscriber.
//
// The zero value is not usable; construct with NewRedisHub.
type RedisHub struct {
	client *redis.Client
}

// NewRedisHub creates a RedisHub backed by the given *redis.Client. The
// client is shared, not owned: callers (typically the DI container) remain
// responsible for closing it, since it may also be reused by other
// Redis-backed components (e.g. the rate limiter and session cache added in
// a later step).
func NewRedisHub(client *redis.Client) *RedisHub {
	return &RedisHub{client: client}
}

// Publish serializes event as JSON and publishes it on the Redis channel for
// event.RoomID. Per the domain/event.MessageHub contract, Publish never
// fails the caller: if serialization or the Redis PUBLISH command errors
// (e.g. the connection to Redis is down), the error is logged via slog and
// swallowed rather than returned, since the write that produced this event
// has already completed and must not be rolled back or reported as failed on
// account of a best-effort broadcast.
//
// TargetUserIDs filtering is not applied here: the full event (including
// TargetUserIDs) is published unfiltered, and each Subscribe goroutine
// filters on the receiving side, since Redis Pub/Sub cannot filter per
// consumer.
func (h *RedisHub) Publish(ctx context.Context, ev event.RoomEvent) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.Error("redis event hub: marshal event failed",
			"room_id", ev.RoomID, "event_type", ev.Type, "error", err)
		return
	}

	if err := h.client.Publish(ctx, roomChannel(ev.RoomID), payload).Err(); err != nil {
		slog.Error("redis event hub: publish failed",
			"room_id", ev.RoomID, "event_type", ev.Type, "error", err)
	}
}

// Subscribe opens a dedicated *redis.PubSub subscription on roomID's Redis
// channel and spawns a goroutine that reads every message published to it,
// unmarshals it into a RoomEvent, and forwards it to the returned channel
// only if the event is untargeted (TargetUserIDs is empty) or userID appears
// in TargetUserIDs — mirroring InProcessHub's Publish-time filtering, just
// performed on the receiving side since Redis Pub/Sub has no server-side
// per-consumer filtering.
//
// A message that fails to unmarshal as a RoomEvent (which should not happen
// for events published by this same package, but could occur if an
// incompatible producer ever writes to the same channel) is logged and
// skipped rather than crashing the subscriber goroutine.
//
// The returned unsubscribe function closes the underlying *redis.PubSub
// (which stops and closes its delivery channel, ending the goroutine) and
// then closes the returned output channel. It is safe to call more than
// once — only the first call has any effect, via sync.Once — mirroring
// InProcessHub's unsubscribe contract so callers can unconditionally defer
// it.
//
// If the underlying Redis connection is lost, the goroutine's read from
// pubsub.Channel() ends (the channel is closed by the go-redis client), so
// the output channel is closed and the caller observes end-of-stream; no
// error is surfaced through the channel itself, matching MessageHub's
// no-error Subscribe contract described in domain/event.
func (h *RedisHub) Subscribe(ctx context.Context, roomID, userID string) (<-chan event.RoomEvent, func()) {
	pubsub := h.client.Subscribe(ctx, roomChannel(roomID))

	out := make(chan event.RoomEvent, subscriberChannelCapacity)

	go func() {
		defer close(out)

		for msg := range pubsub.Channel() {
			var ev event.RoomEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				slog.Error("redis event hub: unmarshal event failed",
					"room_id", roomID, "user_id", userID, "error", err)
				continue
			}

			if len(ev.TargetUserIDs) > 0 && !containsUserID(ev.TargetUserIDs, userID) {
				continue
			}

			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			if err := pubsub.Close(); err != nil {
				slog.Warn("redis event hub: close pubsub failed",
					"room_id", roomID, "user_id", userID, "error", err)
			}
		})
	}

	return out, unsubscribe
}

// subscriberChannelCapacity is the buffer size allocated for each
// subscriber's output channel. It mirrors domain/event.InProcessHub's buffer
// (see inprocess_hub.go): a small buffer absorbs brief bursts without making
// the forwarding goroutine block indefinitely, though unlike InProcessHub's
// non-blocking send, RedisHub blocks on a full channel (bounded by ctx.Done)
// rather than dropping, since the forwarding goroutine has no separate
// caller to avoid blocking.
const subscriberChannelCapacity = 16

// containsUserID reports whether ids contains userID.
func containsUserID(ids []string, userID string) bool {
	for _, id := range ids {
		if id == userID {
			return true
		}
	}
	return false
}
