/**
 * Billing-domain types, mirroring the Go API's billing DTOs
 * (`server/internal/interface/handler/dto.go`'s `TokenBalanceResponse` /
 * `TokenTransactionResponse` / `TokenTransactionListResponse`), reached
 * client-side via the Step 4 data-plane proxy at `/api/proxy/billing/*`.
 *
 * `balance` / `amount` / `balance_after` are plain token counts (not
 * currency, per phases.md Phase 16) — never format them with a currency
 * formatter.
 */

/** A user's current token balance, returned by `GET /billing/balance`. */
export interface TokenBalance {
  user_id: string
  balance: number
  updated_at: string
}

/**
 * The kind of ledger entry a `TokenTransaction` represents, matching the
 * `token_transactions.type` CHECK constraint in `server/schema.sql`:
 * `"consumption"` (AI usage debit), `"charge"` (a paid top-up), or
 * `"adjustment"` (a manual/administrative correction).
 */
export type TransactionType = "consumption" | "charge" | "adjustment"

/**
 * A single `token_transactions` ledger row. `amount` is signed (negative for
 * `"consumption"`, positive for `"charge"`/`"adjustment"`); `room_id` is
 * `null` for transactions not tied to any room (e.g. top-ups).
 */
export interface TokenTransaction {
  id: string
  user_id: string
  room_id: string | null
  type: TransactionType
  amount: number
  balance_after: number
  description: string
  created_at: string
}

/** Raw paginated response from `GET /billing/transactions`. */
export interface TokenTransactionPage {
  transactions: TokenTransaction[]
  next_cursor: string | null
}
