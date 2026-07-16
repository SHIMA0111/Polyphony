# Phase 16: Token Balance Management

Tracks `docs/tasks/step42.md`'s scope: token balances/transactions ledger,
a pre-call balance guard + post-completion usage recording wired into
`MessageUsecase.SendAIMessage`/`RegenerateAIMessage`, the balance/usage
history endpoints, and a dev seed CLI for local top-ups — plus
`docs/tasks/step48.md`'s web-side balance badge and usage history UI,
reconciled into this file at Step 60 (previously tracked as out of scope
below; Step 48 has since landed).

## Scope (Step 42: server)

- [x] `schema.sql`: add `token_balances` and `token_transactions` tables +
      `idx_token_transactions_user_created` index.
- [x] `server/internal/domain/errors.go`: add `ErrInsufficientBalance`.
- [x] `server/internal/domain/billing/entity.go`: `TransactionType`,
      `TokenBalance`, `TokenTransaction`, `TransactionPage`.
- [x] `server/internal/domain/billing/repository.go`: `BalanceRepository`
      port (`GetOrCreateBalance`, `DebitAndRecord`, `CreditAndRecord`,
      `ListTransactions`).
- [x] `server/internal/interface/repository/postgres/billing_repository.go`:
      `BillingRepository` implementing `BalanceRepository`.
- [x] `server/internal/usecase/billing/usecase.go`: `BillingUsecase`
      (`CheckBalance`, `RecordUsage`, `GetBalance`, `ListTransactions`).
- [x] `server/internal/usecase/message/billing_guard.go`: `BillingGuard`
      interface, kept isolated from Step 23's context-assembly edits.
- [x] `server/internal/usecase/message/usecase.go`: wire `billing
      BillingGuard` into `MessageUsecase`/`NewMessageUsecase`; guard +
      fire-and-forget `RecordUsage` calls in `SendAIMessage` and
      `RegenerateAIMessage`.
- [x] `server/internal/interface/handler/dto.go`: Billing DTOs
      (`TokenBalanceResponse`, `TokenTransactionResponse`,
      `TokenTransactionListResponse`).
- [x] `server/internal/interface/handler/billing_handler.go`:
      `BillingHandler` (`GetBalance`, `ListTransactions`).
- [x] `server/internal/interface/handler/message_handler.go`: map
      `domain.ErrInsufficientBalance` to HTTP 402.
- [x] Wire billing into the DI container (`container.go`,
      `routes_billing.go`, `router.go`).
- [x] `server/cmd/seed-tokens/main.go`: dev top-up CLI.
- [x] `Taskfile.yml`: `billing:topup` task.
- [x] Tests: `usecase/billing`, `usecase/message` (billing guard cases),
      `interface/handler` (billing handler + message handler 402 case),
      `interface/repository/postgres` (testcontainers, incl. concurrency).
- [x] Update this progress file.

## Scope (Step 48: web)

- [x] `web/src/features/billing/types.ts` — `TokenBalance`, `TransactionType`, `TokenTransaction`,
      `TokenTransactionPage`
- [x] `web/src/features/billing/api/get-balance.ts` / `get-usage-history.ts` — thin fetch modules
- [x] `web/src/features/billing/hooks/use-balance.ts` (polling + invalidation on AI-send success) /
      `use-usage-history.ts` (cursor-paginated)
- [x] `web/src/features/billing/components/BalanceBadge.tsx` — top-bar widget, low-balance visual treatment,
      links to `/billing/usage`
- [x] `web/src/features/billing/components/UsageHistoryList.tsx` — paginated transaction table
- [x] `web/src/app/(main)/billing/usage/page.tsx`; `BalanceBadge` inserted into `app/(main)/layout.tsx`'s top bar
- [x] `httpClient`/fetch wrapper's error type preserves HTTP status so callers can branch on `error.status === 402`
- [x] `ChatRoom.tsx`'s `handleSendWithAI` surfaces a distinct inline error on 402 and invalidates
      `["billing", "balance"]` on success; `MessageInput.tsx` gains an optional `aiError` prop
- [x] Component tests (`BalanceBadge.test.tsx`, `UsageHistoryList.test.tsx`, MSW-backed) and
      `web/e2e/billing.spec.ts`

## Verification

- [x] `cd server && go build ./... && go vet ./... && go test ./...`
- [x] `task migrate:generate -- add_token_balances`
- [x] `cd server && go test -tags=integration ./internal/interface/repository/postgres/... -run TestBillingRepository -v` (testcontainers, Docker required)
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run` (includes `BalanceBadge`/`UsageHistoryList` tests)
- [ ] `task up` + curl end-to-end flow (requires the full Docker Compose stack — left for post-merge integration verification)
- [x] `docker compose exec db psql ...` schema inspection — verified live in the wave-9 integration review: `\dt`
      lists `token_balances`, `token_transactions`, `subscriptions`, and `payment_history` alongside the rest of
      the schema.
- [x] `web/e2e/billing.spec.ts` against the live compose stack — verified live in the wave-9 integration review:
      `billing.spec.ts` and `regression/billing.spec.ts` passed in the full-suite E2E runs (Stripe Checkout legs
      self-skip without credentials, per the documented steady state).

## Out of scope (per step42.md / step48.md)

- Stripe integration / subscription plans (Step 49; web UI in Step 53 — see `.ai_progress/phase17.md`).
- Automated initial balance provisioning for new users.
- Per-token-type or per-provider pricing/cost multipliers.
- Estimating in-flight request cost before completion.
- Room-role-based authorization on billing endpoints.
- Refunds/chargebacks beyond the generic `adjustment` transaction type.
- Click-to-purchase from `BalanceBadge` (passive indicator only; purchasing lives in Step 53's billing UI).
