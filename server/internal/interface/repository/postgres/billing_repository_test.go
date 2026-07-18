package postgres

import (
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// TestValidateDebitAmountRejectsNonPositive proves validateDebitAmount
// (the guard DebitAndRecord runs before any database mutation) rejects
// zero and negative amounts with billing.ErrInvalidAmount.
func TestValidateDebitAmountRejectsNonPositive(t *testing.T) {
	for _, amount := range []int64{0, -1, -1000} {
		if err := validateDebitAmount(amount); !errors.Is(err, billing.ErrInvalidAmount) {
			t.Errorf("amount=%d: expected billing.ErrInvalidAmount, got %v", amount, err)
		}
	}
}

// TestValidateDebitAmountAcceptsPositive proves validateDebitAmount passes
// through any strictly positive amount.
func TestValidateDebitAmountAcceptsPositive(t *testing.T) {
	for _, amount := range []int64{1, 100, 1_000_000} {
		if err := validateDebitAmount(amount); err != nil {
			t.Errorf("amount=%d: expected nil error, got %v", amount, err)
		}
	}
}

// TestValidateCreditParamsRejectsNonPositiveAmount proves
// validateCreditParams (the guard CreditAndRecord runs before any database
// mutation) rejects zero and negative amounts with billing.ErrInvalidAmount
// even when txType is otherwise valid.
func TestValidateCreditParamsRejectsNonPositiveAmount(t *testing.T) {
	for _, amount := range []int64{0, -1, -1000} {
		if err := validateCreditParams(billing.TransactionTypeCharge, amount); !errors.Is(err, billing.ErrInvalidAmount) {
			t.Errorf("amount=%d: expected billing.ErrInvalidAmount, got %v", amount, err)
		}
	}
}

// TestValidateCreditParamsRejectsInvalidTxType proves validateCreditParams
// rejects any txType other than TransactionTypeCharge/
// TransactionTypeAdjustment with billing.ErrInvalidAmount — in particular,
// TransactionTypeConsumption (a debit-only type) must never be accepted by
// the credit path, since accepting it would let a caller record a
// "consumption" ledger row for what is actually a credit.
func TestValidateCreditParamsRejectsInvalidTxType(t *testing.T) {
	for _, txType := range []billing.TransactionType{billing.TransactionTypeConsumption, billing.TransactionType("bogus")} {
		if err := validateCreditParams(txType, 100); !errors.Is(err, billing.ErrInvalidAmount) {
			t.Errorf("txType=%q: expected billing.ErrInvalidAmount, got %v", txType, err)
		}
	}
}

// TestValidateCreditParamsAcceptsBothCreditTxTypes proves
// validateCreditParams passes for both allowed credit types when amount is
// positive.
func TestValidateCreditParamsAcceptsBothCreditTxTypes(t *testing.T) {
	for _, txType := range []billing.TransactionType{billing.TransactionTypeCharge, billing.TransactionTypeAdjustment} {
		if err := validateCreditParams(txType, 100); err != nil {
			t.Errorf("txType=%q: expected nil error, got %v", txType, err)
		}
	}
}
