package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

const subscriptionColumns = `id, user_id, stripe_customer_id, stripe_subscription_id, stripe_price_id, plan_code,
	status, monthly_token_allocation, current_period_start, current_period_end, cancel_at_period_end,
	canceled_at, created_at, updated_at`

// SubscriptionRepository implements the billing.SubscriptionRepository
// interface using PostgreSQL. It is a separate type from BillingRepository
// (rather than an additional method set on it) purely because both
// domain interfaces this package implements happen to declare a method
// named Create with a different signature — Go does not support per-
// interface method overloading on one receiver type.
type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

// NewSubscriptionRepository creates a new SubscriptionRepository backed by
// the given connection pool.
func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

// Create persists a new Subscription row. See
// billing.SubscriptionRepository.Create.
func (r *SubscriptionRepository) Create(ctx context.Context, sub *billing.Subscription) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions (id, user_id, stripe_customer_id, stripe_subscription_id, stripe_price_id, plan_code,
		 status, monthly_token_allocation, current_period_start, current_period_end, cancel_at_period_end,
		 canceled_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		sub.ID, sub.UserID, sub.StripeCustomerID, sub.StripeSubscriptionID, sub.StripePriceID, sub.PlanCode,
		sub.Status, sub.MonthlyTokenAllocation, sub.CurrentPeriodStart, sub.CurrentPeriodEnd, sub.CancelAtPeriodEnd,
		sub.CanceledAt, sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
			pgErr.ConstraintName == "subscriptions_stripe_subscription_id_unique" {
			return domain.ErrSubscriptionAlreadyExists
		}
		return err
	}
	return nil
}

// GetByUserID returns userID's Subscription. See
// billing.SubscriptionRepository.GetByUserID.
func (r *SubscriptionRepository) GetByUserID(ctx context.Context, userID string) (*billing.Subscription, error) {
	return r.scanSubscription(r.pool.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions WHERE user_id = $1`, userID))
}

// GetByStripeSubscriptionID returns the Subscription matching the given
// Stripe subscription ID. See
// billing.SubscriptionRepository.GetByStripeSubscriptionID.
func (r *SubscriptionRepository) GetByStripeSubscriptionID(ctx context.Context, stripeSubscriptionID string) (*billing.Subscription, error) {
	return r.scanSubscription(r.pool.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions WHERE stripe_subscription_id = $1`, stripeSubscriptionID))
}

// Update persists changes to an existing Subscription row (matched by ID).
// See billing.SubscriptionRepository.Update.
func (r *SubscriptionRepository) Update(ctx context.Context, sub *billing.Subscription) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE subscriptions SET
		 stripe_customer_id = $1, stripe_subscription_id = $2, stripe_price_id = $3, plan_code = $4,
		 status = $5, monthly_token_allocation = $6, current_period_start = $7, current_period_end = $8,
		 cancel_at_period_end = $9, canceled_at = $10, updated_at = NOW()
		 WHERE id = $11`,
		sub.StripeCustomerID, sub.StripeSubscriptionID, sub.StripePriceID, sub.PlanCode,
		sub.Status, sub.MonthlyTokenAllocation, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.CanceledAt, sub.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SubscriptionRepository) scanSubscription(row pgx.Row) (*billing.Subscription, error) {
	var sub billing.Subscription
	err := row.Scan(
		&sub.ID, &sub.UserID, &sub.StripeCustomerID, &sub.StripeSubscriptionID, &sub.StripePriceID, &sub.PlanCode,
		&sub.Status, &sub.MonthlyTokenAllocation, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.CancelAtPeriodEnd,
		&sub.CanceledAt, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}
