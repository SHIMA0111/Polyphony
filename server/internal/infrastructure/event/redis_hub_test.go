package event

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	domainevent "github.com/SHIMA0111/multi-user-ai/server/internal/domain/event"
)

// TestNewRedisHubConstruction verifies that NewRedisHub returns a non-nil,
// ready-to-use RedisHub wrapping the given client, without requiring a real
// Redis connection (construction alone must not dial).
func TestNewRedisHubConstruction(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	defer func() { _ = client.Close() }()

	hub := NewRedisHub(client)
	if hub == nil {
		t.Fatal("expected NewRedisHub to return a non-nil RedisHub")
	}
	if hub.client != client {
		t.Error("expected RedisHub to wrap the given client")
	}
}

// TestRedisHubPublishNeverErrorsToCaller verifies RedisHub.Publish's
// best-effort contract: given a client that cannot reach Redis (an
// unroutable address with a short timeout), Publish must still return
// promptly without panicking and without any error return value (it has
// none, per the MessageHub interface), matching the "publishing never fails
// the caller" guarantee documented on both InProcessHub and RedisHub.
func TestRedisHubPublishNeverErrorsToCaller(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "10.255.255.1:1", // unroutable per RFC 5737-style test range; connection attempt fails fast via timeout
		DialTimeout: 100 * time.Millisecond,
		MaxRetries:  -1, // disable go-redis's built-in retries so the test doesn't wait through several dial attempts
	})
	defer func() { _ = client.Close() }()

	hub := NewRedisHub(client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Publish(ctx, domainevent.RoomEvent{
			Type:   domainevent.EventMessageCreated,
			RoomID: "room-unreachable",
		})
	}()

	select {
	case <-done:
		// Publish returned without panicking, as required.
	case <-time.After(3 * time.Second):
		t.Fatal("Publish did not return promptly against an unreachable Redis")
	}
}

// TestContainsUserID exercises the containsUserID helper directly, since it
// implements the per-user targeting filter that is RedisHub's key behavioral
// contract beyond what InProcessHub already covers via its own tests.
func TestContainsUserID(t *testing.T) {
	tests := []struct {
		name   string
		ids    []string
		userID string
		want   bool
	}{
		{name: "empty list", ids: nil, userID: "user-1", want: false},
		{name: "present", ids: []string{"user-1", "user-2"}, userID: "user-2", want: true},
		{name: "absent", ids: []string{"user-1", "user-2"}, userID: "user-3", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := containsUserID(tc.ids, tc.userID)
			if got != tc.want {
				t.Errorf("containsUserID(%v, %q) = %v, want %v", tc.ids, tc.userID, got, tc.want)
			}
		})
	}
}
