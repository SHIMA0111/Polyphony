# Step 11: Infra: Ory Kratos services in docker-compose

## Meta
- **Type**: infra
- **Components**: infra
- **Wave**: 2 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 6: Infra tooling baseline: compose healthchecks, Taskfile, env staging, migration convention
- **Unlocks**: Step 20: Server: Kratos session auth swap + identity migration | Step 44: OAuth social login via Kratos OIDC (dex mock + Google/GitHub) | Step 55: Ory Hydra OAuth2/OIDC provider + first-party client demo
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Phase 9 of `phases.md` requires migrating auth from SimpleJWT to Ory Kratos, but that swap (server middleware + web login/registration flow, tracked separately) needs a live, locally-runnable Kratos deployment to develop and test against. This step delivers only the infrastructure: Kratos itself, its own migration job, its own Postgres-backed store, an identity schema matching the app's `users` traits (email + username), self-service flow configuration, and Mailslurper to catch verification/recovery emails in dev. No Go/web code is touched here — `AuthService` keeps using SimpleJWT until Step 20 flips it behind an env flag, per the plan's auth data-plane sequencing.

## Goal
After this PR, `docker compose up` brings up a working Ory Kratos instance (public API on 4433, admin API on 4434) backed by its own dedicated Postgres database, with migrations applied by a one-shot job, a checked-in identity schema (email + username traits, password credential, email verification/recovery), self-service flow configuration pointing at placeholder web UI URLs, and Mailslurper capturing outgoing courier emails. Everything is verifiable with `curl` against the self-service registration API and the admin identities API, without any dependency on server/ or web/ code changes.

## Scope
- [x] Create `ory/kratos/kratos.yml` — Kratos configuration: `dsn` from `KRATOS_DSN` env var, `serve.public.base_url` / `serve.admin.base_url`, `secrets.cookie` / `secrets.cipher` from env, `identity.default_schema_id: default` + `identity.schemas` pointing at the mounted identity schema file, `selfservice.default_browser_return_url` and `allowed_return_urls` pointing at placeholder `http://localhost:3000/*` routes, `selfservice.methods.password.enabled: true`, `selfservice.flows.registration|login|settings|recovery|verification|logout` each with a placeholder `ui_url` under `http://localhost:3000/...` (the real web pages land in Step 20/44), and `courier.smtp.connection_uri` pointing at the `mailslurper` service.
- [x] Create `ory/kratos/identity.schema.json` — JSON Schema for the default identity: `traits.email` (format `email`, used as the password identifier, `verification.via: email`, `recovery.via: email`) and `traits.username` (string, 3-100 chars), both `required`, mirroring the `email`/`username` columns already on the `users` table in `server/schema.sql`.
- [x] Add `kratos-db` service to `docker-compose.yml` — `postgres:17-alpine`, dedicated named volume, its own `POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB` (`KRATOS_DB_*` env vars), `pg_isready` healthcheck matching the existing `db` service's pattern.
- [x] Add `kratos-migrate` service to `docker-compose.yml` — one-shot job using the same Kratos image tag as the `kratos` service, running `kratos migrate sql -e --yes`, `depends_on: kratos-db` with `condition: service_healthy`, mounts `./ory/kratos:/etc/config/kratos:ro`.
- [x] Add `kratos` service to `docker-compose.yml` — runs `kratos serve -c /etc/config/kratos/kratos.yml --dev`, mounts `./ory/kratos:/etc/config/kratos:ro`, exposes `4433:4433` (public) and `4434:4434` (admin), `depends_on: kratos-migrate` with `condition: service_completed_successfully`, healthcheck against `GET /health/ready` on the admin port. (Also added `--watch-courier`, beyond the literal doc text — see report/concerns: without it the courier never dispatches queued emails and verification step 7 cannot pass.)
- [x] Add `mailslurper` service to `docker-compose.yml` — `oryd/mailslurper:latest-smtps`, exposes `4436:4436` (web UI) and `4437:4437` (REST API used by the UI and by verification below); no host port needed for the internal SMTP listener (1025) since only `kratos` talks to it over the compose network.
- [x] Insert the four new blocks into `docker-compose.yml` in alphabetical position per the Step 6 convention, so the final service order is: `api`, `db`, `kratos`, `kratos-db`, `kratos-migrate`, `llm-gateway`, `mailslurper`, `migrate`, `web`.
- [x] Add a `# === Ory Kratos ===` section to `.env.example` (and mirror into local `.env`) with `KRATOS_DB_USER`, `KRATOS_DB_PASSWORD`, `KRATOS_DB_NAME`, `KRATOS_DSN`, `KRATOS_PUBLIC_URL`, `KRATOS_ADMIN_URL`, `KRATOS_COOKIE_SECRET`, `KRATOS_CIPHER_SECRET` (cipher secret must be exactly 32 characters — document this constraint inline as a comment). (`.env.example` updated; no root `.env` existed in this worktree to mirror into — it is gitignored and was not present.)
- [x] Document the new services (ports, purpose, how to reach the Mailslurper UI, how to inspect identities via the admin API) in a short `ory/README.md`.
- [x] Verify no existing service definition (`db`, `migrate`, `api`, `llm-gateway`, `web`) is modified — this PR only adds new blocks and new files.

## Out of scope
- Any change to Go server code (`server/internal/domain/auth`, `server/internal/interface/middleware/auth.go`) — the `AuthService` interface keeps its SimpleJWT implementation; the Kratos-backed implementation and the `kratos_identity_id` column on `users` land in Step 20.
- Any change to web login/registration pages or API client — Step 20 (initial swap) and Step 44 (social login pages) own that.
- OIDC/social login provider configuration in `kratos.yml` (`selfservice.methods.oidc`) — added in Step 44.
- Ory Hydra and any OAuth2/OIDC *provider* setup — added in Step 55.
- Rate limiting, Redis-backed session caching — Phase 10 / later steps.
- Any `schema.sql` change — this step's `dbChanges` is empty; Kratos owns its schema entirely inside `kratos-db` via its own migration tool, independent of Atlas.

## Implementation notes
- Reference files to read before editing: `docker-compose.yml` (existing `db`/`migrate` service pair is the template for the `kratos-db`/`kratos-migrate` pair — same `depends_on: condition: service_healthy` / `service_completed_successfully` pattern), `.env.example` (existing section style: `# === Name ===` headers), `server/schema.sql` lines 4-13 (the `users` table — `email`, `username` are the two traits the identity schema must mirror), `Taskfile.yml` (`migrate:apply` task shows the `docker compose up -d db && docker compose run --rm migrate` pattern you are mirroring for Kratos, but no new Taskfile task is required by this step's scope — leave `Taskfile.yml` untouched).
- Pin the Kratos image to a specific tag (not `latest`) for reproducibility, e.g. `oryd/kratos:v1.3.1`, and use the exact same tag for both the `kratos` and `kratos-migrate` services so schema and server versions never drift.
- `kratos-db` and `db` must be fully independent Postgres instances/volumes (own service, own named volume, e.g. `kratos_pgdata`) — do not add a second database inside the existing `db` container, since that would require editing the `db` service's init behavior and violates the additive-only convention for this wave.
- `kratos.yml` secrets (`secrets.cookie`, `secrets.cipher`) and the `dsn` must be templated from environment variables using Kratos's `${VAR}` env-substitution support in the config loader, or injected via `kratos serve --config` plus environment-variable overrides (`KRATOS_DSN`, `SECRETS_COOKIE`, `SECRETS_CIPHER` map to `dsn`, `secrets.cookie`, `secrets.cipher` per Kratos's env-var-to-config-key convention: uppercase, dot→underscore). Prefer passing `DSN`, `SECRETS_COOKIE`, `SECRETS_CIPHER` as container environment variables in `docker-compose.yml` (sourced from the `KRATOS_*` `.env` vars) over hardcoding secrets in the checked-in `kratos.yml`.
- `selfservice.flows.*.ui_url` values should point at plausible future web routes (e.g. `http://localhost:3000/login`, `/registration`, `/settings`, `/recovery`, `/verification`) even though those pages don't exist yet — Kratos only redirects browsers there for the *browser* flow; this step's verification exclusively exercises the *API* flow (`/self-service/registration/api`), which returns JSON directly and never redirects, so the absence of those pages does not block verification.
- Conflict notes (per skeleton): this step only adds new, disjoint service blocks to `docker-compose.yml`/new files under `ory/`; it does not touch any table in `server/schema.sql` or any line in an existing service block, so it cannot conflict with any other same-wave step touching `docker-compose.yml` (steps 10, 12) as long as each inserts its own alphabetically-positioned block.
- Follow English-only comments in `kratos.yml`/`ory/README.md` per repository convention.

## Verification
1. `docker compose up -d kratos-db` then `docker compose ps kratos-db` — status is `healthy`.
2. `docker compose up kratos-migrate` (or `docker compose up -d kratos-migrate` then `docker compose logs kratos-migrate`) — exits 0 and logs report the Kratos SQL schema was applied with no errors; `docker compose ps -a kratos-migrate` shows `Exited (0)`.
3. `docker compose up -d kratos mailslurper` then `docker compose ps` — both `kratos` and `mailslurper` show as `running`/`healthy`.
4. `curl -sf http://localhost:4433/health/ready` and `curl -sfL http://localhost:4434/health/ready` — both return HTTP 200 with a JSON body indicating the service is ready (the admin port 307-redirects to `/admin/health/ready`, hence `-L`).
5. Initiate an API registration flow and register a test identity:
   ```bash
   FLOW=$(curl -s -H "Accept: application/json" http://localhost:4433/self-service/registration/api | jq -r '.id')
   curl -s -X POST "http://localhost:4433/self-service/registration?flow=${FLOW}" \
     -H "Content-Type: application/json" -H "Accept: application/json" \
     -d '{"method":"password","password":"Str0ngP@ssw0rd!","traits":{"email":"kratos-test@example.com","username":"kratos_test"}}'
   ```
   Expect HTTP 200 with a JSON body containing an `identity` object whose `traits.email` is `kratos-test@example.com` and `traits.username` is `kratos_test`.
6. `curl -s http://localhost:4434/admin/identities | jq '.[].traits'` — includes the identity created in step 5.
7. `curl -s http://localhost:4437/mail | jq '.mailItems[0].toAddress'` (Mailslurper API) — shows the verification email sent to `kratos-test@example.com` (confirms the courier→Mailslurper SMTP path works end-to-end).
8. `docker compose down` — all four new services stop cleanly with no errors; `docker compose down -v` additionally removes the new `kratos_pgdata`-style volume without affecting the existing `pgdata` volume.
9. `task test` (or `go test ./... ` in `server/`, `cargo test` in `llm-gateway/`) — unaffected, still passes, confirming no application code was touched.

## Completion criteria
- [x] `ory/kratos/kratos.yml` and `ory/kratos/identity.schema.json` exist and are checked in.
- [x] `ory/README.md` documents the new services, ports, and how to inspect identities/emails.
- [x] `docker-compose.yml` gains `kratos`, `kratos-db`, `kratos-migrate`, `mailslurper` as new, alphabetically-positioned, additive blocks; no existing service block is modified.
- [x] `.env.example` (and local `.env`) document all new `KRATOS_*` variables with clear comments, including the 32-character cipher secret constraint.
- [x] No files under `server/`, `llm-gateway/`, or `web/` are modified by this PR.
- [x] All verification checks above pass locally against a clean `docker compose down -v && docker compose up -d` (run in an isolated `docker compose -p step11test up -d kratos-db kratos-migrate kratos mailslurper` project, scoped to just this step's four services, then torn down with `down -v`; the app services `db`/`api`/`web`/`migrate`/`llm-gateway` were intentionally left untouched — see run notes/concerns).
