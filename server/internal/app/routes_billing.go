package app

import "github.com/labstack/echo/v4"

// registerBillingRoutes registers the authenticated token balance / usage
// history endpoints on the given group (the shared authenticated group
// built in NewRouter). Every endpoint is scoped to the authenticated
// caller's own data — no room-role-gated authorization applies here.
func registerBillingRoutes(g *echo.Group, c *Container) {
	g.GET("/billing/balance", c.BillingHandler.GetBalance)
	g.GET("/billing/transactions", c.BillingHandler.ListTransactions)
}
