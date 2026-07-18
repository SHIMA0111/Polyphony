package mocks

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// PaymentRepo is an in-memory, map-backed fake implementing
// billing.PaymentRepository. The zero value (mocks.PaymentRepo{}) is ready
// to use; the backing maps are initialized lazily on first write.
// CreateAndCredit's balance-crediting half is delegated to BalanceRepo (set
// it before use if a test needs the credit side effect, e.g. via
// mocks.BalanceRepo.SeedBalance), mirroring how the real
// postgres.PaymentRepository shares its balance-mutation SQL with
// postgres.BillingRepository within one transaction.
//
// PaymentRepo is safe for concurrent use.
type PaymentRepo struct {
	mu          sync.Mutex
	byID        map[string]*billing.PaymentRecord
	byEventID   map[string]*billing.PaymentRecord
	BalanceRepo *BalanceRepo
}

func (r *PaymentRepo) ensureInit() {
	if r.byID == nil {
		r.byID = make(map[string]*billing.PaymentRecord)
	}
	if r.byEventID == nil {
		r.byEventID = make(map[string]*billing.PaymentRecord)
	}
}

// Create inserts a payment_history row without crediting any balance. It is
// idempotent on payment.StripeEventID, returning alreadyRecorded=true for a
// replayed event.
func (r *PaymentRepo) Create(_ context.Context, payment *billing.PaymentRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if _, ok := r.byEventID[payment.StripeEventID]; ok {
		return true, nil
	}

	cp := *payment
	if cp.PaymentRail == "" {
		cp.PaymentRail = "stripe"
	}
	cp.CreatedAt = time.Now()
	r.byID[cp.ID] = &cp
	r.byEventID[cp.StripeEventID] = &cp
	return false, nil
}

// CreateAndCredit atomically inserts payment and, only when it is newly
// inserted, credits userID's balance via BalanceRepo, holding the mutex for
// the whole operation and only publishing payment into byID/byEventID after
// both credit calls succeed. This mirrors postgres.PaymentRepository.
// CreateAndCredit's single-transaction semantics: if a credit call fails,
// the payment is left unrecorded so a subsequent delivery of the same
// event retries the credit instead of alreadyProcessed short-circuiting it
// and silently losing the tokens. It returns alreadyProcessed=true (with
// the balance left untouched) for a replayed payment.StripeEventID.
func (r *PaymentRepo) CreateAndCredit(ctx context.Context, payment *billing.PaymentRecord, userID string, amount int64, description string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	if _, ok := r.byEventID[payment.StripeEventID]; ok {
		return true, nil
	}

	if r.BalanceRepo != nil {
		if _, err := r.BalanceRepo.GetOrCreateBalance(ctx, userID); err != nil {
			return false, err
		}
		if _, err := r.BalanceRepo.CreditAndRecord(ctx, userID, billing.TransactionTypeCharge, amount, description); err != nil {
			return false, err
		}
	}

	cp := *payment
	if cp.PaymentRail == "" {
		cp.PaymentRail = "stripe"
	}
	cp.CreatedAt = time.Now()
	r.byID[cp.ID] = &cp
	r.byEventID[cp.StripeEventID] = &cp
	return false, nil
}

// ListByUserID returns a cursor-paginated page of userID's payment history,
// newest first, mirroring BalanceRepo.ListTransactions's pagination
// contract.
func (r *PaymentRepo) ListByUserID(_ context.Context, userID, cursor string, limit int) (*billing.PaymentHistoryPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	var all []*billing.PaymentRecord
	for _, p := range r.byID {
		if p.UserID == userID {
			all = append(all, p)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	start := 0
	if cursor != "" {
		found := false
		for i, p := range all {
			if p.ID == cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, domain.ErrNotFound
		}
	}

	page := &billing.PaymentHistoryPage{}
	remaining := all[start:]
	if len(remaining) > limit {
		page.Payments = remaining[:limit]
		nextCursor := page.Payments[len(page.Payments)-1].ID
		page.NextCursor = &nextCursor
	} else {
		page.Payments = remaining
	}
	return page, nil
}
