# Ory Kratos (local development)

This directory holds the configuration for the Ory Kratos identity service used by
Polyphony's Docker Compose stack. Kratos itself is not yet wired into the Go server or
web frontend — `AuthService` keeps using SimpleJWT until Step 20 flips it behind an env
flag (see `phases.md`, Phase 9). This step only stands up a working, independently
verifiable Kratos deployment.

## Services

| Service          | Purpose                                                   | Ports (host)     |
|------------------|------------------------------------------------------------|------------------|
| `kratos`         | Kratos public + admin API server (`kratos serve --dev`)     | `4433` (public), `4434` (admin) |
| `kratos-db`      | Dedicated Postgres instance backing Kratos (independent of the app's `db`) | none (internal only) |
| `kratos-migrate` | One-shot job that applies Kratos's own SQL migrations (`kratos migrate sql`) against `kratos-db` | none |
| `mailslurper`    | Fake SMTP server + web UI/REST API that captures outgoing courier emails (verification, recovery) in dev | `4436` (web UI), `4437` (REST API) |
| `dex`            | Mock OIDC provider (dexidp/dex) used for local/E2E social-login testing (Step 44) | `5556` |

Bring the whole group up with:

```bash
docker compose up -d kratos-db kratos-migrate kratos mailslurper dex
```

(`kratos` depends on `kratos-migrate` completing successfully, and `kratos-migrate`
depends on `kratos-db` being healthy, so `docker compose up -d kratos mailslurper` alone
is also enough — Compose resolves the rest of the dependency chain.)

## Configuration files

- `ory/kratos/kratos.yml` — Kratos configuration: self-service flows (registration,
  login, settings, recovery, verification, logout), the password method, the identity
  schema reference, and the Mailslurper-backed courier SMTP connection.
- `ory/kratos/identity.schema.json` — the default identity schema. Mirrors the `users`
  table in `server/schema.sql`: `traits.email` (used as the password identifier,
  verification/recovery via email) and `traits.username` (3-100 characters), both
  required.
- `ory/kratos/oidc/{dex,google,github}.jsonnet` — Step 44's trait mappers, one per OIDC
  provider. Each reads the provider's ID token claims and emits `traits.email`/
  `traits.username`, matching `identity.schema.json`'s contract above.
- `ory/dex/config.yaml` — Step 44's mock OIDC provider configuration (see "Social login
  (OIDC)" below).

`selfservice.flows.*.ui_url` values point at placeholder web routes
(`http://localhost:3000/login`, `/registration`, `/settings`, `/recovery`,
`/verification`) that don't exist yet — those pages land in Step 20/44. This only
affects the browser-based self-service flow (which redirects there); the API flow used
below returns JSON directly and never redirects.

## Secrets and the database connection string

`kratos.yml` intentionally omits the `dsn` and `secrets` keys. Kratos maps environment
variables to configuration keys (uppercase, dot -> underscore), and the `kratos` /
`kratos-migrate` service blocks in `docker-compose.yml` set `DSN`, `SECRETS_COOKIE`, and
`SECRETS_CIPHER` directly from the `KRATOS_DSN`, `KRATOS_COOKIE_SECRET`, and
`KRATOS_CIPHER_SECRET` variables in `.env`. **`KRATOS_CIPHER_SECRET` must be exactly 32
characters** — Kratos uses it as an AES-256-GCM key and rejects any other length at
startup.

## Inspecting identities

List all identities and their traits via the admin API:

```bash
curl -s http://localhost:4434/admin/identities | jq '.[].traits'
```

Fetch a single identity by ID:

```bash
curl -s http://localhost:4434/admin/identities/<identity-id> | jq
```

## Inspecting captured emails (Mailslurper)

Open the web UI in a browser at [http://localhost:4436](http://localhost:4436), or query
the REST API directly:

```bash
curl -s http://localhost:4437/mail | jq '.mailItems[] | {toAddresses, subject, body}'
```

## Verifying the stack end-to-end (API self-service flow)

```bash
# 1. Initiate an API registration flow.
FLOW=$(curl -s -H "Accept: application/json" \
  http://localhost:4433/self-service/registration/api | jq -r '.id')

# 2. Submit registration traits + password.
curl -s -X POST "http://localhost:4433/self-service/registration?flow=${FLOW}" \
  -H "Content-Type: application/json" -H "Accept: application/json" \
  -d '{"method":"password","password":"<a-strong-unique-password>","traits":{"email":"kratos-test@example.com","username":"kratos_test"}}' \
  | jq '.identity.traits'

# 3. Confirm the identity exists via the admin API.
curl -s http://localhost:4434/admin/identities | jq '.[].traits'

# 4. Confirm the verification email was captured by Mailslurper.
curl -s http://localhost:4437/mail | jq '.mailItems[0].toAddresses'
```

Note: Kratos's default password policy rejects passwords found in known data breaches
(an online HaveIBeenPwned lookup) — use a unique, non-dictionary password when testing
registration, not a well-known example string.

## Health checks

```bash
curl -sf http://localhost:4433/health/ready         # public API
curl -sf http://localhost:4434/admin/health/ready   # admin API (bare /health/ready 307-redirects here, and -f doesn't follow)
curl -sf http://localhost:5556/dex/healthz          # dex (Step 44)
```

## Social login (OIDC)

Step 44 turns on `selfservice.methods.oidc` and configures three providers — `dex`,
`google`, `github` — via a single `KRATOS_OIDC_PROVIDERS_JSON` environment variable (see
`.env.example`), never as a `config.providers` list checked into `ory/kratos/kratos.yml`.
Each provider has a Jsonnet trait mapper under `ory/kratos/oidc/` that maps its ID
token claims onto this app's `traits.email`/`traits.username` identity schema.

### dex: the always-on, fully automated path

[dex](https://dexidp.io/) is a small OIDC provider with an in-memory, single-user
"local passwords" backend (`ory/dex/config.yaml`) — it stands in for a real IdP so
"Continue with Dex" can be exercised end to end without ever leaving Docker Compose.
Its one static test user's credentials live in `.env`'s `DEX_STATIC_TEST_EMAIL`/
`DEX_STATIC_TEST_PASSWORD` (also consumed by `web/e2e/support/fixtures.ts` and
`web/e2e/oauth-dex.spec.ts`).

`docker compose up -d dex kratos` brings up everything needed; from `/login` or
`/register`, click "Continue with Dex" and sign in with the fixture credentials above.
A first-time sign-in creates a new identity (with an `oidc` credential, no `password`
credential with a real hash) and signs the browser straight in; repeating the same dex
login later re-authenticates the same identity rather than creating a duplicate (Kratos
matches on the `(provider, subject)` pair from the ID token, not on- email or username).

Testing this by hand in a real (non-Playwright) browser requires the host machine to
resolve the `dex` hostname, because dex's `issuer` (`http://dex:5556/dex`) must be
identical for both Kratos's server-side calls (Docker's internal DNS resolves `dex`
for every container automatically) and the browser's redirect to dex's authorization
endpoint (which the host does **not** resolve by default). Either:

- add a `127.0.0.1 dex` entry to `/etc/hosts`, or
- launch Chrome/Chromium with `--host-resolver-rules="MAP dex:5556 127.0.0.1:5556"`.

(The Playwright E2E suite needs neither — `web/playwright.config.ts` passes the
equivalent `--host-resolver-rules` flag automatically via `launchOptions.args`.)

### Optional: real Google/GitHub credentials

Google and GitHub are configured in `KRATOS_OIDC_PROVIDERS_JSON` alongside `dex`, but
ship with clearly-labeled placeholder client ids/secrets — no automated test exercises
either, since a real OAuth app requires external, human-only registration. To enable
one for manual testing:

1. Create an OAuth app in the provider's developer console (Google Cloud Console →
   "APIs & Services" → "Credentials"; GitHub → Settings → "Developer settings" →
   "OAuth Apps").
2. Set its authorization callback URL to
   `http://localhost:4433/self-service/methods/oidc/callback/google` (or `/github`).
3. Copy the app's client id/secret into your local `.env`'s `KRATOS_OIDC_PROVIDERS_JSON`,
   replacing that provider's placeholder `client_id`/`client_secret` values (keep the
   `dex` entry's values in sync with `ory/dex/config.yaml` unchanged).
4. `docker compose up -d kratos` to restart Kratos with the new config.

### Account linking and no-silent-merge

Kratos never silently merges a new password-based registration into an existing
OIDC-created identity, even when the submitted `traits.email` matches exactly: the
identity schema marks `traits.email` as the password method's unique identifier, so
Kratos reserves it (as an unusable placeholder `password` credential with no hash) the
moment *any* identity — OIDC-created or not — claims that email. A subsequent password
registration attempt with the same email fails with a `4000007`
"An account with the same identifier ... exists already" error instead of merging.
Linking is a deliberate, explicit action (Kratos's self-service **settings** flow, e.g.
`PUT /self-service/settings/api` with an oidc `link` action) — this app has no
dedicated UI for it yet (a future `/settings` page's concern); verify it directly
against Kratos's self-service/admin APIs in the meantime.

## Out of scope (this step)

- Go server / web frontend integration (Step 20, Step 44).
- Ory Hydra and OAuth2/OIDC provider setup (Step 55).
- A web `/settings` page for managing/unlinking linked social accounts (see "Account
  linking and no-silent-merge" above).
- Actually registering real Google/GitHub OAuth applications (documented above as an
  optional manual path only).
