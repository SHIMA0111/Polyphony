package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// testPlans/testPackages are a small fixed catalog shared by the Step 49
// tests below.
func testPlans() []domainbilling.Plan {
	return []domainbilling.Plan{
		{Code: "starter", StripePriceID: "price_starter", Name: "Starter", Description: "100K tokens/month",
			PriceCents: 500, Currency: "usd", MonthlyTokenAllocation: 100000},
	}
}

func testPackages() []domainbilling.TokenPackage {
	return []domainbilling.TokenPackage{
		{Code: "topup_small", StripePriceID: "price_topup_small", Name: "Small top-up", Description: "50K tokens",
			PriceCents: 300, Currency: "usd", Tokens: 50000},
	}
}

// newStripeTestUsecase builds a BillingUsecase with every Step 49
// dependency wired to a fresh in-memory mock, for tests exercising
// checkout/subscription/webhook behavior. gw is typed as the
// domainbilling.StripeGateway interface (rather than *mocks.StripeGateway)
// so that callers testing the "Stripe unconfigured" path can pass a literal
// nil and get a true nil interface — passing a nil *mocks.StripeGateway
// instead would produce a non-nil interface wrapping a nil pointer, which
// would defeat BillingUsecase's `stripeGateway == nil` check.
func newStripeTestUsecase(gw domainbilling.StripeGateway) (*BillingUsecase, *mocks.BalanceRepo, *mocks.SubscriptionRepo, *mocks.PaymentRepo) {
	balanceRepo := &mocks.BalanceRepo{}
	roomRepo := &mocks.RoomRepo{}
	subRepo := &mocks.SubscriptionRepo{}
	paymentRepo := &mocks.PaymentRepo{BalanceRepo: balanceRepo}

	uc := NewBillingUsecase(balanceRepo, roomRepo, subRepo, paymentRepo, gw,
		testPlans(), testPackages(), "https://example.com/success", "https://example.com/cancel")
	return uc, balanceRepo, subRepo, paymentRepo
}

func TestCreateSubscriptionCheckoutSessionUnknownPlan(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(&mocks.StripeGateway{})
	_, err := uc.CreateSubscriptionCheckoutSession(context.Background(), "user-1", "does-not-exist")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown plan code, got %v", err)
	}
}

func TestCreateSubscriptionCheckoutSessionSuccess(t *testing.T) {
	gw := &mocks.StripeGateway{CheckoutURL: "https://checkout.stripe.com/session-1"}
	uc, _, _, _ := newStripeTestUsecase(gw)

	url, err := uc.CreateSubscriptionCheckoutSession(context.Background(), "user-1", "starter")
	if err != nil {
		t.Fatalf("CreateSubscriptionCheckoutSession failed: %v", err)
	}
	if url != gw.CheckoutURL {
		t.Fatalf("expected checkout url %s, got %s", gw.CheckoutURL, url)
	}
	if gw.LastSubscriptionCheckoutParams == nil {
		t.Fatal("expected gateway to be called")
	}
	if gw.LastSubscriptionCheckoutParams.PriceID != "price_starter" {
		t.Fatalf("expected price_starter, got %s", gw.LastSubscriptionCheckoutParams.PriceID)
	}
	if gw.LastSubscriptionCheckoutParams.UserID != "user-1" {
		t.Fatalf("expected user-1, got %s", gw.LastSubscriptionCheckoutParams.UserID)
	}
}

func TestCreateSubscriptionCheckoutSessionStripeNotConfigured(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(nil)
	_, err := uc.CreateSubscriptionCheckoutSession(context.Background(), "user-1", "starter")
	if !errors.Is(err, domain.ErrStripeNotConfigured) {
		t.Fatalf("expected ErrStripeNotConfigured, got %v", err)
	}
}

func TestCreateTokenPurchaseCheckoutSessionUnknownPackage(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(&mocks.StripeGateway{})
	_, err := uc.CreateTokenPurchaseCheckoutSession(context.Background(), "user-1", "does-not-exist")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown package code, got %v", err)
	}
}

func TestCreateTokenPurchaseCheckoutSessionSuccess(t *testing.T) {
	gw := &mocks.StripeGateway{CheckoutURL: "https://checkout.stripe.com/session-2"}
	uc, _, _, _ := newStripeTestUsecase(gw)

	url, err := uc.CreateTokenPurchaseCheckoutSession(context.Background(), "user-1", "topup_small")
	if err != nil {
		t.Fatalf("CreateTokenPurchaseCheckoutSession failed: %v", err)
	}
	if url != gw.CheckoutURL {
		t.Fatalf("expected checkout url %s, got %s", gw.CheckoutURL, url)
	}
	if gw.LastTokenPurchaseCheckoutParams == nil || gw.LastTokenPurchaseCheckoutParams.PriceID != "price_topup_small" {
		t.Fatalf("expected price_topup_small, got %+v", gw.LastTokenPurchaseCheckoutParams)
	}
}

func TestGetSubscriptionNotFound(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(&mocks.StripeGateway{})
	_, err := uc.GetSubscription(context.Background(), "user-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCancelSubscriptionNoSubscription(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(&mocks.StripeGateway{})
	_, err := uc.CancelSubscription(context.Background(), "user-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCancelSubscriptionSuccess(t *testing.T) {
	gw := &mocks.StripeGateway{}
	uc, _, subRepo, _ := newStripeTestUsecase(gw)

	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
		StripePriceID: "price_starter", PlanCode: "starter", Status: "active", MonthlyTokenAllocation: 100000,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	sub, err := uc.CancelSubscription(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("CancelSubscription failed: %v", err)
	}
	if !sub.CancelAtPeriodEnd {
		t.Fatal("expected CancelAtPeriodEnd to be true")
	}
	if gw.LastCanceledStripeSubscriptionID != "sub_1" {
		t.Fatalf("expected gateway called with sub_1, got %s", gw.LastCanceledStripeSubscriptionID)
	}

	persisted, err := subRepo.GetByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if !persisted.CancelAtPeriodEnd {
		t.Fatal("expected persisted subscription to have CancelAtPeriodEnd=true")
	}
}

func TestCancelSubscriptionStripeNotConfigured(t *testing.T) {
	uc, _, subRepo, _ := newStripeTestUsecase(nil)
	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeSubscriptionID: "sub_1",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	_, err := uc.CancelSubscription(context.Background(), "user-1")
	if !errors.Is(err, domain.ErrStripeNotConfigured) {
		t.Fatalf("expected ErrStripeNotConfigured, got %v", err)
	}
}

func TestCreateBillingPortalSessionNoSubscription(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(&mocks.StripeGateway{})
	_, err := uc.CreateBillingPortalSession(context.Background(), "user-1", "https://example.com/return")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateBillingPortalSessionSuccess(t *testing.T) {
	gw := &mocks.StripeGateway{PortalURL: "https://billing.stripe.com/portal-1"}
	uc, _, subRepo, _ := newStripeTestUsecase(gw)
	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	url, err := uc.CreateBillingPortalSession(context.Background(), "user-1", "https://example.com/return")
	if err != nil {
		t.Fatalf("CreateBillingPortalSession failed: %v", err)
	}
	if url != gw.PortalURL {
		t.Fatalf("expected portal url %s, got %s", gw.PortalURL, url)
	}
	if gw.LastPortalStripeCustomerID != "cus_1" {
		t.Fatalf("expected cus_1, got %s", gw.LastPortalStripeCustomerID)
	}
}

func TestListPlanCatalogMergesPlansAndPackages(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(&mocks.StripeGateway{})
	entries := uc.ListPlanCatalog()
	if len(entries) != 2 {
		t.Fatalf("expected 2 catalog entries (1 plan + 1 package), got %d", len(entries))
	}

	var plan, pkg *domainbilling.PlanCatalogEntry
	for i := range entries {
		switch entries[i].Code {
		case "starter":
			plan = &entries[i]
		case "topup_small":
			pkg = &entries[i]
		}
	}
	if plan == nil || plan.Interval != "month" || plan.TokenAllowance != 100000 {
		t.Fatalf("expected starter plan with interval=month, token_allowance=100000, got %+v", plan)
	}
	if pkg == nil || pkg.Interval != "one_time" || pkg.TokenAllowance != 50000 {
		t.Fatalf("expected topup_small package with interval=one_time, token_allowance=50000, got %+v", pkg)
	}
}

func TestListPlanCatalogEmptyWhenUnconfigured(t *testing.T) {
	balanceRepo := &mocks.BalanceRepo{}
	roomRepo := &mocks.RoomRepo{}
	uc := NewBillingUsecase(balanceRepo, roomRepo, nil, nil, nil, nil, nil, "", "")
	entries := uc.ListPlanCatalog()
	if len(entries) != 0 {
		t.Fatalf("expected an empty catalog, got %d entries", len(entries))
	}
}

// --- HandleWebhookEvent ---

func TestHandleWebhookEventStripeNotConfigured(t *testing.T) {
	uc, _, _, _ := newStripeTestUsecase(nil)
	err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig")
	if !errors.Is(err, domain.ErrStripeNotConfigured) {
		t.Fatalf("expected ErrStripeNotConfigured, got %v", err)
	}
}

func TestHandleWebhookEventSignatureFailure(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookErr: domain.ErrInvalidWebhookSignature}
	uc, _, _, _ := newStripeTestUsecase(gw)

	err := uc.HandleWebhookEvent(context.Background(), []byte("tampered"), "bad-sig")
	if !errors.Is(err, domain.ErrInvalidWebhookSignature) {
		t.Fatalf("expected ErrInvalidWebhookSignature, got %v", err)
	}
}

func TestHandleWebhookEventUnrecognizedTypeNoOp(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{ID: "evt_1", Type: "customer.created"}}
	uc, _, _, _ := newStripeTestUsecase(gw)

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("expected nil error (safe no-op) for an unrecognized event type, got %v", err)
	}
}

func TestHandleWebhookEventCheckoutSessionCompletedTokenPurchaseCreditsBalance(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_token_purchase_1",
		Type: domainbilling.EventTypeCheckoutSessionCompleted,
		CheckoutSession: &domainbilling.CheckoutSessionData{
			SessionID: "cs_1", Mode: domainbilling.CheckoutModePayment, Kind: "token_purchase",
			UserID: "user-1", PackageCode: "topup_small", AmountTotal: 300, Currency: "usd",
		},
	}}
	uc, balanceRepo, _, paymentRepo := newStripeTestUsecase(gw)

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhookEvent failed: %v", err)
	}

	bal, err := balanceRepo.GetOrCreateBalance(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 50000 {
		t.Fatalf("expected balance credited by 50000 tokens, got %d", bal.Balance)
	}

	page, err := paymentRepo.ListByUserID(context.Background(), "user-1", "", 10)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if len(page.Payments) != 1 {
		t.Fatalf("expected exactly 1 payment_history row, got %d", len(page.Payments))
	}
	if page.Payments[0].Kind != domainbilling.PaymentKindTokenPurchase || page.Payments[0].TokensCredited != 50000 {
		t.Fatalf("expected a token_purchase payment crediting 50000 tokens, got %+v", page.Payments[0])
	}
}

func TestHandleWebhookEventReplayIsIdempotentNoOp(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_replay_1",
		Type: domainbilling.EventTypeCheckoutSessionCompleted,
		CheckoutSession: &domainbilling.CheckoutSessionData{
			SessionID: "cs_1", Mode: domainbilling.CheckoutModePayment, Kind: "token_purchase",
			UserID: "user-1", PackageCode: "topup_small", AmountTotal: 300, Currency: "usd",
		},
	}}
	uc, balanceRepo, _, paymentRepo := newStripeTestUsecase(gw)

	// Deliver the identical event twice, simulating Stripe's at-least-once
	// webhook redelivery.
	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("first HandleWebhookEvent failed: %v", err)
	}
	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("second (replayed) HandleWebhookEvent failed: %v", err)
	}

	bal, err := balanceRepo.GetOrCreateBalance(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 50000 {
		t.Fatalf("expected balance credited exactly once (50000), got %d — replay was not idempotent", bal.Balance)
	}

	page, err := paymentRepo.ListByUserID(context.Background(), "user-1", "", 10)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if len(page.Payments) != 1 {
		t.Fatalf("expected exactly 1 payment_history row despite the replay, got %d", len(page.Payments))
	}
}

func TestHandleWebhookEventInvoicePaidCreditsSubscriptionAllocation(t *testing.T) {
	gw := &mocks.StripeGateway{}
	uc, balanceRepo, subRepo, paymentRepo := newStripeTestUsecase(gw)

	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
		StripePriceID: "price_starter", PlanCode: "starter", Status: "active", MonthlyTokenAllocation: 100000,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	gw.WebhookEvent = domainbilling.WebhookEvent{
		ID:   "evt_invoice_1",
		Type: domainbilling.EventTypeInvoicePaid,
		Invoice: &domainbilling.InvoiceData{
			InvoiceID: "in_1", StripeSubscriptionID: "sub_1", BillingReason: "subscription_cycle",
			AmountPaid: 500, Currency: "usd",
		},
	}

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhookEvent failed: %v", err)
	}

	bal, err := balanceRepo.GetOrCreateBalance(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 100000 {
		t.Fatalf("expected balance credited by 100000 (the plan's monthly allocation), got %d", bal.Balance)
	}

	page, err := paymentRepo.ListByUserID(context.Background(), "user-1", "", 10)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if len(page.Payments) != 1 || page.Payments[0].Kind != domainbilling.PaymentKindSubscription {
		t.Fatalf("expected exactly 1 subscription-kind payment row, got %+v", page.Payments)
	}
}

func TestHandleWebhookEventInvoicePaidIgnoresOtherBillingReasons(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_invoice_2",
		Type: domainbilling.EventTypeInvoicePaid,
		Invoice: &domainbilling.InvoiceData{
			InvoiceID: "in_2", StripeSubscriptionID: "sub_1", BillingReason: "manual",
			AmountPaid: 500, Currency: "usd",
		},
	}}
	uc, _, _, paymentRepo := newStripeTestUsecase(gw)

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("expected nil error (safe no-op) for a non-renewal billing_reason, got %v", err)
	}

	page, err := paymentRepo.ListByUserID(context.Background(), "user-1", "", 10)
	if err != nil {
		t.Fatalf("ListByUserID failed: %v", err)
	}
	if len(page.Payments) != 0 {
		t.Fatalf("expected no payment_history row, got %d", len(page.Payments))
	}
}

func TestHandleWebhookEventSubscriptionUpdated(t *testing.T) {
	gw := &mocks.StripeGateway{}
	uc, _, subRepo, _ := newStripeTestUsecase(gw)

	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeSubscriptionID: "sub_1", Status: "active",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	periodEnd := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	gw.WebhookEvent = domainbilling.WebhookEvent{
		ID:   "evt_sub_updated_1",
		Type: domainbilling.EventTypeSubscriptionUpdated,
		Subscription: &domainbilling.SubscriptionEventData{
			StripeSubscriptionID: "sub_1", Status: "past_due", CancelAtPeriodEnd: true, CurrentPeriodEnd: periodEnd,
		},
	}

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhookEvent failed: %v", err)
	}

	sub, err := subRepo.GetByStripeSubscriptionID(context.Background(), "sub_1")
	if err != nil {
		t.Fatalf("GetByStripeSubscriptionID failed: %v", err)
	}
	if sub.Status != "past_due" || !sub.CancelAtPeriodEnd {
		t.Fatalf("expected status=past_due, cancel_at_period_end=true, got %+v", sub)
	}
	if !sub.CurrentPeriodEnd.Equal(periodEnd) {
		t.Fatalf("expected current_period_end %v, got %v", periodEnd, sub.CurrentPeriodEnd)
	}
}

// TestHandleWebhookEventSubscriptionUpdatedPlanChangeResyncsEntitlements
// asserts that a customer.subscription.updated event reporting a new Stripe
// price (a Billing Portal plan change) resyncs StripePriceID, PlanCode, and
// MonthlyTokenAllocation together, so the next invoice.paid credits the new
// plan's allocation rather than the stale one it was created with.
func TestHandleWebhookEventSubscriptionUpdatedPlanChangeResyncsEntitlements(t *testing.T) {
	plans := []domainbilling.Plan{
		{Code: "starter", StripePriceID: "price_starter", Name: "Starter",
			PriceCents: 500, Currency: "usd", MonthlyTokenAllocation: 100000},
		{Code: "pro", StripePriceID: "price_pro", Name: "Pro",
			PriceCents: 2000, Currency: "usd", MonthlyTokenAllocation: 500000},
	}
	balanceRepo := &mocks.BalanceRepo{}
	roomRepo := &mocks.RoomRepo{}
	subRepo := &mocks.SubscriptionRepo{}
	paymentRepo := &mocks.PaymentRepo{BalanceRepo: balanceRepo}
	gw := &mocks.StripeGateway{}
	uc := NewBillingUsecase(balanceRepo, roomRepo, subRepo, paymentRepo, gw,
		plans, nil, "https://example.com/success", "https://example.com/cancel")

	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
		StripePriceID: "price_starter", PlanCode: "starter", Status: "active", MonthlyTokenAllocation: 100000,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	gw.WebhookEvent = domainbilling.WebhookEvent{
		ID:   "evt_sub_plan_change_1",
		Type: domainbilling.EventTypeSubscriptionUpdated,
		Subscription: &domainbilling.SubscriptionEventData{
			StripeSubscriptionID: "sub_1", Status: "active", StripePriceID: "price_pro",
		},
	}

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhookEvent failed: %v", err)
	}

	sub, err := subRepo.GetByStripeSubscriptionID(context.Background(), "sub_1")
	if err != nil {
		t.Fatalf("GetByStripeSubscriptionID failed: %v", err)
	}
	if sub.StripePriceID != "price_pro" || sub.PlanCode != "pro" || sub.MonthlyTokenAllocation != 500000 {
		t.Fatalf("expected resync to price_pro/pro/500000, got %+v", sub)
	}
}

// TestHandleWebhookEventSubscriptionUpdatedUnrecognizedPriceSkipsResync
// asserts that a customer.subscription.updated event reporting a Stripe
// price not present in the configured plan catalog leaves
// StripePriceID/PlanCode/MonthlyTokenAllocation untouched (warn-and-skip),
// rather than desyncing StripePriceID from PlanCode/MonthlyTokenAllocation.
func TestHandleWebhookEventSubscriptionUpdatedUnrecognizedPriceSkipsResync(t *testing.T) {
	gw := &mocks.StripeGateway{}
	uc, _, subRepo, _ := newStripeTestUsecase(gw)

	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
		StripePriceID: "price_starter", PlanCode: "starter", Status: "active", MonthlyTokenAllocation: 100000,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	gw.WebhookEvent = domainbilling.WebhookEvent{
		ID:   "evt_sub_plan_change_unrecognized",
		Type: domainbilling.EventTypeSubscriptionUpdated,
		Subscription: &domainbilling.SubscriptionEventData{
			StripeSubscriptionID: "sub_1", Status: "active", StripePriceID: "price_does_not_exist",
		},
	}

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhookEvent failed: %v", err)
	}

	sub, err := subRepo.GetByStripeSubscriptionID(context.Background(), "sub_1")
	if err != nil {
		t.Fatalf("GetByStripeSubscriptionID failed: %v", err)
	}
	if sub.StripePriceID != "price_starter" || sub.PlanCode != "starter" || sub.MonthlyTokenAllocation != 100000 {
		t.Fatalf("expected unrecognized price to leave plan/entitlements unchanged, got %+v", sub)
	}
	// Status should still update even when the plan resync is skipped.
	if sub.Status != "active" {
		t.Fatalf("expected status to still sync, got %s", sub.Status)
	}
}

// TestHandleWebhookEventDispatchNilRepoReturnsBillingNotConfigured asserts
// that each dispatched handler guards its required repositories and returns
// domain.ErrBillingNotConfigured (rather than panicking on a nil dereference)
// when they are unset.
func TestHandleWebhookEventDispatchNilRepoReturnsBillingNotConfigured(t *testing.T) {
	tests := []struct {
		name  string
		event domainbilling.WebhookEvent
	}{
		{
			name: "checkout_session_completed_token_purchase",
			event: domainbilling.WebhookEvent{
				ID: "evt_nil_1", Type: domainbilling.EventTypeCheckoutSessionCompleted,
				CheckoutSession: &domainbilling.CheckoutSessionData{
					SessionID: "cs_1", Mode: domainbilling.CheckoutModePayment, Kind: "token_purchase",
					UserID: "user-1", PackageCode: "topup_small", AmountTotal: 300, Currency: "usd",
				},
			},
		},
		{
			name: "checkout_session_completed_subscription",
			event: domainbilling.WebhookEvent{
				ID: "evt_nil_2", Type: domainbilling.EventTypeCheckoutSessionCompleted,
				CheckoutSession: &domainbilling.CheckoutSessionData{
					SessionID: "cs_2", Mode: domainbilling.CheckoutModeSubscription,
					UserID: "user-1", PlanCode: "starter", StripeSubscriptionID: "sub_1", StripeCustomerID: "cus_1",
				},
			},
		},
		{
			name: "invoice_paid",
			event: domainbilling.WebhookEvent{
				ID: "evt_nil_3", Type: domainbilling.EventTypeInvoicePaid,
				Invoice: &domainbilling.InvoiceData{
					InvoiceID: "in_1", StripeSubscriptionID: "sub_1", BillingReason: "subscription_cycle",
					AmountPaid: 500, Currency: "usd",
				},
			},
		},
		{
			name: "subscription_updated",
			event: domainbilling.WebhookEvent{
				ID: "evt_nil_4", Type: domainbilling.EventTypeSubscriptionUpdated,
				Subscription: &domainbilling.SubscriptionEventData{StripeSubscriptionID: "sub_1", Status: "active"},
			},
		},
		{
			name: "subscription_deleted",
			event: domainbilling.WebhookEvent{
				ID: "evt_nil_5", Type: domainbilling.EventTypeSubscriptionDeleted,
				Subscription: &domainbilling.SubscriptionEventData{StripeSubscriptionID: "sub_1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &mocks.StripeGateway{WebhookEvent: tt.event}
			// balanceRepo, subscriptionRepo, and paymentRepo are all nil —
			// only stripeGateway and the plan catalog are wired.
			uc := NewBillingUsecase(nil, &mocks.RoomRepo{}, nil, nil, gw,
				testPlans(), testPackages(), "https://example.com/success", "https://example.com/cancel")

			err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig")
			if !errors.Is(err, domain.ErrBillingNotConfigured) {
				t.Fatalf("expected domain.ErrBillingNotConfigured, got %v", err)
			}
		})
	}
}

// TestHandleWebhookEventInvoicePaidUnrecognizedSubscriptionReturnsError
// asserts that an invoice.paid event for a subscription this server has no
// local record of returns a non-nil (not-found) error rather than a silent
// no-op, so the webhook handler responds non-2xx and Stripe redelivers
// until the corresponding checkout.session.completed has landed.
func TestHandleWebhookEventInvoicePaidUnrecognizedSubscriptionReturnsError(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_invoice_unrecognized",
		Type: domainbilling.EventTypeInvoicePaid,
		Invoice: &domainbilling.InvoiceData{
			InvoiceID: "in_3", StripeSubscriptionID: "sub_does_not_exist", BillingReason: "subscription_cycle",
			AmountPaid: 500, Currency: "usd",
		},
	}}
	uc, _, _, _ := newStripeTestUsecase(gw)

	err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig")
	if err == nil {
		t.Fatal("expected a non-nil error for an unrecognized subscription, got nil (event would be ACKed and dropped)")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the error to wrap domain.ErrNotFound, got %v", err)
	}
}

// TestHandleWebhookEventSubscriptionUpdatedUnrecognizedSubscriptionReturnsError
// asserts that a customer.subscription.updated event for a subscription
// this server has no local record of returns a non-nil (not-found) error
// rather than a silent no-op, for the same redelivery reason as the
// invoice.paid case above.
func TestHandleWebhookEventSubscriptionUpdatedUnrecognizedSubscriptionReturnsError(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_sub_updated_unrecognized",
		Type: domainbilling.EventTypeSubscriptionUpdated,
		Subscription: &domainbilling.SubscriptionEventData{
			StripeSubscriptionID: "sub_does_not_exist", Status: "past_due",
		},
	}}
	uc, _, _, _ := newStripeTestUsecase(gw)

	err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig")
	if err == nil {
		t.Fatal("expected a non-nil error for an unrecognized subscription, got nil (event would be ACKed and dropped)")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the error to wrap domain.ErrNotFound, got %v", err)
	}
}

// TestHandleWebhookEventSubscriptionDeletedUnrecognizedSubscriptionReturnsError
// asserts that a customer.subscription.deleted event for a subscription
// this server has no local record of returns a non-nil (not-found) error
// rather than a silent no-op, for the same redelivery reason as the
// invoice.paid case above.
func TestHandleWebhookEventSubscriptionDeletedUnrecognizedSubscriptionReturnsError(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_sub_deleted_unrecognized",
		Type: domainbilling.EventTypeSubscriptionDeleted,
		Subscription: &domainbilling.SubscriptionEventData{
			StripeSubscriptionID: "sub_does_not_exist",
		},
	}}
	uc, _, _, _ := newStripeTestUsecase(gw)

	err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig")
	if err == nil {
		t.Fatal("expected a non-nil error for an unrecognized subscription, got nil (event would be ACKed and dropped)")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the error to wrap domain.ErrNotFound, got %v", err)
	}
}

func TestHandleWebhookEventSubscriptionDeleted(t *testing.T) {
	gw := &mocks.StripeGateway{}
	uc, _, subRepo, _ := newStripeTestUsecase(gw)

	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeSubscriptionID: "sub_1", Status: "active",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	gw.WebhookEvent = domainbilling.WebhookEvent{
		ID:   "evt_sub_deleted_1",
		Type: domainbilling.EventTypeSubscriptionDeleted,
		Subscription: &domainbilling.SubscriptionEventData{
			StripeSubscriptionID: "sub_1",
		},
	}

	if err := uc.HandleWebhookEvent(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhookEvent failed: %v", err)
	}

	sub, err := subRepo.GetByStripeSubscriptionID(context.Background(), "sub_1")
	if err != nil {
		t.Fatalf("GetByStripeSubscriptionID failed: %v", err)
	}
	if sub.Status != "canceled" {
		t.Fatalf("expected status=canceled, got %s", sub.Status)
	}
	if sub.CanceledAt == nil {
		t.Fatal("expected CanceledAt to be set")
	}
}
