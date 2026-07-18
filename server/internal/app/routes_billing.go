package app

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

// registerBillingRoutes registers the authenticated token balance / usage
// history / Stripe checkout-and-subscription endpoints on the given group
// (the shared authenticated group built in NewRouter), plus the public
// POST /webhooks/stripe route directly on e — mirroring
// registerWebSocketRoutes's e/g split, since Stripe cannot present a JWT on
// its webhook request. Every authenticated endpoint here is scoped to the
// caller's own data — no room-role-gated authorization applies.
func registerBillingRoutes(e *echo.Echo, g *echo.Group, c *Container) {
	g.GET("/billing/balance", c.BillingHandler.GetBalance)
	g.GET("/billing/transactions", c.BillingHandler.ListTransactions)
	g.GET("/billing/plans", c.BillingHandler.ListPlans)
	g.POST("/billing/checkout-session", c.BillingHandler.CreateCheckoutSession)
	g.POST("/billing/portal-session", c.BillingHandler.CreatePortalSession)
	g.GET("/billing/subscription", c.BillingHandler.GetSubscription)
	g.POST("/billing/subscription/cancel", c.BillingHandler.CancelSubscription)
	g.GET("/billing/payments", c.BillingHandler.ListPayments)

	// BodyLimit guards against unauthenticated callers buffering unbounded
	// request bodies before Stripe signature verification even runs — this
	// route accepts no JWT, so it has no other admission control ahead of
	// the handler.
	e.POST("/webhooks/stripe", c.BillingHandler.HandleStripeWebhook, echomw.BodyLimit("1M"))
}
