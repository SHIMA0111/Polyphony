package mocks

import (
	"context"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// SubscriptionRepo is an in-memory, map-backed fake implementing
// billing.SubscriptionRepository. The zero value (mocks.SubscriptionRepo{})
// is ready to use; the backing maps are initialized lazily on first write.
//
// SubscriptionRepo is safe for concurrent use.
type SubscriptionRepo struct {
	mu sync.Mutex
	// ByID stores every subscription by ID; ByUserID/ByStripeSubID index the
	// same *billing.Subscription values by their other lookup keys.
	ByID          map[string]*billing.Subscription
	ByUserID      map[string]*billing.Subscription
	ByStripeSubID map[string]*billing.Subscription
}

func (r *SubscriptionRepo) ensureInit() {
	if r.ByID == nil {
		r.ByID = make(map[string]*billing.Subscription)
	}
	if r.ByUserID == nil {
		r.ByUserID = make(map[string]*billing.Subscription)
	}
	if r.ByStripeSubID == nil {
		r.ByStripeSubID = make(map[string]*billing.Subscription)
	}
}

// Create persists a new Subscription row.
func (r *SubscriptionRepo) Create(_ context.Context, sub *billing.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if _, ok := r.ByStripeSubID[sub.StripeSubscriptionID]; ok {
		return domain.ErrSubscriptionAlreadyExists
	}

	cp := *sub
	r.ByID[cp.ID] = &cp
	r.ByUserID[cp.UserID] = &cp
	r.ByStripeSubID[cp.StripeSubscriptionID] = &cp
	return nil
}

// GetByUserID returns userID's Subscription, or domain.ErrNotFound if none exists.
func (r *SubscriptionRepo) GetByUserID(_ context.Context, userID string) (*billing.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	sub, ok := r.ByUserID[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *sub
	return &cp, nil
}

// GetByStripeSubscriptionID returns the Subscription matching the given
// Stripe subscription ID, or domain.ErrNotFound if none exists.
func (r *SubscriptionRepo) GetByStripeSubscriptionID(_ context.Context, stripeSubscriptionID string) (*billing.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	sub, ok := r.ByStripeSubID[stripeSubscriptionID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *sub
	return &cp, nil
}

// Update persists changes to an existing Subscription row (matched by ID),
// or returns domain.ErrNotFound if no row with that ID exists.
func (r *SubscriptionRepo) Update(_ context.Context, sub *billing.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	existing, ok := r.ByID[sub.ID]
	if !ok {
		return domain.ErrNotFound
	}

	cp := *sub
	// Keep the old index entries pointing at stale keys clean when a
	// mutation changes a subscription's UserID/StripeSubscriptionID — not
	// exercised by this step's tests, but kept correct defensively.
	delete(r.ByUserID, existing.UserID)
	delete(r.ByStripeSubID, existing.StripeSubscriptionID)
	r.ByID[cp.ID] = &cp
	r.ByUserID[cp.UserID] = &cp
	r.ByStripeSubID[cp.StripeSubscriptionID] = &cp
	return nil
}
