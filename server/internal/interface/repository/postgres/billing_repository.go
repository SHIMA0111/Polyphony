package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// BillingRepository implements the billing.BalanceRepository interface using PostgreSQL.
type BillingRepository struct {
	pool *pgxpool.Pool
}

// NewBillingRepository creates a new BillingRepository backed by the given connection pool.
func NewBillingRepository(pool *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{pool: pool}
}

// GetOrCreateBalance returns userID's TokenBalance, inserting a zero-balance
// row on first access. The ON CONFLICT branch performs a deliberate no-op
// assignment (user_id = token_balances.user_id) purely so RETURNING yields
// the pre-existing row on conflict; a plain DO NOTHING would return zero
// rows on the common "row already exists" path.
func (r *BillingRepository) GetOrCreateBalance(ctx context.Context, userID string) (*billing.TokenBalance, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO token_balances (user_id, balance, updated_at) VALUES ($1, 0, NOW())
		 ON CONFLICT (user_id) DO UPDATE SET user_id = token_balances.user_id
		 RETURNING user_id, balance, updated_at`,
		userID,
	)

	var b billing.TokenBalance
	if err := row.Scan(&b.UserID, &b.Balance, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

// DebitAndRecord atomically decrements userID's balance by amount (applied
// as a debit) and inserts a matching TransactionTypeConsumption row, both in
// a single database transaction. See billing.BalanceRepository.DebitAndRecord
// for the negative-balance/ErrNotFound contract.
func (r *BillingRepository) DebitAndRecord(ctx context.Context, userID, roomID, messageID string, amount int64, description string) (*billing.TokenTransaction, error) {
	return r.mutateAndRecord(ctx, userID, &roomID, &messageID, billing.TransactionTypeConsumption, -amount, description)
}

// CreditAndRecord atomically increments userID's balance by amount and
// inserts a matching row of the given txType, both in a single database
// transaction. See billing.BalanceRepository.CreditAndRecord for the
// ErrNotFound contract.
func (r *BillingRepository) CreditAndRecord(ctx context.Context, userID string, txType billing.TransactionType, amount int64, description string) (*billing.TokenTransaction, error) {
	return r.mutateAndRecord(ctx, userID, nil, nil, txType, amount, description)
}

// mutateAndRecord applies signedAmount (positive to credit, negative to
// debit) to userID's balance and inserts a matching token_transactions row,
// atomically in a single DB transaction. roomID/messageID are nil for
// transactions not tied to any room/message (e.g. top-ups).
func (r *BillingRepository) mutateAndRecord(
	ctx context.Context,
	userID string,
	roomID, messageID *string,
	txType billing.TransactionType,
	signedAmount int64,
	description string,
) (*billing.TokenTransaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	// Safe no-op after a successful Commit below.
	defer func() { _ = tx.Rollback(ctx) }()

	txn, err := mutateWithinTx(ctx, tx, userID, roomID, messageID, txType, signedAmount, description)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return txn, nil
}

// mutateWithinTx is mutateAndRecord's body, extracted so
// PaymentRepository.CreateAndCredit (server/internal/interface/repository/postgres/payment_repository.go)
// can share the exact same balance-mutation SQL within its own open
// transaction — crediting a balance and inserting its payment_history row
// must commit or roll back together (see step49.md's atomicity requirement).
func mutateWithinTx(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	roomID, messageID *string,
	txType billing.TransactionType,
	signedAmount int64,
	description string,
) (*billing.TokenTransaction, error) {
	var newBalance int64
	err := tx.QueryRow(ctx,
		`UPDATE token_balances SET balance = balance + $1, updated_at = NOW() WHERE user_id = $2 RETURNING balance`,
		signedAmount, userID,
	).Scan(&newBalance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	txn := &billing.TokenTransaction{
		ID:           uuid.New().String(),
		UserID:       userID,
		RoomID:       roomID,
		Type:         txType,
		Amount:       signedAmount,
		BalanceAfter: newBalance,
		Description:  description,
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO token_transactions (id, user_id, room_id, message_id, type, amount, balance_after, description, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		 RETURNING created_at`,
		txn.ID, txn.UserID, txn.RoomID, messageID, string(txn.Type), txn.Amount, txn.BalanceAfter, txn.Description,
	).Scan(&txn.CreatedAt)
	if err != nil {
		return nil, err
	}

	return txn, nil
}

const transactionColumns = `id, user_id, room_id, type, amount, balance_after, description, created_at`

// scanTokenTransaction scans a token_transactions row into a TokenTransaction.
func scanTokenTransaction(scanner interface{ Scan(dest ...any) error }) (*billing.TokenTransaction, error) {
	var txn billing.TokenTransaction
	var txType string
	if err := scanner.Scan(&txn.ID, &txn.UserID, &txn.RoomID, &txType, &txn.Amount, &txn.BalanceAfter, &txn.Description, &txn.CreatedAt); err != nil {
		return nil, err
	}
	txn.Type = billing.TransactionType(txType)
	return &txn, nil
}

// ListTransactions returns a cursor-paginated page of userID's transactions,
// newest first (created_at DESC, id DESC), mirroring
// MessageRepository.ListByRoom's cursor pattern.
func (r *BillingRepository) ListTransactions(ctx context.Context, userID, cursor string, limit int) (*billing.TransactionPage, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+transactionColumns+` FROM token_transactions WHERE user_id = $1
			 ORDER BY created_at DESC, id DESC LIMIT $2`,
			userID, limit+1,
		)
	} else {
		var cursorCreatedAt time.Time
		err = r.pool.QueryRow(ctx,
			`SELECT created_at FROM token_transactions WHERE id = $1 AND user_id = $2`, cursor, userID,
		).Scan(&cursorCreatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("invalid cursor: %w", domain.ErrNotFound)
			}
			return nil, err
		}

		rows, err = r.pool.Query(ctx,
			`SELECT `+transactionColumns+` FROM token_transactions WHERE user_id = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			userID, cursorCreatedAt, cursor, limit+1,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*billing.TokenTransaction
	for rows.Next() {
		txn, err := scanTokenTransaction(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, txn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	page := &billing.TransactionPage{}

	if len(transactions) > limit {
		transactions = transactions[:limit]
		lastID := transactions[len(transactions)-1].ID
		page.NextCursor = &lastID
	}

	page.Transactions = transactions
	return page, nil
}
