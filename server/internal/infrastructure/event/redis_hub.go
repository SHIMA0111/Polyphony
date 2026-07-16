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
// local tracks every subscription this specific process instance has open,
// keyed by roomID, purely so Revoke can find and close them — it plays the
// same role InProcessHub.subs plays, but here it is a bookkeeping side
// registry rather than the thing Publish reads from (Publish/Subscribe still
// go entirely through Redis Pub/Sub; local is never consulted for delivery).
//
// The zero value is not usable; construct with NewRedisHub.
type RedisHub struct {
	client *redis.Client

	mu    sync.Mutex
	local map[string][]*localSubscription // roomID -> this instance's subscriptions
}

// localSubscription is one Subscribe call's bookkeeping entry in
// RedisHub.local: which user it belongs to, and how to tear it down.
type localSubscription struct {
	userID    string
	unsub     func()
	closeOnce *sync.Once
}

// NewRedisHub creates a RedisHub backed by the given *redis.Client. The
// client is shared, not owned: callers (typically the DI container) remain
// responsible for closing it, since it may also be reused by other
// Redis-backed components (e.g. the rate limiter and session cache added in
// a later step).
func NewRedisHub(client *redis.Client) *RedisHub {
	return &RedisHub{
		client: client,
		local:  make(map[string][]*localSubscription),
	}
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
// (which stops and closes its delivery channel, ending the goroutine),
// closes the returned output channel, and removes this subscription's entry
// from RedisHub.local (see that field's doc comment). It is safe to call
// more than once — only the first call has any effect, via sync.Once —
// mirroring InProcessHub's unsubscribe contract so callers can
// unconditionally defer it. The same sync.Once also guards Revoke closing
// this subscription from the outside, so the two can never race into a
// double-close.
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

	local := &localSubscription{userID: userID, closeOnce: &sync.Once{}}
	local.unsub = func() {
		local.closeOnce.Do(func() {
			if err := pubsub.Close(); err != nil {
				slog.Warn("redis event hub: close pubsub failed",
					"room_id", roomID, "user_id", userID, "error", err)
			}
			h.removeLocal(roomID, local)
		})
	}

	h.mu.Lock()
	h.local[roomID] = append(h.local[roomID], local)
	h.mu.Unlock()

	return out, local.unsub
}

// removeLocal removes local from RedisHub.local's entry for roomID.
func (h *RedisHub) removeLocal(roomID string, local *localSubscription) {
	h.mu.Lock()
	defer h.mu.Unlock()

	list := h.local[roomID]
	for i, l := range list {
		if l == local {
			h.local[roomID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.local[roomID]) == 0 {
		delete(h.local, roomID)
	}
}

// Revoke closes every subscription userID currently holds on roomID that
// was opened via Subscribe on this same RedisHub instance (i.e. this API
// server process/replica).
//
// Unlike InProcessHub, whose registry is the only place any process holds a
// room's subscribers, RedisHub's Publish/Subscribe fan-out goes entirely
// through Redis: a WebSocket connection for userID could just as easily be
// held open by a different API server replica, whose RedisHub instance has
// its own independent, unshared local registry. This Revoke call has no way
// to reach that other replica's in-memory state, so it cannot close a
// subscription it does not itself own. Closing this out-of-scope for the
// current step requires either a dedicated Redis Pub/Sub "revocation"
// channel every RedisHub instance also subscribes to, or moving subscriber
// bookkeeping into Redis itself — until one of those lands, a removed
// member's WebSocket connection on another replica keeps receiving events
// until it is closed for some other reason (e.g. the client reconnects and
// the room-membership check on the next request rejects it).
func (h *RedisHub) Revoke(_ context.Context, roomID, userID string) {
	h.mu.Lock()
	var matches []*localSubscription
	for _, local := range h.local[roomID] {
		if local.userID == userID {
			matches = append(matches, local)
		}
	}
	h.mu.Unlock()

	for _, local := range matches {
		local.unsub()
	}
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
