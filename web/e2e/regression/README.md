# Regression suite: billing and room-fork environment notes

This note documents the environment variables `regression/billing.spec.ts`
needs to exercise its full Stripe Checkout / webhook / subscription-cancel
journey, and the gap that currently makes that portion skip locally. See
`regression/billing.spec.ts`'s own header doc comment for the same
information inline; this file exists as a second, easy-to-find pointer.

## The gap

`api-e2e` (this repo's isolated E2E compose profile, `docker-compose.yml`)
has **no** `STRIPE_*` environment configured, and there is no `stripe-cli`
(or equivalent) webhook-forwarding service wired into the `test` profile.
Compare this to the dev stack's `api` service, which reads
`STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` / `STRIPE_PLANS_JSON` /
`STRIPE_TOKEN_PACKAGES_JSON` from `.env` (see `.env.example`), and to the
dev-only `stripe-cli` compose service (`task stripe:listen`) that forwards
webhooks to it.

Per this step's (Step 59) scope, adding that infra to the `test` profile is
out of bounds — it is a transitive dependency of Step 49's server-side
Stripe integration, not something an E2E-spec-only step is allowed to
build. `regression/billing.spec.ts`'s Checkout-journey test probes for this
configuration at runtime (`GET /billing/plans` + a real
`POST /billing/checkout-session` call) and calls `test.skip()` with a clear
reason when it is absent, after first asserting everything that *is*
testable without live Stripe credentials (the plan catalog's
Stripe-API-free `200`, the `/billing/plans` page's empty state, and the
`POST /billing/checkout-session` error path's documented `503`).

## Running the full Checkout journey locally

1. Add the following to `api-e2e`'s `environment:` block in
   `docker-compose.yml` (see `.env.example` for the exact shape of the JSON
   catalog vars — copy real Stripe test-mode values, not the placeholders):
   - `STRIPE_SECRET_KEY` — a real Stripe test-mode secret key
     (`sk_test_...`), from your own Stripe dashboard (test mode).
   - `STRIPE_WEBHOOK_SECRET` — see step 2 below.
   - `STRIPE_PLANS_JSON` / `STRIPE_TOKEN_PACKAGES_JSON` — at least one
     monthly plan, referencing a real test-mode Stripe Price ID you created
     yourself (Stripe's API is never queried to populate this catalog; the
     Price ID must already exist).
2. Run `stripe login` once, then start forwarding webhooks to the E2E API's
   published host port:
   ```
   stripe listen --forward-to localhost:8090/webhooks/stripe --print-secret
   ```
   Copy the printed `whsec_...` value into `STRIPE_WEBHOOK_SECRET` above.
3. Restart `api-e2e` (`docker compose -p polyphony-e2e --profile test up -d api-e2e`)
   so it picks up the new environment.
4. Keep `stripe listen` running for the duration of the test run — the
   Checkout-completion webhook it forwards is what actually updates the
   subscription record; without it, the spec's post-redirect polling on
   `/billing/subscription` would time out.

## Room fork

`regression/room-fork.spec.ts` needs no additional environment — it reuses
the same `setRoomArchived` (direct `db-e2e` `UPDATE rooms SET is_archived`)
pattern Step 52's own `room-fork.spec.ts` established to deterministically
re-create the transient archived-destination window, since the real fork
job on this stack's tiny fixture rooms completes in milliseconds and the
room page is RSC-prefetched server-side, so neither a live race nor
client-side response interception can observe it reliably.
