package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
)

// TestReserveSequenceRangeRejectsNonPositiveCount proves that
// ReserveSequenceRange rejects a zero or negative count with
// domain.ErrInvalidArgument before ever reaching the pool -- exercised
// against a MessageRepository constructed with a nil pool, which would
// panic on any attempt to actually issue the UPDATE, so this test doubles
// as proof the guard runs first.
func TestReserveSequenceRangeRejectsNonPositiveCount(t *testing.T) {
	r := NewMessageRepository(nil)

	for _, count := range []int64{0, -1, -1000} {
		if _, err := r.ReserveSequenceRange(context.Background(), "any-room", count); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("count=%d: expected domain.ErrInvalidArgument, got %v", count, err)
		}
	}
}
