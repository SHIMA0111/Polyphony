// Package billing defines the token balance / transaction entities and the
// repository port used to meter AI usage per the "room master pays" model
// described in CLAUDE.md's Token billing section (phases.md Phase 16/17).
package billing

import "time"

// TransactionType identifies why a TokenTransaction was recorded. Values
// match the token_transactions.type CHECK constraint in schema.sql exactly,
// and are serialized verbatim in the JSON API contract Step 48's web client
// assumes (web/src/features/billing/types.ts) — do not rename or add values
// without updating both.
type TransactionType string

const (
	// TransactionTypeConsumption marks a debit for AI usage (negative Amount).
	TransactionTypeConsumption TransactionType = "consumption"

	// TransactionTypeCharge marks a credit from a top-up/purchase (positive
	// Amount), e.g. the dev seed CLI (cmd/seed-tokens) today, and Step 49's
	// Stripe webhook in the future.
	TransactionTypeCharge TransactionType = "charge"

	// TransactionTypeAdjustment marks a manual correction credit (positive
	// Amount), reserved for future refund/reversal flows; no such flow
	// exists yet (see step42.md's Out of scope).
	TransactionTypeAdjustment TransactionType = "adjustment"
)

// TokenBalance is a user's current token balance. Every user lazily gets a
// zero-balance row on first access (see BalanceRepository.GetOrCreateBalance)
// — there is no automated non-zero initial provisioning (phases.md Phase 16
// explicitly excludes it for this phase).
type TokenBalance struct {
	UserID    string
	Balance   int64
	UpdatedAt time.Time
}

// TokenTransaction is a single immutable ledger entry against a user's
// TokenBalance. Amount is signed: negative for TransactionTypeConsumption
// (AI usage debits), positive for TransactionTypeCharge/TransactionTypeAdjustment
// (credits). BalanceAfter is the resulting balance immediately after this
// transaction was applied, captured atomically with the balance mutation so
// it never drifts from the ledger. RoomID is nil for transactions not tied
// to any room (e.g. top-ups).
//
// TokenTransaction deliberately has no MessageID field: the underlying
// token_transactions.message_id column exists purely for audit/debugging and
// is not part of the API contract Step 48 depends on.
type TokenTransaction struct {
	ID           string
	UserID       string
	RoomID       *string
	Type         TransactionType
	Amount       int64
	BalanceAfter int64
	Description  string
	CreatedAt    time.Time
}

// TransactionPage holds a cursor-paginated page of TokenTransaction rows,
// newest first. It mirrors message.CursorPage's shape so the handler layer
// can apply the same pagination conventions to both.
type TransactionPage struct {
	Transactions []*TokenTransaction
	NextCursor   *string // nil when no more pages
}

// --- Step 49: Stripe subscription / payment entities ---

// Subscription is a user's recurring Stripe Checkout subscription, tracking
// the plan they are on and the current billing period. There is at most one
// Subscription row per user in this step's scope (a single flat
// plan-code → price-id → monthly-token-allocation mapping — see
// step49.md's Out of scope for the explicit exclusion of upgrades/downgrades
// and multi-seat billing). CanceledAt is nil until
// customer.subscription.deleted is processed; CancelAtPeriodEnd is set
// eagerly by BillingUsecase.CancelSubscription and does not by itself mean
// the subscription has ended yet — Status/CanceledAt only flip once Stripe's
// webhook confirms the period actually ended.
type Subscription struct {
	ID                     string
	UserID                 string
	StripeCustomerID       string
	StripeSubscriptionID   string
	StripePriceID          string
	PlanCode               string
	Status                 string
	MonthlyTokenAllocation int64
	CurrentPeriodStart     time.Time
	CurrentPeriodEnd       time.Time
	CancelAtPeriodEnd      bool
	CanceledAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// PaymentKind identifies what a PaymentRecord paid for.
type PaymentKind string

const (
	// PaymentKindSubscription marks a recurring subscription-renewal payment
	// (an invoice.paid event with billing_reason subscription_create/cycle).
	PaymentKindSubscription PaymentKind = "subscription"

	// PaymentKindTokenPurchase marks a one-time token top-up purchase (a
	// checkout.session.completed event in "payment" mode).
	PaymentKindTokenPurchase PaymentKind = "token_purchase"
)

// PaymentRecord is a single immutable row in the payment_history ledger,
// recording one Stripe payment event (subscription renewal or one-time
// token purchase) and how many tokens it credited. StripeEventID is the
// idempotency key: PaymentRepository.Create/CreateAndCredit treat a
// unique-constraint violation on it as "already processed" rather than an
// error, since Stripe delivers webhooks at-least-once. SubscriptionID is nil
// for a token-purchase payment (not tied to any subscription).
type PaymentRecord struct {
	ID                string
	UserID            string
	SubscriptionID    *string
	PaymentRail       string
	StripeEventID     string
	StripeReferenceID string
	Kind              PaymentKind
	AmountCents       int64
	Currency          string
	TokensCredited    int64
	Status            string
	CreatedAt         time.Time
}

// PaymentHistoryPage holds a cursor-paginated page of PaymentRecord rows,
// newest first, mirroring TransactionPage's shape.
type PaymentHistoryPage struct {
	Payments   []*PaymentRecord
	NextCursor *string // nil when no more pages
}

// Plan is a purchasable monthly subscription plan. It is built by the
// composition root (app.NewContainer) from the infrastructure config
// package's config.StripePlan so that the domain/usecase layers never import
// infrastructure config directly (see CLAUDE.md's Clean Architecture rule).
type Plan struct {
	Code                   string
	StripePriceID          string
	Name                   string
	Description            string
	PriceCents             int64
	Currency               string
	MonthlyTokenAllocation int64
}

// TokenPackage is a purchasable one-time token top-up package, converted
// from config.StripeTokenPackage the same way Plan is converted from
// config.StripePlan.
type TokenPackage struct {
	Code          string
	StripePriceID string
	Name          string
	Description   string
	PriceCents    int64
	Currency      string
	Tokens        int64
}

// PlanCatalogEntry is a single row of the purchasable catalog served by
// GET /billing/plans, merging Plan and TokenPackage into one display shape.
// Interval is "month" for a Plan or "one_time" for a TokenPackage.
// StripePriceID is deliberately not carried onto this type — the catalog
// endpoint never exposes Stripe price IDs to clients.
type PlanCatalogEntry struct {
	Code           string
	Name           string
	Description    string
	PriceCents     int64
	Currency       string
	Interval       string
	TokenAllowance int64
}
