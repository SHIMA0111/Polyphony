package mocks

import (
	"context"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
)

// StripeGateway is a scriptable fake implementing billing.StripeGateway.
// Set the *Err/*URL/WebhookEvent fields before exercising a usecase/handler
// method to control its return value; the Last* fields capture the most
// recent call's arguments for assertions. It deliberately never performs a
// real Stripe signature check or network call — that is exactly the point
// of testing against this port (see step49.md's Scope note that
// HandleWebhookEvent's unit tests must be driven entirely by mocked
// WebhookEvent values, never a real Stripe signature).
type StripeGateway struct {
	CheckoutURL string
	CheckoutErr error

	PortalURL string
	PortalErr error

	CancelErr error

	WebhookEvent billing.WebhookEvent
	WebhookErr   error

	LastSubscriptionCheckoutParams   *billing.SubscriptionCheckoutParams
	LastTokenPurchaseCheckoutParams  *billing.TokenPurchaseCheckoutParams
	LastPortalStripeCustomerID       string
	LastPortalReturnURL              string
	LastCanceledStripeSubscriptionID string
	LastWebhookPayload               []byte
	LastWebhookSigHeader             string
}

// CreateSubscriptionCheckoutSession returns g.CheckoutURL/g.CheckoutErr,
// recording params in LastSubscriptionCheckoutParams.
func (g *StripeGateway) CreateSubscriptionCheckoutSession(_ context.Context, params billing.SubscriptionCheckoutParams) (string, error) {
	g.LastSubscriptionCheckoutParams = &params
	return g.CheckoutURL, g.CheckoutErr
}

// CreateTokenPurchaseCheckoutSession returns g.CheckoutURL/g.CheckoutErr,
// recording params in LastTokenPurchaseCheckoutParams.
func (g *StripeGateway) CreateTokenPurchaseCheckoutSession(_ context.Context, params billing.TokenPurchaseCheckoutParams) (string, error) {
	g.LastTokenPurchaseCheckoutParams = &params
	return g.CheckoutURL, g.CheckoutErr
}

// CreateBillingPortalSession returns g.PortalURL/g.PortalErr, recording the
// call's arguments.
func (g *StripeGateway) CreateBillingPortalSession(_ context.Context, stripeCustomerID, returnURL string) (string, error) {
	g.LastPortalStripeCustomerID = stripeCustomerID
	g.LastPortalReturnURL = returnURL
	return g.PortalURL, g.PortalErr
}

// CancelSubscriptionAtPeriodEnd returns g.CancelErr, recording the
// subscription ID it was called with.
func (g *StripeGateway) CancelSubscriptionAtPeriodEnd(_ context.Context, stripeSubscriptionID string) error {
	g.LastCanceledStripeSubscriptionID = stripeSubscriptionID
	return g.CancelErr
}

// ConstructWebhookEvent returns g.WebhookEvent/g.WebhookErr, recording the
// raw payload/signature header it was called with.
func (g *StripeGateway) ConstructWebhookEvent(payload []byte, sigHeader string) (billing.WebhookEvent, error) {
	g.LastWebhookPayload = payload
	g.LastWebhookSigHeader = sigHeader
	return g.WebhookEvent, g.WebhookErr
}
