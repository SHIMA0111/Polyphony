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
//
// closeOnce guards the actual removal-and-close so that the two independent
// callers who can each trigger it — the unsubscribe function Subscribe
// returns, and Revoke closing this same subscription from the outside — can
// race harmlessly: whichever runs first performs the removal/close, and the
// other's call becomes a no-op instead of double-closing sub.ch (which would
// panic).
type subscriber struct {
	userID    string
	ch        chan RoomEvent
	closeOnce sync.Once
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
// mu is a RWMutex rather than a plain Mutex so that Publish can hold a read
// lock across its entire snapshot-and-send loop: this is what prevents the
// send-after-close race described on Publish's doc comment, at the cost of
// serializing Publish with Subscribe/unsubscribe (never with other
// concurrent Publish calls, which only need read access).
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
// Publish holds the hub's read lock for the entire snapshot-and-send loop
// (not just the snapshot) so that it can never race with an in-flight
// unsubscribe: without this, Publish could read the subscriber list, have
// unsubscribe concurrently remove and close that subscriber's channel, and
// then send on the now-closed channel, panicking. Holding the read lock
// throughout still allows unlimited concurrent Publish calls (RWMutex
// readers don't block each other) and only serializes against
// Subscribe/unsubscribe, which are comparatively rare.
func (h *InProcessHub) Publish(_ context.Context, event RoomEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var targets map[string]struct{}
	if len(event.TargetUserIDs) > 0 {
		targets = make(map[string]struct{}, len(event.TargetUserIDs))
		for _, id := range event.TargetUserIDs {
			targets[id] = struct{}{}
		}
	}

	for _, sub := range h.subs[event.RoomID] {
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
// channel. It is safe to call more than once — only the first call has any
// effect — so callers may unconditionally defer it without needing to track
// whether they already called it elsewhere; the same guard also makes it
// safe to race against Revoke closing this same subscription from the
// outside (see removeSubscriber's doc comment for why the locking is
// structured the way it is).
func (h *InProcessHub) Subscribe(_ context.Context, roomID, userID string) (<-chan RoomEvent, func()) {
	sub := &subscriber{
		userID: userID,
		ch:     make(chan RoomEvent, subscriberChannelCapacity),
	}

	h.mu.Lock()
	h.subs[roomID] = append(h.subs[roomID], sub)
	h.mu.Unlock()

	unsubscribe := func() {
		sub.closeOnce.Do(func() {
			h.removeSubscriber(roomID, sub)
		})
	}

	return sub.ch, unsubscribe
}

// removeSubscriber removes sub from roomID's subscriber list and closes its
// channel, acquiring the write lock itself.
//
// It is always called from inside sub.closeOnce.Do (by both unsubscribe and
// Revoke), and deliberately does not expect the lock to already be held:
// the two closeOnce.Do call sites run concurrently from independent
// goroutines with no shared lock between them, and sync.Once.Do blocks every
// caller until the winning call's function returns. If Revoke instead held
// h.mu for the duration of its own closeOnce.Do call (as an earlier version
// of this method did), a concurrent unsubscribe call that raced to become
// the Once's winner would block forever trying to acquire h.mu from inside
// that same Do call — which Revoke, itself blocked waiting for that same Do
// call to return, would never release. Keeping the lock acquisition inside
// the Once-guarded function itself (here) instead of around the Do call
// avoids that self-deadlock: whichever of Revoke/unsubscribe wins the race
// acquires h.mu, does the removal, and releases it before Do returns to
// either caller. The channel is still closed under the write lock (see
// Publish's doc comment) so no concurrent Publish call can ever observe a
// stale reference to this subscriber after its channel has been closed.
func (h *InProcessHub) removeSubscriber(roomID string, sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()

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
}

// Revoke closes every open subscription userID currently holds on roomID.
// Matching subscribers are snapshotted under the hub's read lock (so the
// snapshot itself can never race with a concurrent Subscribe/unsubscribe
// mutating the slice), then each match is removed and closed via
// removeSubscriber, guarded by the same sub.closeOnce its own unsubscribe
// function uses — see removeSubscriber's doc comment for why the lock
// acquisition must happen inside that guarded call rather than around it.
// It is a no-op if userID has no open subscription on roomID.
//
// A Subscribe call for (roomID, userID) that lands after this snapshot but
// before Revoke returns is not seen by this call and so is not revoked; this
// mirrors the same narrow window every caller of LeaveRoom already
// tolerates (a concurrent Subscribe attempt for a user whose membership was
// just removed will itself be rejected at the membership check that
// precedes hub.Subscribe in the WebSocket handler, in all but the most
// improbable interleavings).
func (h *InProcessHub) Revoke(_ context.Context, roomID, userID string) {
	h.mu.RLock()
	var matches []*subscriber
	for _, sub := range h.subs[roomID] {
		if sub.userID == userID {
			matches = append(matches, sub)
		}
	}
	h.mu.RUnlock()

	for _, sub := range matches {
		sub.closeOnce.Do(func() {
			h.removeSubscriber(roomID, sub)
		})
	}
}
