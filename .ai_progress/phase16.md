# Phase 16: Token Balance Management (Step 42)

Tracks `docs/tasks/step42.md`'s scope: token balances/transactions ledger,
a pre-call balance guard + post-completion usage recording wired into
`MessageUsecase.SendAIMessage`/`RegenerateAIMessage`, the balance/usage
history endpoints, and a dev seed CLI for local top-ups.

## Scope

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

## Verification

- [x] `go build ./...`
- [x] `go vet ./...`
- [x] `go test ./...`
- [x] `task migrate:generate -- add_token_balances`
- [x] `go test ./internal/interface/repository/postgres/... -run TestBillingRepository -v` (testcontainers, Docker required)
- [ ] `task up` + curl end-to-end flow (requires the full Docker Compose stack — left for post-merge integration verification)
- [ ] `docker compose exec db psql ...` schema inspection (same as above)

## Out of scope (per step42.md)

- Stripe integration / subscription plans (Step 49).
- Automated initial balance provisioning for new users.
- Per-token-type or per-provider pricing/cost multipliers.
- Estimating in-flight request cost before completion.
- Web frontend balance/usage UI (Step 48).
- Room-role-based authorization on billing endpoints.
- Refunds/chargebacks beyond the generic `adjustment` transaction type.
