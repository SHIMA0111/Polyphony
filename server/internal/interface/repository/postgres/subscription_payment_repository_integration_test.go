//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// seedSubscription builds and persists a minimal Subscription row for
// userID, returning it.
func seedSubscription(ctx context.Context, t *testing.T, subRepo *SubscriptionRepository, userID, stripeSubscriptionID string, monthlyTokenAllocation int64) *billing.Subscription {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Second)
	sub := &billing.Subscription{
		ID:                     uuid.New().String(),
		UserID:                 userID,
		StripeCustomerID:       "cus_" + stripeSubscriptionID,
		StripeSubscriptionID:   stripeSubscriptionID,
		StripePriceID:          "price_test",
		PlanCode:               "starter",
		Status:                 "active",
		MonthlyTokenAllocation: monthlyTokenAllocation,
		CurrentPeriodStart:     now,
		CurrentPeriodEnd:       now.AddDate(0, 1, 0),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	return sub
}

// TestSubscriptionRepositoryCreateAndGetByUserID proves Create persists a
// Subscription row retrievable by GetByUserID with every field intact.
func TestSubscriptionRepositoryCreateAndGetByUserID(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	subRepo := NewSubscriptionRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "sub-create")
	seeded := seedSubscription(ctx, t, subRepo, userID, "sub_create_1", 100000)

	got, err := subRepo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if got.ID != seeded.ID || got.StripeSubscriptionID != "sub_create_1" || got.PlanCode != "starter" {
		t.Fatalf("expected persisted subscription to match seeded values, got %+v", got)
	}
	if got.MonthlyTokenAllocation != 100000 {
		t.Fatalf("expected monthly_token_allocation 100000, got %d", got.MonthlyTokenAllocation)
	}
	if got.CancelAtPeriodEnd {
		t.Fatalf("expected cancel_at_period_end false by default, got true")
	}
	if got.CanceledAt != nil {
		t.Fatalf("expected canceled_at nil, got %v", got.CanceledAt)
	}
}

// TestSubscriptionRepositoryGetByUserIDNotFound proves GetByUserID returns
// domain.ErrNotFound for a user with no subscription row.
func TestSubscriptionRepositoryGetByUserIDNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	subRepo := NewSubscriptionRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "sub-not-found")

	if _, err := subRepo.GetByUserID(ctx, userID); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestSubscriptionRepositoryGetByStripeSubscriptionID proves the
// Stripe-subscription-ID lookup path webhook processing relies on.
func TestSubscriptionRepositoryGetByStripeSubscriptionID(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	subRepo := NewSubscriptionRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "sub-by-stripe-id")
	seeded := seedSubscription(ctx, t, subRepo, userID, "sub_lookup_1", 50000)

	got, err := subRepo.GetByStripeSubscriptionID(ctx, "sub_lookup_1")
	if err != nil {
		t.Fatalf("GetByStripeSubscriptionID failed: %v", err)
	}
	if got.ID != seeded.ID {
		t.Fatalf("expected subscription %s, got %s", seeded.ID, got.ID)
	}

	if _, err := subRepo.GetByStripeSubscriptionID(ctx, "sub_does_not_exist"); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for an unknown stripe subscription id, got %v", err)
	}
}

// TestSubscriptionRepositoryUpdate proves Update persists status/period/
// cancellation field changes, mirroring the fields
// BillingUsecase.CancelSubscription and the customer.subscription.updated/
// .deleted webhook handlers mutate.
func TestSubscriptionRepositoryUpdate(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	subRepo := NewSubscriptionRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "sub-update")
	sub := seedSubscription(ctx, t, subRepo, userID, "sub_update_1", 100000)

	sub.Status = "canceled"
	sub.CancelAtPeriodEnd = true
	canceledAt := time.Now().UTC().Truncate(time.Second)
	sub.CanceledAt = &canceledAt

	if err := subRepo.Update(ctx, sub); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := subRepo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if got.Status != "canceled" || !got.CancelAtPeriodEnd {
		t.Fatalf("expected status=canceled, cancel_at_period_end=true, got %+v", got)
	}
	if got.CanceledAt == nil || !got.CanceledAt.Equal(canceledAt) {
		t.Fatalf("expected canceled_at %v, got %v", canceledAt, got.CanceledAt)
	}
}

// TestSubscriptionRepositoryStripeCheckoutSessionIDRoundTrip proves that
// stripe_checkout_session_id defaults to "" for a Subscription created
// without it (a row predating the field, per its NOT NULL DEFAULT ''
// schema.sql column), and that Update persists a new value for it —
// mirroring BillingUsecase.upsertSubscriptionFromCheckout stamping this
// field on both its create and update paths.
func TestSubscriptionRepositoryStripeCheckoutSessionIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	subRepo := NewSubscriptionRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "sub-checkout-session-id")
	sub := seedSubscription(ctx, t, subRepo, userID, "sub_checkout_session_1", 100000)

	got, err := subRepo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if got.StripeCheckoutSessionID != "" {
		t.Fatalf("expected stripe_checkout_session_id to default to \"\", got %q", got.StripeCheckoutSessionID)
	}

	sub.StripeCheckoutSessionID = "cs_round_trip"
	if err := subRepo.Update(ctx, sub); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err = subRepo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID after update failed: %v", err)
	}
	if got.StripeCheckoutSessionID != "cs_round_trip" {
		t.Fatalf("expected stripe_checkout_session_id %q, got %q", "cs_round_trip", got.StripeCheckoutSessionID)
	}
}

// TestSubscriptionRepositoryUpdateNotFound proves Update returns
// domain.ErrNotFound for an ID with no matching row.
func TestSubscriptionRepositoryUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	subRepo := NewSubscriptionRepository(pool)

	err := subRepo.Update(ctx, &billing.Subscription{ID: uuid.New().String()})
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestPaymentRepositoryCreateIdempotentOnStripeEventID proves Create's
// idempotency contract directly: inserting two PaymentRecords with the same
// StripeEventID succeeds once and reports alreadyRecorded=true on the
// second attempt, without erroring.
func TestPaymentRepositoryCreateIdempotentOnStripeEventID(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	paymentRepo := NewPaymentRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "payment-create-idempotent")

	payment := &billing.PaymentRecord{
		ID: uuid.New().String(), UserID: userID, StripeEventID: "evt_create_idempotent_1",
		Kind: billing.PaymentKindTokenPurchase, AmountCents: 300, Currency: "usd",
		TokensCredited: 50000, Status: "succeeded",
	}

	alreadyRecorded, err := paymentRepo.Create(ctx, payment)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	if alreadyRecorded {
		t.Fatal("expected alreadyRecorded=false on the first insert")
	}

	// Re-insert an equivalent row with a fresh ID but the same StripeEventID.
	replay := &billing.PaymentRecord{
		ID: uuid.New().String(), UserID: userID, StripeEventID: "evt_create_idempotent_1",
		Kind: billing.PaymentKindTokenPurchase, AmountCents: 300, Currency: "usd",
		TokensCredited: 50000, Status: "succeeded",
	}
	alreadyRecorded, err = paymentRepo.Create(ctx, replay)
	if err != nil {
		t.Fatalf("replayed Create returned an error instead of alreadyRecorded=true: %v", err)
	}
	if !alreadyRecorded {
		t.Fatal("expected alreadyRecorded=true on the replayed insert")
	}

	page, err := paymentRepo.ListByUserID(ctx, userID, "", 10)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if len(page.Payments) != 1 {
		t.Fatalf("expected exactly 1 payment_history row despite the replay, got %d", len(page.Payments))
	}
}

// TestPaymentRepositoryCreateAndCreditCreditsBalanceAndIsIdempotent proves
// CreateAndCredit both inserts the payment_history row and credits
// token_balances atomically, and that replaying the identical
// StripeEventID a second time is a no-op on the balance (Stripe's
// at-least-once webhook delivery guarantee).
func TestPaymentRepositoryCreateAndCreditCreditsBalanceAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	billingRepo := NewBillingRepository(pool)
	paymentRepo := NewPaymentRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "payment-create-and-credit")
	if _, err := billingRepo.GetOrCreateBalance(ctx, userID); err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}

	payment := &billing.PaymentRecord{
		ID: uuid.New().String(), UserID: userID, StripeEventID: "evt_credit_1",
		StripeReferenceID: "cs_1", Kind: billing.PaymentKindTokenPurchase, AmountCents: 300, Currency: "usd",
		TokensCredited: 50000, Status: "succeeded",
	}
	alreadyProcessed, err := paymentRepo.CreateAndCredit(ctx, payment, userID, 50000, "token purchase")
	if err != nil {
		t.Fatalf("CreateAndCredit failed: %v", err)
	}
	if alreadyProcessed {
		t.Fatal("expected alreadyProcessed=false on the first delivery")
	}

	bal, err := billingRepo.GetOrCreateBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 50000 {
		t.Fatalf("expected balance credited by 50000, got %d", bal.Balance)
	}

	// Redeliver the identical event.
	replay := &billing.PaymentRecord{
		ID: uuid.New().String(), UserID: userID, StripeEventID: "evt_credit_1",
		StripeReferenceID: "cs_1", Kind: billing.PaymentKindTokenPurchase, AmountCents: 300, Currency: "usd",
		TokensCredited: 50000, Status: "succeeded",
	}
	alreadyProcessed, err = paymentRepo.CreateAndCredit(ctx, replay, userID, 50000, "token purchase (replay)")
	if err != nil {
		t.Fatalf("replayed CreateAndCredit returned an error instead of alreadyProcessed=true: %v", err)
	}
	if !alreadyProcessed {
		t.Fatal("expected alreadyProcessed=true on the replayed delivery")
	}

	bal, err = billingRepo.GetOrCreateBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 50000 {
		t.Fatalf("expected balance to remain 50000 (not double-credited) after the replay, got %d", bal.Balance)
	}

	page, err := paymentRepo.ListByUserID(ctx, userID, "", 10)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if len(page.Payments) != 1 {
		t.Fatalf("expected exactly 1 payment_history row despite the replay, got %d", len(page.Payments))
	}
}

// TestPaymentRepositoryListByUserIDCursorPagination proves ListByUserID
// returns pages in created_at DESC, id DESC order with a correct
// NextCursor, mirroring TestBillingRepositoryListTransactionsCursorPagination.
func TestPaymentRepositoryListByUserIDCursorPagination(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)
	billingRepo := NewBillingRepository(pool)
	paymentRepo := NewPaymentRepository(pool)

	userID := seedBillingUser(ctx, t, userRepo, "payment-pagination")
	if _, err := billingRepo.GetOrCreateBalance(ctx, userID); err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}

	const total = 5
	var wantIDs []string
	for i := 0; i < total; i++ {
		payment := &billing.PaymentRecord{
			ID: uuid.New().String(), UserID: userID, StripeEventID: uuid.New().String(),
			Kind: billing.PaymentKindTokenPurchase, AmountCents: 100, Currency: "usd",
			TokensCredited: 1000, Status: "succeeded",
		}
		if _, err := paymentRepo.CreateAndCredit(ctx, payment, userID, 1000, "topup"); err != nil {
			t.Fatalf("seed payment %d: %v", i, err)
		}
		wantIDs = append(wantIDs, payment.ID)
	}
	// wantIDs is oldest-first; ListByUserID returns newest-first.
	for i, j := 0, len(wantIDs)-1; i < j; i, j = i+1, j-1 {
		wantIDs[i], wantIDs[j] = wantIDs[j], wantIDs[i]
	}

	const pageSize = 2
	var gotIDs []string
	cursor := ""
	for {
		page, err := paymentRepo.ListByUserID(ctx, userID, cursor, pageSize)
		if err != nil {
			t.Fatalf("ListByUserID failed: %v", err)
		}
		for _, p := range page.Payments {
			gotIDs = append(gotIDs, p.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}

	if len(gotIDs) != total {
		t.Fatalf("expected %d payments across all pages, got %d", total, len(gotIDs))
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("expected payment order %v, got %v", wantIDs, gotIDs)
		}
	}
}
