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
- [x] Add `ory/dex/config.yaml`: dex static configuration — `issuer: http://dex:5556/dex`, in-memory storage, a single `staticClients` entry (id `polyphony-web`, matching the pre-existing `.env.example` `DEX_CLIENT_ID` placeholder from Step 6, per the planner briefing's allowance to change the illustrative `kratos-dex-client` id) whose `redirectURIs` list includes Kratos's OIDC callback URL for the `dex` provider on both the dev Kratos instance and the E2E Kratos instance (see Implementation notes), `oauth2.skipApprovalScreen: true` (no consent screen, needed for unattended automation), and one `staticPasswords` entry (a fixture test user with a real, generated bcrypt hash — `htpasswd -bnBC 10`, verified against a live dex container). Verified end to end (real authorization-code login) against a standalone dex container while implementing this step.
- [x] Add a `dex` service to `docker-compose.yml`: pinned `dexidp/dex:v2.43.1` image tag, mounts `./ory/dex/config.yaml` read-only, `command: dex serve /etc/dex/cfg/config.yaml`, host port `5556:5556`, a healthcheck against dex's `/dex/healthz` endpoint. No `profiles:` key.
- [x] Insert the new `dex` block into `docker-compose.yml` at its alphabetical position among the existing top-level services (before `kratos`), touching no existing service block.
- [x] Extend the existing `kratos` service block in `docker-compose.yml` with `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS: ${KRATOS_OIDC_PROVIDERS_JSON}` and `depends_on: dex` (`condition: service_healthy`). The merged repo's compose file *does* have an isolated `kratos-e2e` (added by a wave-4 review fix, not by Step 30 as this doc assumed) — the same env var (plus `SERVE_PUBLIC_BASE_URL`/`SELFSERVICE_DEFAULT_BROWSER_RETURN_URL`/`SELFSERVICE_ALLOWED_RETURN_URLS` overrides and a dedicated `8093`/`8094` host port, all verified end to end against a standalone Kratos+dex pair — see this step's PR description/commit for the reasoning) was applied there too.
- [x] Edit `ory/kratos/kratos.yml` to add `selfservice.methods.oidc.enabled: true` (no `config.providers` committed). Also added `selfservice.flows.registration.after.oidc.hooks: [{hook: session}]` — a gap discovered while verifying end to end: without it, a first-time OIDC sign-up creates the identity but never establishes a session, matching only the `password` method's existing hook.
- [x] Create three Jsonnet trait mappers under `ory/kratos/oidc/`: `dex.jsonnet`, `google.jsonnet`, `github.jsonnet`. `dex.jsonnet` verified end to end (real identity created with correct `traits.email`/`traits.username` via a live Kratos+dex run); `google.jsonnet`/`github.jsonnet` share the same structure/logic (not independently exercised — no automated test drives them).
- [x] Add a `# === Ory Kratos: Social Login (OIDC) ===`-equivalent section to `.env.example`: extended the pre-existing `# === Dex mock OIDC provider ===` section (from Step 6) in place with `DEX_STATIC_TEST_EMAIL`, `DEX_STATIC_TEST_PASSWORD`, and `KRATOS_OIDC_PROVIDERS_JSON`, reusing the existing `DEX_CLIENT_ID`/`DEX_CLIENT_SECRET`/`DEX_ISSUER_URL` values for the `dex` entry.
- [x] Create `web/src/features/auth/components/SocialLoginButtons.tsx`: takes this repo's actual `UiContainer` flow shape (`flow.nodes`/`flow.action`/`flow.method`, not a nested `flow.ui.*`) as a prop, filters for `group === "oidc"`, renders a real HTML `<form>` (via `toRelativeKratosAction(flow.action)`, matching every other flow submission in this app) with a hidden `csrf_token` input and one submit `Button` per oidc node.
- [x] Wire `SocialLoginButtons` into `LoginForm.tsx`/`RegisterForm.tsx`, rendered below the password form, conditionally (returns `null` internally when there are no oidc nodes).
- [x] Update `ory/README.md` with a "Social login (OIDC)" section covering dex's static test user and the optional real Google/GitHub credential path.
- [x] Add the host-resolution note to `ory/README.md`.
- [x] Extend `web/playwright.config.ts` with Chromium `launchOptions.args: ["--host-resolver-rules=MAP dex:5556 127.0.0.1:5556"]` — verified standalone (chromium launches with this flag and successfully resolves `dex` to a locally-running dex container's `/dex/healthz`).
- [x] Add the dex fixture credentials to `web/e2e/support/fixtures.ts` as `DEX_FIXTURE_USER`, sourced from `DEX_STATIC_TEST_EMAIL`/`DEX_STATIC_TEST_PASSWORD` env vars with the same literal defaults baked into `ory/dex/config.yaml`.
- [x] Create `web/e2e/oauth-dex.spec.ts` with two cases (registration via `/register`, repeat login via `/login`), the second verified via the Kratos admin API (`kratos-e2e`'s dedicated `8094` host port) rather than the UI. Not run against the live compose stack (see skippedComposeChecks) — the underlying Kratos+dex OIDC flow was, however, fully verified end to end standalone.
- [x] Add co-located `SocialLoginButtons.test.tsx` covering: one button per oidc node, form action/method, `name`/`value`/`type` on each button, the csrf hidden field, and the no-oidc-nodes empty-render case.

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

**Note (isolated-worktree agent):** items 1, 3, 5, 6, 8, 10 need the actual `docker compose`/`task test:e2e:up` stack on fixed host ports, which this isolated-worktree implementation pass explicitly does not run (see the workflow's `skippedComposeChecks`). Instead, the entire OIDC flow — dex config, Kratos provider config, the Jsonnet mapper, the registration-after-oidc session hook, the account-linking/no-silent-merge behavior, and the `kratos-e2e` return-URL/callback-port overrides — was verified end to end against **standalone** dex + Kratos + kratos-db containers (same pinned images, same config files, arbitrary non-conflicting host ports) built and torn down for this purpose. Items 2, 4, 7, 9, 11 either don't need the compose stack at all or have a direct standalone/local equivalent below.

1. [x] Verified in the wave-5 integration review: `docker compose up -d` brings up `dex` with status `healthy`, and `curl http://localhost:5556/dex/.well-known/openid-configuration` returns issuer `http://dex:5556/dex`.
2. [x] `curl -sf http://localhost:5556/dex/.well-known/openid-configuration | jq '.issuer'` — returns `"http://dex:5556/dex"`. Run against a standalone dex container on host port 5556; confirmed.
3. [x] Verified in the wave-5 integration review: the compose `kratos` service's registration browser flow lists `oidc` submit nodes for `dex`, `github`, and `google`, and the web login/register pages render "Continue with Dex/GitHub/Google" buttons.
4. [x] `cd web && bun install && bunx playwright install --with-deps chromium` — ran; exits 0.
5. [x] Verified in the wave-5 integration review: `bunx playwright test oauth-dex.spec.ts` against the `task test:e2e:up` stack — both serial cases pass. Caveat: the e2e project's `dex` publishes host port 5556, which collides with a running dev stack's `dex` (the dev `dex` had to be stopped first) — see the wave-5 review findings.
6. [x] Verified in the wave-5 integration review: after both oauth-dex.spec.ts logins, `kratos-e2e`'s admin API (`:8094/admin/identities`) reports exactly 1 identity for the dex fixture email, and `?include_credential=oidc` shows an `oidc` credential on it.
7. [x] Standalone equivalent run: `admin/identities/<id>?include_credential=oidc` on the standalone Kratos instance's created identity reported `credentials` keys including `oidc` (and, before the registration-after-oidc hook fix below, also confirmed the identity had no `Set-Cookie` session at all without it).
8. [x] Verified in the wave-5 integration review against the live `kratos-e2e`: `POST .../self-service/registration?flow=...` with `method: password` and the dex-created identity's email returned error `4000007` ("account with the same identifier ... exists already") — no silent merge.
9. [x] `bun run lint` in `web/` — 0 errors, 4 warnings (all pre-existing, unrelated to this step's files).
10. [x] Verified in the wave-5 integration review: `task test:e2e:down` removed all e2e containers/volumes/network and `docker compose down` left no orphaned containers.
11. [x] `task test:server && task test:gateway` — ran (`go test ./...` in `server/`, `cargo test` in `llm-gateway/`); both pass, confirming no Go/Rust regression. Also ran `go vet ./...` and `cargo clippy --all-targets -- -D warnings`, both clean.

## Completion criteria
- [x] `ory/dex/config.yaml` exists with a working static client and a static test-user password login (verified with a real, generated bcrypt hash against a live container).
- [x] `docker-compose.yml` has a new, alphabetically-placed `dex` service (no `profiles:` restriction) and the pre-existing `kratos` and `kratos-e2e` service blocks read `SELFSERVICE_METHODS_OIDC_CONFIG_PROVIDERS` from `.env`.
- [x] `ory/kratos/kratos.yml` enables `selfservice.methods.oidc` with no secrets committed to the file.
- [x] `ory/kratos/oidc/{dex,google,github}.jsonnet` exist and produce traits matching `identity.schema.json` (`dex.jsonnet` verified live; `google`/`github` share the same logic, not independently exercised).
- [x] `.env.example` documents `DEX_STATIC_TEST_EMAIL`, `DEX_STATIC_TEST_PASSWORD`, and `KRATOS_OIDC_PROVIDERS_JSON` (dex fully functional, google/github as clearly-labeled placeholders).
- [x] `web/src/features/auth/components/SocialLoginButtons.tsx` exists, is wired into both `LoginForm.tsx` and `RegisterForm.tsx`, and has co-located tests.
- [x] `web/e2e/oauth-dex.spec.ts` exists, is syntactically valid (`playwright test --list` enumerates both cases), and the OIDC flow it drives was verified end to end standalone — not run against the live E2E harness in this pass (see Verification note and `skippedComposeChecks`).
- [x] `ory/README.md` documents the dex testing setup and the optional real Google/GitHub credential path.
- [x] Account-linking / no-silent-merge behavior is verified — standalone against a live Kratos instance (see item 8 above), not against the compose stack.
- [x] All verification checks above pass — the compose-dependent checks (1, 3, 5, 6, 8, 10) were completed in the wave-5 integration review. Known caveat: the e2e project's `dex` service publishes host port 5556 and therefore collides with a concurrently-running dev stack's `dex` (see the wave-5 review findings; the dev `dex` must currently be stopped before `task test:e2e:up`).
