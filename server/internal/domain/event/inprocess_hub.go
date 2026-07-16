package event

import (
	"context"
	"log/slog"
	"sync"
)

// subscriberChannelCapacity is the buffer size allocated for each
// subscriber's channel. A small buffer absorbs brief bursts (e.g. a message
// and its immediate AI response) without requiring Publish to block; once
// full, Publish drops the event for that subscriber rather than waiting.
const subscriberChannelCapacity = 16

// subscriber holds one registered listener for a room: the user it belongs
// to (used for TargetUserIDs filtering) and the channel events are sent on.
type subscriber struct {
	userID string
	ch     chan RoomEvent
}

// InProcessHub is the initial, in-memory implementation of MessageHub. It
// keeps a registry of subscribers per room guarded by a mutex and delivers
// events via non-blocking buffered sends, so a slow or stuck subscriber can
// never block message persistence.
//
// InProcessHub only fans out within a single process; it does not survive
// restarts and does not coordinate across multiple API server instances. Per
// CLAUDE.md's Interface Swap Points table, a Redis-backed MessageHub
// implementation providing multi-instance fan-out is expected in a later
// phase.
//
// The zero value is not usable; construct with NewInProcessHub.
//
// mu is a sync.RWMutex rather than a plain Mutex because Publish must hold the
// lock across its entire send loop (not just while snapshotting the
// subscriber list) to prevent a send-on-closed-channel race against
// unsubscribe: unsubscribe removes the subscriber from subs and closes its
// channel atomically under the write lock, so Publish can never observe a
// subscriber that is concurrently being torn down. RWMutex lets concurrent
// Publish calls (and readers in general) still run in parallel while holding
// the lock for the duration of the send loop.
type InProcessHub struct {
	mu   sync.RWMutex
	subs map[string][]*subscriber // roomID -> subscribers
}

// NewInProcessHub creates a ready-to-use InProcessHub.
func NewInProcessHub() *InProcessHub {
	return &InProcessHub{
		subs: make(map[string][]*subscriber),
	}
}

// Publish broadcasts event to every subscriber of event.RoomID, filtered by
// event.TargetUserIDs when non-empty. Delivery to each subscriber is a
// non-blocking buffered channel send: if a subscriber's channel is full, the
// event is dropped for that subscriber and a warning is logged, rather than
// blocking the caller. Publish never returns an error and never blocks on
// slow subscribers, which is what guarantees that broadcasting can never
// roll back or fail the write that produced the event.
//
// Publish holds the read lock across both the subscriber snapshot and the
// entire send loop below, not just the snapshot. unsubscribe only closes a
// subscriber's channel while holding the write lock, so as long as Publish
// holds the read lock for its full duration, it can never observe (and send
// on) a channel that unsubscribe has closed or is in the process of closing.
func (h *InProcessHub) Publish(_ context.Context, event RoomEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs := h.subs[event.RoomID]

	var targets map[string]struct{}
	if len(event.TargetUserIDs) > 0 {
		targets = make(map[string]struct{}, len(event.TargetUserIDs))
		for _, id := range event.TargetUserIDs {
			targets[id] = struct{}{}
		}
	}

	for _, sub := range subs {
		if targets != nil {
			if _, ok := targets[sub.userID]; !ok {
				continue
			}
		}

		select {
		case sub.ch <- event:
		default:
			slog.Warn("event: dropping event for subscriber with full channel",
				"room_id", event.RoomID, "user_id", sub.userID, "event_type", event.Type)
		}
	}
}

// Subscribe registers userID as a listener for events in roomID. It
// allocates a buffered channel (capacity subscriberChannelCapacity),
// registers it under roomID, and returns the receive end of the channel plus
// an unsubscribe function.
//
// The returned unsubscribe function removes the subscription and closes the
// channel, both while holding the write lock, so a concurrent Publish (which
// holds the read lock for its entire send loop, see Publish) can never send
// on a channel that has already been, or is concurrently being, closed. It is
// safe to call more than once — only the first call has any effect — so
// callers may unconditionally defer it without needing to track whether they
// already called it elsewhere.
func (h *InProcessHub) Subscribe(_ context.Context, roomID, userID string) (<-chan RoomEvent, func()) {
	sub := &subscriber{
		userID: userID,
		ch:     make(chan RoomEvent, subscriberChannelCapacity),
	}

	h.mu.Lock()
	h.subs[roomID] = append(h.subs[roomID], sub)
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			list := h.subs[roomID]
			for i, s := range list {
				if s == sub {
					h.subs[roomID] = append(list[:i], list[i+1:]...)
					break
				}
			}
			if len(h.subs[roomID]) == 0 {
				delete(h.subs, roomID)
			}
			close(sub.ch)
			h.mu.Unlock()
		})
	}

	return sub.ch, unsubscribe
}
