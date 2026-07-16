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
func TestInProcessHubConcurrentPublishAndUnsubscribeDoesNotRace(t *testing.T) {
	hub := NewInProcessHub()
	ctx := context.Background()

	const iterations = 200
	var wg sync.WaitGroup

	for i := 0; i < iterations; i++ {
		ch, unsubscribe := hub.Subscribe(ctx, "room-race", "user-race")

		wg.Add(3)
		go func() {
			defer wg.Done()
			hub.Publish(ctx, RoomEvent{
				Type:       EventMessageCreated,
				RoomID:     "room-race",
				Message:    &message.Message{ID: "msg-race"},
				OccurredAt: time.Now(),
			})
		}()
		go func() {
			defer wg.Done()
			unsubscribe()
		}()
		go func() {
			defer wg.Done()
			// Drain concurrently with the other two goroutines so a
			// send racing a close also has a reader on the other end.
			select {
			case <-ch:
			case <-time.After(100 * time.Millisecond):
			}
		}()
	}

	wg.Wait()
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
