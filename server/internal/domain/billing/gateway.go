package billing

import (
	"context"
	"time"
)

// Stripe webhook event type strings this package's WebhookEvent.Type is
// expected to carry for the events HandleWebhookEvent dispatches on. Any
// other value is a recognized-by-Stripe-but-unhandled-by-us event type and
// is treated as a no-op (see step49.md's webhook dispatch table).
const (
	EventTypeCheckoutSessionCompleted = "checkout.session.completed"
	EventTypeInvoicePaid              = "invoice.paid"
	EventTypeSubscriptionUpdated      = "customer.subscription.updated"
	EventTypeSubscriptionDeleted      = "customer.subscription.deleted"
)

// Checkout Session modes, mirroring Stripe's own "payment"/"subscription"
// mode strings, surfaced on CheckoutSessionData.Mode so the usecase can
// branch without importing the stripe-go SDK.
const (
	CheckoutModePayment      = "payment"
	CheckoutModeSubscription = "subscription"
)

// SubscriptionCheckoutParams configures a Stripe Checkout Session for a
// recurring subscription plan.
type SubscriptionCheckoutParams struct {
	UserID     string
	PlanCode   string
	PriceID    string
	SuccessURL string
	CancelURL  string
}

// TokenPurchaseCheckoutParams configures a Stripe Checkout Session for a
// one-time token top-up purchase.
type TokenPurchaseCheckoutParams struct {
	UserID      string
	PackageCode string
	PriceID     string
	SuccessURL  string
	CancelURL   string
}

// CheckoutSessionData carries the fields HandleWebhookEvent needs out of a
// checkout.session.completed event, for either a subscription-mode or a
// one-time-payment-mode Checkout Session.
type CheckoutSessionData struct {
	// SessionID is the Stripe Checkout Session ID (cs_...), used as
	// PaymentRecord.StripeReferenceID.
	SessionID string
	// Mode is CheckoutModePayment or CheckoutModeSubscription.
	Mode string
	// Kind is metadata.kind, expected to be "token_purchase" for
	// Mode==CheckoutModePayment; empty for a subscription-mode session.
	Kind string
	// UserID is the local user ID, read from client_reference_id (falling
	// back to metadata.user_id — see step49.md's belt-and-suspenders note).
	UserID string
	// PlanCode is metadata.plan_code, populated for a subscription-mode session.
	PlanCode string
	// PackageCode is metadata.package_code, populated for a token-purchase session.
	PackageCode string
	// StripeCustomerID is the Stripe customer created/reused for this session.
	StripeCustomerID string
	// StripeSubscriptionID is the Stripe subscription created by a
	// subscription-mode session; empty for a one-time payment.
	StripeSubscriptionID string
	// AmountTotal is the total charged, in the smallest currency unit (cents for usd).
	AmountTotal int64
	// Currency is the three-letter ISO currency code (e.g. "usd").
	Currency string
}

// InvoiceData carries the fields HandleWebhookEvent needs out of an
// invoice.paid event.
type InvoiceData struct {
	// InvoiceID is the Stripe invoice ID (in_...), used as PaymentRecord.StripeReferenceID.
	InvoiceID string
	// StripeSubscriptionID identifies which local Subscription this invoice
	// renews, looked up via SubscriptionRepository.GetByStripeSubscriptionID.
	StripeSubscriptionID string
	// BillingReason is Stripe's invoice.billing_reason (e.g.
	// "subscription_create"/"subscription_cycle"); only these two values
	// trigger a token credit (see step49.md's dispatch table).
	BillingReason string
	AmountPaid    int64
	Currency      string
}

// SubscriptionEventData carries the fields HandleWebhookEvent needs out of a
// customer.subscription.updated/.deleted event.
type SubscriptionEventData struct {
	StripeSubscriptionID string
	StripeCustomerID     string
	StripePriceID        string
	Status               string
	CurrentPeriodStart   time.Time
	CurrentPeriodEnd     time.Time
	CancelAtPeriodEnd    bool
}

// WebhookEvent is a small, domain-owned representation of a verified Stripe
// webhook event: just the event ID/Type plus whichever of the typed data
// fields below is populated for that Type, so the usecase layer never
// imports the stripe-go SDK directly. Exactly one of CheckoutSession,
// Invoice, or Subscription is non-nil, matching Type; all are nil for an
// event type this package does not handle (a safe no-op for the usecase).
type WebhookEvent struct {
	ID   string
	Type string

	CheckoutSession *CheckoutSessionData
	Invoice         *InvoiceData
	Subscription    *SubscriptionEventData
}

// StripeGateway is the outbound port for Stripe Checkout/Billing Portal/
// webhook operations. server/internal/interface/gateway/stripe_client.go
// implements it using the stripe-go SDK, following the same
// adapter-implementing-a-domain-port pattern as ai.LLMGateway/LLMClient.
type StripeGateway interface {
	// CreateSubscriptionCheckoutSession creates a Stripe Checkout Session in
	// subscription mode and returns its hosted checkout_url.
	CreateSubscriptionCheckoutSession(ctx context.Context, params SubscriptionCheckoutParams) (checkoutURL string, err error)

	// CreateTokenPurchaseCheckoutSession creates a Stripe Checkout Session
	// in one-time payment mode and returns its hosted checkout_url.
	CreateTokenPurchaseCheckoutSession(ctx context.Context, params TokenPurchaseCheckoutParams) (checkoutURL string, err error)

	// CreateBillingPortalSession creates a Stripe Billing Portal session for
	// the given Stripe customer and returns its hosted portal_url.
	CreateBillingPortalSession(ctx context.Context, stripeCustomerID, returnURL string) (portalURL string, err error)

	// CancelSubscriptionAtPeriodEnd schedules stripeSubscriptionID to cancel
	// at the end of its current billing period (not immediately).
	CancelSubscriptionAtPeriodEnd(ctx context.Context, stripeSubscriptionID string) error

	// ConstructWebhookEvent verifies payload's Stripe-Signature (sigHeader)
	// and, on success, maps the event into a WebhookEvent. It returns a
	// domain.ErrInvalidWebhookSignature-wrapped error if signature
	// verification itself fails.
	ConstructWebhookEvent(payload []byte, sigHeader string) (WebhookEvent, error)
}
