// Package billing implements the token balance / usage-metering use case:
// a coarse pre-call balance guard, post-completion usage recording,
// balance/transaction-history lookups for the authenticated user's own data
// (Step 42), and Stripe Checkout/subscription-lifecycle/webhook processing
// that credits those balances (Step 49).
package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// stripeCheckoutSessionIDPlaceholder is Stripe's own Checkout template
// placeholder: Stripe substitutes it with the real Checkout Session ID when
// redirecting the customer back to the success URL. Passed through
// verbatim, never resolved locally.
const stripeCheckoutSessionIDPlaceholder = "{CHECKOUT_SESSION_ID}"

// withCheckoutSessionIDParam appends a `session_id` query parameter carrying
// Stripe's checkout-session-ID placeholder to successURL, so the web success
// page (billing/checkout/success/page.tsx) can read `session_id` from the
// redirect and correlate it with a payment_history row's
// stripe_reference_id (see PaymentRecord.StripeReferenceID) once the
// corresponding webhook lands. Joins with "&" if successURL already has a
// query string, "?" otherwise, so an operator-configured success URL that
// carries its own query parameters (e.g. a UTM tag) is not clobbered.
func withCheckoutSessionIDParam(successURL string) string {
	separator := "?"
	if strings.Contains(successURL, "?") {
		separator = "&"
	}
	return successURL + separator + "session_id=" + stripeCheckoutSessionIDPlaceholder
}

// defaultTransactionLimit is applied to ListTransactions when the caller
// passes a non-positive limit, mirroring MessageHandler.List's clamp.
const defaultTransactionLimit = 20

// maxTransactionLimit is the upper bound ListTransactions clamps limit to.
const maxTransactionLimit = 100

// BillingUsecase provides token balance and usage-history business logic
// (Step 42), plus Stripe Checkout Session creation, subscription lifecycle,
// payment history, and webhook processing (Step 49). The billed identity for
// a room is always its owner (room.Room.OwnerID) — not the RBAC "master"
// role — per phases.md Phase 3's ownership-transfer note (see CLAUDE.md's
// Token billing section): balances are keyed by user so transferring a
// room's ownership naturally moves who pays for its AI usage.
type BillingUsecase struct {
	balanceRepo      domainbilling.BalanceRepository
	roomRepo         room.RoomRepository
	subscriptionRepo domainbilling.SubscriptionRepository
	paymentRepo      domainbilling.PaymentRepository
	// stripeGateway is nil when STRIPE_SECRET_KEY is unconfigured; every
	// method that needs it checks for nil first and returns
	// domain.ErrStripeNotConfigured rather than panicking.
	stripeGateway      domainbilling.StripeGateway
	plans              []domainbilling.Plan
	tokenPackages      []domainbilling.TokenPackage
	checkoutSuccessURL string
	checkoutCancelURL  string
}

// NewBillingUsecase creates a new BillingUsecase. subscriptionRepo,
// paymentRepo, and stripeGateway may be nil (e.g. in tests exercising only
// the Step 42 balance/transaction methods, or when Stripe is unconfigured);
// every Step 49 method that needs one of them checks for nil and returns a
// domain error rather than panicking.
func NewBillingUsecase(
	balanceRepo domainbilling.BalanceRepository,
	roomRepo room.RoomRepository,
	subscriptionRepo domainbilling.SubscriptionRepository,
	paymentRepo domainbilling.PaymentRepository,
	stripeGateway domainbilling.StripeGateway,
	plans []domainbilling.Plan,
	tokenPackages []domainbilling.TokenPackage,
	checkoutSuccessURL, checkoutCancelURL string,
) *BillingUsecase {
	return &BillingUsecase{
		balanceRepo:        balanceRepo,
		roomRepo:           roomRepo,
		subscriptionRepo:   subscriptionRepo,
		paymentRepo:        paymentRepo,
		stripeGateway:      stripeGateway,
		plans:              plans,
		tokenPackages:      tokenPackages,
		checkoutSuccessURL: checkoutSuccessURL,
		checkoutCancelURL:  checkoutCancelURL,
	}
}

// CheckBalance loads roomID's owner and returns domain.ErrInsufficientBalance
// if their token balance is at or below zero.
//
// This is a coarse guard only: it checks "balance > 0", not "balance covers
// the exact upcoming cost" — the cost of the request is unknown until the
// LLM Gateway responds with actual token counts (see RecordUsage). It
// returns any error from the underlying room/balance lookups unchanged.
func (u *BillingUsecase) CheckBalance(ctx context.Context, roomID string) error {
	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	bal, err := u.balanceRepo.GetOrCreateBalance(ctx, rm.OwnerID)
	if err != nil {
		return err
	}

	if bal.Balance <= 0 {
		return domain.ErrInsufficientBalance
	}
	return nil
}

// RecordUsage debits roomID's owner by promptTokens+outputTokens and records
// an immutable token_transactions row describing the AI invocation that
// produced aiMessageID with model. If the total is <= 0 (the gateway
// reported no usage), it is a no-op and returns nil without debiting.
//
// This debits the raw token count as-is; converting tokens to a currency
// cost via per-token-type/per-provider pricing is Phase 17's concern, not
// this method's. It returns any error from the underlying room/balance
// lookups unchanged; callers (usecase/message) treat a non-nil error here as
// fire-and-forget and log it rather than failing an already-persisted
// message.
func (u *BillingUsecase) RecordUsage(ctx context.Context, roomID, aiMessageID, model string, promptTokens, outputTokens int) error {
	total := int64(promptTokens + outputTokens)
	if total <= 0 {
		return nil
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	description := fmt.Sprintf("AI response using %s (%d prompt + %d output tokens)", model, promptTokens, outputTokens)
	_, err = u.balanceRepo.DebitAndRecord(ctx, rm.OwnerID, roomID, aiMessageID, total, description)
	return err
}

// GetBalance returns userID's current TokenBalance, lazily creating a
// zero-balance row on first access. See
// domainbilling.BalanceRepository.GetOrCreateBalance.
func (u *BillingUsecase) GetBalance(ctx context.Context, userID string) (*domainbilling.TokenBalance, error) {
	return u.balanceRepo.GetOrCreateBalance(ctx, userID)
}

// ListTransactions returns a cursor-paginated page of userID's transaction
// history, newest first. limit is clamped to [1, 100], defaulting to 20 when
// <= 0 (mirroring MessageHandler.List's clamp).
func (u *BillingUsecase) ListTransactions(ctx context.Context, userID, cursor string, limit int) (*domainbilling.TransactionPage, error) {
	if limit <= 0 {
		limit = defaultTransactionLimit
	}
	if limit > maxTransactionLimit {
		limit = maxTransactionLimit
	}
	return u.balanceRepo.ListTransactions(ctx, userID, cursor, limit)
}

// --- Step 49: Stripe checkout / subscription lifecycle / webhook processing ---

// findPlan returns the configured Plan with the given code, or nil if none matches.
func (u *BillingUsecase) findPlan(planCode string) *domainbilling.Plan {
	for i := range u.plans {
		if u.plans[i].Code == planCode {
			return &u.plans[i]
		}
	}
	return nil
}

// findPlanByStripePriceID returns the configured Plan with the given Stripe
// price ID, or nil if none matches. Used by handleSubscriptionUpdated to
// resync PlanCode/MonthlyTokenAllocation when a Billing Portal plan change
// reports a new price.
func (u *BillingUsecase) findPlanByStripePriceID(stripePriceID string) *domainbilling.Plan {
	for i := range u.plans {
		if u.plans[i].StripePriceID == stripePriceID {
			return &u.plans[i]
		}
	}
	return nil
}

// findTokenPackage returns the configured TokenPackage with the given code, or nil if none matches.
func (u *BillingUsecase) findTokenPackage(packageCode string) *domainbilling.TokenPackage {
	for i := range u.tokenPackages {
		if u.tokenPackages[i].Code == packageCode {
			return &u.tokenPackages[i]
		}
	}
	return nil
}

// CreateSubscriptionCheckoutSession resolves planCode against the
// configured plan catalog and returns a Stripe Checkout Session URL for it.
// It returns domain.ErrStripeNotConfigured if Stripe is unconfigured, or
// domain.ErrNotFound for an unknown planCode.
func (u *BillingUsecase) CreateSubscriptionCheckoutSession(ctx context.Context, userID, planCode string) (string, error) {
	if u.stripeGateway == nil {
		return "", domain.ErrStripeNotConfigured
	}
	plan := u.findPlan(planCode)
	if plan == nil {
		return "", domain.ErrNotFound
	}
	return u.stripeGateway.CreateSubscriptionCheckoutSession(ctx, domainbilling.SubscriptionCheckoutParams{
		UserID:     userID,
		PlanCode:   plan.Code,
		PriceID:    plan.StripePriceID,
		SuccessURL: withCheckoutSessionIDParam(u.checkoutSuccessURL),
		CancelURL:  u.checkoutCancelURL,
	})
}

// CreateTokenPurchaseCheckoutSession resolves packageCode against the
// configured token-package catalog and returns a Stripe Checkout Session
// URL for it. It returns domain.ErrStripeNotConfigured if Stripe is
// unconfigured, or domain.ErrNotFound for an unknown packageCode.
func (u *BillingUsecase) CreateTokenPurchaseCheckoutSession(ctx context.Context, userID, packageCode string) (string, error) {
	if u.stripeGateway == nil {
		return "", domain.ErrStripeNotConfigured
	}
	pkg := u.findTokenPackage(packageCode)
	if pkg == nil {
		return "", domain.ErrNotFound
	}
	return u.stripeGateway.CreateTokenPurchaseCheckoutSession(ctx, domainbilling.TokenPurchaseCheckoutParams{
		UserID:      userID,
		PackageCode: pkg.Code,
		PriceID:     pkg.StripePriceID,
		SuccessURL:  withCheckoutSessionIDParam(u.checkoutSuccessURL),
		CancelURL:   u.checkoutCancelURL,
	})
}

// GetSubscription returns userID's current Subscription. It returns
// domain.ErrNotFound if the user has no subscription row, or
// domain.ErrBillingNotConfigured if subscriptionRepo is nil.
func (u *BillingUsecase) GetSubscription(ctx context.Context, userID string) (*domainbilling.Subscription, error) {
	if u.subscriptionRepo == nil {
		return nil, domain.ErrBillingNotConfigured
	}
	return u.subscriptionRepo.GetByUserID(ctx, userID)
}

// CancelSubscription loads userID's subscription, asks Stripe to cancel it
// at the end of the current billing period (not immediately), and persists
// CancelAtPeriodEnd=true locally. The final status transition to "canceled"
// arrives later via the customer.subscription.deleted webhook, not
// synchronously here. It returns domain.ErrNotFound if the user has no
// subscription, domain.ErrStripeNotConfigured if Stripe is unconfigured, or
// domain.ErrBillingNotConfigured if subscriptionRepo is nil.
func (u *BillingUsecase) CancelSubscription(ctx context.Context, userID string) (*domainbilling.Subscription, error) {
	if u.subscriptionRepo == nil {
		return nil, domain.ErrBillingNotConfigured
	}
	sub, err := u.subscriptionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.stripeGateway == nil {
		return nil, domain.ErrStripeNotConfigured
	}
	if err := u.stripeGateway.CancelSubscriptionAtPeriodEnd(ctx, sub.StripeSubscriptionID); err != nil {
		return nil, err
	}

	sub.CancelAtPeriodEnd = true
	if err := u.subscriptionRepo.Update(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// CreateBillingPortalSession returns a Stripe Billing Portal session URL for
// userID's Stripe customer, for self-service plan/payment-method
// management. It returns domain.ErrNotFound if the user has no subscription
// row yet (nothing to manage), domain.ErrStripeNotConfigured if Stripe is
// unconfigured, or domain.ErrBillingNotConfigured if subscriptionRepo is
// nil.
func (u *BillingUsecase) CreateBillingPortalSession(ctx context.Context, userID, returnURL string) (string, error) {
	if u.subscriptionRepo == nil {
		return "", domain.ErrBillingNotConfigured
	}
	sub, err := u.subscriptionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if u.stripeGateway == nil {
		return "", domain.ErrStripeNotConfigured
	}
	return u.stripeGateway.CreateBillingPortalSession(ctx, sub.StripeCustomerID, returnURL)
}

// ListPaymentHistory returns a cursor-paginated page of userID's payment
// history, newest first. limit is clamped to [1, 100], defaulting to 20
// when <= 0, mirroring ListTransactions. It returns
// domain.ErrBillingNotConfigured if paymentRepo is nil.
func (u *BillingUsecase) ListPaymentHistory(ctx context.Context, userID, cursor string, limit int) (*domainbilling.PaymentHistoryPage, error) {
	if u.paymentRepo == nil {
		return nil, domain.ErrBillingNotConfigured
	}
	if limit <= 0 {
		limit = defaultTransactionLimit
	}
	if limit > maxTransactionLimit {
		limit = maxTransactionLimit
	}
	return u.paymentRepo.ListByUserID(ctx, userID, cursor, limit)
}

// ListPlanCatalog returns the purchasable plan/token-package catalog served
// by GET /billing/plans: a pure, DB-free, Stripe-API-free read of the
// configured plans (interval "month") and token packages (interval
// "one_time"). It returns an empty (nil) slice when neither is configured —
// this is the one billing endpoint that never errors when Stripe is
// unconfigured.
func (u *BillingUsecase) ListPlanCatalog() []domainbilling.PlanCatalogEntry {
	entries := make([]domainbilling.PlanCatalogEntry, 0, len(u.plans)+len(u.tokenPackages))
	for _, p := range u.plans {
		entries = append(entries, domainbilling.PlanCatalogEntry{
			Code:           p.Code,
			Name:           p.Name,
			Description:    p.Description,
			PriceCents:     p.PriceCents,
			Currency:       p.Currency,
			Interval:       "month",
			TokenAllowance: p.MonthlyTokenAllocation,
		})
	}
	for _, pkg := range u.tokenPackages {
		entries = append(entries, domainbilling.PlanCatalogEntry{
			Code:           pkg.Code,
			Name:           pkg.Name,
			Description:    pkg.Description,
			PriceCents:     pkg.PriceCents,
			Currency:       pkg.Currency,
			Interval:       "one_time",
			TokenAllowance: pkg.Tokens,
		})
	}
	return entries
}

// HandleWebhookEvent verifies and dispatches a Stripe webhook payload. It
// returns domain.ErrStripeNotConfigured if Stripe is unconfigured, or
// domain.ErrInvalidWebhookSignature (from StripeGateway.ConstructWebhookEvent)
// if signature verification fails. Any other returned error is an
// unexpected internal failure (e.g. a DB error) during dispatch. An
// unrecognized event type, or an idempotency short-circuit on a
// already-processed stripe_event_id, are treated as a successful no-op
// (nil error) — see step49.md's webhook dispatch table.
func (u *BillingUsecase) HandleWebhookEvent(ctx context.Context, payload []byte, sigHeader string) error {
	if u.stripeGateway == nil {
		return domain.ErrStripeNotConfigured
	}

	event, err := u.stripeGateway.ConstructWebhookEvent(payload, sigHeader)
	if err != nil {
		return err
	}

	switch event.Type {
	case domainbilling.EventTypeCheckoutSessionCompleted:
		return u.handleCheckoutSessionCompleted(ctx, event)
	case domainbilling.EventTypeInvoicePaid:
		return u.handleInvoicePaid(ctx, event)
	case domainbilling.EventTypeSubscriptionUpdated:
		return u.handleSubscriptionUpdated(ctx, event)
	case domainbilling.EventTypeSubscriptionDeleted:
		return u.handleSubscriptionDeleted(ctx, event)
	default:
		return nil
	}
}

// handleCheckoutSessionCompleted processes a checkout.session.completed
// event: a token-purchase session credits the balance immediately; a
// subscription-mode session upserts the local Subscription row (the first
// token credit for a subscription arrives later, via invoice.paid, so both
// kinds of payment share the same crediting code path).
func (u *BillingUsecase) handleCheckoutSessionCompleted(ctx context.Context, event domainbilling.WebhookEvent) error {
	session := event.CheckoutSession
	if session == nil {
		return nil
	}

	if session.Mode == domainbilling.CheckoutModePayment && session.Kind == "token_purchase" {
		if u.balanceRepo == nil || u.paymentRepo == nil {
			return domain.ErrBillingNotConfigured
		}
		pkg := u.findTokenPackage(session.PackageCode)
		if pkg == nil || session.UserID == "" {
			slog.Warn("checkout.session.completed token_purchase with unrecognized package/user, ignoring",
				"package_code", session.PackageCode, "user_id", session.UserID)
			return nil
		}

		// Ensure a token_balances row exists before crediting: a user who has
		// never called GET /billing/balance or owned a room yet would
		// otherwise have no row for CreateAndCredit's UPDATE to match. This
		// upsert is independent of (and does not need to share a
		// transaction with) the payment+credit write below — it is a no-op
		// once the row exists.
		if _, err := u.balanceRepo.GetOrCreateBalance(ctx, session.UserID); err != nil {
			return err
		}

		payment := &domainbilling.PaymentRecord{
			ID:                uuid.New().String(),
			UserID:            session.UserID,
			PaymentRail:       "stripe",
			StripeEventID:     event.ID,
			StripeReferenceID: session.SessionID,
			Kind:              domainbilling.PaymentKindTokenPurchase,
			AmountCents:       session.AmountTotal,
			Currency:          session.Currency,
			TokensCredited:    pkg.Tokens,
			Status:            "succeeded",
		}
		description := fmt.Sprintf("Token purchase: %s (%d tokens)", pkg.Name, pkg.Tokens)
		_, err := u.paymentRepo.CreateAndCredit(ctx, payment, session.UserID, pkg.Tokens, description)
		return err
	}

	if session.Mode == domainbilling.CheckoutModeSubscription {
		if u.subscriptionRepo == nil {
			return domain.ErrBillingNotConfigured
		}
		return u.upsertSubscriptionFromCheckout(ctx, session)
	}

	return nil
}

// upsertSubscriptionFromCheckout creates or updates the local Subscription
// row for a completed subscription-mode Checkout Session. No token credit
// happens here — see handleCheckoutSessionCompleted's doc comment.
func (u *BillingUsecase) upsertSubscriptionFromCheckout(ctx context.Context, session *domainbilling.CheckoutSessionData) error {
	if session.StripeSubscriptionID == "" || session.UserID == "" {
		slog.Warn("checkout.session.completed subscription mode missing subscription/user id, ignoring",
			"user_id", session.UserID)
		return nil
	}

	plan := u.findPlan(session.PlanCode)
	if plan == nil {
		slog.Warn("checkout.session.completed subscription with unrecognized plan_code, ignoring",
			"plan_code", session.PlanCode)
		return nil
	}

	now := time.Now().UTC()
	existing, err := u.subscriptionRepo.GetByStripeSubscriptionID(ctx, session.StripeSubscriptionID)
	if err != nil {
		if !isNotFound(err) {
			return err
		}
		sub := &domainbilling.Subscription{
			ID:                     uuid.New().String(),
			UserID:                 session.UserID,
			StripeCustomerID:       session.StripeCustomerID,
			StripeSubscriptionID:   session.StripeSubscriptionID,
			StripePriceID:          plan.StripePriceID,
			PlanCode:               plan.Code,
			Status:                 "active",
			MonthlyTokenAllocation: plan.MonthlyTokenAllocation,
			CurrentPeriodStart:     now,
			CurrentPeriodEnd:       now,
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		return u.subscriptionRepo.Create(ctx, sub)
	}

	existing.StripeCustomerID = session.StripeCustomerID
	existing.StripePriceID = plan.StripePriceID
	existing.PlanCode = plan.Code
	existing.MonthlyTokenAllocation = plan.MonthlyTokenAllocation
	return u.subscriptionRepo.Update(ctx, existing)
}

// handleInvoicePaid processes an invoice.paid event: for a subscription
// creation/renewal invoice, it credits the subscription owner's balance by
// its plan's monthly token allocation and records a payment_history row.
// Any other billing_reason is a no-op. An invoice for a subscription this
// server has no local record of returns the underlying not-found error
// (rather than a nil no-op) so the webhook handler responds non-2xx and
// Stripe redelivers the event: the missing row is most likely a
// checkout.session.completed that has not landed yet due to Stripe's
// unordered delivery, not a genuinely unknown subscription, and treating it
// as success would permanently drop the token credit.
func (u *BillingUsecase) handleInvoicePaid(ctx context.Context, event domainbilling.WebhookEvent) error {
	invoice := event.Invoice
	if invoice == nil {
		return nil
	}
	if invoice.BillingReason != "subscription_create" && invoice.BillingReason != "subscription_cycle" {
		return nil
	}
	if u.subscriptionRepo == nil || u.balanceRepo == nil || u.paymentRepo == nil {
		return domain.ErrBillingNotConfigured
	}

	sub, err := u.subscriptionRepo.GetByStripeSubscriptionID(ctx, invoice.StripeSubscriptionID)
	if err != nil {
		if isNotFound(err) {
			slog.Warn("invoice.paid for unrecognized subscription, will retry on redelivery",
				"stripe_subscription_id", invoice.StripeSubscriptionID)
		}
		return err
	}

	// See handleCheckoutSessionCompleted's identical comment: guarantee a
	// token_balances row exists before crediting.
	if _, err := u.balanceRepo.GetOrCreateBalance(ctx, sub.UserID); err != nil {
		return err
	}

	subID := sub.ID
	payment := &domainbilling.PaymentRecord{
		ID:                uuid.New().String(),
		UserID:            sub.UserID,
		SubscriptionID:    &subID,
		PaymentRail:       "stripe",
		StripeEventID:     event.ID,
		StripeReferenceID: invoice.InvoiceID,
		Kind:              domainbilling.PaymentKindSubscription,
		AmountCents:       invoice.AmountPaid,
		Currency:          invoice.Currency,
		TokensCredited:    sub.MonthlyTokenAllocation,
		Status:            "succeeded",
	}
	description := fmt.Sprintf("Subscription renewal: %s (%d tokens)", sub.PlanCode, sub.MonthlyTokenAllocation)
	_, err = u.paymentRepo.CreateAndCredit(ctx, payment, sub.UserID, sub.MonthlyTokenAllocation, description)
	return err
}

// handleSubscriptionUpdated processes a customer.subscription.updated
// event, syncing status/period/cancellation fields onto the matching local
// Subscription row. An update for a subscription this server has no local
// record of returns the underlying not-found error (rather than a nil
// no-op): Stripe delivers events out of order, so this update may simply
// have raced the corresponding checkout.session.completed, and returning
// non-nil causes the webhook handler to respond non-2xx so Stripe
// redelivers until the local row exists.
func (u *BillingUsecase) handleSubscriptionUpdated(ctx context.Context, event domainbilling.WebhookEvent) error {
	data := event.Subscription
	if data == nil {
		return nil
	}
	if u.subscriptionRepo == nil {
		return domain.ErrBillingNotConfigured
	}

	sub, err := u.subscriptionRepo.GetByStripeSubscriptionID(ctx, data.StripeSubscriptionID)
	if err != nil {
		if isNotFound(err) {
			slog.Warn("customer.subscription.updated for unrecognized subscription, will retry on redelivery",
				"stripe_subscription_id", data.StripeSubscriptionID)
		}
		return err
	}

	sub.Status = data.Status
	sub.CancelAtPeriodEnd = data.CancelAtPeriodEnd
	if !data.CurrentPeriodStart.IsZero() {
		sub.CurrentPeriodStart = data.CurrentPeriodStart
	}
	if !data.CurrentPeriodEnd.IsZero() {
		sub.CurrentPeriodEnd = data.CurrentPeriodEnd
	}
	// A Stripe Billing Portal plan change delivers the new price on this
	// event; resync StripePriceID, PlanCode, and MonthlyTokenAllocation
	// together so the next invoice.paid credits the new plan's allocation
	// instead of permanently crediting the stale one.
	if data.StripePriceID != "" && data.StripePriceID != sub.StripePriceID {
		plan := u.findPlanByStripePriceID(data.StripePriceID)
		if plan == nil {
			slog.Warn("customer.subscription.updated with unrecognized stripe price id, skipping plan/entitlement resync",
				"stripe_subscription_id", data.StripeSubscriptionID, "stripe_price_id", data.StripePriceID)
		} else {
			sub.StripePriceID = plan.StripePriceID
			sub.PlanCode = plan.Code
			sub.MonthlyTokenAllocation = plan.MonthlyTokenAllocation
		}
	}
	return u.subscriptionRepo.Update(ctx, sub)
}

// handleSubscriptionDeleted processes a customer.subscription.deleted
// event, marking the matching local Subscription row canceled. A deletion
// for a subscription this server has no local record of returns the
// underlying not-found error (rather than a nil no-op), for the same
// out-of-order-delivery reason documented on handleSubscriptionUpdated, so
// Stripe redelivers rather than the event being permanently dropped.
func (u *BillingUsecase) handleSubscriptionDeleted(ctx context.Context, event domainbilling.WebhookEvent) error {
	data := event.Subscription
	if data == nil {
		return nil
	}
	if u.subscriptionRepo == nil {
		return domain.ErrBillingNotConfigured
	}

	sub, err := u.subscriptionRepo.GetByStripeSubscriptionID(ctx, data.StripeSubscriptionID)
	if err != nil {
		if isNotFound(err) {
			slog.Warn("customer.subscription.deleted for unrecognized subscription, will retry on redelivery",
				"stripe_subscription_id", data.StripeSubscriptionID)
		}
		return err
	}

	now := time.Now().UTC()
	sub.Status = "canceled"
	sub.CanceledAt = &now
	return u.subscriptionRepo.Update(ctx, sub)
}

// isNotFound reports whether err wraps domain.ErrNotFound.
func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
