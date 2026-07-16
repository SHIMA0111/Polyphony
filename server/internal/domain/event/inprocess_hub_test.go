package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
)

// waitFor blocks until ch receives a value or the timeout elapses, failing
// the test in the latter case.
func waitForEvent(t *testing.T, ch <-chan RoomEvent) RoomEvent {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return RoomEvent{}
	}
}

func TestInProcessHubPublishNoSubscribersDoesNotBlockOrPanic(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Publish(ctx, RoomEvent{
			Type:       EventMessageCreated,
			RoomID:     "room-1",
			Message:    &message.Message{ID: "msg-1"},
			OccurredAt: time.Now(),
		})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked with no subscribers")
	}
}

func TestInProcessHubSubscriberReceivesEvent(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	ch, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	want := RoomEvent{
		Type:       EventMessageCreated,
		RoomID:     "room-1",
		Message:    &message.Message{ID: "msg-1"},
		OccurredAt: time.Now(),
	}
	hub.Publish(ctx, want)

	got := waitForEvent(t, ch)
	if got.Type != want.Type || got.RoomID != want.RoomID || got.Message.ID != want.Message.ID {
		t.Fatalf("received event %+v, want %+v", got, want)
	}
}

// TestInProcessHubDeliversTokenChunkEvents asserts an EventTokenChunk event
// (a non-nil Chunk, nil Message -- Step 51's AI streaming payload shape) is
// delivered/filtered identically to the existing message_created/
// message_updated event types: InProcessHub itself is agnostic to
// RoomEvent's payload shape and needs no special-casing.
func TestInProcessHubDeliversTokenChunkEvents(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	ch, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	want := RoomEvent{
		Type:   EventTokenChunk,
		RoomID: "room-1",
		Chunk: &StreamChunkEvent{
			MessageID:   "ai-msg-1",
			Delta:       "Hel",
			SummaryUsed: true,
		},
		OccurredAt: time.Now(),
	}
	hub.Publish(ctx, want)

	got := waitForEvent(t, ch)
	if got.Type != EventTokenChunk {
		t.Fatalf("expected type %q, got %q", EventTokenChunk, got.Type)
	}
	if got.Message != nil {
		t.Fatalf("expected nil Message on a token_chunk event, got %+v", got.Message)
	}
	if got.Chunk == nil || got.Chunk.MessageID != "ai-msg-1" || got.Chunk.Delta != "Hel" || !got.Chunk.SummaryUsed {
		t.Fatalf("unexpected Chunk payload: %+v", got.Chunk)
	}
}

func TestInProcessHubTargetUserIDsFiltersDelivery(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	matchCh, unsubMatch := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubMatch()
	otherCh, unsubOther := hub.Subscribe(ctx, "room-1", "user-2")
	defer unsubOther()

	hub.Publish(ctx, RoomEvent{
		Type:          EventMessageUpdated,
		RoomID:        "room-1",
		Message:       &message.Message{ID: "msg-1"},
		TargetUserIDs: []string{"user-1"},
		OccurredAt:    time.Now(),
	})

	waitForEvent(t, matchCh)

	select {
	case evt := <-otherCh:
		t.Fatalf("expected no event delivered to non-target subscriber, got %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// expected: no delivery
	}
}

func TestInProcessHubUnsubscribeStopsDeliveryAndIsSafeToCallOnce(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	ch, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	unsubscribe()

	// The channel must be closed by unsubscribe.
	if _, ok := <-ch; ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}

	// Publishing after unsubscribe must not panic or deliver anything.
	hub.Publish(ctx, RoomEvent{
		Type:       EventMessageCreated,
		RoomID:     "room-1",
		Message:    &message.Message{ID: "msg-2"},
		OccurredAt: time.Now(),
	})

	// Calling unsubscribe again must be safe (no panic on double-close).
	unsubscribe()
}

// TestInProcessHubConcurrentPublishAndUnsubscribeDoesNotRace exercises the
// send-after-close race Publish and Subscribe's doc comments describe:
// concurrent Publish and unsubscribe calls hammering the same room/subscriber
// must never panic ("send on closed channel") and must never data-race on
// the shared subscriber list. Run with -race to catch either failure mode.
//
// Publishers run continuously in the background, coordinated by a stop
// channel that is only closed once every subscribe/unsubscribe iteration
// below has finished, so publishing stays live for the whole test instead of
// a single Publish call per iteration that could complete before its
// corresponding unsubscribe and leave later iterations racing against
// nothing.
func TestInProcessHubConcurrentPublishAndUnsubscribeDoesNotRace(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	const iterations = 200
	const publishers = 4

	stop := make(chan struct{})
	var publishWG sync.WaitGroup

	// Publishers continuously broadcast events to room-race until stop is
	// closed below.
	for p := 0; p < publishers; p++ {
		publishWG.Add(1)
		go func() {
			defer publishWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
					hub.Publish(ctx, RoomEvent{
						Type:       EventMessageCreated,
						RoomID:     "room-race",
						Message:    &message.Message{ID: "msg-race"},
						OccurredAt: time.Now(),
					})
				}
			}
		}()
	}

	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		ch, unsubscribe := hub.Subscribe(ctx, "room-race", "user-race")

		wg.Add(2)
		go func() {
			defer wg.Done()
			unsubscribe()
		}()
		go func() {
			defer wg.Done()
			// Drain concurrently with unsubscribe and the background
			// publishers above so a send racing a close also has a
			// reader on the other end.
			select {
			case <-ch:
			case <-time.After(100 * time.Millisecond):
			}
		}()
	}

	wg.Wait()
	close(stop)
	publishWG.Wait()
}

// TestInProcessHubConcurrentRevokeAndUnsubscribeDoesNotRace mirrors
// TestInProcessHubConcurrentPublishAndUnsubscribeDoesNotRace above, but races
// Revoke against the subscriber's own unsubscribe function instead of racing
// Publish against unsubscribe: both can independently trigger the same
// close, and subscriber.closeOnce is what must make that safe regardless of
// which one wins. Run with -race to catch either a double-close panic or a
// data race on the subscriber registry.
func TestInProcessHubConcurrentRevokeAndUnsubscribeDoesNotRace(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	// No background Publish load here (unlike
	// TestInProcessHubConcurrentPublishAndUnsubscribeDoesNotRace above): that
	// test already covers Publish racing unsubscribe under the hub's RWMutex.
	// This test isolates the race this change actually adds — Revoke racing
	// the subscriber's own unsubscribe function on the same subscription —
	// which subscriber.closeOnce guards independently of Publish.
	const iterations = 200

	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		ch, unsubscribe := hub.Subscribe(ctx, "room-revoke-race", "user-race")

		wg.Add(3)
		go func() {
			defer wg.Done()
			unsubscribe()
		}()
		go func() {
			defer wg.Done()
			hub.Revoke(ctx, "room-revoke-race", "user-race")
		}()
		go func() {
			defer wg.Done()
			select {
			case <-ch: // closed by whichever of the above runs first
			case <-time.After(time.Second):
				t.Error("expected channel to be closed by unsubscribe or Revoke within 1s")
			}
		}()
	}

	wg.Wait()
}

// TestInProcessHubRevokeClosesOnlyTargetUsersSubscriptions proves that
// Revoke closes every subscription belonging to the given (roomID, userID)
// pair — including more than one, e.g. multiple browser tabs open on the
// same room — while leaving other users' subscriptions on the same room,
// and the same user's subscriptions on other rooms, untouched.
func TestInProcessHubRevokeClosesOnlyTargetUsersSubscriptions(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	revokedCh1, _ := hub.Subscribe(ctx, "room-1", "user-1")
	revokedCh2, _ := hub.Subscribe(ctx, "room-1", "user-1") // second tab, same user/room
	otherUserCh, unsubOtherUser := hub.Subscribe(ctx, "room-1", "user-2")
	defer unsubOtherUser()
	otherRoomCh, unsubOtherRoom := hub.Subscribe(ctx, "room-2", "user-1")
	defer unsubOtherRoom()

	hub.Revoke(ctx, "room-1", "user-1")

	if _, ok := <-revokedCh1; ok {
		t.Fatal("expected first revoked subscription's channel to be closed")
	}
	if _, ok := <-revokedCh2; ok {
		t.Fatal("expected second revoked subscription's channel to be closed")
	}

	// Confirm the untouched subscriptions still work end-to-end.
	hub.Publish(ctx, RoomEvent{Type: EventMessageCreated, RoomID: "room-1", Message: &message.Message{ID: "m1"}, OccurredAt: time.Now()})
	waitForEvent(t, otherUserCh)
	hub.Publish(ctx, RoomEvent{Type: EventMessageCreated, RoomID: "room-2", Message: &message.Message{ID: "m2"}, OccurredAt: time.Now()})
	waitForEvent(t, otherRoomCh)
}

// TestInProcessHubRevokeThenUnsubscribeIsSafe proves that calling the
// subscriber's own unsubscribe function after Revoke has already closed its
// channel is a safe no-op (no double-close panic) — the two share the same
// underlying guard (subscriber.closeOnce).
func TestInProcessHubRevokeThenUnsubscribeIsSafe(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	ch, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")

	hub.Revoke(ctx, "room-1", "user-1")
	if _, ok := <-ch; ok {
		t.Fatal("expected channel to be closed by Revoke")
	}

	// Must not panic.
	unsubscribe()
}

// TestInProcessHubUnsubscribeThenRevokeIsSafe proves the reverse ordering:
// calling Revoke after the caller already unsubscribed on their own must not
// panic or affect any other subscriber, since the matching subscriber is
// already gone from the registry by the time Revoke runs.
func TestInProcessHubUnsubscribeThenRevokeIsSafe(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	_, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	unsubscribe()

	// Must not panic, even though the subscriber is already gone.
	hub.Revoke(ctx, "room-1", "user-1")
}

// TestInProcessHubRevokeNoSubscribersIsNoop proves Revoke on a room/user with
// no open subscription is a harmless no-op.
func TestInProcessHubRevokeNoSubscribersIsNoop(t *testing.T) {
	hub := NewInProcessHub()
	hub.Revoke(context.Background(), "room-1", "user-1")
}

func TestInProcessHubPublishNonBlockingOnFullChannel(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	ch, unsubscribe := hub.Subscribe(ctx, "room-1", "user-1")
	defer unsubscribe()

	// Fill the subscriber's buffer well past capacity without reading from
	// ch; Publish must never block even when the channel is full.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < subscriberChannelCapacity*2; i++ {
			hub.Publish(ctx, RoomEvent{
				Type:       EventMessageCreated,
				RoomID:     "room-1",
				Message:    &message.Message{ID: "msg-flood"},
				OccurredAt: time.Now(),
			})
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber channel")
	}

	// Drain to avoid leaking a goroutine expectation; channel should still
	// have buffered events up to capacity.
	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			if drained == 0 {
				t.Fatal("expected at least one buffered event to have been delivered")
			}
			return
		}
	}
}
