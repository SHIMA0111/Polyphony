package billing

import (
	"context"
	"fmt"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// newTestUsecase builds a BillingUsecase over the given Step 42
// balance/room repos with every Step 49 dependency left at its zero value
// (nil subscription/payment repos and Stripe gateway, no configured
// plans/packages), for tests exercising only the Step 42
// balance/transaction-history behavior. Step 49-specific tests build their
// own BillingUsecase directly via NewBillingUsecase so they can supply
// mocks.SubscriptionRepo/mocks.PaymentRepo/mocks.StripeGateway and a plan/
// package catalog.
func newTestUsecase(balanceRepo domainbilling.BalanceRepository, roomRepo domainroom.RoomRepository) *BillingUsecase {
	return NewBillingUsecase(balanceRepo, roomRepo, nil, nil, nil, nil, nil, "", "")
}

func TestCheckBalancePositive(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	if err := roomRepo.Create(context.Background(), &domainroom.Room{ID: "room-1", OwnerID: "owner-1"}); err != nil {
		t.Fatalf("seed room: %v", err)
	}
	balanceRepo := &mocks.BalanceRepo{}
	balanceRepo.SeedBalance("owner-1", 100)

	uc := newTestUsecase(balanceRepo, roomRepo)
	if err := uc.CheckBalance(context.Background(), "room-1"); err != nil {
		t.Fatalf("expected nil error for positive balance, got %v", err)
	}
}

func TestCheckBalanceZeroOrNegative(t *testing.T) {
	for _, bal := range []int64{0, -5} {
		t.Run(fmt.Sprintf("balance=%d", bal), func(t *testing.T) {
			roomRepo := &mocks.RoomRepo{}
			if err := roomRepo.Create(context.Background(), &domainroom.Room{ID: "room-1", OwnerID: "owner-1"}); err != nil {
				t.Fatalf("seed room: %v", err)
			}
			balanceRepo := &mocks.BalanceRepo{}
			balanceRepo.SeedBalance("owner-1", bal)

			uc := newTestUsecase(balanceRepo, roomRepo)
			err := uc.CheckBalance(context.Background(), "room-1")
			if err != domain.ErrInsufficientBalance {
				t.Fatalf("expected ErrInsufficientBalance, got %v", err)
			}
		})
	}
}

func TestCheckBalanceRoomLookupFailurePropagates(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	balanceRepo := &mocks.BalanceRepo{}

	uc := newTestUsecase(balanceRepo, roomRepo)
	err := uc.CheckBalance(context.Background(), "nonexistent-room")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound to propagate from roomRepo.GetByID, got %v", err)
	}
}

func TestRecordUsageDebitsWithComputedTotal(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	if err := roomRepo.Create(context.Background(), &domainroom.Room{ID: "room-1", OwnerID: "owner-1"}); err != nil {
		t.Fatalf("seed room: %v", err)
	}
	balanceRepo := &mocks.BalanceRepo{}
	balanceRepo.SeedBalance("owner-1", 1000)

	uc := newTestUsecase(balanceRepo, roomRepo)
	if err := uc.RecordUsage(context.Background(), "room-1", "msg-1", "test-model", 30, 20); err != nil {
		t.Fatalf("RecordUsage failed: %v", err)
	}

	txns := balanceRepo.Transactions["owner-1"]
	if len(txns) != 1 {
		t.Fatalf("expected exactly 1 transaction recorded, got %d", len(txns))
	}
	if txns[0].Amount != -50 {
		t.Fatalf("expected debit amount -50 (30+20 total), got %d", txns[0].Amount)
	}
	if txns[0].RoomID == nil || *txns[0].RoomID != "room-1" {
		t.Fatalf("expected RoomID room-1, got %v", txns[0].RoomID)
	}

	bal, err := balanceRepo.GetOrCreateBalance(context.Background(), "owner-1")
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 950 {
		t.Fatalf("expected balance 950 after debiting 50 from 1000, got %d", bal.Balance)
	}
}

func TestRecordUsageNoOpWhenTotalNonPositive(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	if err := roomRepo.Create(context.Background(), &domainroom.Room{ID: "room-1", OwnerID: "owner-1"}); err != nil {
		t.Fatalf("seed room: %v", err)
	}
	balanceRepo := &mocks.BalanceRepo{}
	balanceRepo.SeedBalance("owner-1", 1000)

	uc := newTestUsecase(balanceRepo, roomRepo)
	if err := uc.RecordUsage(context.Background(), "room-1", "msg-1", "test-model", 0, 0); err != nil {
		t.Fatalf("expected nil error for zero usage, got %v", err)
	}

	if len(balanceRepo.Transactions["owner-1"]) != 0 {
		t.Fatalf("expected no transaction recorded for non-positive total, got %d", len(balanceRepo.Transactions["owner-1"]))
	}
}

func TestRecordUsageRoomLookupFailurePropagates(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	balanceRepo := &mocks.BalanceRepo{}

	uc := newTestUsecase(balanceRepo, roomRepo)
	err := uc.RecordUsage(context.Background(), "nonexistent-room", "msg-1", "test-model", 10, 10)
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound to propagate from roomRepo.GetByID, got %v", err)
	}
}

func TestGetBalanceDelegates(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	balanceRepo := &mocks.BalanceRepo{}
	balanceRepo.SeedBalance("user-1", 42)

	uc := newTestUsecase(balanceRepo, roomRepo)
	bal, err := uc.GetBalance(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if bal.Balance != 42 {
		t.Fatalf("expected balance 42, got %d", bal.Balance)
	}
}

func TestListTransactionsDelegatesAndClampsLimit(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	balanceRepo := &mocks.BalanceRepo{}
	balanceRepo.SeedBalance("user-1", 100)
	for i := 0; i < 5; i++ {
		if _, err := balanceRepo.CreditAndRecord(context.Background(), "user-1", "charge", 10, "topup"); err != nil {
			t.Fatalf("seed transaction: %v", err)
		}
	}

	uc := newTestUsecase(balanceRepo, roomRepo)

	// limit <= 0 defaults to 20
	page, err := uc.ListTransactions(context.Background(), "user-1", "", 0)
	if err != nil {
		t.Fatalf("ListTransactions failed: %v", err)
	}
	if len(page.Transactions) != 5 {
		t.Fatalf("expected 5 transactions (fewer than default limit 20), got %d", len(page.Transactions))
	}

	// limit > 100 clamps to 100 (still returns all 5 available)
	page, err = uc.ListTransactions(context.Background(), "user-1", "", 1000)
	if err != nil {
		t.Fatalf("ListTransactions failed: %v", err)
	}
	if len(page.Transactions) != 5 {
		t.Fatalf("expected 5 transactions, got %d", len(page.Transactions))
	}

	// explicit small limit is respected and returns a next cursor
	page, err = uc.ListTransactions(context.Background(), "user-1", "", 2)
	if err != nil {
		t.Fatalf("ListTransactions failed: %v", err)
	}
	if len(page.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(page.Transactions))
	}
	if page.NextCursor == nil {
		t.Fatal("expected a next cursor when more transactions remain")
	}
}
