package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// TestMutateWithinTxValidatesBeforeTouchingTx exercises mutateWithinTx's
// amount/txType validation (shared by BillingRepository.DebitAndRecord,
// BillingRepository.CreditAndRecord, and PaymentRepository.CreateAndCredit)
// without a real database connection: every case here is expected to be
// rejected before mutateWithinTx ever issues a query, so passing a nil
// pgx.Tx is safe -- a bug that skipped validation and touched tx would panic
// this test with a nil pointer dereference instead of silently passing.
func TestMutateWithinTxValidatesBeforeTouchingTx(t *testing.T) {
	t.Parallel()

	roomID := "room-1"
	messageID := "message-1"

	tests := []struct {
		name         string
		roomID       *string
		messageID    *string
		txType       billing.TransactionType
		signedAmount int64
	}{
		{
			name:         "consumption with zero amount",
			roomID:       &roomID,
			messageID:    &messageID,
			txType:       billing.TransactionTypeConsumption,
			signedAmount: 0,
		},
		{
			name:         "consumption with positive amount (should be negative)",
			roomID:       &roomID,
			messageID:    &messageID,
			txType:       billing.TransactionTypeConsumption,
			signedAmount: 100,
		},
		{
			name:         "charge with zero amount",
			txType:       billing.TransactionTypeCharge,
			signedAmount: 0,
		},
		{
			name:         "charge with negative amount",
			txType:       billing.TransactionTypeCharge,
			signedAmount: -100,
		},
		{
			name:         "adjustment with zero amount",
			txType:       billing.TransactionTypeAdjustment,
			signedAmount: 0,
		},
		{
			name:         "adjustment with negative amount",
			txType:       billing.TransactionTypeAdjustment,
			signedAmount: -50,
		},
		{
			name:         "consumption txType used for a credit-direction amount",
			txType:       billing.TransactionTypeConsumption,
			signedAmount: 100,
		},
		{
			name:         "unsupported transaction type",
			txType:       billing.TransactionType("refund"),
			signedAmount: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			txn, err := mutateWithinTx(context.Background(), nil, "user-1", tt.roomID, tt.messageID, tt.txType, tt.signedAmount, "test")

			if err == nil {
				t.Fatalf("expected an error, got txn=%+v", txn)
			}
			if !errors.Is(err, billing.ErrInvalidAmount) {
				t.Fatalf("expected billing.ErrInvalidAmount, got %v", err)
			}
			if txn != nil {
				t.Fatalf("expected a nil transaction on validation failure, got %+v", txn)
			}
		})
	}
}
