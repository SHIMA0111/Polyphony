//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// TestContextSummaryRepository_GetNotFound proves that Get returns
// domain.ErrNotFound for a room with no cached summary.
func TestContextSummaryRepository_GetNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-notfound-owner")

	_, err := summaryRepo.Get(ctx, rm.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

// TestContextSummaryRepository_UpsertAndGet proves that Upsert followed by
// Get round-trips every field correctly for a room with no prior cached
// summary (the INSERT branch of the ON CONFLICT upsert).
func TestContextSummaryRepository_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-upsert-owner")

	summary := &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 42,
		SummaryText:         "The conversation covered onboarding steps.",
		TokenCount:          123,
	}
	if err := summaryRepo.Upsert(ctx, summary, 0); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := summaryRepo.Get(ctx, rm.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RoomID != rm.ID {
		t.Errorf("RoomID = %q, want %q", got.RoomID, rm.ID)
	}
	if got.Model != "gpt-5-mini" {
		t.Errorf("Model = %q, want %q", got.Model, "gpt-5-mini")
	}
	if got.CoveredUpToSequence != 42 {
		t.Errorf("CoveredUpToSequence = %d, want 42", got.CoveredUpToSequence)
	}
	if got.SummaryText != "The conversation covered onboarding steps." {
		t.Errorf("SummaryText = %q, want %q", got.SummaryText, "The conversation covered onboarding steps.")
	}
	if got.TokenCount != 123 {
		t.Errorf("TokenCount = %d, want 123", got.TokenCount)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("expected CreatedAt/UpdatedAt to be populated, got %v / %v", got.CreatedAt, got.UpdatedAt)
	}
}

// TestContextSummaryRepository_UpsertReplacesExistingRow proves the ON
// CONFLICT (room_id) DO UPDATE behavior: a second Upsert for the same room
// replaces every field of the first row rather than erroring or inserting a
// second row (room_id is the table's PRIMARY KEY, so a second row for the
// same room is not even possible -- this test proves the update path is
// taken instead of an error).
func TestContextSummaryRepository_UpsertReplacesExistingRow(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-replace-owner")

	first := &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 10,
		SummaryText:         "first summary",
		TokenCount:          50,
	}
	if err := summaryRepo.Upsert(ctx, first, 0); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second := &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "claude-opus-4-6",
		CoveredUpToSequence: 99,
		SummaryText:         "second, replacing summary",
		TokenCount:          200,
	}
	if err := summaryRepo.Upsert(ctx, second, 0); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := summaryRepo.Get(ctx, rm.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want the replaced value %q", got.Model, "claude-opus-4-6")
	}
	if got.CoveredUpToSequence != 99 {
		t.Errorf("CoveredUpToSequence = %d, want the replaced value 99", got.CoveredUpToSequence)
	}
	if got.SummaryText != "second, replacing summary" {
		t.Errorf("SummaryText = %q, want the replaced value %q", got.SummaryText, "second, replacing summary")
	}
	if got.TokenCount != 200 {
		t.Errorf("TokenCount = %d, want the replaced value 200", got.TokenCount)
	}
}

// TestContextSummaryRepository_DeleteByRoom proves that DeleteByRoom removes
// a cached summary (a subsequent Get returns domain.ErrNotFound) and that
// calling it again for a room with no cached row is a no-op, not an error.
func TestContextSummaryRepository_DeleteByRoom(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-delete-owner")

	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 5,
		SummaryText:         "to be deleted",
		TokenCount:          10,
	}, 0); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := summaryRepo.DeleteByRoom(ctx, rm.ID); err != nil {
		t.Fatalf("DeleteByRoom: %v", err)
	}

	if _, err := summaryRepo.Get(ctx, rm.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound after delete, got %v", err)
	}

	// Deleting again (no row exists) must not be an error.
	if err := summaryRepo.DeleteByRoom(ctx, rm.ID); err != nil {
		t.Fatalf("expected DeleteByRoom on an already-empty room to succeed, got %v", err)
	}
}

// TestContextSummaryRepository_GetRevisionDefaultsToZero proves that a room
// with no DeleteByRoom history reports revision 0 (see
// ai.ContextSummaryRepository.GetRevision).
func TestContextSummaryRepository_GetRevisionDefaultsToZero(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-revision-default-owner")

	revision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 0 {
		t.Errorf("revision = %d, want 0 for a room with no DeleteByRoom history", revision)
	}
}

// TestContextSummaryRepository_DeleteByRoomIncrementsRevision proves that
// each DeleteByRoom call advances the room's revision by exactly 1,
// regardless of whether a cached summary existed to delete.
func TestContextSummaryRepository_DeleteByRoomIncrementsRevision(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-revision-increment-owner")

	if err := summaryRepo.DeleteByRoom(ctx, rm.ID); err != nil {
		t.Fatalf("first DeleteByRoom: %v", err)
	}
	revision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("revision = %d, want 1 after the first DeleteByRoom", revision)
	}

	if err := summaryRepo.DeleteByRoom(ctx, rm.ID); err != nil {
		t.Fatalf("second DeleteByRoom: %v", err)
	}
	revision, err = summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 2 {
		t.Fatalf("revision = %d, want 2 after the second DeleteByRoom", revision)
	}
}

// TestContextSummaryRepository_UpsertNoopsOnRevisionMismatch proves the core
// revision-fencing behavior (see ai.ContextSummaryRepository's "Revision
// fencing" doc comment): an Upsert whose expectedRevision no longer matches
// the room's current revision -- because a DeleteByRoom landed after the
// caller captured it, simulating a concurrent invalidation racing an
// in-flight summarization -- writes nothing and returns no error, and a
// subsequent Upsert using the now-current revision succeeds normally.
func TestContextSummaryRepository_UpsertNoopsOnRevisionMismatch(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-revision-mismatch-owner")

	// Capture the room's revision (0, since DeleteByRoom has never been
	// called for it) as summaryOrCompute would before starting
	// summarization.
	capturedRevision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	// A concurrent delete/exclude-toggle lands and invalidates the room
	// while the (simulated) summarization above is still in flight.
	if err := summaryRepo.DeleteByRoom(ctx, rm.ID); err != nil {
		t.Fatalf("DeleteByRoom: %v", err)
	}

	// The in-flight summarization now finishes and tries to cache its
	// result against the stale, pre-invalidation revision it captured.
	// This must no-op rather than persist the stale summary.
	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 7,
		SummaryText:         "stale summary that should never be persisted",
		TokenCount:          10,
	}, capturedRevision); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if _, err := summaryRepo.Get(ctx, rm.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the stale Upsert to have no-op'd (still no cached summary), got err=%v", err)
	}

	// A subsequent Upsert using the now-current revision must succeed
	// normally.
	currentRevision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 7,
		SummaryText:         "fresh summary computed after the invalidation",
		TokenCount:          10,
	}, currentRevision); err != nil {
		t.Fatalf("Upsert with current revision: %v", err)
	}

	got, err := summaryRepo.Get(ctx, rm.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SummaryText != "fresh summary computed after the invalidation" {
		t.Errorf("SummaryText = %q, want the fresh summary to have been persisted", got.SummaryText)
	}
}

// TestContextSummaryRepository_UpsertDeleteByRoomConcurrentRace proves the
// pg_advisory_xact_lock serialization added to Upsert/DeleteByRoom closes
// the READ COMMITTED interleaving race described in both methods' doc
// comments. Launched concurrently for the same room and revision, the two
// possible total orderings the lock now enforces (Upsert-then-Delete,
// Delete-then-Upsert) both converge to the same final state -- no cached
// summary and an incremented revision -- since DeleteByRoom unconditionally
// removes whatever Upsert may have just written first. Before the fix, a
// third interleaving was possible: Upsert's revision check reads a stale
// (pre-delete) revision, but its write commits after DeleteByRoom's commit,
// resurrecting the stale summary right after the delete that was supposed
// to invalidate it -- a state this test catches as Get unexpectedly
// succeeding instead of returning domain.ErrNotFound. Repeated over several
// rounds so either of the two now-only-possible orderings is exercised at
// least once.
func TestContextSummaryRepository_UpsertDeleteByRoomConcurrentRace(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "summary-race-owner")

	const rounds = 20
	for i := 0; i < rounds; i++ {
		capturedRevision, err := summaryRepo.GetRevision(ctx, rm.ID)
		if err != nil {
			t.Fatalf("round %d: GetRevision: %v", i, err)
		}

		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			errs <- summaryRepo.Upsert(ctx, &ai.ContextSummary{
				RoomID:              rm.ID,
				Model:               "gpt-5-mini",
				CoveredUpToSequence: int64(i),
				SummaryText:         fmt.Sprintf("round %d summary", i),
				TokenCount:          10,
			}, capturedRevision)
		}()
		go func() {
			defer wg.Done()
			<-start
			errs <- summaryRepo.DeleteByRoom(ctx, rm.ID)
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: concurrent call failed: %v", i, err)
			}
		}

		if _, err := summaryRepo.Get(ctx, rm.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("round %d: expected no cached summary to survive the concurrent Upsert/DeleteByRoom pair (stale-resurrection race), got err=%v", i, err)
		}

		revision, err := summaryRepo.GetRevision(ctx, rm.ID)
		if err != nil {
			t.Fatalf("round %d: GetRevision: %v", i, err)
		}
		if revision != capturedRevision+1 {
			t.Fatalf("round %d: revision = %d, want %d", i, revision, capturedRevision+1)
		}
	}
}
