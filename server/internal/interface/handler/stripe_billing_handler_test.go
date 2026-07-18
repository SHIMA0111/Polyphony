package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	billingusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/billing"
)

// setupStripeBillingTest builds a BillingHandler backed by a real
// BillingUsecase with every Step 49 dependency wired to a fresh in-memory
// mock and a small fixed plan/package catalog, for tests exercising the
// Stripe checkout/subscription/webhook endpoints.
func setupStripeBillingTest(gw *mocks.StripeGateway) (*echo.Echo, *BillingHandler, *mocks.BalanceRepo, *mocks.SubscriptionRepo) {
	balanceRepo := &mocks.BalanceRepo{}
	roomRepo := &mocks.RoomRepo{}
	subRepo := &mocks.SubscriptionRepo{}
	paymentRepo := &mocks.PaymentRepo{BalanceRepo: balanceRepo}

	plans := []domainbilling.Plan{
		{Code: "starter", StripePriceID: "price_starter", Name: "Starter", Description: "100K tokens/month",
			PriceCents: 500, Currency: "usd", MonthlyTokenAllocation: 100000},
	}
	packages := []domainbilling.TokenPackage{
		{Code: "topup_small", StripePriceID: "price_topup_small", Name: "Small top-up", Description: "50K tokens",
			PriceCents: 300, Currency: "usd", Tokens: 50000},
	}

	var gateway domainbilling.StripeGateway
	if gw != nil {
		gateway = gw
	}

	uc := billingusecase.NewBillingUsecase(balanceRepo, roomRepo, subRepo, paymentRepo, gateway,
		plans, packages, "https://example.com/success", "https://example.com/cancel")
	return echo.New(), NewBillingHandler(uc), balanceRepo, subRepo
}

// TestBillingHandlerCreateCheckoutSessionMissingType asserts that
// CreateCheckoutSession returns HTTP 400 when the request body's "type"
// field is missing or unrecognized.
func TestBillingHandlerCreateCheckoutSessionMissingType(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodPost, "/billing/checkout-session", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CreateCheckoutSession(c); err != nil {
		t.Fatalf("CreateCheckoutSession error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing/unknown type, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBillingHandlerCreateCheckoutSessionSubscriptionMissingPlanCode
// asserts that CreateCheckoutSession returns HTTP 400 for a
// "type":"subscription" request missing plan_code.
func TestBillingHandlerCreateCheckoutSessionSubscriptionMissingPlanCode(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodPost, "/billing/checkout-session", strings.NewReader(`{"type":"subscription"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CreateCheckoutSession(c); err != nil {
		t.Fatalf("CreateCheckoutSession error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing plan_code, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBillingHandlerCreateCheckoutSessionUnknownPlan asserts that
// CreateCheckoutSession returns HTTP 404 for a subscription request
// referencing a plan_code not in the configured catalog.
func TestBillingHandlerCreateCheckoutSessionUnknownPlan(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodPost, "/billing/checkout-session",
		strings.NewReader(`{"type":"subscription","plan_code":"does-not-exist"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CreateCheckoutSession(c); err != nil {
		t.Fatalf("CreateCheckoutSession error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown plan_code, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBillingHandlerCreateCheckoutSessionTokenPurchaseSuccess asserts that
// CreateCheckoutSession returns HTTP 201 with the gateway's checkout_url for
// a valid token_purchase request.
func TestBillingHandlerCreateCheckoutSessionTokenPurchaseSuccess(t *testing.T) {
	gw := &mocks.StripeGateway{CheckoutURL: "https://checkout.stripe.com/session-1"}
	e, h, _, _ := setupStripeBillingTest(gw)

	req := httptest.NewRequest(http.MethodPost, "/billing/checkout-session",
		strings.NewReader(`{"type":"token_purchase","package_code":"topup_small"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CreateCheckoutSession(c); err != nil {
		t.Fatalf("CreateCheckoutSession error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), gw.CheckoutURL) {
		t.Fatalf("expected checkout_url in response, got %s", rec.Body.String())
	}
}

// TestBillingHandlerCreatePortalSessionMissingReturnURL asserts that
// CreatePortalSession returns HTTP 400 when the request body's return_url
// field is missing.
func TestBillingHandlerCreatePortalSessionMissingReturnURL(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodPost, "/billing/portal-session", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CreatePortalSession(c); err != nil {
		t.Fatalf("CreatePortalSession error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing return_url, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBillingHandlerCreatePortalSessionNoSubscription asserts that
// CreatePortalSession returns HTTP 404 for a user with no subscription row
// (nothing to manage in the billing portal).
func TestBillingHandlerCreatePortalSessionNoSubscription(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodPost, "/billing/portal-session",
		strings.NewReader(`{"return_url":"https://example.com/return"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CreatePortalSession(c); err != nil {
		t.Fatalf("CreatePortalSession error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a user with no stripe customer yet, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBillingHandlerGetSubscriptionNoContent asserts that GetSubscription
// returns HTTP 204 for a user with no subscription row.
func TestBillingHandlerGetSubscriptionNoContent(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodGet, "/billing/subscription", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.GetSubscription(c); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for a user with no subscription, got %d", rec.Code)
	}
}

// TestBillingHandlerGetSubscriptionFound asserts that GetSubscription
// returns HTTP 200 with the caller's subscription details when a
// subscription row exists.
func TestBillingHandlerGetSubscriptionFound(t *testing.T) {
	e, h, _, subRepo := setupStripeBillingTest(&mocks.StripeGateway{})
	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
		PlanCode: "starter", Status: "active", MonthlyTokenAllocation: 100000,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/billing/subscription", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.GetSubscription(c); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"plan_code":"starter"`) {
		t.Fatalf("expected plan_code starter in response, got %s", rec.Body.String())
	}
}

// TestBillingHandlerCancelSubscriptionNotFound asserts that
// CancelSubscription returns HTTP 404 for a user with no subscription row.
func TestBillingHandlerCancelSubscriptionNotFound(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodPost, "/billing/subscription/cancel", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CancelSubscription(c); err != nil {
		t.Fatalf("CancelSubscription error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a user with no subscription, got %d", rec.Code)
	}
}

// TestBillingHandlerCancelSubscriptionSuccess asserts that
// CancelSubscription returns HTTP 200 with cancel_at_period_end=true when
// canceling an existing subscription.
func TestBillingHandlerCancelSubscriptionSuccess(t *testing.T) {
	e, h, _, subRepo := setupStripeBillingTest(&mocks.StripeGateway{})
	if err := subRepo.Create(context.Background(), &domainbilling.Subscription{
		ID: "sub-row-1", UserID: "user-1", StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
		PlanCode: "starter", Status: "active", MonthlyTokenAllocation: 100000,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/billing/subscription/cancel", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.CancelSubscription(c); err != nil {
		t.Fatalf("CancelSubscription error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"cancel_at_period_end":true`) {
		t.Fatalf("expected cancel_at_period_end:true in response, got %s", rec.Body.String())
	}
}

// TestBillingHandlerListPlans asserts that ListPlans returns HTTP 200 with
// both the configured plan and token package, and never exposes the
// internal Stripe price ID.
func TestBillingHandlerListPlans(t *testing.T) {
	e, h, _, _ := setupStripeBillingTest(&mocks.StripeGateway{})

	req := httptest.NewRequest(http.MethodGet, "/billing/plans", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.ListPlans(c); err != nil {
		t.Fatalf("ListPlans error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"starter"`) || !strings.Contains(body, `"interval":"month"`) {
		t.Fatalf("expected starter plan with interval=month, got %s", body)
	}
	if !strings.Contains(body, `"code":"topup_small"`) || !strings.Contains(body, `"interval":"one_time"`) {
		t.Fatalf("expected topup_small package with interval=one_time, got %s", body)
	}
	if strings.Contains(body, "price_id") {
		t.Fatalf("expected stripe_price_id to never be exposed on the catalog, got %s", body)
	}
}

// TestBillingHandlerListPlansEmptyWhenUnconfigured asserts that ListPlans
// returns HTTP 200 with an empty plans array (never an error) when neither
// plans nor token packages are configured.
func TestBillingHandlerListPlansEmptyWhenUnconfigured(t *testing.T) {
	balanceRepo := &mocks.BalanceRepo{}
	roomRepo := &mocks.RoomRepo{}
	uc := billingusecase.NewBillingUsecase(balanceRepo, roomRepo, nil, nil, nil, nil, nil, "", "")
	h := NewBillingHandler(uc)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/billing/plans", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.ListPlans(c); err != nil {
		t.Fatalf("ListPlans error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even when Stripe is unconfigured, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"plans":[]`) {
		t.Fatalf("expected an empty plans array, got %s", rec.Body.String())
	}
}

// TestBillingHandlerStripeWebhookValidSignature asserts that
// HandleStripeWebhook returns HTTP 200 and passes the raw request body and
// Stripe-Signature header through verbatim for a validly-signed event, even
// when its type is unrecognized.
func TestBillingHandlerStripeWebhookValidSignature(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID: "evt_1", Type: "customer.created", // unrecognized type: exercises the "processed successfully" 200 path
	}}
	e, h, _, _ := setupStripeBillingTest(gw)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader([]byte(`{"id":"evt_1"}`)))
	req.Header.Set("Stripe-Signature", "t=1,v1=validsig")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStripeWebhook(c); err != nil {
		t.Fatalf("HandleStripeWebhook error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a validly-signed (even if unrecognized-type) event, got %d: %s", rec.Code, rec.Body.String())
	}
	if string(gw.LastWebhookPayload) != `{"id":"evt_1"}` {
		t.Fatalf("expected the raw request body to be passed through verbatim, got %s", gw.LastWebhookPayload)
	}
	if gw.LastWebhookSigHeader != "t=1,v1=validsig" {
		t.Fatalf("expected the Stripe-Signature header to be passed through, got %s", gw.LastWebhookSigHeader)
	}
}

// TestBillingHandlerStripeWebhookInvalidSignature asserts that
// HandleStripeWebhook returns HTTP 400 when signature verification fails.
func TestBillingHandlerStripeWebhookInvalidSignature(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookErr: domain.ErrInvalidWebhookSignature}
	e, h, _, _ := setupStripeBillingTest(gw)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader([]byte(`tampered-payload`)))
	req.Header.Set("Stripe-Signature", "t=1,v1=wrongsig")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStripeWebhook(c); err != nil {
		t.Fatalf("HandleStripeWebhook error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a signature-verification failure, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBillingHandlerStripeWebhookIdempotentReplayReturns200 asserts that
// HandleStripeWebhook returns HTTP 200 on both an event's first delivery and
// a redelivered replay of the same event, crediting the balance exactly
// once.
func TestBillingHandlerStripeWebhookIdempotentReplayReturns200(t *testing.T) {
	gw := &mocks.StripeGateway{WebhookEvent: domainbilling.WebhookEvent{
		ID:   "evt_replay_handler_1",
		Type: domainbilling.EventTypeCheckoutSessionCompleted,
		CheckoutSession: &domainbilling.CheckoutSessionData{
			SessionID: "cs_1", Mode: domainbilling.CheckoutModePayment, Kind: "token_purchase",
			UserID: "user-1", PackageCode: "topup_small", AmountTotal: 300, Currency: "usd",
		},
	}}
	e, h, balanceRepo, _ := setupStripeBillingTest(gw)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Stripe-Signature", "t=1,v1=validsig")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.HandleStripeWebhook(c); err != nil {
			t.Fatalf("HandleStripeWebhook error on delivery %d: %v", i+1, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on delivery %d (including the replay), got %d", i+1, rec.Code)
		}
	}

	bal, err := balanceRepo.GetOrCreateBalance(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateBalance failed: %v", err)
	}
	if bal.Balance != 50000 {
		t.Fatalf("expected the balance credited exactly once (50000) despite two deliveries, got %d", bal.Balance)
	}
}
