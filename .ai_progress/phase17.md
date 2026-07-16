# Phase 17: Stripe Billing

**Goal**: Purchase subscriptions and on-demand tokens via Stripe.

**Delivered by**: `docs/tasks/step49.md` (Server: Stripe billing, test mode, + subscription lifecycle),
`docs/tasks/step53.md` (Web: plans, Stripe Checkout, subscription management, billing history UI). See those files
for the full per-file implementation notes, and `phases.md` → Deviations from this plan item 5 / this repo's
`web/e2e/regression/README.md` for the accepted steady state around the Checkout/webhook E2E legs (they self-skip
without real Stripe test-mode credentials).

---

## Step 49: Server Stripe billing (test mode) + subscription lifecycle

- [x] `server/schema.sql`: `subscriptions` / `payment_history` tables; migration via `task migrate:generate --
      add_subscriptions_and_payment_history`
- [x] `github.com/stripe/stripe-go` added; billing domain package (Step 42's `domain/billing`) extended with
      `Subscription`/`PaymentRecord` entities, `SubscriptionRepository`/`PaymentRepository` (idempotent on
      `stripe_event_id`)
- [x] `StripeGateway` port (`domain/billing/gateway.go`) — Checkout session creation (subscription + one-time token
      purchase), billing-portal session, cancel-at-period-end, webhook-event construction — never imports
      `stripe-go` from the domain layer
- [x] `server/internal/interface/gateway/stripe_client.go` — `StripeGateway` implementation, following the
      `llm_client.go` pattern; handles `checkout.session.completed`, `customer.subscription.updated/.deleted`,
      `invoice.paid`
- [x] Billing usecase extended: checkout-session creation, webhook processing (subscription upsert, token credit
      on invoice paid, idempotent replay), cancel-subscription
- [x] `POST /webhooks/stripe` — public, unauthenticated, raw-body signature verification (`Stripe-Signature`
      header); always `200` for processed/ignored events, `400` only on signature failure
- [x] DTOs: `CreateCheckoutSessionRequest/Response`, `SubscriptionResponse`, `PaymentRecordResponse`,
      `BillingPlanResponse`/`ListResponse`
- [x] `Config.StripeSecretKey`/`WebhookSecret` (optional; billing endpoints 503 cleanly when unconfigured),
      `StripePlans`/`StripeTokenPackages` parsed from single-line JSON env vars (display fields duplicated in
      config, not fetched from Stripe)
- [x] `docker-compose.yml`: `stripe-cli` service (`stripe listen`, forwards webhooks to `api`); `Taskfile.yml`:
      `stripe:listen` task
- [x] `.env.example`: `# === Stripe (Phase 16-17, test mode) ===` section

## Step 53: Web plans, Checkout, subscription management, billing history

- [x] `web/src/features/billing/types.ts` extended (additively, alongside Step 48's types) with
      `BillingPlan`/`Subscription`/`PaymentRecord`/`PaymentHistoryPage`
- [x] `api/get-plans.ts`, `create-checkout-session.ts`, `get-subscription.ts`, `create-billing-portal-session.ts`,
      `get-payment-history.ts` — thin fetch modules
- [x] Hooks: `use-plans`, `use-subscription`, `use-create-checkout-session` (full-page redirect to Stripe-hosted
      Checkout), `use-create-billing-portal-session`, `use-payment-history` (cursor-paginated)
- [x] `PlanCard`/`PlanList`, `SubscriptionSummary` (status badge, cancel-at-period-end notice, manage/change-plan
      actions), `PaymentHistoryList` (paginated table)
- [x] `app/(main)/billing/layout.tsx` — sub-navigation (Plans/Subscription/History/Usage) wrapping Step 48's
      existing `/billing/usage` route without editing it
- [x] `web/e2e/billing.spec.ts` / `web/e2e/regression/billing.spec.ts` — Checkout-journey legs self-skip without
      live Stripe credentials (documented in `web/e2e/regression/README.md`), everything reachable without them
      (plan catalog, empty states, error paths) is asserted

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [ ] `web/e2e/billing.spec.ts` / `regression/billing.spec.ts` full Checkout journey against the live compose stack
      with real Stripe test-mode credentials — requires the full stack plus `stripe listen`; skipped (accepted
      steady state, see `web/e2e/regression/README.md`)
