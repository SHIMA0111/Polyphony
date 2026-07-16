//go:build integration

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// TestMessageRepositoryGetNextSequenceConcurrency proves that
// MessageRepository.GetNextSequence's `UPDATE room_sequences ... RETURNING
// next_sequence - 1` pattern atomically allocates sequence numbers under
// concurrent access: N goroutines racing on the same room must each receive
// a distinct sequence number, and the resulting set must be a contiguous
// run with no gaps or duplicates.
func TestMessageRepositoryGetNextSequenceConcurrency(t *testing.T) {
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

	var wg sync.WaitGroup
	seqCh := make(chan int64, goroutines)
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seq, err := msgRepo.GetNextSequence(ctx, rm.ID)
			if err != nil {
				errCh <- err
				return
			}
			seqCh <- seq
		}()
	}
	wg.Wait()
	close(seqCh)
	close(errCh)

	for err := range errCh {
		t.Fatalf("GetNextSequence returned an error under concurrency: %v", err)
	}

	seen := make(map[int64]bool, goroutines)
	for seq := range seqCh {
		if seen[seq] {
			t.Fatalf("GetNextSequence allocated duplicate sequence number %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != goroutines {
		t.Fatalf("expected %d unique sequence numbers, got %d", goroutines, len(seen))
	}

	// The room starts with next_sequence=1, so goroutines concurrently
	// draining it must produce exactly the contiguous run [1, goroutines].
	for i := int64(1); i <= goroutines; i++ {
		if !seen[i] {
			t.Fatalf("sequence %d missing from allocated set: gap detected (non-atomic allocation)", i)
		}
	}
}
