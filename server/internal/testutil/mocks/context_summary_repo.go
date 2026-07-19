package mocks

import (
	"context"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// ContextSummaryRepo is an in-memory, map-backed fake implementing
// ai.ContextSummaryRepository, mirroring MessageRepo's pattern. The zero
// value (mocks.ContextSummaryRepo{}) is ready to use; its maps are
// initialized lazily on first write.
//
// ContextSummaryRepo is safe for concurrent use.
type ContextSummaryRepo struct {
	mu        sync.Mutex
	Summaries map[string]*ai.ContextSummary // roomID -> cached summary
	Revisions map[string]int64              // roomID -> current invalidation revision

	// GetCallCount/UpsertCallCount/DeleteCallCount let tests assert how
	// many times each method was invoked (e.g. to prove a cache hit avoided
	// a redundant Upsert, or that DeleteByRoom was actually called by
	// MessageUsecase.DeleteMessage/SetExcludeFromAI).
	GetCallCount    int
	UpsertCallCount int
	DeleteCallCount int

	// UpsertNoopCount counts Upsert calls that no-op'd because
	// expectedRevision didn't match the room's current revision at call
	// time -- mirroring postgres.ContextSummaryRepository's
	// zero-rows-affected outcome. Tests use this to assert the
	// DeleteByRoom-during-summarization race is actually fenced (see
	// ai.ContextSummaryRepository's "Revision fencing" doc comment).
	UpsertNoopCount int

	// DeleteByRoomErr, if non-nil, makes DeleteByRoom return it instead of
	// succeeding (without deleting the cached summary or bumping the
	// revision) -- for tests exercising callers' handling of an
	// invalidation failure (MessageUsecase.DeleteMessage/SetExcludeFromAI
	// now propagate this error; see their doc comments).
	DeleteByRoomErr error
}

func (r *ContextSummaryRepo) ensureInit() {
	if r.Summaries == nil {
		r.Summaries = make(map[string]*ai.ContextSummary)
	}
	if r.Revisions == nil {
		r.Revisions = make(map[string]int64)
	}
}

// cloneContextSummary returns a shallow copy of summary. ContextSummary has
// no pointer/slice fields, so a plain `cp := *summary` copy is a complete,
// independent clone. Used by both Get and Upsert so neither hands out --
// nor stores -- the caller's own *ai.ContextSummary pointer: without this, a
// caller retaining the pointer it passed to Upsert (or received from Get)
// could mutate it later and silently corrupt the mock's stored state (or a
// previously-returned Get result) without going through the mutex at all.
func cloneContextSummary(summary *ai.ContextSummary) *ai.ContextSummary {
	cp := *summary
	return &cp
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
	return cloneContextSummary(s), nil
}

// Upsert creates or replaces the cached summary for summary.RoomID, but
// only if roomID's current revision equals expectedRevision -- mirroring
// postgres.ContextSummaryRepository's revision-guarded Upsert. A mismatch
// increments UpsertNoopCount and returns nil (no-op, not an error). The
// stored copy is cloned from the caller's summary (see cloneContextSummary),
// so a later caller-side mutation of *summary cannot bleed into the mock's
// stored state.
func (r *ContextSummaryRepo) Upsert(_ context.Context, summary *ai.ContextSummary, expectedRevision int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.UpsertCallCount++

	if r.Revisions[summary.RoomID] != expectedRevision {
		r.UpsertNoopCount++
		return nil
	}

	r.Summaries[summary.RoomID] = cloneContextSummary(summary)
	return nil
}

// GetRevision returns roomID's current invalidation revision, or 0 if
// DeleteByRoom has never been called for this room.
func (r *ContextSummaryRepo) GetRevision(_ context.Context, roomID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	return r.Revisions[roomID], nil
}

// DeleteByRoom invalidates (deletes) the cached summary for roomID, if any,
// and increments roomID's invalidation revision -- mirroring
// postgres.ContextSummaryRepository's atomic delete-and-bump. It is not an
// error to call this for a room with no cached summary. If DeleteByRoomErr
// is set, it is returned instead and neither the cached summary nor the
// revision is mutated.
func (r *ContextSummaryRepo) DeleteByRoom(_ context.Context, roomID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.DeleteCallCount++

	if r.DeleteByRoomErr != nil {
		return r.DeleteByRoomErr
	}

	delete(r.Summaries, roomID)
	r.Revisions[roomID]++
	return nil
}
