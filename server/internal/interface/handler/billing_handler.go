package handler

import (
	"errors"
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
func handleBillingError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "not found"})
	}
	middleware.GetLogger(c).Error("unhandled billing error", "error", err)
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}
