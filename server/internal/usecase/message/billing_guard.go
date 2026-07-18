package message

import "context"

// BillingGuard is the narrow, locally-defined surface MessageUsecase depends
// on for token-balance enforcement, mirroring how MessageUsecase already
// depends on ai.LLMGateway rather than a concrete gateway type: usecase/message
// depends on this minimal interface instead of importing usecase/billing
// directly, keeping the two usecase packages decoupled.
//
// Since Go interface satisfaction is structural, *billingusecase.BillingUsecase
// (server/internal/usecase/billing) implements BillingGuard automatically —
// no explicit adapter type is needed, and the DI container passes a
// *billingusecase.BillingUsecase straight into NewMessageUsecase.
type BillingGuard interface {
	// CheckBalance returns domain.ErrInsufficientBalance if roomID's owner's
	// token balance is at or below zero.
	CheckBalance(ctx context.Context, roomID string) error

	// RecordUsage debits roomID's owner for an AI invocation that produced
	// aiMessageID with model, by promptTokens+outputTokens. It is a no-op if
	// the total is <= 0.
	RecordUsage(ctx context.Context, roomID, aiMessageID, model string, promptTokens, outputTokens int) error
}
