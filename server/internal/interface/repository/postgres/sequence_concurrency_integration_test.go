//go:build integration

package postgres

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// TestReserveSequenceRangeConcurrency proves that
// MessageRepository.ReserveSequenceRange's `UPDATE room_sequences ...
// RETURNING next_sequence - $2` pattern atomically allocates sequence
// *ranges* under concurrent access, even when goroutines request different
// range sizes (mirroring SendMessage reserving 1 and SendAIMessage reserving
// 2): every goroutine must receive a range disjoint from every other
// goroutine's range, the union of all reserved sequence numbers must have
// zero duplicates, and its size must equal the sum of every requested count.
func TestReserveSequenceRangeConcurrency(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "seq-owner@example.com",
		Username:     "seq-owner",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        "Concurrency Room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("create room: %v", err)
	}

	const goroutines = 50

	// Mixed counts: alternate between reserving 1 (SendMessage's usage) and
	// 2 (SendAIMessage's usage) contiguous sequence numbers.
	counts := make([]int64, goroutines)
	var wantTotal int64
	for i := range counts {
		if i%2 == 0 {
			counts[i] = 1
		} else {
			counts[i] = 2
		}
		wantTotal += counts[i]
	}
	// Shuffle so goroutine start order doesn't correlate with count.
	rand.Shuffle(len(counts), func(i, j int) { counts[i], counts[j] = counts[j], counts[i] })

	type reservation struct {
		first int64
		count int64
	}

	var wg sync.WaitGroup
	resCh := make(chan reservation, goroutines)
	errCh := make(chan error, goroutines)

	for _, count := range counts {
		wg.Add(1)
		go func(count int64) {
			defer wg.Done()
			first, err := msgRepo.ReserveSequenceRange(ctx, rm.ID, count)
			if err != nil {
				errCh <- err
				return
			}
			resCh <- reservation{first: first, count: count}
		}(count)
	}
	wg.Wait()
	close(resCh)
	close(errCh)

	for err := range errCh {
		t.Fatalf("ReserveSequenceRange returned an error under concurrency: %v", err)
	}

	seen := make(map[int64]bool, wantTotal)
	var gotTotal int64
	for res := range resCh {
		for seq := res.first; seq < res.first+res.count; seq++ {
			if seen[seq] {
				t.Fatalf("ReserveSequenceRange allocated duplicate/overlapping sequence number %d", seq)
			}
			seen[seq] = true
			gotTotal++
		}
	}

	if gotTotal != wantTotal {
		t.Fatalf("expected %d total reserved sequence numbers, got %d", wantTotal, gotTotal)
	}

	// The room starts with next_sequence=1, so goroutines concurrently
	// draining it must produce exactly the contiguous run [1, wantTotal].
	for i := int64(1); i <= wantTotal; i++ {
		if !seen[i] {
			t.Fatalf("sequence %d missing from allocated set: gap detected (non-atomic allocation)", i)
		}
	}
}
