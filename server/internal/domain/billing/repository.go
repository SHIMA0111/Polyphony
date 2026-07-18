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
	// (amount must be strictly > 0; it is applied as a debit, i.e.
	// subtracted) and inserts a TransactionTypeConsumption row recording the
	// debit, in a single database transaction. roomID/messageID identify the
	// AI invocation the debit pays for.
	//
	// The resulting balance is allowed to go negative: this method does not
	// itself reject the debit once it drives the balance to or below zero —
	// BillingUsecase.CheckBalance's pre-call guard is what blocks *further*
	// invocations once that happens. Returns ErrInvalidAmount if amount <= 0
	// (checked before any mutation is attempted), or domain.ErrNotFound if
	// userID has no existing balance row (GetOrCreateBalance must be called
	// first).
	DebitAndRecord(ctx context.Context, userID, roomID, messageID string, amount int64, description string) (*TokenTransaction, error)

	// CreditAndRecord atomically increments userID's balance by amount
	// (amount must be strictly > 0) and inserts a row of the given txType,
	// which must be TransactionTypeCharge or TransactionTypeAdjustment (the
	// only two transaction types that represent a credit), recording the
	// credit, in a single database transaction. Used by the dev seed CLI
	// (cmd/seed-tokens) today, and reserved for Step 49's Stripe webhook
	// credits. Returns ErrInvalidAmount if amount <= 0 or txType is not one
	// of the two allowed credit types (checked before any mutation is
	// attempted), or domain.ErrNotFound if userID has no existing balance
	// row.
	CreditAndRecord(ctx context.Context, userID string, txType TransactionType, amount int64, description string) (*TokenTransaction, error)

	// ListTransactions returns a cursor-paginated page of userID's
	// transactions, newest first. cursor is a transaction ID (empty starts
	// from the newest), matching message.MessageRepository.ListByRoom's
	// cursor convention.
	ListTransactions(ctx context.Context, userID, cursor string, limit int) (*TransactionPage, error)
}
