//go:build integration

package event

import (
	"context"
	"testing"
	"time"

	domainevent "github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
	testutilredis "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/redis"
)

// deliveryTimeout bounds how long the test waits for an event to arrive over
// a subscription channel before failing. It is generous relative to local
// Redis Pub/Sub latency (sub-millisecond) to stay robust in CI.
const deliveryTimeout = 5 * time.Second

// subscribeSettleDelay is how long the test waits after calling Subscribe
// before publishing. Subscribe returns as soon as the SUBSCRIBE command is
// issued, not once Redis has fully registered the subscription; without this
// delay, a publish immediately after Subscribe can race ahead of the
// subscription taking effect and be missed, which is a testcontainers-timing
// artifact rather than anything RedisHub itself needs to guarantee.
const subscribeSettleDelay = 200 * time.Millisecond

// newTestRedisHubPair starts a single disposable Redis testcontainer (via
// testutil/redis) and returns two independent *RedisHub instances (each with
// its own *redis.Client) pointed at it, simulating two API server replicas
// that share the same Redis instance for MessageHub fan-out.
func newTestRedisHubPair(ctx context.Context, t *testing.T) (a, b *RedisHub) {
	t.Helper()

	connStr := testutilredis.NewConnString(ctx, t)
	clientA := testutilredis.NewClient(ctx, t, connStr)
	clientB := testutilredis.NewClient(ctx, t, connStr)

	return NewRedisHub(clientA), NewRedisHub(clientB)
}

// TestRedisHubCrossInstanceDelivery proves the core Step 31 guarantee: an
// event published through one RedisHub instance (simulating one API server
// replica) is delivered to a subscription registered on a different RedisHub
// instance (simulating another replica), both backed by the same Redis Pub/
// Sub deployment. This is exactly the cross-instance delivery InProcessHub
// cannot provide.
func TestRedisHubCrossInstanceDelivery(t *testing.T) {
	ctx := context.Background()
	replicaA, replicaB := newTestRedisHubPair(ctx, t)

	const roomID = "room-cross-instance"
	const userID = "user-1"

	ch, unsubscribe := replicaA.Subscribe(ctx, roomID, userID)
	defer unsubscribe()

	time.Sleep(subscribeSettleDelay)

	published := domainevent.RoomEvent{
		Type:       domainevent.EventMessageCreated,
		RoomID:     roomID,
		OccurredAt: time.Now(),
	}
	replicaB.Publish(ctx, published)

	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before delivering the published event")
		}
		if got.RoomID != roomID {
			t.Errorf("expected RoomID %q, got %q", roomID, got.RoomID)
		}
		if got.Type != domainevent.EventMessageCreated {
			t.Errorf("expected Type %q, got %q", domainevent.EventMessageCreated, got.Type)
		}
	case <-time.After(deliveryTimeout):
		t.Fatal("timed out waiting for cross-instance event delivery")
	}
}

// TestRedisHubPerUserTargeting proves that RedisHub preserves
// InProcessHub's per-user (TargetUserIDs) filtering semantics even though
// filtering happens client-side (Redis Pub/Sub delivers every message on a
// channel to every subscriber): a subscription for a user listed in
// TargetUserIDs receives the event, while a differently-targeted
// subscription on the very same room does not.
func TestRedisHubPerUserTargeting(t *testing.T) {
	ctx := context.Background()
	replicaA, replicaB := newTestRedisHubPair(ctx, t)

	const roomID = "room-targeting"
	const targetUserID = "user-target"
	const otherUserID = "user-other"

	targetCh, unsubTarget := replicaA.Subscribe(ctx, roomID, targetUserID)
	defer unsubTarget()
	otherCh, unsubOther := replicaA.Subscribe(ctx, roomID, otherUserID)
	defer unsubOther()

	time.Sleep(subscribeSettleDelay)

	replicaB.Publish(ctx, domainevent.RoomEvent{
		Type:          domainevent.EventMessageCreated,
		RoomID:        roomID,
		TargetUserIDs: []string{targetUserID},
		OccurredAt:    time.Now(),
	})

	select {
	case got, ok := <-targetCh:
		if !ok {
			t.Fatal("target channel closed before delivering the published event")
		}
		if got.RoomID != roomID {
			t.Errorf("expected RoomID %q, got %q", roomID, got.RoomID)
		}
	case <-time.After(deliveryTimeout):
		t.Fatal("timed out waiting for targeted event delivery")
	}

	select {
	case got, ok := <-otherCh:
		if ok {
			t.Fatalf("expected no event delivered to non-targeted subscriber, got %+v", got)
		}
	case <-time.After(500 * time.Millisecond):
		// No event arrived within the wait window: this is the expected
		// outcome for a subscriber that was not in TargetUserIDs.
	}
}

// TestRedisHubUnsubscribeClosesChannel proves that calling the unsubscribe
// function closes the output channel, and that calling it a second time does
// not panic, mirroring InProcessHub's idempotent-unsubscribe contract.
func TestRedisHubUnsubscribeClosesChannel(t *testing.T) {
	ctx := context.Background()
	client := testutilredis.New(ctx, t)
	hub := NewRedisHub(client)

	ch, unsubscribe := hub.Subscribe(ctx, "room-unsub", "user-1")

	unsubscribe()
	unsubscribe() // must not panic on a repeated call

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	case <-time.After(deliveryTimeout):
		t.Fatal("timed out waiting for channel to close after unsubscribe")
	}
}
