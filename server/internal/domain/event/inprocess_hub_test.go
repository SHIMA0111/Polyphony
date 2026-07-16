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

// TestInProcessHubConcurrentPublishAndUnsubscribeIsRaceFree is a regression
// test for a send-on-closed-channel race: Publish used to snapshot the
// subscriber slice under the lock but send to subscribers' channels *outside*
// the lock, while unsubscribe closed a subscriber's channel outside the lock
// too (after removing it from the registry). That let a Publish goroutine
// hold a reference to a subscriber whose channel unsubscribe had already
// closed, so `sub.ch <- event` could panic with "send on closed channel".
//
// This test hammers Publish concurrently with repeated Subscribe/unsubscribe
// cycles on the same room so that race is likely to manifest under `go test
// -race`. Publishers run continuously, coordinated by a stop channel that is
// only closed once every subscribe/unsubscribe cycle below has finished, so
// the race window spans the whole test instead of just its earliest cycles —
// a fixed publish count could otherwise finish in milliseconds while later
// subscriber cycles ran with no concurrent publishing at all. It must be
// run with -race to be meaningful (see the `test:
// go test -race ./internal/domain/event/...` verification step) — without
// -race a panic may still occur but is less reliably triggered.
func TestInProcessHubConcurrentPublishAndUnsubscribeIsRaceFree(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()
	const roomID = "room-race"

	const publishers = 4
	const subscribeCycles = 8

	stop := make(chan struct{})
	var publishWG sync.WaitGroup

	// Publishers continuously broadcast events to roomID until stop is
	// closed below, so publishing stays live for the full duration of the
	// subscribe/unsubscribe cycles rather than racing only their early
	// iterations.
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
						RoomID:     roomID,
						Message:    &message.Message{ID: "msg-race"},
						OccurredAt: time.Now(),
					})
				}
			}
		}()
	}

	// Subscribers repeatedly subscribe, receive a couple of events (if any
	// arrive before they unsubscribe), and unsubscribe, racing against the
	// publishers above and against each other.
	var subWG sync.WaitGroup
	for s := 0; s < subscribeCycles; s++ {
		subWG.Add(1)
		go func(userID string) {
			defer subWG.Done()
			for i := 0; i < subscribeCycles; i++ {
				ch, unsubscribe := hub.Subscribe(ctx, roomID, userID)

				// Drain whatever happens to be available without blocking;
				// the point of this test is the concurrent teardown, not
				// delivery guarantees.
				select {
				case <-ch:
				case <-time.After(time.Millisecond):
				}

				unsubscribe()
			}
		}(userIDForCycle(s))
	}

	done := make(chan struct{})
	go func() {
		subWG.Wait()
		close(stop)
		publishWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for concurrent Publish/Subscribe/unsubscribe cycles")
	}
}

// userIDForCycle generates a distinct subscriber userID per subscribe-cycle
// goroutine index so TestInProcessHubConcurrentPublishAndUnsubscribeIsRaceFree
// exercises multiple independent subscribers rather than repeatedly
// subscribing the same identity.
func userIDForCycle(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	return "user-race-" + string(letters[i%len(letters)])
}
