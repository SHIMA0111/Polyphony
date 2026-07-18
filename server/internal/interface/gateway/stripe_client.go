package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	stripe "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// StripeClient implements the billing.StripeGateway interface using the
// stripe-go SDK, following the same struct-wrapping-a-configured-client,
// one-method-per-port-operation, domain-error-on-failure pattern as
// LLMClient.
type StripeClient struct {
	client        *stripe.Client
	webhookSecret string
}

// NewStripeClient creates a new StripeClient authenticated with secretKey.
// webhookSecret is used by ConstructWebhookEvent to verify the
// Stripe-Signature header; it may be empty (verification will then always
// fail with domain.ErrInvalidWebhookSignature until it is set — see
// container.go/step49.md for how a developer obtains it locally via
// `docker compose logs stripe-cli`).
func NewStripeClient(secretKey, webhookSecret string) *StripeClient {
	return &StripeClient{
		client:        stripe.NewClient(secretKey),
		webhookSecret: webhookSecret,
	}
}

// CreateSubscriptionCheckoutSession creates a Stripe Checkout Session in
// subscription mode and returns its hosted checkout_url. It returns a
// wrapped error on any Stripe API failure.
func (c *StripeClient) CreateSubscriptionCheckoutSession(ctx context.Context, params billing.SubscriptionCheckoutParams) (string, error) {
	session, err := c.client.V1CheckoutSessions.Create(ctx, &stripe.CheckoutSessionCreateParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String(params.SuccessURL),
		CancelURL:         stripe.String(params.CancelURL),
		ClientReferenceID: stripe.String(params.UserID),
		Metadata: map[string]string{
			"user_id":   params.UserID,
			"plan_code": params.PlanCode,
		},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			{
				Price:    stripe.String(params.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create subscription checkout session: %w", err)
	}
	return session.URL, nil
}

// CreateTokenPurchaseCheckoutSession creates a Stripe Checkout Session in
// one-time payment mode and returns its hosted checkout_url. It returns a
// wrapped error on any Stripe API failure.
func (c *StripeClient) CreateTokenPurchaseCheckoutSession(ctx context.Context, params billing.TokenPurchaseCheckoutParams) (string, error) {
	session, err := c.client.V1CheckoutSessions.Create(ctx, &stripe.CheckoutSessionCreateParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:        stripe.String(params.SuccessURL),
		CancelURL:         stripe.String(params.CancelURL),
		ClientReferenceID: stripe.String(params.UserID),
		Metadata: map[string]string{
			"user_id":      params.UserID,
			"kind":         "token_purchase",
			"package_code": params.PackageCode,
		},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			{
				Price:    stripe.String(params.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create token purchase checkout session: %w", err)
	}
	return session.URL, nil
}

// CreateBillingPortalSession creates a Stripe Billing Portal session for the
// given Stripe customer and returns its hosted portal_url.
func (c *StripeClient) CreateBillingPortalSession(ctx context.Context, stripeCustomerID, returnURL string) (string, error) {
	session, err := c.client.V1BillingPortalSessions.Create(ctx, &stripe.BillingPortalSessionCreateParams{
		Customer:  stripe.String(stripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	})
	if err != nil {
		return "", fmt.Errorf("create billing portal session: %w", err)
	}
	return session.URL, nil
}

// CancelSubscriptionAtPeriodEnd schedules stripeSubscriptionID to cancel at
// the end of its current billing period, rather than immediately.
func (c *StripeClient) CancelSubscriptionAtPeriodEnd(ctx context.Context, stripeSubscriptionID string) error {
	_, err := c.client.V1Subscriptions.Update(ctx, stripeSubscriptionID, &stripe.SubscriptionUpdateParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("cancel subscription at period end: %w", err)
	}
	return nil
}

// ConstructWebhookEvent verifies payload's Stripe-Signature (sigHeader)
// using webhookSecret and maps the event into a billing.WebhookEvent.
// Unrecognized event types map to a WebhookEvent carrying only ID/Type, so
// HandleWebhookEvent can safely no-op on anything it doesn't handle. It
// returns a domain.ErrInvalidWebhookSignature-wrapped error if signature
// verification itself fails.
func (c *StripeClient) ConstructWebhookEvent(payload []byte, sigHeader string) (billing.WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, sigHeader, c.webhookSecret)
	if err != nil {
		return billing.WebhookEvent{}, fmt.Errorf("%w: %v", domain.ErrInvalidWebhookSignature, err)
	}

	result := billing.WebhookEvent{ID: event.ID, Type: string(event.Type)}

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return billing.WebhookEvent{}, fmt.Errorf("decode checkout.session.completed payload: %w", err)
		}
		result.CheckoutSession = checkoutSessionData(&session)

	case stripe.EventTypeInvoicePaid:
		var invoice stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
			return billing.WebhookEvent{}, fmt.Errorf("decode invoice.paid payload: %w", err)
		}
		result.Invoice = invoiceData(&invoice)

	case stripe.EventTypeCustomerSubscriptionUpdated, stripe.EventTypeCustomerSubscriptionDeleted:
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return billing.WebhookEvent{}, fmt.Errorf("decode %s payload: %w", event.Type, err)
		}
		result.Subscription = subscriptionEventData(&sub)
	}

	return result, nil
}

// checkoutSessionData maps a stripe.CheckoutSession into the domain's
// CheckoutSessionData, reading the metadata this package's own
// CreateSubscriptionCheckoutSession/CreateTokenPurchaseCheckoutSession set.
func checkoutSessionData(session *stripe.CheckoutSession) *billing.CheckoutSessionData {
	data := &billing.CheckoutSessionData{
		SessionID:   session.ID,
		Mode:        string(session.Mode),
		UserID:      session.ClientReferenceID,
		AmountTotal: session.AmountTotal,
		Currency:    string(session.Currency),
	}
	if session.Metadata != nil {
		data.Kind = session.Metadata["kind"]
		data.PlanCode = session.Metadata["plan_code"]
		data.PackageCode = session.Metadata["package_code"]
		if data.UserID == "" {
			data.UserID = session.Metadata["user_id"]
		}
	}
	if session.Customer != nil {
		data.StripeCustomerID = session.Customer.ID
	}
	if session.Subscription != nil {
		data.StripeSubscriptionID = session.Subscription.ID
	}
	return data
}

// invoiceData maps a stripe.Invoice into the domain's InvoiceData. The
// subscription reference lives at invoice.Parent.SubscriptionDetails.Subscription
// in this API version (Stripe's 2025-era "invoice parent" restructuring),
// not directly on the invoice.
func invoiceData(invoice *stripe.Invoice) *billing.InvoiceData {
	data := &billing.InvoiceData{
		InvoiceID:     invoice.ID,
		BillingReason: string(invoice.BillingReason),
		AmountPaid:    invoice.AmountPaid,
		Currency:      string(invoice.Currency),
	}
	if invoice.Parent != nil && invoice.Parent.SubscriptionDetails != nil && invoice.Parent.SubscriptionDetails.Subscription != nil {
		data.StripeSubscriptionID = invoice.Parent.SubscriptionDetails.Subscription.ID
	}
	return data
}

// subscriptionEventData maps a stripe.Subscription into the domain's
// SubscriptionEventData. CurrentPeriodStart/End and the price ID are read
// off the subscription's first line item, since this API version moved
// those fields from the subscription itself onto each SubscriptionItem
// (Stripe's multi-item-subscription support); this step's pricing model is
// always a single price per subscription (see step49.md's Out of scope), so
// the first item is authoritative.
func subscriptionEventData(sub *stripe.Subscription) *billing.SubscriptionEventData {
	data := &billing.SubscriptionEventData{
		StripeSubscriptionID: sub.ID,
		Status:               string(sub.Status),
		CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
	}
	if sub.Customer != nil {
		data.StripeCustomerID = sub.Customer.ID
	}
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		item := sub.Items.Data[0]
		data.CurrentPeriodStart = time.Unix(item.CurrentPeriodStart, 0).UTC()
		data.CurrentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0).UTC()
		if item.Price != nil {
			data.StripePriceID = item.Price.ID
		}
	}
	return data
}
