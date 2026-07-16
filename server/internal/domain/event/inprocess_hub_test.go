package event

import (
	"context"
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
