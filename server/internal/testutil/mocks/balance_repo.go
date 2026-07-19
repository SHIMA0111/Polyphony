package mocks

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// BalanceRepo is an in-memory, map-backed fake implementing
// billing.BalanceRepository. The zero value (mocks.BalanceRepo{}) is ready
// to use; the backing maps are initialized lazily on first write.
//
// BalanceRepo is safe for concurrent use.
type BalanceRepo struct {
	mu           sync.Mutex
	Balances     map[string]*billing.TokenBalance
	Transactions map[string][]*billing.TokenTransaction // userID -> transactions, oldest first
}

func (r *BalanceRepo) ensureInit() {
	if r.Balances == nil {
		r.Balances = make(map[string]*billing.TokenBalance)
	}
	if r.Transactions == nil {
		r.Transactions = make(map[string][]*billing.TokenTransaction)
	}
}

// SeedBalance sets userID's balance directly, creating the row if absent.
// It lets tests set up a non-zero starting balance without going through
// CreditAndRecord.
func (r *BalanceRepo) SeedBalance(userID string, balance int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.Balances[userID] = &billing.TokenBalance{UserID: userID, Balance: balance, UpdatedAt: time.Now()}
}

// GetOrCreateBalance returns userID's balance, creating a zero-balance row
// on first access.
func (r *BalanceRepo) GetOrCreateBalance(_ context.Context, userID string) (*billing.TokenBalance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if b, ok := r.Balances[userID]; ok {
		cp := *b
		return &cp, nil
	}
	b := &billing.TokenBalance{UserID: userID, Balance: 0, UpdatedAt: time.Now()}
	r.Balances[userID] = b
	cp := *b
	return &cp, nil
}

// DebitAndRecord decrements userID's balance by amount and records a
// TransactionTypeConsumption row. Returns billing.ErrInvalidAmount if
// amount <= 0 (checked before any mutation, mirroring
// postgres.BillingRepository), or domain.ErrNotFound if userID has no
// existing balance row.
func (r *BalanceRepo) DebitAndRecord(_ context.Context, userID, roomID, messageID string, amount int64, description string) (*billing.TokenTransaction, error) {
	if amount <= 0 {
		return nil, billing.ErrInvalidAmount
	}
	return r.mutate(userID, &roomID, billing.TransactionTypeConsumption, -amount, description)
}

// CreditAndRecord increments userID's balance by amount and records a row
// of the given txType. Returns billing.ErrInvalidAmount if amount <= 0 or
// txType is not TransactionTypeCharge/TransactionTypeAdjustment (checked
// before any mutation, mirroring postgres.BillingRepository), or
// domain.ErrNotFound if userID has no existing balance row.
func (r *BalanceRepo) CreditAndRecord(_ context.Context, userID string, txType billing.TransactionType, amount int64, description string) (*billing.TokenTransaction, error) {
	if amount <= 0 {
		return nil, billing.ErrInvalidAmount
	}
	if txType != billing.TransactionTypeCharge && txType != billing.TransactionTypeAdjustment {
		return nil, billing.ErrInvalidAmount
	}
	return r.mutate(userID, nil, txType, amount, description)
}

func (r *BalanceRepo) mutate(userID string, roomID *string, txType billing.TransactionType, signedAmount int64, description string) (*billing.TokenTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	b, ok := r.Balances[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	b.Balance += signedAmount
	b.UpdatedAt = time.Now()

	txn := &billing.TokenTransaction{
		ID:           uuid.New().String(),
		UserID:       userID,
		RoomID:       roomID,
		Type:         txType,
		Amount:       signedAmount,
		BalanceAfter: b.Balance,
		Description:  description,
		CreatedAt:    time.Now(),
	}
	r.Transactions[userID] = append(r.Transactions[userID], txn)

	cp := *txn
	return &cp, nil
}

// ListTransactions returns a cursor-paginated page of userID's transactions,
// newest first, mirroring the real repository's pagination contract.
func (r *BalanceRepo) ListTransactions(_ context.Context, userID, cursor string, limit int) (*billing.TransactionPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	all := append([]*billing.TokenTransaction(nil), r.Transactions[userID]...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	start := 0
	if cursor != "" {
		found := false
		for i, txn := range all {
			if txn.ID == cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, domain.ErrNotFound
		}
	}

	page := &billing.TransactionPage{}
	remaining := all[start:]
	if len(remaining) > limit {
		page.Transactions = remaining[:limit]
		nextCursor := page.Transactions[len(page.Transactions)-1].ID
		page.NextCursor = &nextCursor
	} else {
		page.Transactions = remaining
	}
	return page, nil
}
