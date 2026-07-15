# Step 44: OAuth social login via Kratos OIDC (dex mock + Google/GitHub)

## Meta
- **Type**: feature
- **Components**: web, infra
- **Wave**: 5 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 10: Playwright E2E harness + seeded compose test stack + LLM stub; Step 11: Infra: Ory Kratos services in docker-compose; Step 30: Web: Kratos-driven login/registration + auth data-plane flip
- **Unlocks**: Step 56: E2E regression: auth, OAuth/Hydra, rooms, members, invitations, groups
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Phase 15 of `phases.md` requires "Login with Google/GitHub accounts". Step 11 stood up a bare Ory Kratos deployment with only the `password` method enabled, and Step 30 flipped the web app's auth data plane onto Kratos-driven browser flows (cookie session, BFF proxy forwarding the Kratos session cookie). This step turns on Kratos's `oidc` self-service method and adds the provider configuration, trait mapping, and web UI entry points needed to actually sign in with a third-party identity. Because real Google/GitHub OAuth apps require external, non-reproducible manual registration (a redirect URI, a client id/secret pair from a console only a human can create), this step's locally-and-automatically verifiable target is a `dex` (dexidp/dex) mock OIDC provider running in Docker Compose with a built-in static test user — the same mechanism Ory's own quickstarts use to test social login without a real IdP. Google and GitHub providers are wired into the same Kratos config so real credentials can be dropped into `.env` later, but they are not exercised by automated tests.

## Goal
After this PR, `docker compose up` brings up a `dex` container alongside the existing `kratos`/`kratos-db`/`kratos-migrate`/`mailslurper` services from Step 11, Kratos has `selfservice.methods.oidc` enabled with three configured providers (`dex`, `google`, `github`) whose client secrets and non-secret config live entirely in `.env` (never hardcoded in the checked-in `kratos.yml`), and each provider has a Jsonnet trait mapper producing `traits.email`/`traits.username` compatible with the identity schema from Step 11. The web login and registration pages (from Step 30) render "Continue with Dex" / "Continue with Google" / "Continue with GitHub" buttons that submit into Kratos's real browser OIDC flow. A new Playwright spec drives the dex path end to end on the Step 10 harness: a brand-new user can sign up via dex and land in a room, and re-authenticating with the same dex identity logs into the same account rather than creating a duplicate. Account-linking behavior (linking dex to an existing password account, and Kratos's default no-silent-merge behavior when emails collide) is verified directly against Kratos's self-service/admin APIs. Google/GitHub remain a documented, optional, manual path for anyone who registers real OAuth apps.

## Scope
- [ ] Add `ory/dex/config.yaml`: dex static configuration — `issuer: http://dex:5556/dex`, in-memory storage, a single `staticClients` entry (`id: kratos-dex-client`) whose `redirectURIs` list includes Kratos's OIDC callback URL for the `dex` provider on both the dev Kratos instance and the E2E Kratos instance (see Implementation notes), `oauth2.skipApprovalScreen: true` (no consent screen, needed for unattended automation), and one `staticPasswords` entry (a fixture test user with a bcrypt-hashed password) so Playwright can complete a real login without any external service.
- [ ] Add a `dex` service to `docker-compose.yml`: pinned `dexidp/dex` image tag, mounts `./ory/dex/config.yaml` read-only, `command: dex serve /etc/dex/cfg/config.yaml`, host port `5556:5556`, a healthcheck against dex's `/dex/healthz` (or `/healthz`) endpoint. No `profiles:` key — an unprofiled service always starts with plain `docker compose up`, and `docker compose --profile test up` also starts every unprofiled service in addition to the `test`-profiled ones, so a single `dex` block automatically serves both the dev stack and the Step 10 E2E stack; do not create a duplicate `dex-e2e` service.
- [ ] Insert the new `dex` block into `docker-compose.yml` at its alphabetical position among the existing top-level services (before `kratos`), touching no existing service block.
- [ ] Extend the existing `kratos` service block in `docker-compose.yml` (added by Step 11) with one new environment variable, `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS: ${KRATOS_OIDC_PROVIDERS_JSON}`, and `depends_on: dex` (`condition: service_healthy`). Note that Step 30 deliberately did NOT add an isolated `kratos-e2e` service — its `api-e2e`/`web-e2e` point at the single shared, unprofiled `kratos` service from Step 11 — so applying the env var to that one `kratos` block covers both the dev and E2E stacks. If the merged compose file differs (e.g. a later step did isolate a `kratos-e2e`), apply the same environment variable there too — the requirement is unchanged: every Kratos instance the web/E2E stacks talk to must read the same `KRATOS_OIDC_PROVIDERS_JSON`-shaped config.
- [ ] Edit `ory/kratos/kratos.yml` to add `selfservice.methods.oidc.enabled: true`. Do **not** add a `config.providers` list to this checked-in file — the entire providers array (including client secrets) is supplied at container start via the `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS` environment variable using Kratos's documented whole-value JSON-string override for array-typed config keys, so no secret ever lands in version control.
- [ ] Create three Jsonnet trait mappers under `ory/kratos/oidc/`: `dex.jsonnet`, `google.jsonnet`, `github.jsonnet`. Each reads `std.extVar('claims')` and emits `{ identity: { traits: { email: ..., username: ... } } }` matching the `traits.email`/`traits.username` fields required by `ory/kratos/identity.schema.json` (from Step 11). Derive `username` from `claims.preferred_username`/`claims.nickname`/`claims.given_name` where available, falling back to the local part of `claims.email`, since none of dex/Google/GitHub guarantee a claim named `username`.
- [ ] Add a `# === Ory Kratos: Social Login (OIDC) ===` section to `.env.example`: `DEX_STATIC_TEST_EMAIL`, `DEX_STATIC_TEST_PASSWORD` (plaintext, documented as test-only), and `KRATOS_OIDC_PROVIDERS_JSON` — a single-line JSON array literal with all three providers' `id`/`provider`/`client_id`/`client_secret`/`issuer_url`/`mapper_url`/`scope` fields, using real working values for `dex` (matching `ory/dex/config.yaml`'s static client) and clearly-labeled placeholder strings (`your-google-oauth-client-id`, etc.) for `google`/`github`. Add an inline comment stating that the `dex` entry's `client_secret` must stay in sync with `ory/dex/config.yaml`'s `staticClients[0].secret`.
- [ ] Create `web/src/features/auth/components/SocialLoginButtons.tsx`: a small Chakra v3 component that takes the current Kratos flow object (login or registration) as a prop, filters `flow.ui.nodes` for `group === "oidc"`, and renders a real HTML `<form action={flow.ui.action} method={flow.ui.method}>` wrapping a hidden `csrf_token` input (from the matching `default` group node) plus one `<Button type="submit" name="provider" value={node.attributes.value}>` per oidc node, each labeled "Continue with {Provider}" with a `lucide-react` icon. Submission must be a real form POST (full browser navigation), not a `fetch`/XHR call, because Kratos's OIDC flow requires following a 302 redirect chain out to the provider and back.
- [ ] Wire `SocialLoginButtons` into `web/src/features/auth/components/LoginForm.tsx` and `web/src/features/auth/components/RegisterForm.tsx` (both rewritten by Step 30 to fetch and render a Kratos self-service browser flow), rendering it below the password form with a `Text`/`Separator`-style "or continue with" divider when — and only when — the fetched flow contains at least one `oidc`-group node (so the UI degrades gracefully if OIDC is ever disabled).
- [ ] Update `ory/README.md` (from Step 11) with a "Social login (OIDC)" section: how dex's static test user works for local/automated testing, and a step-by-step "optional: use real Google/GitHub credentials" guide (create an OAuth app, set the authorization callback URL to `http://localhost:4433/self-service/methods/oidc/callback/{google|github}`, paste the client id/secret into the local `.env`'s `KRATOS_OIDC_PROVIDERS_JSON`, restart `kratos`).
- [ ] Add a note to `ory/README.md` that manually testing social login in a real (non-Playwright) browser requires the host machine to resolve the `dex` hostname (e.g. an `/etc/hosts` entry `127.0.0.1 dex`, or Chrome's `--host-resolver-rules="MAP dex:5556 127.0.0.1:5556"` flag), because dex's `issuer` must be identical for both Kratos's server-side calls (resolved via Docker's internal DNS) and the browser's redirect to dex's authorization endpoint (resolved via the host).
- [ ] Extend `web/playwright.config.ts` (from Step 10) with Chromium `launchOptions.args: ["--host-resolver-rules=MAP dex:5556 127.0.0.1:5556"]` so the Playwright-driven browser can resolve the `dex` hostname used in Kratos's redirects without requiring any host-machine `/etc/hosts` edit.
- [ ] Add the dex fixture credentials (`email`, `password`) to `web/e2e/support/fixtures.ts` (from Step 10), sourced from the same `DEX_STATIC_TEST_EMAIL`/`DEX_STATIC_TEST_PASSWORD` values baked into `ory/dex/config.yaml`.
- [ ] Create `web/e2e/oauth-dex.spec.ts` with two cases: (1) a brand-new browser session clicks "Continue with Dex" from `/register` (or `/login`, whichever Step 30 wired to accept first-time OIDC sign-ins), completes dex's static-password login form, and lands authenticated in the app (e.g. on `/rooms`); (2) opening a fresh session and repeating the same dex login a second time results in the same account (no duplicate-registration prompt), verified either through the UI (same username shown) or via a follow-up `curl` to the Kratos admin API confirming only one identity exists for the fixture email.
- [ ] Add unit-test-equivalent coverage for `SocialLoginButtons.tsx` per the project's component-testing conventions (co-located test rendering the component with a mock flow object containing oidc nodes, asserting one button renders per provider and the form posts to `flow.ui.action`).

## Out of scope
- Ory Hydra and any first-party OAuth2/OIDC provider role for this app — that is Step 55, which owns `ory/hydra/` config and new `app/(auth)/oauth/*` consent/login routes; disjoint files from this step.
- A dedicated web `/settings` page for end users to manage/unlink their linked social accounts — no such page exists yet in this codebase. Account-linking behavior in this step is verified directly against Kratos's self-service settings API via `curl` (see Verification), not through new UI; a settings UI is a future step's concern.
- Actually registering real Google/GitHub OAuth applications — only documented as an optional manual path; no automated test exercises Google/GitHub.
- Any change to `server/` (Go) or `llm-gateway/` (Rust) code — Kratos handles OIDC entirely; the Go API's session verification (added in Step 20) is unaffected because a Kratos session cookie looks the same regardless of which method authenticated it.
- Any `schema.sql`/Atlas migration — Kratos owns its own identity/credential storage in `kratos-db`, unchanged from Step 11.
- Redis-backed rate limiting on the OIDC callback endpoint (Phase 10 territory, already delivered in an earlier wave).
- SSO/SAML (excluded through Phase 26 per `phases.md`).

## Implementation notes

### Reference files to read before editing
- `docker-compose.yml` — existing `kratos`/`kratos-db`/`kratos-migrate`/`mailslurper` blocks (Step 11) are the template for `depends_on`/healthcheck style; find and reuse whatever `-e2e` Kratos block Step 30 added to the `test` profile.
- `ory/kratos/kratos.yml`, `ory/kratos/identity.schema.json`, `ory/README.md` — all from Step 11; `identity.schema.json`'s `traits.email`/`traits.username` fields are the mapper output contract.
- `.env.example` — existing `# === Name ===` section header convention (already used by `# === Ory Kratos ===` from Step 11).
- `web/src/features/auth/components/LoginForm.tsx`, `web/src/features/auth/components/RegisterForm.tsx` — current file (pre-Step-30) posts directly to `apiClient.login`/`apiClient.register`; Step 30 rewrites these to fetch and submit against a Kratos self-service browser flow. Locate wherever Step 30 stores the fetched flow object (a hook, a loader, or local state) and hang `SocialLoginButtons` off of it. The Kratos flow response contract itself is stable regardless of how Step 30 wrapped it: `flow.ui.action` (string URL), `flow.ui.method` (`"GET"`/`"POST"`), `flow.ui.nodes[]` where each node has `.group` (`"default"`, `"password"`, `"oidc"`, ...) and `.attributes.name`/`.attributes.value`/`.attributes.type`.
- `web/playwright.config.ts`, `web/e2e/support/fixtures.ts`, `web/e2e/smoke.spec.ts` — from Step 10; this step's spec follows the same structure and imports the same fixtures module.
- `.claude/rules/chakra-ui.md` — `SocialLoginButtons.tsx` must import plain components (`Button`, `Flex`, `Separator`, `Text`) from `@chakra-ui/react` directly (no snippet needed) and icons from `lucide-react`, matching `LoginForm.tsx`'s existing import style.

### Kratos OIDC provider config (the secret-free `kratos.yml` diff)
```yaml
selfservice:
  methods:
    password:
      enabled: true
    oidc:
      enabled: true
      # config.providers is intentionally omitted here — supplied at runtime via the
      # SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS environment variable (see docker-compose.yml
      # and .env.example) so client secrets never land in version control.
```
Kratos supports overriding any config key with an environment variable named by uppercasing the dotted path and replacing `.` with `_`; for array-typed keys (like `selfservice.methods.oidc.config.providers`), the environment variable's value must be the entire array encoded as a JSON string. That variable is `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS`, sourced from `.env`'s `KRATOS_OIDC_PROVIDERS_JSON`.

### `.env.example` addition (illustrative; keep it one line in the real file)
```
# === Ory Kratos: Social Login (OIDC) ===
# dex is a mock OIDC provider used for local/E2E verification (see ory/dex/config.yaml).
DEX_STATIC_TEST_EMAIL=oidc-fixture@example.com
DEX_STATIC_TEST_PASSWORD=DexFixtureP@ssw0rd!
# NOTE: the "dex" entry's client_secret below must match ory/dex/config.yaml's
# staticClients[0].secret exactly. The google/github entries are optional placeholders —
# replace with real OAuth app credentials to enable those providers (see ory/README.md).
KRATOS_OIDC_PROVIDERS_JSON=[{"id":"dex","provider":"generic","client_id":"kratos-dex-client","client_secret":"kratos-dex-client-secret","issuer_url":"http://dex:5556/dex","mapper_url":"file:///etc/config/kratos/oidc/dex.jsonnet","scope":["openid","profile","email"]},{"id":"google","provider":"google","client_id":"your-google-oauth-client-id","client_secret":"your-google-oauth-client-secret","mapper_url":"file:///etc/config/kratos/oidc/google.jsonnet","scope":["openid","profile","email"]},{"id":"github","provider":"github","client_id":"your-github-oauth-client-id","client_secret":"your-github-oauth-client-secret","mapper_url":"file:///etc/config/kratos/oidc/github.jsonnet","scope":["read:user","user:email"]}]
```

### `ory/dex/config.yaml` (illustrative shape)
```yaml
issuer: http://dex:5556/dex
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
staticClients:
  - id: kratos-dex-client
    secret: kratos-dex-client-secret # must match KRATOS_OIDC_PROVIDERS_JSON's "dex" entry
    name: Polyphony (via Kratos)
    redirectURIs:
      - http://localhost:4433/self-service/methods/oidc/callback/dex   # dev kratos
      - http://kratos:4433/self-service/methods/oidc/callback/dex      # dev kratos (container-to-container)
      # (Step 30 reuses the shared kratos service for E2E, so no separate kratos-e2e callback URL is needed;
      #  add one here only if an isolated E2E Kratos instance is introduced later)
enablePasswordDB: true
staticPasswords:
  - email: oidc-fixture@example.com
    # generate with: docker run --rm dexidp/dex:<pinned-tag> dex hash-password 'DexFixtureP@ssw0rd!'
    hash: "$2a$10$REPLACE_WITH_GENERATED_BCRYPT_HASH"
    username: oidc-fixture
    userID: oidc-fixture-static-user
```
Generate the bcrypt hash locally and paste the result in; do not hand-write a bcrypt hash.

### Why the callback URL and issuer need care
Kratos redirects the *browser* to the provider's authorization endpoint, which is derived from `issuer_url`'s OIDC discovery document. If dex's issuer were an internal-only Docker hostname unreachable from the host browser, the redirect would fail to resolve outside the compose network. Because `issuer: http://dex:5556/dex` must be identical for both server-side calls (Kratos containers resolve `dex` fine via Docker's built-in DNS) and the host browser (which cannot resolve `dex` by default), the browser side is fixed with Chromium's `--host-resolver-rules` flag in Playwright's config (fully automated, no host `/etc/hosts` edit needed for the E2E suite) and documented as an optional manual `/etc/hosts` entry for anyone testing social login by hand in a real browser.

### Conflict notes
Per the plan's same-wave boundary: this step owns the login/register page social-login UI and `ory/kratos/kratos.yml`'s `selfservice.methods.oidc` section; Step 55 (same wave) owns `ory/hydra/` configuration and new consent/login route handlers under `app/(auth)/oauth/*` — entirely disjoint files. `docker-compose.yml` edits here are limited to one new, alphabetically-placed `dex` block plus one additive environment-variable line on the pre-existing shared `kratos` block (added by Step 11; Step 30 reuses it for E2E rather than isolating a `kratos-e2e`); do not reorder or reformat any other service block. Do not touch `Taskfile.yml` — no new task is required by this step.

## Verification
1. `docker compose up -d dex` then `docker compose ps dex` — status is `healthy`.
2. `curl -sf http://localhost:5556/dex/.well-known/openid-configuration | jq '.issuer'` — returns `"http://dex:5556/dex"`.
3. `docker compose up -d kratos` (recreates it with the new env var) then `curl -s http://localhost:4433/self-service/registration/browser -H 'Accept: application/json' | jq '.ui.nodes[] | select(.group=="oidc") | .attributes.value'` — lists `"dex"`, `"google"`, `"github"`.
4. `cd web && bun install && bunx playwright install --with-deps chromium` (if not already installed by Step 10) — exits 0.
5. `task test:e2e:up` then `cd web && bunx playwright test oauth-dex.spec.ts` — both cases in `oauth-dex.spec.ts` pass: a new user can register via dex and land authenticated in the app, and repeating the dex login does not create a second account.
6. `curl -s http://localhost:4434/admin/identities?credentials_identifier=oidc-fixture@example.com | jq 'length'` (adjust query param to whatever the Kratos admin API version in use supports; alternatively `curl -s http://localhost:4434/admin/identities | jq '[.[] | select(.traits.email=="oidc-fixture@example.com")] | length'`) — returns `1` after both Playwright cases have run, confirming no duplicate identity was created on the second dex login.
7. `curl -s http://localhost:4434/admin/identities | jq '.[] | select(.traits.email=="oidc-fixture@example.com") | .credentials | keys'` — includes `"oidc"`, confirming the dex-created identity has an OIDC credential (proves the Jsonnet mapper ran and populated traits correctly, since the identity would not exist at all otherwise).
8. Account-linking check: register a password-based identity via `curl -s http://localhost:4433/self-service/registration/api` with `traits.email` equal to `oidc-fixture@example.com` and any password — expect this to succeed only if no such traits-unique-identifier conflict exists; if it fails with a `4xx` "identifier already exists" error instead (because the dex-created identity from step 6 already claimed that email as its unique password-method identifier), that failure itself is the expected, documented safe-by-default behavior to record — Kratos does not silently merge a new password identity into an OIDC-created one with a matching email.
9. `bun run lint` (or `task lint:web`) in `web/` — passes with no new errors.
10. `task test:e2e:down` then `docker compose down` — both stacks tear down cleanly with no orphaned containers.
11. `task test:server && task test:gateway` — unaffected, still pass (regression check that no Go/Rust source was touched).

## Completion criteria
- [ ] `ory/dex/config.yaml` exists with a working static client and a static test-user password login.
- [ ] `docker-compose.yml` has a new, alphabetically-placed `dex` service (no `profiles:` restriction) and the pre-existing `kratos` (and E2E-equivalent) service blocks read `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS` from `.env`.
- [ ] `ory/kratos/kratos.yml` enables `selfservice.methods.oidc` with no secrets committed to the file.
- [ ] `ory/kratos/oidc/{dex,google,github}.jsonnet` exist and produce traits matching `identity.schema.json`.
- [ ] `.env.example` documents `DEX_STATIC_TEST_EMAIL`, `DEX_STATIC_TEST_PASSWORD`, and `KRATOS_OIDC_PROVIDERS_JSON` (dex fully functional, google/github as clearly-labeled placeholders).
- [ ] `web/src/features/auth/components/SocialLoginButtons.tsx` exists, is wired into both `LoginForm.tsx` and `RegisterForm.tsx`, and has co-located tests.
- [ ] `web/e2e/oauth-dex.spec.ts` exists and passes against the Step 10 harness, covering both first-time dex sign-in and repeat-login-same-account.
- [ ] `ory/README.md` documents the dex testing setup and the optional real Google/GitHub credential path.
- [ ] Account-linking / no-silent-merge behavior is verified against the Kratos API as described in Verification.
- [ ] All verification checks above pass.
