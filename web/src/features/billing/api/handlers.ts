import { http, HttpResponse } from "msw"
import type { TokenBalance, TokenTransaction, TokenTransactionPage } from "../types"

/**
 * MSW request handlers for the billing feature, shared by the Node
 * `setupServer` (Vitest, see `src/test/msw/server.ts`) and the browser
 * `setupWorker` (`src/test/msw/browser.ts`).
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the
 * Go API.
 */

export const fixtureBalance: TokenBalance = {
  user_id: "user-1",
  balance: 4200,
  updated_at: "2026-01-03T00:00:00Z",
}

export const fixtureZeroBalance: TokenBalance = {
  user_id: "user-1",
  balance: 0,
  updated_at: "2026-01-03T00:00:00Z",
}

export const fixtureTransactionPage1: TokenTransaction[] = [
  {
    id: "txn-2",
    user_id: "user-1",
    room_id: "room-1",
    type: "consumption",
    amount: -300,
    balance_after: 4200,
    description: "AI response using gpt-5-mini (200 prompt + 100 output tokens)",
    created_at: "2026-01-03T00:00:00Z",
  },
  {
    id: "txn-1",
    user_id: "user-1",
    room_id: null,
    type: "charge",
    amount: 5000,
    balance_after: 4500,
    description: "Token top-up",
    created_at: "2026-01-01T00:00:00Z",
  },
]

export const fixtureTransactionPage2: TokenTransaction[] = [
  {
    id: "txn-0",
    user_id: "user-1",
    room_id: null,
    type: "adjustment",
    amount: -500,
    balance_after: -500,
    description: "Manual correction",
    created_at: "2025-12-31T00:00:00Z",
  },
]

export const billingHandlers = [
  http.get("/api/proxy/billing/balance", () => {
    return HttpResponse.json<TokenBalance>(fixtureBalance)
  }),

  http.get("/api/proxy/billing/transactions", ({ request }) => {
    const url = new URL(request.url)
    const cursor = url.searchParams.get("cursor")

    if (cursor === "txn-1") {
      return HttpResponse.json<TokenTransactionPage>({
        transactions: fixtureTransactionPage2,
        next_cursor: null,
      })
    }

    return HttpResponse.json<TokenTransactionPage>({
      transactions: fixtureTransactionPage1,
      next_cursor: "txn-1",
    })
  }),
]
