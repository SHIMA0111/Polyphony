//go:build integration

package postgres

import (
	"context"
	"errors"
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
	if err := summaryRepo.Upsert(ctx, summary); err != nil {
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
	if err := summaryRepo.Upsert(ctx, first); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second := &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "claude-opus-4-6",
		CoveredUpToSequence: 99,
		SummaryText:         "second, replacing summary",
		TokenCount:          200,
	}
	if err := summaryRepo.Upsert(ctx, second); err != nil {
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
	}); err != nil {
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
