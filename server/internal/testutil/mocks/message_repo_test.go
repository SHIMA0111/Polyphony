package mocks

import (
	"context"
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
)

// TestMessageRepoReserveSequenceRangeRejectsNonPositiveCount proves
// ReserveSequenceRange rejects a zero or negative count with
// domain.ErrInvalidArgument, mirroring
// postgres.MessageRepository.ReserveSequenceRange's guard, and leaves the
// room's sequence counter untouched -- a subsequent valid reservation must
// still start from 1.
func TestMessageRepoReserveSequenceRangeRejectsNonPositiveCount(t *testing.T) {
	repo := &MessageRepo{}
	ctx := context.Background()

	for _, count := range []int64{0, -1, -1000} {
		if _, err := repo.ReserveSequenceRange(ctx, "room-1", count); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("count=%d: expected domain.ErrInvalidArgument, got %v", count, err)
		}
	}

	first, err := repo.ReserveSequenceRange(ctx, "room-1", 1)
	if err != nil {
		t.Fatalf("ReserveSequenceRange(1) failed after rejected non-positive counts: %v", err)
	}
	if first != 1 {
		t.Fatalf("expected the first valid reservation to start at 1 (untouched by the rejected calls), got %d", first)
	}
}
