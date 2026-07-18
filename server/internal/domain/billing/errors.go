package billing

import "errors"

// ErrInvalidAmount is returned by BalanceRepository.DebitAndRecord and
// BalanceRepository.CreditAndRecord when amount is not strictly positive
// (<= 0), or by CreditAndRecord when txType is not TransactionTypeCharge or
// TransactionTypeAdjustment.
//
// Every caller of a debit/credit must supply a positive amount: a
// zero-amount call would insert a no-op ledger row that misrepresents an
// actual balance change, and a negative amount would let a caller invert
// the sign convention DebitAndRecord/CreditAndRecord each already apply
// internally (DebitAndRecord negates amount itself; CreditAndRecord applies
// it as-is), silently crediting a "debit" call or debiting a "credit" call.
// Both are rejected before any mutation is attempted, so a rejected call
// never partially applies.
//
// It carries no external dependencies (only the standard library), so it
// can be used from any layer without creating an upward dependency from the
// domain layer.
var ErrInvalidAmount = errors.New("billing: amount must be positive")
