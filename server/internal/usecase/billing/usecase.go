// Package billing implements the token balance / usage-metering use case:
// a coarse pre-call balance guard, post-completion usage recording, and
// balance/transaction-history lookups for the authenticated user's own data.
package billing

import (
	"context"
	"fmt"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// defaultTransactionLimit is applied to ListTransactions when the caller
// passes a non-positive limit, mirroring MessageHandler.List's clamp.
const defaultTransactionLimit = 20

// maxTransactionLimit is the upper bound ListTransactions clamps limit to.
const maxTransactionLimit = 100

// BillingUsecase provides token balance and usage-history business logic.
// The billed identity for a room is always its owner (room.Room.OwnerID) —
// not the RBAC "master" role — per phases.md Phase 3's ownership-transfer
// note (see CLAUDE.md's Token billing section): balances are keyed by user
// so transferring a room's ownership naturally moves who pays for its AI
// usage.
type BillingUsecase struct {
	balanceRepo domainbilling.BalanceRepository
	roomRepo    room.RoomRepository
}

// NewBillingUsecase creates a new BillingUsecase.
func NewBillingUsecase(balanceRepo domainbilling.BalanceRepository, roomRepo room.RoomRepository) *BillingUsecase {
	return &BillingUsecase{balanceRepo: balanceRepo, roomRepo: roomRepo}
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
