package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	billingusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/billing"
)

// setupBillingTest builds a BillingHandler backed by a real BillingUsecase
// over mock repositories, mirroring how ModelHandler's tests build a real
// ModelUsecase over a mocked ai.LLMGateway rather than mocking the usecase
// layer itself.
func setupBillingTest() (*echo.Echo, *BillingHandler, *mocks.BalanceRepo) {
	balanceRepo := &mocks.BalanceRepo{}
	roomRepo := &mocks.RoomRepo{}
	uc := billingusecase.NewBillingUsecase(balanceRepo, roomRepo)
	return echo.New(), NewBillingHandler(uc), balanceRepo
}

func TestBillingHandlerGetBalance(t *testing.T) {
	e, h, balanceRepo := setupBillingTest()
	balanceRepo.SeedBalance("user-1", 500)

	req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.GetBalance(c); err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"user_id":"user-1"`) {
		t.Fatalf("expected user_id in response, got %s", body)
	}
	if !strings.Contains(body, `"balance":500`) {
		t.Fatalf("expected balance 500 in response, got %s", body)
	}
	if !strings.Contains(body, `"updated_at"`) {
		t.Fatalf("expected updated_at in response, got %s", body)
	}
}

func TestBillingHandlerGetBalanceLazilyCreatesZeroBalance(t *testing.T) {
	e, h, _ := setupBillingTest()

	req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "new-user")

	if err := h.GetBalance(c); err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"balance":0`) {
		t.Fatalf("expected balance 0 for a brand-new user, got %s", rec.Body.String())
	}
}

func TestBillingHandlerListTransactions(t *testing.T) {
	e, h, balanceRepo := setupBillingTest()
	balanceRepo.SeedBalance("user-1", 1000)
	for i := 0; i < 3; i++ {
		if _, err := balanceRepo.CreditAndRecord(context.Background(), "user-1", "charge", 10, "topup"); err != nil {
			t.Fatalf("seed transaction: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/billing/transactions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.ListTransactions(c); err != nil {
		t.Fatalf("ListTransactions error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"transactions"`) {
		t.Fatalf("expected transactions field, got %s", body)
	}
	if !strings.Contains(body, `"next_cursor"`) {
		t.Fatalf("expected next_cursor field, got %s", body)
	}
	if !strings.Contains(body, `"type":"charge"`) {
		t.Fatalf("expected a charge-type transaction, got %s", body)
	}
}

func TestBillingHandlerListTransactionsLimitClamp(t *testing.T) {
	e, h, balanceRepo := setupBillingTest()
	balanceRepo.SeedBalance("user-1", 1000)
	for i := 0; i < 5; i++ {
		if _, err := balanceRepo.CreditAndRecord(context.Background(), "user-1", "charge", 10, "topup"); err != nil {
			t.Fatalf("seed transaction: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/billing/transactions?limit=2", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.ListTransactions(c); err != nil {
		t.Fatalf("ListTransactions error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"next_cursor":"`) {
		t.Fatalf("expected a non-nil next_cursor when limit=2 clamps below available transactions, got %s", rec.Body.String())
	}
}
