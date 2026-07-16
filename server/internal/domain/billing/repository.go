package billing

import "context"

// BalanceRepository defines persistence operations for token balances and
// their transaction ledger.
type BalanceRepository interface {
	// GetOrCreateBalance returns the caller's TokenBalance, lazily creating a
	// zero-balance row on first access. New users always start at zero —
	// automated non-zero provisioning is explicitly out of scope for this
	// phase (phases.md Phase 16); a developer must top up a balance
	// out-of-band (see cmd/seed-tokens / `task billing:topup`).
	GetOrCreateBalance(ctx context.Context, userID string) (*TokenBalance, error)

	// DebitAndRecord atomically decrements userID's balance by amount
	// (amount must be >= 0; it is applied as a debit, i.e. subtracted) and
	// inserts a TransactionTypeConsumption row recording the debit, in a
	// single database transaction. roomID/messageID identify the AI
	// invocation the debit pays for.
	//
	// The resulting balance is allowed to go negative: this method does not
	// itself reject the debit once it drives the balance to or below zero —
	// BillingUsecase.CheckBalance's pre-call guard is what blocks *further*
	// invocations once that happens. Returns domain.ErrNotFound if userID
	// has no existing balance row (GetOrCreateBalance must be called first).
	DebitAndRecord(ctx context.Context, userID, roomID, messageID string, amount int64, description string) (*TokenTransaction, error)

	// CreditAndRecord atomically increments userID's balance by amount
	// (amount must be >= 0) and inserts a row of the given txType (Charge or
	// Adjustment) recording the credit, in a single database transaction.
	// Used by the dev seed CLI (cmd/seed-tokens) today, and reserved for
	// Step 49's Stripe webhook credits. Returns domain.ErrNotFound if userID
	// has no existing balance row.
	CreditAndRecord(ctx context.Context, userID string, txType TransactionType, amount int64, description string) (*TokenTransaction, error)

	// ListTransactions returns a cursor-paginated page of userID's
	// transactions, newest first. cursor is a transaction ID (empty starts
	// from the newest), matching message.MessageRepository.ListByRoom's
	// cursor convention.
	ListTransactions(ctx context.Context, userID, cursor string, limit int) (*TransactionPage, error)
}

// SubscriptionRepository defines persistence operations for a user's Stripe
// subscription state (Step 49). There is at most one Subscription row per
// user in this step's scope.
type SubscriptionRepository interface {
	// Create persists a new Subscription row.
	Create(ctx context.Context, sub *Subscription) error

	// GetByUserID returns userID's Subscription. It returns
	// domain.ErrNotFound if the user has no subscription row.
	GetByUserID(ctx context.Context, userID string) (*Subscription, error)

	// GetByStripeSubscriptionID returns the Subscription matching the given
	// Stripe subscription ID, used by webhook processing to map a
	// customer.subscription.updated/.deleted/invoice.paid event back to its
	// local row. It returns domain.ErrNotFound if no row matches.
	GetByStripeSubscriptionID(ctx context.Context, stripeSubscriptionID string) (*Subscription, error)

	// Update persists changes to an existing Subscription row (matched by
	// ID). It returns domain.ErrNotFound if no row with that ID exists.
	Update(ctx context.Context, sub *Subscription) error
}

// PaymentRepository defines persistence operations for the payment_history
// ledger (Step 49), including the atomic "record payment + credit balance"
// path Stripe webhook processing needs so a credited balance and its
// payment record can never diverge.
type PaymentRepository interface {
	// Create inserts a payment_history row without crediting any balance.
	// It is idempotent on the unique stripe_event_id constraint: if a row
	// with the same payment.StripeEventID already exists (Stripe's
	// at-least-once webhook delivery redelivering an already-processed
	// event), it returns alreadyRecorded=true and nil error rather than
	// erroring, so callers can treat replay as a successful no-op.
	Create(ctx context.Context, payment *PaymentRecord) (alreadyRecorded bool, err error)

	// CreateAndCredit atomically inserts payment (idempotent on
	// stripe_event_id, exactly like Create) and, only when the row is newly
	// inserted, credits userID's token balance by amount (a
	// TransactionTypeCharge token_transactions row, via the same
	// mutation logic BalanceRepository.CreditAndRecord uses) — both writes
	// commit or roll back together in one DB transaction. If a
	// payment_history row with the same StripeEventID already exists, it
	// returns alreadyProcessed=true and leaves the balance untouched.
	CreateAndCredit(ctx context.Context, payment *PaymentRecord, userID string, amount int64, description string) (alreadyProcessed bool, err error)

	// ListByUserID returns a cursor-paginated page of userID's payment
	// history, newest first, mirroring BalanceRepository.ListTransactions's
	// cursor/limit convention.
	ListByUserID(ctx context.Context, userID, cursor string, limit int) (*PaymentHistoryPage, error)
}
