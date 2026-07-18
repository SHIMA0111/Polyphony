package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainbilling "github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	billingusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/billing"
)

// BillingHandler handles HTTP requests for the authenticated caller's own
// token balance and usage/transaction history. It delegates business logic
// to BillingUsecase. Every endpoint is scoped to "the authenticated user's
// own data" — there is no room-role-gated authorization here (see
// step42.md's Out of scope).
type BillingHandler struct {
	usecase *billingusecase.BillingUsecase
}

// NewBillingHandler creates a new BillingHandler with the given BillingUsecase.
func NewBillingHandler(usecase *billingusecase.BillingUsecase) *BillingHandler {
	return &BillingHandler{usecase: usecase}
}

// GetBalance handles GET /billing/balance. It returns the authenticated
// user's own current token balance, lazily creating a zero-balance row on
// first access. On success it returns HTTP 200 with a TokenBalanceResponse.
// It returns HTTP 500 for unexpected errors.
func (h *BillingHandler) GetBalance(c echo.Context) error {
	userID := middleware.GetUserID(c)

	bal, err := h.usecase.GetBalance(c.Request().Context(), userID)
	if err != nil {
		return handleBillingError(c, err)
	}

	return c.JSON(http.StatusOK, toTokenBalanceResponse(bal))
}

// ListTransactions handles GET /billing/transactions. It returns a
// cursor-paginated page of the authenticated user's own transaction
// history, newest first. Pagination is controlled by the optional "cursor"
// and "limit" query parameters, identically to MessageHandler.List: the
// limit is clamped between 1 and 100, defaulting to 20. On success it
// returns HTTP 200 with a TokenTransactionListResponse.
func (h *BillingHandler) ListTransactions(c echo.Context) error {
	userID := middleware.GetUserID(c)
	cursor := c.QueryParam("cursor")

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	// Clamp limit between 1 and 100
	limit = max(1, min(100, limit))

	page, err := h.usecase.ListTransactions(c.Request().Context(), userID, cursor, limit)
	if err != nil {
		return handleBillingError(c, err)
	}

	transactions := make([]TokenTransactionResponse, len(page.Transactions))
	for i, txn := range page.Transactions {
		transactions[i] = toTokenTransactionResponse(txn)
	}

	return c.JSON(http.StatusOK, TokenTransactionListResponse{
		Transactions: transactions,
		NextCursor:   page.NextCursor,
	})
}

func toTokenBalanceResponse(bal *domainbilling.TokenBalance) TokenBalanceResponse {
	return TokenBalanceResponse{
		UserID:    bal.UserID,
		Balance:   bal.Balance,
		UpdatedAt: bal.UpdatedAt,
	}
}

func toTokenTransactionResponse(txn *domainbilling.TokenTransaction) TokenTransactionResponse {
	return TokenTransactionResponse{
		ID:           txn.ID,
		UserID:       txn.UserID,
		RoomID:       txn.RoomID,
		Type:         string(txn.Type),
		Amount:       txn.Amount,
		BalanceAfter: txn.BalanceAfter,
		Description:  txn.Description,
		CreatedAt:    txn.CreatedAt,
	}
}

// handleBillingError maps billing usecase errors to HTTP responses.
// domain.ErrNotFound is mapped to 404 for defense-in-depth/consistency with
// the rest of the handler package, even though a balance lookup for the
// authenticated user's own ID should never realistically 404 in practice
// (GetOrCreateBalance always creates the row on first access).
// domain.ErrStripeNotConfigured and domain.ErrBillingNotConfigured are both
// mapped to 503, distinguishing "billing isn't set up yet" from a genuine
// client/server error (Step 49).
func handleBillingError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "not found"})
	}
	if errors.Is(err, domain.ErrStripeNotConfigured) {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Message: "stripe is not configured"})
	}
	if errors.Is(err, domain.ErrBillingNotConfigured) {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Message: "billing is not configured"})
	}
	middleware.GetLogger(c).Error("unhandled billing error", "error", err)
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}

// --- Step 49: Stripe checkout / subscription lifecycle / webhook handlers ---

// CreateCheckoutSession handles POST /billing/checkout-session. The request
// body's "type" must be "subscription" (with "plan_code") or
// "token_purchase" (with "package_code"). On success it returns HTTP 201
// with a CheckoutSessionResponse. It returns HTTP 400 for an invalid
// type/missing code combination, and the domain-error-mapped status
// (404 for an unknown plan/package code, 503 if Stripe is unconfigured)
// from handleBillingError otherwise.
func (h *BillingHandler) CreateCheckoutSession(c echo.Context) error {
	userID := middleware.GetUserID(c)

	var req CreateCheckoutSessionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}

	var checkoutURL string
	var err error
	switch req.Type {
	case "subscription":
		if req.PlanCode == "" {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "plan_code is required for type=subscription"})
		}
		checkoutURL, err = h.usecase.CreateSubscriptionCheckoutSession(c.Request().Context(), userID, req.PlanCode)
	case "token_purchase":
		if req.PackageCode == "" {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "package_code is required for type=token_purchase"})
		}
		checkoutURL, err = h.usecase.CreateTokenPurchaseCheckoutSession(c.Request().Context(), userID, req.PackageCode)
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: `type must be "subscription" or "token_purchase"`})
	}
	if err != nil {
		return handleBillingError(c, err)
	}

	return c.JSON(http.StatusCreated, CheckoutSessionResponse{CheckoutURL: checkoutURL})
}

// CreatePortalSession handles POST /billing/portal-session. On success it
// returns HTTP 200 with a BillingPortalResponse. It returns HTTP 400 for a
// missing return_url, and the domain-error-mapped status (404 if the user
// has no Stripe customer yet, 503 if Stripe is unconfigured) from
// handleBillingError otherwise.
func (h *BillingHandler) CreatePortalSession(c echo.Context) error {
	userID := middleware.GetUserID(c)

	var req BillingPortalRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.ReturnURL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "return_url is required"})
	}

	portalURL, err := h.usecase.CreateBillingPortalSession(c.Request().Context(), userID, req.ReturnURL)
	if err != nil {
		return handleBillingError(c, err)
	}

	return c.JSON(http.StatusOK, BillingPortalResponse{PortalURL: portalURL})
}

// GetSubscription handles GET /billing/subscription. It returns HTTP 200
// with a SubscriptionResponse, or HTTP 204 with no body if the user has no
// subscription.
func (h *BillingHandler) GetSubscription(c echo.Context) error {
	userID := middleware.GetUserID(c)

	sub, err := h.usecase.GetSubscription(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return c.NoContent(http.StatusNoContent)
		}
		return handleBillingError(c, err)
	}

	return c.JSON(http.StatusOK, toSubscriptionResponse(sub))
}

// CancelSubscription handles POST /billing/subscription/cancel. On success
// it returns HTTP 200 with the updated SubscriptionResponse
// (cancel_at_period_end: true). It returns HTTP 404 if the user has no
// subscription, or 503 if Stripe is unconfigured.
func (h *BillingHandler) CancelSubscription(c echo.Context) error {
	userID := middleware.GetUserID(c)

	sub, err := h.usecase.CancelSubscription(c.Request().Context(), userID)
	if err != nil {
		return handleBillingError(c, err)
	}

	return c.JSON(http.StatusOK, toSubscriptionResponse(sub))
}

// ListPayments handles GET /billing/payments. It returns a cursor-paginated
// page of the authenticated user's own payment history, newest first,
// following the exact same "cursor"/"limit" query parameter and response
// shape conventions as ListTransactions.
func (h *BillingHandler) ListPayments(c echo.Context) error {
	userID := middleware.GetUserID(c)
	cursor := c.QueryParam("cursor")

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	limit = max(1, min(100, limit))

	page, err := h.usecase.ListPaymentHistory(c.Request().Context(), userID, cursor, limit)
	if err != nil {
		return handleBillingError(c, err)
	}

	payments := make([]PaymentRecordResponse, len(page.Payments))
	for i, p := range page.Payments {
		payments[i] = toPaymentRecordResponse(p)
	}

	return c.JSON(http.StatusOK, PaymentHistoryResponse{
		Payments:   payments,
		NextCursor: page.NextCursor,
	})
}

// ListPlans handles GET /billing/plans. It returns HTTP 200 with the merged
// plan/token-package catalog (a pure config read — see
// BillingUsecase.ListPlanCatalog's doc comment); it returns 200 with an
// empty "plans" array when Stripe is unconfigured, rather than 503, since
// this endpoint performs no Stripe API call.
func (h *BillingHandler) ListPlans(c echo.Context) error {
	entries := h.usecase.ListPlanCatalog()

	plans := make([]BillingPlanResponse, len(entries))
	for i, e := range entries {
		plans[i] = BillingPlanResponse{
			Code:           e.Code,
			Name:           e.Name,
			Description:    e.Description,
			PriceCents:     e.PriceCents,
			Currency:       e.Currency,
			Interval:       e.Interval,
			TokenAllowance: e.TokenAllowance,
		}
	}

	return c.JSON(http.StatusOK, BillingPlanListResponse{Plans: plans})
}

// HandleStripeWebhook handles POST /webhooks/stripe. It is registered as a
// public (non-JWT) route since Stripe cannot present an Authorization
// header. It reads the raw request body (not c.Bind, since Stripe's
// signature is computed over the exact raw bytes) and the Stripe-Signature
// header, then delegates to BillingUsecase.HandleWebhookEvent. Per Stripe's
// webhook contract, it returns HTTP 200 for any event that was processed or
// intentionally ignored (an unrecognized event type, or an
// idempotency short-circuit on an already-processed event) so Stripe does
// not retry; it returns HTTP 400 only when signature verification itself
// fails, and 503 if Stripe or billing itself is unconfigured.
func (h *BillingHandler) HandleStripeWebhook(c echo.Context) error {
	payload, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "unable to read request body"})
	}
	sigHeader := c.Request().Header.Get("Stripe-Signature")

	err = h.usecase.HandleWebhookEvent(c.Request().Context(), payload, sigHeader)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidWebhookSignature) {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid stripe webhook signature"})
		}
		if errors.Is(err, domain.ErrStripeNotConfigured) {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Message: "stripe is not configured"})
		}
		if errors.Is(err, domain.ErrBillingNotConfigured) {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Message: "billing is not configured"})
		}
		middleware.GetLogger(c).Error("failed to process stripe webhook event", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
	}

	return c.NoContent(http.StatusOK)
}

func toSubscriptionResponse(sub *domainbilling.Subscription) SubscriptionResponse {
	return SubscriptionResponse{
		Status:                 sub.Status,
		PlanCode:               sub.PlanCode,
		MonthlyTokenAllocation: sub.MonthlyTokenAllocation,
		CurrentPeriodStart:     sub.CurrentPeriodStart,
		CurrentPeriodEnd:       sub.CurrentPeriodEnd,
		CancelAtPeriodEnd:      sub.CancelAtPeriodEnd,
		CanceledAt:             sub.CanceledAt,
	}
}

func toPaymentRecordResponse(p *domainbilling.PaymentRecord) PaymentRecordResponse {
	return PaymentRecordResponse{
		ID:             p.ID,
		Kind:           string(p.Kind),
		AmountCents:    p.AmountCents,
		Currency:       p.Currency,
		TokensCredited: p.TokensCredited,
		Status:         p.Status,
		CreatedAt:      p.CreatedAt,
	}
}
