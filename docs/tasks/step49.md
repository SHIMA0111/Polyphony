# Step 49: Server: Stripe billing (test mode) + subscription lifecycle

## Meta
- **Type**: feature
- **Components**: server, infra
- **Wave**: 5 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 42: Server: token balance management
- **Unlocks**: Step 53: Web: plans, Stripe Checkout, subscription management, billing history UI
- **Size**: 1 PR (a few hours for one AI agent)

## Context
`phases.md` Phase 17 ("Stripe Billing") lets a user actually pay for the token balance that Step 42 introduced: monthly subscription plans that grant a recurring token allocation, and on-demand token top-up purchases, both via Stripe Checkout, with server-side webhook processing crediting `token_balances`. Step 42 already ships the read side of billing — a balance endpoint, a paginated transaction-history endpoint (`TokenTransaction.type` already includes a `"charge"` variant reserved for exactly this kind of external credit), and a pre-call balance guard in `SendAIMessage` that returns HTTP 402 on insufficient balance — but today nothing can ever *add* tokens to a balance outside of whatever manual/seed mechanism Step 42 used for local testing. This step closes that gap end-to-end in test mode: Stripe Checkout Sessions for both subscriptions and one-time token purchases, a signature-verified webhook handler that credits balances and tracks subscription state, and subscription status/cancellation endpoints — all runnable and verifiable locally against Stripe's test-mode API via `stripe-cli` forwarding, with no dependency on a publicly reachable webhook URL.

## Goal
After this PR, an authenticated user can request a Stripe Checkout Session for either a subscription plan or a one-time token package and receives a redirect URL to Stripe's test-mode Checkout page; completing checkout (or, for automated verification, `stripe trigger`-fired test events forwarded by `stripe-cli`) causes the Go API's webhook endpoint to verify the event signature, persist/update a `subscriptions` row and a `payment_history` row, and credit the user's `token_balances` via Step 42's balance-mutation path with a `"charge"`-typed `token_transactions` entry — atomically, and idempotently against Stripe's at-least-once delivery guarantee. A user can fetch their current subscription status and cancel it (Stripe cancels at the end of the current billing period, not immediately), or request a Stripe Billing Portal session URL for self-service plan/payment-method management. Everything runs locally via `docker compose up` plus `stripe-cli listen --forward-to`, using Stripe test-mode keys documented in `.env.example`; no real card or webhook infrastructure is required to verify the feature.

## Scope
- [x] Add `subscriptions` and `payment_history` tables to `server/schema.sql` (exact DDL in Implementation notes) and generate the Atlas migration with `task migrate:generate -- add_subscriptions_and_payment_history`.
- [x] Add `github.com/stripe/stripe-go/v81` (or the latest `v8x` major available at implementation time) to `server/go.mod` via `go get`, then `go mod tidy`. This is an additive dependency addition only.
- [x] Locate the domain/usecase/handler package Step 42 created for billing (most likely `server/internal/domain/billing`, `server/internal/usecase/billing`, `server/internal/interface/handler/billing_handler.go`, mounted under a `/billing/*` route group — confirm the actual names before editing rather than assuming) and **extend it** with this step's subscription/payment concerns; do not create a second, parallel billing package. If Step 42 instead used different names, adapt every reference below to match what actually exists — the requirement is that subscription/payment logic lives alongside, and reuses the balance-mutation path of, Step 42's existing billing code, not in an isolated silo.
- [x] Add `Subscription` and `PaymentRecord` entities to the billing domain package (`.../domain/billing/entity.go` or equivalent): `Subscription{ID, UserID, StripeCustomerID, StripeSubscriptionID, StripePriceID, PlanCode, Status, MonthlyTokenAllocation, CurrentPeriodStart, CurrentPeriodEnd, CancelAtPeriodEnd, CanceledAt *time.Time, CreatedAt, UpdatedAt}`; `PaymentRecord{ID, UserID, SubscriptionID *string, PaymentRail, StripeEventID, StripeReferenceID, Kind, AmountCents, Currency, TokensCredited, Status, CreatedAt}`.
- [x] Add `SubscriptionRepository` (`Create`, `GetByUserID`, `GetByStripeSubscriptionID`, `Update`) and `PaymentRepository` (`Create` — must be idempotent on the unique `stripe_event_id` constraint, returning a distinguishable "already recorded" outcome rather than erroring the whole webhook handler; `ListByUserID` with cursor pagination matching Step 42's `ListTransactions` cursor convention) interfaces to the billing domain package's `repository.go` (GoDoc on every method per `server/internal/domain/room/repository.go`'s style, documenting `domain.ErrNotFound` behavior).
- [x] Add a `StripeGateway` port interface to the billing domain package (e.g. `.../domain/billing/gateway.go`): `CreateSubscriptionCheckoutSession(ctx, params SubscriptionCheckoutParams) (checkoutURL string, err error)`, `CreateTokenPurchaseCheckoutSession(ctx, params TokenPurchaseCheckoutParams) (checkoutURL string, err error)`, `CreateBillingPortalSession(ctx, stripeCustomerID, returnURL string) (portalURL string, err error)`, `CancelSubscriptionAtPeriodEnd(ctx, stripeSubscriptionID string) error`, `ConstructWebhookEvent(payload []byte, sigHeader string) (WebhookEvent, error)` where `WebhookEvent` is a small domain-owned struct (`ID, Type string`, plus the minimal typed fields each handled event needs — see Implementation notes) so the domain layer never imports the `stripe-go` SDK directly.
- [x] Implement `StripeGateway` in `server/internal/interface/gateway/stripe_client.go` using `stripe-go`, following the existing `gateway.LLMClient` pattern in `server/internal/interface/gateway/llm_client.go` (a struct wrapping a configured client, one method per port operation, domain-level errors on failure). `ConstructWebhookEvent` wraps `webhook.ConstructEvent` from `stripe-go`'s `webhook` package and maps the handled event types (`checkout.session.completed`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.paid`) into the domain `WebhookEvent` struct; unrecognized event types map to a `WebhookEvent{Type: event.Type}` with no extra fields so the usecase can safely no-op on anything it doesn't handle.
- [x] Extend the billing usecase with:
  - `CreateSubscriptionCheckoutSession(ctx, userID, planCode string) (checkoutURL string, err error)` — resolves `planCode` against the configured plan list (see config below), returning a domain error (e.g. `domain.ErrNotFound`) for an unknown code; passes `userID` as Stripe Checkout's `client_reference_id` and as `metadata.user_id` (belt-and-suspenders, since the webhook needs the mapping back to a local user and Checkout Sessions don't always echo `client_reference_id` onto every downstream event) plus `metadata.plan_code`.
  - `CreateTokenPurchaseCheckoutSession(ctx, userID, packageCode string) (checkoutURL string, err error)` — same shape, for one-time token packages, with `metadata.kind = "token_purchase"` and `metadata.package_code`.
  - `GetSubscription(ctx, userID string) (*billing.Subscription, error)` — returns `domain.ErrNotFound` if the user has no subscription row.
  - `CancelSubscription(ctx, userID string) error` — loads the user's subscription, calls `StripeGateway.CancelSubscriptionAtPeriodEnd`, and persists `cancel_at_period_end = true` locally (final period-end status transition arrives later via the `customer.subscription.updated`/`.deleted` webhook, not synchronously here).
  - `CreateBillingPortalSession(ctx, userID, returnURL string) (portalURL string, err error)` — requires an existing `stripe_customer_id` on the user's subscription row; returns a domain error if none exists yet (nothing to manage).
  - `ListPaymentHistory(ctx, userID string, cursor string, limit int) (*PaymentHistoryPage, error)` — mirrors Step 42's `ListTransactions` cursor/limit contract.
  - `HandleWebhookEvent(ctx, payload []byte, sigHeader string) error` — calls `StripeGateway.ConstructWebhookEvent`, then dispatches by `event.Type` (see the exact per-event behavior in Implementation notes). Every code path that credits tokens or writes `payment_history` must do so via a single call encompassing (a) inserting the `payment_history` row keyed on the unique `stripe_event_id`, using its idempotency outcome to short-circuit before touching the balance if the event was already processed, and (b) crediting `token_balances`/inserting a `"charge"`-typed `token_transactions` row through whatever balance-mutation entry point Step 42's usecase exposes (a public method if one exists, or the same repository calls Step 42's own credit path uses) — both writes in one DB transaction so a credited balance and its payment record can never diverge.
- [x] Add the corresponding HTTP endpoints to the billing handler and wire them into the authenticated route group (mirroring how `/billing/balance`/`/billing/transactions` are already registered from Step 42):
  - `POST /billing/checkout-session` — body `{"type": "subscription"|"token_purchase", "plan_code"?: string, "package_code"?: string}` → `201 {"checkout_url": string}`; `400` for an unknown type/missing code combination, `404`/domain-mapped error for an unknown plan/package code.
  - `POST /billing/portal-session` — body `{"return_url": string}` → `200 {"portal_url": string}`; `404`/domain error if the user has no Stripe customer yet.
  - `GET /billing/subscription` → `200` with the subscription, or `204 No Content` if none exists.
  - `POST /billing/subscription/cancel` → `200` with the updated subscription (`cancel_at_period_end: true`); `404` if none exists.
  - `GET /billing/payments?cursor=&limit=` → `200 {"payments": [...], "next_cursor": string|null}`, following the exact same cursor-pagination response shape as Step 42's `GET /billing/transactions`.
  - `GET /billing/plans` → `200 {"plans": [{"code", "name", "description", "price_cents", "currency", "interval": "month"|"one_time", "token_allowance"}]}` — the purchasable catalog Step 53's plan-selection UI renders. Pure config read: merges `StripePlans` (as `interval: "month"`, `token_allowance = monthly_token_allocation`) and `StripeTokenPackages` (as `interval: "one_time"`, `token_allowance = tokens`) from the loaded `Config`; no Stripe API call, no DB access, `stripe_price_id` is deliberately NOT exposed. Registered on the authenticated group like the other `/billing/*` reads. Returns `200` with an empty `plans` array when Stripe is unconfigured (this endpoint alone never `503`s).
- [x] Add a **public** (unauthenticated, outside the JWT-protected route group) `POST /webhooks/stripe` route, registered alongside `/health`/`/auth/*` in the same route-registration location Step 42/earlier steps use. This handler must read the **raw** request body (`io.ReadAll(c.Request().Body)`, *not* `c.Bind`, since Stripe's signature is computed over the exact raw bytes) and the `Stripe-Signature` header, then call `HandleWebhookEvent`. Always return `200` for any event the handler processed or intentionally ignored (unrecognized `event.Type`, already-processed idempotency short-circuit); return `400` only when signature verification itself fails.
- [x] Add DTOs to the handler package's `dto.go` (or wherever Step 42 put billing DTOs), following the file's existing `--- Section ---` comment-grouping convention: `CreateCheckoutSessionRequest`, `CheckoutSessionResponse`, `BillingPortalRequest`, `BillingPortalResponse`, `SubscriptionResponse` (JSON fields: `status`, `plan_code`, `monthly_token_allocation`, `current_period_start`, `current_period_end`, `cancel_at_period_end`, `canceled_at` — Step 53's web types are written against exactly these names), `PaymentRecordResponse` (JSON fields: `id`, `kind`, `amount_cents`, `currency`, `tokens_credited`, `status`, `created_at`), `PaymentHistoryResponse` (`payments`, `next_cursor`), and `BillingPlanResponse`/`BillingPlanListResponse` for the catalog endpoint (`code`, `name`, `description`, `price_cents`, `currency`, `interval`, `token_allowance`).
- [x] Extend `server/internal/infrastructure/config/config.go`'s `Config` struct and `Load()` with: `StripeSecretKey`, `StripeWebhookSecret` (both optional/empty-default, matching this file's existing lenient handling of `LLM_GATEWAY_URL`-style optional third-party config — do not make server startup fail without them; billing-checkout/webhook endpoints instead return a clear `503`/domain error when Stripe isn't configured), `StripePlans []StripePlan` and `StripeTokenPackages []StripeTokenPackage` parsed from single-line JSON env vars (`STRIPE_PLANS_JSON`, `STRIPE_TOKEN_PACKAGES_JSON`) using the same whole-array-as-JSON-string convention already established for `KRATOS_OIDC_PROVIDERS_JSON`. Each plan/package entry carries both the Stripe wiring fields (`plan_code`/`package_code`, `price_id`, `monthly_token_allocation`/`tokens`) and the display fields the `GET /billing/plans` catalog serves (`name`, `description`, `price_cents`, `currency`) — prices are duplicated in config rather than fetched from Stripe so the catalog works without a Stripe API call; keeping them in sync with the dashboard's test Prices is the developer's responsibility, noted in `.env.example`. Also add `StripeCheckoutSuccessURL`/`StripeCheckoutCancelURL`, defaulting to `http://localhost:3000/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}` / `http://localhost:3000/billing/checkout/cancel` (matching Step 53's post-checkout routes; `{CHECKOUT_SESSION_ID}` is Stripe's own template placeholder, passed through verbatim to the Checkout Session's `success_url`).
- [x] Register the new repository/usecase methods and the webhook route in the DI container (`server/internal/app/container.go`, if Step 1's app-container refactor has landed by the time this step is implemented — otherwise locate the actual current wiring in `server/cmd/api/main.go` and extend it there instead) and add the new routes to whatever route registrar Step 42 used for `/billing/*` (e.g. `server/internal/app/routes_billing.go`), plus the new public `/webhooks/stripe` route alongside the other public routes.
- [x] Add a `stripe-cli` service to `docker-compose.yml`, inserted at its alphabetical position among top-level service blocks (see conflictNotes) — image `stripe/stripe-cli` (pin a specific tag), `command: listen --api-key ${STRIPE_SECRET_KEY} --forward-to api:8080/webhooks/stripe --skip-verify`, `depends_on: api` (`condition: service_started`; the `api` container doesn't need to be healthy, just running, before `stripe listen` starts retrying its connection). Document in `.env.example`/README that `docker compose logs stripe-cli` prints the ephemeral webhook signing secret (`whsec_...`) the first time it starts, which must be copied into `.env`'s `STRIPE_WEBHOOK_SECRET` and the `api` service restarted for signature verification to succeed locally.
- [x] Add a `stripe:listen` task to `Taskfile.yml` (new `# Billing (Stripe test mode)` section, additive) that runs `docker compose up -d stripe-cli && docker compose logs -f stripe-cli` so a developer can grab the printed webhook secret without hunting through general logs.
- [x] Add a `# === Stripe (test mode) ===` section to `.env.example` with `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PLANS_JSON`, `STRIPE_TOKEN_PACKAGES_JSON`, `STRIPE_CHECKOUT_SUCCESS_URL`, `STRIPE_CHECKOUT_CANCEL_URL` — real-shaped test-mode placeholder values (Stripe test secret keys/price IDs are safe to show as illustrative placeholders since they only work against a developer's own test-mode Stripe account).
- [x] Add unit tests for the extended billing usecase (hand-written mocks per the project's existing convention / `server/internal/testutil/mocks` if Step 2's shared-mocks package exists by then): checkout-session creation for both types (unknown plan/package code errors), `GetSubscription`/`CancelSubscription`/`CreateBillingPortalSession` (including the "no Stripe customer yet" error path), and — most importantly — `HandleWebhookEvent` driven entirely by **mocked `WebhookEvent` values** (never a real Stripe signature or network call): `checkout.session.completed` for a token purchase credits the right token amount and writes one `payment_history` row; `invoice.paid` for a subscription renewal credits `monthly_token_allocation` tokens; `customer.subscription.updated`/`.deleted` update local subscription status/period fields; replaying the identical event a second time (same `stripe_event_id`) is a no-op on the balance (only one `token_transactions` row is ever created for that event) while still returning success.
- [x] Add unit tests for the new/extended billing handler (mocked usecase), covering request validation, the raw-body/signature-verification path for `/webhooks/stripe` (valid signature → success; tampered payload or wrong secret → `400`), the `204`/`404` no-subscription paths, and `GET /billing/plans` (returns the merged, correctly-mapped catalog from a fixture config; returns `200` with an empty array when no plans/packages are configured).
- [x] Add a testcontainers-backed integration test for the new repository methods (`server/internal/interface/repository/postgres/..._test.go`, using Step 2's `server/internal/testutil/postgres` helper if that harness exists by then) covering `SubscriptionRepository` CRUD and `PaymentRepository.Create`'s idempotent-on-`stripe_event_id` behavior against a real PostgreSQL instance.

## Out of scope
- Any web frontend work (plan selection page, Stripe.js/redirect-to-checkout wiring, billing history UI) — Step 53, which consumes exactly the endpoints this step ships.
- App Store / Google Play billing and the shared `payment_rail` semantics beyond a `'stripe'`-defaulted column for forward compatibility — Phase 25, explicitly excluded from this project's scope per the binding decisions.
- Real (non-test-mode) Stripe products/prices, live webhooks, or any production deployment concern — everything here is Stripe test mode only, matching the plan's "external SaaS is test-mode only" verification strategy.
- Proration, plan upgrades/downgrades, multi-seat/team billing, coupons/promotion codes, or tax handling — a single flat plan-code → price-id → monthly-token-allocation mapping is the entire pricing model for this step.
- Enforcing/consuming the credited balance during AI sends — that guard already exists from Step 42 (`SendAIMessage`'s pre-call check); this step only adds a way to top it up.
- Any change to `token_balances`/`token_transactions` table shape — this step only reads/calls into Step 42's existing balance-mutation path; it does not alter those tables' columns.
- Redis-backed rate limiting on the webhook endpoint — Phase 10 territory from an earlier wave, already delivered.
- Retrying/backoff logic beyond what Stripe's own webhook delivery retries provide — the handler simply returns `200`/`400` per event; it does not run a background reconciliation job against Stripe's API.

## Implementation notes

### Schema — append to `server/schema.sql` (after the tables Step 42 added, matching the existing file's style)
```sql
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stripe_customer_id VARCHAR(255) NOT NULL,
    stripe_subscription_id VARCHAR(255) NOT NULL,
    stripe_price_id VARCHAR(255) NOT NULL,
    plan_code VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    monthly_token_allocation BIGINT NOT NULL,
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end TIMESTAMPTZ NOT NULL,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
    canceled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subscriptions_stripe_subscription_id_unique UNIQUE (stripe_subscription_id)
);

CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);

CREATE TABLE payment_history (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
    payment_rail VARCHAR(20) NOT NULL DEFAULT 'stripe',
    stripe_event_id VARCHAR(255) NOT NULL,
    stripe_reference_id VARCHAR(255) NOT NULL DEFAULT '',
    kind VARCHAR(20) NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'usd',
    tokens_credited BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_history_stripe_event_id_unique UNIQUE (stripe_event_id)
);

CREATE INDEX idx_payment_history_user_id ON payment_history(user_id, created_at DESC);
```
`payment_rail` defaults to `'stripe'` and is not otherwise used in this step; it exists purely so a future App Store/Google Play billing step (Phase 25, excluded from this project) can reuse the same table without an additional migration, matching `CLAUDE.md`'s domain-logic note that all three rails converge into the same billing tables. `kind` is `'subscription'` or `'token_purchase'`. `stripe_event_id` is the idempotency key: a webhook handler that inserts this row first (inside the same transaction as any balance credit) and treats a unique-violation on this constraint as "already processed, no-op" — following the exact `pgerrcode.UniqueViolation`/`pgconn.PgError`/`ConstraintName` pattern already used in `server/internal/interface/repository/postgres/user_repository.go` — gets Stripe's at-least-once delivery idempotency for free.

Run `task migrate:generate -- add_subscriptions_and_payment_history` from the repo root to produce the Atlas migration file under `server/migrations/`; resolve any `atlas.sum` conflict mechanically by re-running the generator after rebasing, per the project's schema conflict convention.

### Config / env shape (`.env.example` addition, illustrative — keep JSON values one line in the real file)
```
# === Stripe (test mode) ===
STRIPE_SECRET_KEY=sk_test_your-stripe-secret-key
# Populated by `docker compose logs stripe-cli` on first run (see stripe-cli service) — copy the
# printed whsec_... value here and restart the api service.
STRIPE_WEBHOOK_SECRET=whsec_replace-with-value-from-stripe-cli-logs
# Display fields (name/description/price_cents/currency) feed GET /billing/plans — keep price_cents
# in sync with the corresponding test-mode Price in your Stripe dashboard.
STRIPE_PLANS_JSON=[{"plan_code":"starter","price_id":"price_test_starter","name":"Starter","description":"100K tokens per month","price_cents":500,"currency":"usd","monthly_token_allocation":100000},{"plan_code":"pro","price_id":"price_test_pro","name":"Pro","description":"1M tokens per month","price_cents":2000,"currency":"usd","monthly_token_allocation":1000000}]
STRIPE_TOKEN_PACKAGES_JSON=[{"package_code":"topup_small","price_id":"price_test_topup_small","name":"Small top-up","description":"50K tokens, one time","price_cents":300,"currency":"usd","tokens":50000},{"package_code":"topup_large","price_id":"price_test_topup_large","name":"Large top-up","description":"500K tokens, one time","price_cents":1500,"currency":"usd","tokens":500000}]
STRIPE_CHECKOUT_SUCCESS_URL=http://localhost:3000/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}
STRIPE_CHECKOUT_CANCEL_URL=http://localhost:3000/billing/checkout/cancel
```
The `price_test_...` values are placeholders — a developer wanting to exercise a real Checkout redirect (as opposed to the `stripe trigger`-driven webhook verification in this step's own test suite) must create matching test-mode Products/Prices in their own Stripe dashboard and swap these IDs in their local `.env`; this step's own verification does not require that (see Verification).

### Webhook event handling (dispatch table for `HandleWebhookEvent`)
| `event.Type` | Action |
|---|---|
| `checkout.session.completed` (session `metadata.kind == "token_purchase"`) | Insert `payment_history` (`kind="token_purchase"`, `tokens_credited` from the matched package), credit `token_balances` via Step 42's balance-mutation path with a `"charge"` transaction, `room_id = null`. |
| `checkout.session.completed` (subscription mode) | Upsert the `subscriptions` row (`stripe_customer_id`/`stripe_subscription_id`/`plan_code` from `metadata`/the session object); no token credit here — the first credit arrives via `invoice.paid` so subscription and one-time purchase share one crediting code path. |
| `invoice.paid` (`billing_reason` in `subscription_create`/`subscription_cycle`) | Insert `payment_history` (`kind="subscription"`, `tokens_credited = subscription.monthly_token_allocation`), credit `token_balances` via the same `"charge"` path, linked to the local `subscriptions.id`. |
| `customer.subscription.updated` | Update the matching `subscriptions` row's `status`, `current_period_start`/`current_period_end`, `cancel_at_period_end`. |
| `customer.subscription.deleted` | Update `status = "canceled"`, set `canceled_at`. |
| anything else | No-op; return success so Stripe does not retry. |

### Reference files to read before editing
- `server/internal/interface/gateway/llm_client.go` — the exact adapter-implementing-a-domain-port pattern to follow for `stripe_client.go` (struct + configured client + domain-error wrapping).
- `server/internal/interface/repository/postgres/user_repository.go` and `room_repository.go` — `pgerrcode.UniqueViolation`/`ConstraintName` idempotency-detection pattern, transactional multi-statement writes (`tx.Begin`/`defer tx.Rollback`/`tx.Commit`), `pgx.ErrNoRows` → `domain.ErrNotFound` mapping.
- `server/internal/interface/handler/room_handler.go` and `dto.go` — handler/DTO style, `middleware.GetUserID`, `handleXError` helper convention.
- `server/internal/infrastructure/config/config.go` — `Load()`'s existing required-vs-defaulted env var pattern to extend, not replace.
- `server/cmd/api/main.go` — current route registration and public-vs-authenticated route grouping (`e.GET("/health", ...)` vs. `auth := e.Group("", middleware.JWTAuth(authUC))`); the new `/webhooks/stripe` route must be public like `/health`/`/auth/*`, not inside the `auth` group, since Stripe cannot present a JWT.
- `docker-compose.yml` and `Taskfile.yml` — current service/task block shapes and the `${VAR:-default}` convention used throughout.
- `.env.example` — existing `# === Name ===` section header convention.
- Whatever Step 42 actually created for `server/internal/domain/billing`, `server/internal/usecase/billing`, and the `/billing/balance`/`/billing/transactions` handler/routes — read this first and extend it; this is the single most important reference for this step, since duplicating it under a different package name would fragment the billing bounded context.

### Conflict notes
Per the plan's schema conflict convention, this step and Step 42 (wave 4, already merged before this step starts per its `dependsOn` edge) together own the entire billing bounded context but touch disjoint tables — Step 42 owns `token_balances`/`token_transactions`; this step adds only the new `subscriptions`/`payment_history` tables. `docker-compose.yml`'s edit is a single new, alphabetically-placed `stripe-cli` service block (falls between `postgres`/`migrate`-adjacent entries and `web` alphabetically — verify exact placement against whatever service names actually exist by this wave) plus one additive environment-variable set of lines on the pre-existing `api` service block (do not reorder or reformat any other service). `Taskfile.yml` gets one new additive task in a new `# Billing (Stripe test mode)` section. No other step in this wave touches `docker-compose.yml`'s `stripe-cli` block or `Taskfile.yml`'s new section.

## Verification
1. `cd server && go build ./...` — compiles with the new `stripe-go` dependency and billing extensions.
2. `cd server && go vet ./...` (`task lint:server`) — passes with no new issues.
3. `cd server && go test ./...` (`task test:server`) — all unit tests pass, including the new webhook-dispatch tests driven by mocked `WebhookEvent` values (no real Stripe signature/network call involved) and the idempotent-replay-is-a-no-op case.
4. `go test ./internal/interface/repository/postgres/... -run "TestSubscription|TestPayment" -v` (requires a local Docker daemon for testcontainers) — the new repository integration tests pass, including the unique-`stripe_event_id` idempotency case.
5. `task migrate:generate -- add_subscriptions_and_payment_history` then `task migrate:apply` against a clean `db` volume — the migration applies with no errors and `subscriptions`/`payment_history` exist with the documented columns.
6. `docker compose up -d db migrate api llm-gateway stripe-cli` then `docker compose logs stripe-cli` — a `whsec_...` signing secret is printed; copy it into `.env`'s `STRIPE_WEBHOOK_SECRET` and `docker compose up -d api` to recreate it with the new secret.
7. `curl -s localhost:8080/billing/plans -H "Authorization: Bearer <jwt>"` — returns `200` with the merged plan/package catalog from `.env`'s `STRIPE_PLANS_JSON`/`STRIPE_TOKEN_PACKAGES_JSON` (`interval` correctly `"month"` vs `"one_time"`, no `price_id` leaked).
8. With a real Stripe test-mode secret key set in `.env` (`STRIPE_SECRET_KEY=sk_test_...`, obtained free from any Stripe account's test-mode dashboard — no charge/production data involved): `curl -s -X POST localhost:8080/billing/checkout-session -H "Authorization: Bearer <jwt>" -H "Content-Type: application/json" -d '{"type":"token_purchase","package_code":"topup_small"}'` — returns `201` with a `checkout_url` beginning `https://checkout.stripe.com/...`.
9. `docker compose exec stripe-cli stripe trigger checkout.session.completed` (or run `stripe trigger` from a host-installed `stripe` CLI pointed at the same forwarding session) — `docker compose logs api` shows the webhook was received and returned `200`; if the triggered event's metadata doesn't match a real local user/package (expected, since `stripe trigger` fabricates generic sample data), confirm instead via a direct `curl -X POST localhost:8080/webhooks/stripe` request built with the Stripe CLI's `stripe trigger --override` or a manually constructed test payload signed with `stripe-cli`'s `--webhook-secret`, checked against the fixtures used in step 3's mocked unit tests — the objectively verifiable claim either way is: valid signature → `200` and (for a recognized event/metadata shape) a new `payment_history` row and incremented `token_balances.balance` via `GET /billing/balance`; tampered payload/signature → `400`.
10. Re-deliver the identical event a second time (Stripe CLI's `resend` or re-running the same `curl` payload/signature) — still `200`, but `SELECT count(*) FROM payment_history WHERE stripe_event_id = '<id>'` remains `1` and the balance is not double-credited.
11. `curl -s localhost:8080/billing/subscription -H "Authorization: Bearer <jwt>"` — `204` for a user with no subscription; after a subscription-mode checkout completes (webhook processed), returns `200` with the subscription's `status`/`current_period_end`.
12. `curl -s -X POST localhost:8080/billing/subscription/cancel -H "Authorization: Bearer <jwt>"` — `200` with `cancel_at_period_end: true` for a user with an active subscription; `404` for a user with none.
13. `curl -s -X POST localhost:8080/billing/portal-session -H "Authorization: Bearer <jwt>" -H "Content-Type: application/json" -d '{"return_url":"http://localhost:3000/billing"}'` — `200` with a `portal_url` for a user who has a `stripe_customer_id`; a domain-mapped error (e.g. `404`) for one who doesn't.
14. `task test` and `task lint` — full-repo test/lint run remains green (regression check that no other component broke).
15. `docker compose down` — stack (including `stripe-cli`) tears down cleanly with no orphaned containers.

## Completion criteria
- [x] `subscriptions` and `payment_history` tables exist per the documented DDL, added via an Atlas-generated migration.
- [x] `stripe-go` is added to `server/go.mod`; the billing domain/usecase/handler package is extended (not duplicated) with subscription and payment concerns.
- [x] `POST /billing/checkout-session`, `POST /billing/portal-session`, `GET /billing/subscription`, `POST /billing/subscription/cancel`, and `GET /billing/payments` all work as documented (verified via handler-level tests against a real `BillingUsecase`/mocked repos; not yet curl-verified against a running server — see Verification items 7-13, skipped in this isolated worktree run).
- [x] `POST /webhooks/stripe` verifies Stripe signatures, is registered as a public (non-JWT) route, and idempotently credits `token_balances`/writes `payment_history` for `checkout.session.completed` (token purchase) and `invoice.paid` (subscription renewal) events, updating subscription state for `customer.subscription.updated`/`.deleted`.
- [x] A `stripe-cli` service exists in `docker-compose.yml` and a `stripe:listen` task exists in `Taskfile.yml`, both documented in `.env.example`/inline comments for obtaining the local webhook signing secret.
- [x] Unit tests (usecase + handler, mocked Stripe gateway and webhook events) and testcontainers integration tests (repository) are added and pass.
- [ ] All verification checks above pass. (Verification items 1-4 and 14 ran and passed; items 5's `migrate:apply` half and items 6-13/15 require the full docker-compose stack on fixed ports and were not run in this isolated worktree — left for the post-merge integration review.)
