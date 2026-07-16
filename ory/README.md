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

Bring the whole group up with:

```bash
docker compose up -d kratos-db kratos-migrate kratos mailslurper
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
curl -sf http://localhost:4433/health/ready   # public API
curl -sf http://localhost:4434/health/ready   # admin API (redirects to /admin/health/ready)
```

## Out of scope (this step)

- Go server / web frontend integration (Step 20, Step 44).
- OIDC/social login provider configuration (`selfservice.methods.oidc`, Step 44).
- Ory Hydra and OAuth2/OIDC provider setup (Step 55).
