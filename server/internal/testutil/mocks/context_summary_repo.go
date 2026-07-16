package mocks

import (
	"context"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// ContextSummaryRepo is an in-memory, map-backed fake implementing
// ai.ContextSummaryRepository, mirroring MessageRepo's pattern. The zero
// value (mocks.ContextSummaryRepo{}) is ready to use; its map is
// initialized lazily on first write.
//
// ContextSummaryRepo is safe for concurrent use.
type ContextSummaryRepo struct {
	mu        sync.Mutex
	Summaries map[string]*ai.ContextSummary // roomID -> cached summary

	// GetCallCount/UpsertCallCount/DeleteCallCount let tests assert how
	// many times each method was invoked (e.g. to prove a cache hit avoided
	// a redundant Upsert, or that DeleteByRoom was actually called by
	// MessageUsecase.DeleteMessage/SetExcludeFromAI).
	GetCallCount    int
	UpsertCallCount int
	DeleteCallCount int
}

func (r *ContextSummaryRepo) ensureInit() {
	if r.Summaries == nil {
		r.Summaries = make(map[string]*ai.ContextSummary)
	}
}

// Get returns the cached ContextSummary for roomID, or domain.ErrNotFound
// if none is cached.
func (r *ContextSummaryRepo) Get(_ context.Context, roomID string) (*ai.ContextSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.GetCallCount++

	s, ok := r.Summaries[roomID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}

// Upsert creates or replaces the cached summary for summary.RoomID.
func (r *ContextSummaryRepo) Upsert(_ context.Context, summary *ai.ContextSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.UpsertCallCount++

	r.Summaries[summary.RoomID] = summary
	return nil
}

// DeleteByRoom invalidates (deletes) the cached summary for roomID, if any.
// It is not an error to call this for a room with no cached summary.
func (r *ContextSummaryRepo) DeleteByRoom(_ context.Context, roomID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.DeleteCallCount++

	delete(r.Summaries, roomID)
	return nil
}
