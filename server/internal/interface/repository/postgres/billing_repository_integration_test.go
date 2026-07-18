//go:build integration

package postgres

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// seedBillingUser creates a standalone user (no room needed for billing
// tests, since token_balances/token_transactions are keyed directly by
// user_id) and returns its ID.
func seedBillingUser(ctx context.Context, t *testing.T, userRepo *UserRepository, label string) string {
	t.Helper()

	u := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        label + "@example.com",
		Username:     label,
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

// TestBillingRepositoryGetOrCreateBalanceIdempotent proves the first call
// inserts a zero-balance row and the second call for the same user returns
// the same row without resetting balance to zero.
func TestBillingRepositoryGetOrCreateBalanceIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	billingRepo := NewBillingRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "balance-idempotent")

	first, err := billingRepo.GetOrCreateBalance(ctx, userID)
	if err != nil {
		t.Fatalf("first GetOrCreateBalance failed: %v", err)
	}
	if first.Balance != 0 {
		t.Fatalf("expected initial balance 0, got %d", first.Balance)
	}

	if _, err := billingRepo.CreditAndRecord(ctx, userID, billing.TransactionTypeCharge, 500, "topup"); err != nil {
		t.Fatalf("CreditAndRecord failed: %v", err)
	}

	second, err := billingRepo.GetOrCreateBalance(ctx, userID)
	if err != nil {
		t.Fatalf("second GetOrCreateBalance failed: %v", err)
	}
	if second.Balance != 500 {
		t.Fatalf("expected balance 500 preserved across GetOrCreateBalance, got %d", second.Balance)
	}
}

// TestBillingRepositoryDebitAndRecord proves DebitAndRecord decrements the
// balance and inserts a matching token_transactions row with the correct
// signed amount/balance_after/type, atomically.
func TestBillingRepositoryDebitAndRecord(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	billingRepo := NewBillingRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "debit-owner")
	if _, err := billingRepo.GetOrCreateBalance(ctx, userID); err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if _, err := billingRepo.CreditAndRecord(ctx, userID, billing.TransactionTypeCharge, 1000, "initial topup"); err != nil {
		t.Fatalf("seed credit failed: %v", err)
	}

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "debit-room")
	msg := seedMessage(ctx, t, msgRepo, rm.ID, userID)

	txn, err := billingRepo.DebitAndRecord(ctx, userID, rm.ID, msg.ID, 150, "AI response using test-model (100 prompt + 50 output tokens)")
	if err != nil {
		t.Fatalf("DebitAndRecord failed: %v", err)
	}
	if txn.Amount != -150 {
		t.Fatalf("expected signed amount -150, got %d", txn.Amount)
	}
	if txn.BalanceAfter != 850 {
		t.Fatalf("expected balance_after 850, got %d", txn.BalanceAfter)
	}
	if txn.Type != billing.TransactionTypeConsumption {
		t.Fatalf("expected type consumption, got %s", txn.Type)
	}
	if txn.RoomID == nil || *txn.RoomID != rm.ID {
		t.Fatalf("expected room_id %s, got %v", rm.ID, txn.RoomID)
	}

	bal, err := billingRepo.GetOrCreateBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 850 {
		t.Fatalf("expected persisted balance 850, got %d", bal.Balance)
	}
}

// TestBillingRepositoryCreditAndRecord proves CreditAndRecord increments the
// balance and inserts a matching row.
func TestBillingRepositoryCreditAndRecord(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	billingRepo := NewBillingRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "credit-owner")
	if _, err := billingRepo.GetOrCreateBalance(ctx, userID); err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}

	txn, err := billingRepo.CreditAndRecord(ctx, userID, billing.TransactionTypeAdjustment, 250, "manual correction")
	if err != nil {
		t.Fatalf("CreditAndRecord failed: %v", err)
	}
	if txn.Amount != 250 {
		t.Fatalf("expected signed amount 250, got %d", txn.Amount)
	}
	if txn.BalanceAfter != 250 {
		t.Fatalf("expected balance_after 250, got %d", txn.BalanceAfter)
	}
	if txn.Type != billing.TransactionTypeAdjustment {
		t.Fatalf("expected type adjustment, got %s", txn.Type)
	}
	if txn.RoomID != nil {
		t.Fatalf("expected nil room_id for a top-up-style transaction, got %v", txn.RoomID)
	}
}

// TestBillingRepositoryDebitAndRecordConcurrency proves concurrent
// DebitAndRecord calls against the same user never lose an update: the
// final balance equals the initial balance minus the sum of all debited
// amounts, mirroring the concurrency style of
// TestReserveSequenceRangeConcurrency.
func TestBillingRepositoryDebitAndRecordConcurrency(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	billingRepo := NewBillingRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "concurrency-owner")
	const initialBalance = 100000
	if _, err := billingRepo.GetOrCreateBalance(ctx, userID); err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if _, err := billingRepo.CreditAndRecord(ctx, userID, billing.TransactionTypeCharge, initialBalance, "seed"); err != nil {
		t.Fatalf("seed credit failed: %v", err)
	}

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "concurrency-room")

	const goroutines = 50
	const debitAmount = 100

	// Each goroutine debits against its own pre-existing message row, since
	// token_transactions.message_id is a real foreign key into messages.
	messageIDs := make([]string, goroutines)
	for i := range messageIDs {
		messageIDs[i] = seedMessage(ctx, t, msgRepo, rm.ID, userID).ID
	}

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := billingRepo.DebitAndRecord(ctx, userID, rm.ID, messageIDs[i], debitAmount, "concurrent debit"); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("DebitAndRecord returned an error under concurrency: %v", err)
	}

	bal, err := billingRepo.GetOrCreateBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	want := int64(initialBalance - goroutines*debitAmount)
	if bal.Balance != want {
		t.Fatalf("expected final balance %d (no lost updates), got %d", want, bal.Balance)
	}
}

// TestBillingRepositoryListTransactionsCursorPagination proves
// ListTransactions returns pages in created_at DESC, id DESC order with a
// correct NextCursor.
func TestBillingRepositoryListTransactionsCursorPagination(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	billingRepo := NewBillingRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "pagination-owner")
	if _, err := billingRepo.GetOrCreateBalance(ctx, userID); err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}

	const total = 5
	var seeded []*billing.TokenTransaction
	for i := 0; i < total; i++ {
		txn, err := billingRepo.CreditAndRecord(ctx, userID, billing.TransactionTypeCharge, 10, "topup")
		if err != nil {
			t.Fatalf("seed transaction %d: %v", i, err)
		}
		seeded = append(seeded, txn)
	}
	// Sort the captured (id, created_at) pairs by the exact same ORDER BY
	// ListTransactions itself uses (created_at DESC, id DESC), rather than
	// assuming insertion order is a reliable proxy for it: NOW() has only
	// microsecond resolution, so two transactions seeded in the same
	// transaction-commit tick can legitimately share a created_at, in which
	// case only the id DESC tie-break (not simple insertion-order reversal)
	// determines their relative order.
	sort.Slice(seeded, func(i, j int) bool {
		if seeded[i].CreatedAt.Equal(seeded[j].CreatedAt) {
			return seeded[i].ID > seeded[j].ID
		}
		return seeded[i].CreatedAt.After(seeded[j].CreatedAt)
	})
	wantIDs := make([]string, len(seeded))
	for i, txn := range seeded {
		wantIDs[i] = txn.ID
	}

	const pageSize = 2
	var gotIDs []string
	cursor := ""
	for {
		page, err := billingRepo.ListTransactions(ctx, userID, cursor, pageSize)
		if err != nil {
			t.Fatalf("ListTransactions failed: %v", err)
		}
		for _, txn := range page.Transactions {
			gotIDs = append(gotIDs, txn.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}

	if len(gotIDs) != total {
		t.Fatalf("expected %d transactions across all pages, got %d", total, len(gotIDs))
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected transaction order %v, got %v", wantIDs, gotIDs)
		}
	}
}
