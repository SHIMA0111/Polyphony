# Phase 15: OAuth Social Login

**Goal**: Login with Google/GitHub accounts.

**Delivered by**: `docs/tasks/step44.md` (OAuth social login via Kratos OIDC — dex mock + Google/GitHub),
`docs/tasks/step55.md` (Ory Hydra OAuth2/OIDC provider + first-party client demo). See those files for the full
per-file implementation notes, and `phases.md` → Deviations from this plan (items 2-3) for the two accepted
deviations this phase carries (dex mock OIDC standing in for real Google/GitHub; Hydra's demo client being
first-party rather than an external third-party consumer).

---

## Step 44: OAuth social login via Kratos OIDC (dex mock + Google/GitHub)

- [x] `ory/dex/config.yaml` — static `dex` config (issuer, in-memory storage, one `staticClients` entry, one
      `staticPasswords` fixture test user, `skipApprovalScreen: true` for unattended automation)
- [x] `docker-compose.yml`: `dex` service (pinned image, healthchecked); `kratos`/`kratos-e2e` extended with
      `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS` and a `depends_on: dex` health gate
- [x] `ory/kratos/kratos.yml`: `selfservice.methods.oidc.enabled: true`, `registration.after.oidc.hooks: [{hook:
      session}]` (first-time OIDC sign-up establishes a session, not just an identity)
- [x] `ory/kratos/oidc/{dex,google,github}.jsonnet` trait mappers (dex verified end to end; google/github share the
      same structure, unexercised by automation per the accepted deviation)
- [x] `.env.example`: dex section extended with `DEX_STATIC_TEST_EMAIL`/`PASSWORD`, `KRATOS_OIDC_PROVIDERS_JSON`
- [x] `web/src/features/auth/components/SocialLoginButtons.tsx` — renders one submit button per `oidc`-group flow
      node, wired into `LoginForm`/`RegisterForm`
- [x] `web/playwright.config.ts`: dex host-resolution mapping for the chromium project
- [x] `web/e2e/support/fixtures.ts`: `DEX_FIXTURE_USER`; `web/e2e/oauth-dex.spec.ts` (registration + repeat-login
      cases)

## Step 55: Ory Hydra OAuth2/OIDC provider + first-party demo client

- [x] `ory/hydra/hydra.yml` — non-secret Hydra config (cookie/token TTLs, opaque access tokens, login/consent/logout
      URLs); secrets/DSN supplied via env at compose runtime
- [x] `docker-compose.yml`: `hydra-db`, `hydra-migrate`, `hydra` services (mirroring the Kratos service trio),
      alphabetically inserted
- [x] `web/src/lib/kratos-session.ts` (server-only whoami helper), `web/src/lib/hydra-admin.ts` (login/consent
      admin-API client)
- [x] `web/src/app/(auth)/oauth/login/route.ts` / `.../consent/route.ts` — Hydra login/consent challenge handlers
      backed by the existing Kratos session, no interactive consent screen (first-party-only demo scope)
- [x] `Taskfile.yml`: `oauth:hydra:register-demo-client`, `oauth:hydra:test`
- [x] `ory/hydra/test-oauth-flow.ts` — self-contained Bun script driving a full authorization-code round trip
      (register → Hydra authorize → login/consent redirect chain → token exchange → `/userinfo`)
- [x] `.env.example`: `# === Ory Hydra ===` section (`HYDRA_DB_*`, `HYDRA_DSN`, `HYDRA_PUBLIC_URL`/`ADMIN_URL`,
      `HYDRA_LOGIN_UI_URL`/`CONSENT_UI_URL`, `HYDRA_SYSTEM_SECRET`, `HYDRA_DEMO_CLIENT_ID`/`SECRET` placeholders)
- [x] `web/e2e/oauth-hydra.spec.ts` — authorization-code flow spec against `HYDRA_DEMO_FIXTURE_USER`

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./... && go test ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [x] `web/e2e/oauth-dex.spec.ts` / `web/e2e/oauth-hydra.spec.ts` against the live compose stack (dex/Kratos/Hydra
      containers) — verified live in the wave-9 integration review: both specs passed in the full-suite E2E runs
      (dex sign-in + account reuse; full Hydra authorization-code round trip through token exchange).
