package billing

import "errors"

// ErrInvalidAmount is returned by BalanceRepository.DebitAndRecord /
// CreditAndRecord (and PaymentRepository.CreateAndCredit, which shares the
// same underlying mutation) when the caller-supplied amount/txType
// combination violates the documented contract: amount must be strictly
// positive, and a credit-direction mutation (CreditAndRecord,
// CreateAndCredit) must use TransactionTypeCharge or
// TransactionTypeAdjustment — never TransactionTypeConsumption, which is
// reserved for debits. Validated before any balance mutation is applied, so
// a rejected call never partially applies.
var ErrInvalidAmount = errors.New("invalid billing amount")
