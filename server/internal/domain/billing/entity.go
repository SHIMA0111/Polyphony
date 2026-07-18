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
