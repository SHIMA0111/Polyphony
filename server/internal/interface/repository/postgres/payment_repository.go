package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// PaymentRepository implements the billing.PaymentRepository interface
// using PostgreSQL. It is a separate type from BillingRepository/
// SubscriptionRepository purely to avoid a Create method-name collision —
// see SubscriptionRepository's doc comment for the full rationale.
type PaymentRepository struct {
	pool *pgxpool.Pool
}

// NewPaymentRepository creates a new PaymentRepository backed by the given
// connection pool.
func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

const paymentColumns = `id, user_id, subscription_id, payment_rail, stripe_event_id, stripe_reference_id,
	kind, amount_cents, currency, tokens_credited, status, created_at`

// paymentEventIDConstraint is the unique constraint name on
// payment_history.stripe_event_id, checked to detect an already-processed
// webhook redelivery (Stripe's at-least-once delivery guarantee).
const paymentEventIDConstraint = "payment_history_stripe_event_id_unique"

// isPaymentEventIDConflict reports whether err is a unique-constraint
// violation on payment_history's stripe_event_id column.
func isPaymentEventIDConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == paymentEventIDConstraint
}

// insertPayment inserts payment via q (either the pool or an open
// transaction — both satisfy this minimal querier interface), populating
// payment.CreatedAt on success.
func insertPayment(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, payment *billing.PaymentRecord) error {
	return q.QueryRow(ctx,
		`INSERT INTO payment_history (id, user_id, subscription_id, payment_rail, stripe_event_id, stripe_reference_id,
		 kind, amount_cents, currency, tokens_credited, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		 RETURNING created_at`,
		payment.ID, payment.UserID, payment.SubscriptionID, payment.PaymentRail, payment.StripeEventID, payment.StripeReferenceID,
		string(payment.Kind), payment.AmountCents, payment.Currency, payment.TokensCredited, payment.Status,
	).Scan(&payment.CreatedAt)
}

// Create inserts a payment_history row. See billing.PaymentRepository.Create.
func (r *PaymentRepository) Create(ctx context.Context, payment *billing.PaymentRecord) (bool, error) {
	if payment.PaymentRail == "" {
		payment.PaymentRail = "stripe"
	}
	err := insertPayment(ctx, r.pool, payment)
	if err != nil {
		if isPaymentEventIDConflict(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// CreateAndCredit atomically inserts payment and credits userID's balance.
// See billing.PaymentRepository.CreateAndCredit.
func (r *PaymentRepository) CreateAndCredit(ctx context.Context, payment *billing.PaymentRecord, userID string, amount int64, description string) (bool, error) {
	if payment.PaymentRail == "" {
		payment.PaymentRail = "stripe"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	// Safe no-op after a successful Commit below.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := insertPayment(ctx, tx, payment); err != nil {
		if isPaymentEventIDConflict(err) {
			// Already processed by an earlier delivery of the same event —
			// the deferred Rollback discards this attempt's (never
			// committed) insert; the balance is left untouched.
			return true, nil
		}
		return false, err
	}

	if _, err := mutateWithinTx(ctx, tx, userID, nil, nil, billing.TransactionTypeCharge, amount, description); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit transaction: %w", err)
	}

	return false, nil
}

// ListByUserID returns a cursor-paginated page of userID's payment history,
// newest first. See billing.PaymentRepository.ListByUserID.
func (r *PaymentRepository) ListByUserID(ctx context.Context, userID, cursor string, limit int) (*billing.PaymentHistoryPage, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+paymentColumns+` FROM payment_history WHERE user_id = $1
			 ORDER BY created_at DESC, id DESC LIMIT $2`,
			userID, limit+1,
		)
	} else {
		var cursorCreatedAt time.Time
		err = r.pool.QueryRow(ctx,
			`SELECT created_at FROM payment_history WHERE id = $1 AND user_id = $2`, cursor, userID,
		).Scan(&cursorCreatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("invalid cursor: %w", domain.ErrNotFound)
			}
			return nil, err
		}

		rows, err = r.pool.Query(ctx,
			`SELECT `+paymentColumns+` FROM payment_history WHERE user_id = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			userID, cursorCreatedAt, cursor, limit+1,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []*billing.PaymentRecord
	for rows.Next() {
		var p billing.PaymentRecord
		var kind string
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.SubscriptionID, &p.PaymentRail, &p.StripeEventID, &p.StripeReferenceID,
			&kind, &p.AmountCents, &p.Currency, &p.TokensCredited, &p.Status, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		p.Kind = billing.PaymentKind(kind)
		payments = append(payments, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	page := &billing.PaymentHistoryPage{}
	if len(payments) > limit {
		payments = payments[:limit]
		lastID := payments[len(payments)-1].ID
		page.NextCursor = &lastID
	}
	page.Payments = payments
	return page, nil
}
