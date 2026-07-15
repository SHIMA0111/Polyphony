# Step 10: Playwright E2E harness + seeded compose test stack + LLM stub

## Meta
- **Type**: testing
- **Components**: web, infra
- **Wave**: 2 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 4: Web BFF auth + data-plane proxy (httpOnly cookies, middleware guard, typed HTTP client); Step 6: Infra tooling baseline (compose healthchecks, Taskfile, env staging, migration convention)
- **Unlocks**: Step 30: Web Kratos-driven login/registration + auth data-plane flip; Step 37: Web members, invitations, and roles UI; Step 38: Web AI context UI (exclude toggle, delete, token meter); Step 44: OAuth social login via Kratos OIDC; Step 45: Web image upload + attachment UI + Vision send; Step 47: Web private AI mode UI; Step 48: Web token balance + usage history UI; Step 52: Web room fork UI; Step 53: Web plans/Stripe Checkout/billing UI; Step 54: Web streaming AI rendering; Step 56/57/58/59: E2E regression suites
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Every feature step from wave 4 onward is required to ship its own Playwright spec, and four dedicated regression suites (steps 56-59) exercise the whole stack end to end. None of that is possible without a harness that (a) drives a real, isolated instance of the app instead of the developer's own `docker-compose.yml` stack, (b) has deterministic AI responses so assertions don't depend on a live OpenAI account, and (c) has a scriptable way to get a logged-in user and a room without re-running the full registration UI flow in every spec. This step builds that harness once, so it is never improvised or duplicated by later steps — every later spec-shipping step (30/37/38/44/45/47/48/52/53/54 and the four regression suites) declares an explicit dependency on it. It builds on Step 4's httpOnly-cookie BFF proxy (the smoke spec exercises that proxy path) and Step 6's compose/Taskfile/healthcheck conventions (the test profile follows the same patterns).

## Goal
After this PR, `task test:e2e:up` brings up a fully isolated Docker Compose stack (Postgres, migrations, Go API, LLM Gateway, a new canned OpenAI-compatible LLM stub, and the web app) on alternate host ports next to — and safely concurrent with — the normal dev stack, `task test:e2e` seeds an idempotent fixture user/room and runs Playwright against it, and one smoke spec (register → create room → send a plain message through the Step-4 BFF proxy) passes reliably. The LLM stub serves both static non-streaming completions and canned SSE streaming fixtures so that every future AI/streaming/Vision spec (steps 38, 45, 54, 57, 58) can point at it without adding any new provider-stubbing infrastructure of its own.

## Scope
- [ ] Add `@playwright/test` as a dev dependency in `web/package.json` (via `bun add -d @playwright/test`) and a `web/playwright.config.ts` with `testDir: "./e2e"`, a `baseURL` read from `PLAYWRIGHT_BASE_URL` (default `http://localhost:3001`), a `globalSetup` pointing at the seed script, and a single `chromium` project.
- [ ] Create `web/e2e/support/fixtures.ts` exporting the fixed E2E fixture user (`email`, `username`, `password`) and fixture room name as constants, for this and future spec steps to import.
- [ ] Create `web/e2e/seed/seed.ts`: an idempotent seed routine (default-exported `globalSetup` function, plus a `bun run` entrypoint guard) that waits for the API's `/health`, registers the fixture user (falling back to login if the user already exists — HTTP 400/409 from `/auth/register`), and ensures the fixture room exists (list rooms, create if missing by name) via direct `fetch` calls to the test-profile API (`E2E_API_URL`, default `http://localhost:8090`). Must be safe to run repeatedly with no side effects on the second+ run.
- [ ] Create `web/e2e/smoke.spec.ts`: a single Playwright spec that registers a brand-new user (unique email/username per run, no seed dependency), creates a new room, opens it, sends a plain (non-AI) message, and asserts the message appears — driven entirely through the UI so it exercises the Step-4 BFF proxy end to end.
- [ ] Add Playwright artifacts (`web/playwright-report/`, `web/test-results/`, `web/e2e-report/`) to `web/.gitignore`.
- [ ] Create a new `llm-stub/` service: a minimal Bun HTTP server (`llm-stub/server.ts`, `llm-stub/package.json`, `llm-stub/Dockerfile`) implementing an OpenAI-compatible `POST /v1/chat/completions` (matching the shape `OpenAIProvider` in `llm-gateway/src/adapters/outbound/openai.rs` sends/expects) plus `GET /health`.
- [ ] Implement deterministic fixture selection in the stub: if the last user message starts with `[[fixture:NAME]]`, load `llm-stub/fixtures/NAME.json` (non-streaming) or `llm-stub/fixtures/stream/NAME.sse` (when the request body has `"stream": true`); otherwise use `default`. Unknown fixture names return HTTP 404 with a JSON error body so a missing fixture fails a spec loudly instead of silently returning the wrong canned answer.
- [ ] Add `llm-stub/fixtures/default.json` (canned non-streaming completion body) and `llm-stub/fixtures/stream/default.sse` (canned OpenAI-style SSE chunk sequence terminated by `data: [DONE]`), matching OpenAI's real wire formats so later steps can add more `NAME.json` / `NAME.sse` fixtures without touching `server.ts`.
- [ ] Add a minimal `llm-stub/server.test.ts` (run via `bun test`) covering the fixture-selection logic (default fallback, explicit `[[fixture:NAME]]` marker, unknown-fixture 404) and the streaming vs. non-streaming branch.
- [ ] Add a Docker Compose **test profile** to `docker-compose.yml`: new services `api-e2e`, `db-e2e`, `llm-gateway-e2e`, `llm-stub`, `migrate-e2e`, `web-e2e`, each tagged `profiles: ["test"]`, on alternate host ports (`5433`, `8090`, `8091`, `8092`, `3001`) so the test stack can run alongside the normal dev stack without conflicts. `llm-gateway-e2e` sets `OPENAI_BASE_URL` to the internal `llm-stub` address so the gateway talks to the stub instead of the real OpenAI API.
- [ ] Wire `web-e2e`'s server-side proxy target (the internal base-URL env var Step 4's `app/api/proxy/[...path]` route reads) at `http://api-e2e:8080`, matching whatever env var name Step 4 introduced (inspect the proxy route file — see Implementation notes).
- [ ] Add Taskfile tasks: `test:e2e:up` (start the test profile, build if needed), `test:e2e:seed` (run the seed script standalone against the test stack), `test:e2e` (ensure the stack is up, then run Playwright — seeding also happens automatically via `globalSetup`), and `test:e2e:down` (tear the test stack down, including its volumes).
- [ ] Update `README.md` (or add a short section) documenting how to run the E2E stack locally, kept minimal — full docs/progress sync happens in the final gate step.

## Out of scope
- Any spec beyond the one smoke spec (all feature-area specs ship in their own steps: 30, 37, 38, 44, 45, 47, 48, 52, 53, 54; regression suites ship in 56-59).
- Actually consuming the SSE streaming fixture from the Go API or LLM Gateway — the gateway has no streaming port yet (`CompletionChunk`/`stream()` land in a later gateway step); this step only guarantees the fixture format exists and is served, ready for that later step to consume.
- Vision/multipart image fixtures (owned by step 45, once the gateway's `MessageContent` enum and image upload exist).
- Any change to `llm-gateway/src` Rust code — the gateway already reads `OPENAI_BASE_URL` from the environment (see `llm-gateway/src/adapters/outbound/openai.rs`), so pointing it at the stub requires no gateway code changes, only the new compose service and env var.
- Any change to the Go API's auth/room/message handlers or the web app's existing components (`RegisterForm.tsx`, `CreateRoomForm.tsx`, `MessageInput.tsx`, etc.) — the smoke spec is written against their current selectors/placeholders.
- CI pipeline wiring (GitHub Actions) — excluded per the binding scope decision (phases 21-23 excluded); this step only needs to run locally via Taskfile/Docker Compose.
- MSW-based component/unit tests — that is Step 5/9's responsibility; this step is browser-driven E2E only.

## Implementation notes

### LLM Gateway wire format (must match exactly)
`llm-gateway/src/adapters/outbound/openai.rs` posts to `{OPENAI_BASE_URL}/v1/chat/completions` with header `Authorization: Bearer {key}` and JSON body:
```json
{ "model": "...", "messages": [{ "role": "developer|user|assistant|tool", "content": "..." }], "temperature": 0.7, "max_tokens": 1000 }
```
and expects a response shaped like:
```json
{
  "id": "...", "model": "...",
  "choices": [{ "index": 0, "message": { "role": "assistant", "content": "..." }, "finish_reason": "stop" }],
  "usage": { "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0 }
}
```
On non-2xx it tries to parse `{ "error": { "message": "..." } }`. The stub's `default.json` fixture should contain everything except `id`/`model` (server fills `id` as `chatcmpl-e2e-<fixtureName>` and `model` by echoing the request's `model` field, since the gateway does not currently accept arbitrary provider-echoed ids). The stub does **not** need to validate the `Authorization` header value — any bearer token is accepted, since `OPENAI_API_KEY` for `llm-gateway-e2e` is a dummy value (`EnvKeyStore` in `llm-gateway/src/adapters/outbound/env_key.rs` only needs a non-empty `OPENAI_API_KEY` to resolve, per the project's `{PROVIDER}_API_KEY` convention).

### `llm-stub/` service
- Runtime: Bun (`oven/bun:1-alpine` base image in `llm-stub/Dockerfile`, consistent with the project's bun preference).
- `GET /health` → `200 { "status": "ok" }`.
- `POST /v1/chat/completions`:
  - Parse the last message with `role: "user"` from the request body's `messages` array.
  - If its `content` starts with `[[fixture:NAME]]`, use `NAME`; otherwise use `default`.
  - If `body.stream === true`: respond `Content-Type: text/event-stream` and stream the raw contents of `llm-stub/fixtures/stream/{NAME}.sse` (or 404 JSON if the file does not exist).
  - Else: respond `200 application/json` with `llm-stub/fixtures/{NAME}.json` merged with `id`/`model` as described above (or 404 JSON if the file does not exist).
- Keep responses fully static/deterministic — no randomness, no echoing of arbitrary user content into the canned response body (only the fixture-selection marker is inspected).

### Compose test profile (additive to `docker-compose.yml`)
Existing services (`db`, `migrate`, `api`, `llm-gateway`, `web`) are untouched. Add six new blocks, each with `profiles: ["test"]`, grouped together and alphabetized among themselves (`api-e2e`, `db-e2e`, `llm-gateway-e2e`, `llm-stub`, `migrate-e2e`, `web-e2e`) per the project's convention of additive, alphabetized service blocks (this file is touched by multiple later steps — 11/12/31/44/49/55 — so keep the diff scoped to only these six new blocks):

- `db-e2e`: same image/env as `db`, host port `5433:5432`, its own healthcheck and its own named volume (do not share `pgdata` with the dev stack).
- `migrate-e2e`: same `arigaio/atlas` image/command/volume as `migrate`, pointed at `db-e2e`, `depends_on: db-e2e (service_healthy)`.
- `api-e2e`: same build context (`./server`) as `api`, `DATABASE_URL` pointed at `db-e2e`, a distinct `JWT_SECRET` (e.g. `e2e-test-secret`), `LLM_GATEWAY_URL` pointed at `http://llm-gateway-e2e:8081`, `CORS_ORIGINS` set to `http://localhost:3001`, host port `8090:8080`, `depends_on: migrate-e2e (service_completed_successfully)`.
- `llm-stub`: build context `./llm-stub`, host port `8092:8092`, add a healthcheck (`wget`-based, busybox `wget` is present on Alpine) so `llm-gateway-e2e` can `depends_on: llm-stub (service_healthy)`.
- `llm-gateway-e2e`: same build context (`./llm-gateway`) as `llm-gateway`, `OPENAI_API_KEY` set to a dummy non-empty value, `OPENAI_BASE_URL: http://llm-stub:8092`, host port `8091:8081`, `depends_on: llm-stub (service_healthy)`.
- `web-e2e`: same build context (`./web`) as `web`, host port `3001:3000`, `depends_on: api-e2e`. Set whatever internal env var Step 4's `web/src/app/api/proxy/[...path]/route.ts` (or equivalent path — check the actual file Step 4 lands) uses to reach the Go API server-side, to `http://api-e2e:8080` — the container-internal port, not the host-mapped `8090`.

### Taskfile additions (additive to `Taskfile.yml`, new `E2E` section)
```yaml
  test:e2e:up:
    desc: Start the isolated E2E compose stack (test profile, alternate ports)
    cmds:
      - docker compose --profile test up -d --build

  test:e2e:seed:
    desc: Seed the idempotent E2E fixture user + room against the running test stack
    dir: web
    cmds:
      - bun run e2e/seed/seed.ts

  test:e2e:
    desc: Run Playwright E2E specs against the seeded test stack
    dir: web
    deps: [test:e2e:up]
    cmds:
      - bunx playwright test

  test:e2e:down:
    desc: Tear down the E2E compose stack and remove its volumes
    cmds:
      - docker compose --profile test down -v
```
Do not add `test:e2e` to the plain `test` task's `deps` — it requires Docker and is meaningfully slower, so it stays an explicit, separately-invoked task (mirrors how `test:server`/`test:gateway` are fast unit-only tasks).

### Seed idempotency
`web/e2e/seed/seed.ts` must tolerate being run against a test database that already has the fixture user/room (e.g. re-running `task test:e2e` without tearing down first): treat a non-2xx `/auth/register` response as "already exists" and fall back to `/auth/login` with the same credentials; treat an existing room with the fixture name (found via `GET /rooms`) as sufficient — do not create a duplicate. Poll `GET {E2E_API_URL}/health` with backoff (e.g. up to 30s) before the first request, since `docker compose up -d` returns before `api-e2e` has finished booting.

### Chakra/UI conventions
No new UI components are added in this step (the smoke spec only drives existing pages), so `.claude/rules/chakra-ui.md` conventions do not apply here. If the smoke spec needs any selector help, prefer resilient Playwright locators (`getByPlaceholder`, `getByRole`) over new `data-testid` attributes, matching the current `RegisterForm.tsx`/`CreateRoomForm.tsx`/`MessageInput.tsx` markup (e.g. placeholders `"you@example.com"`, `"johndoe"`, `"Create a password"`, `"Confirm your password"`, `"e.g., Product Strategy"`, `"Ask me anything..."`, and button labels `"Create account"`, `"New Room"`, `"Create room"`, `"Send"`).

### Conflict notes
`docker-compose.yml` and `Taskfile.yml` edits in this step are additive only (new service blocks / new tasks); later steps (11, 12, 31, 44, 49, 55) also add their own additive, alphabetized blocks to these same two files — do not reorder or reformat the existing `db`/`migrate`/`api`/`llm-gateway`/`web` blocks or existing tasks. This step is the sole owner of the LLM stub (`llm-stub/` directory and its compose/Taskfile wiring); no later step should add a competing provider stub — steps 38/45/54/57/58 must reuse this one via new fixture files only.

## Verification
1. `cd web && bun install && bunx playwright install --with-deps chromium` — installs the Playwright browser; exits 0.
2. `cd llm-stub && bun test` — the fixture-selection unit tests pass.
3. `task test:e2e:up` — brings up the six test-profile services; `docker compose --profile test ps` shows `db-e2e`, `migrate-e2e` (exited 0), `api-e2e`, `llm-stub`, `llm-gateway-e2e`, `web-e2e` all `Up`/healthy.
4. `curl -s http://localhost:8092/health` → `{"status":"ok"}` (HTTP 200).
5. `curl -s -X POST http://localhost:8091/completions -H 'Content-Type: application/json' -d '{"model":"gpt-5-mini","messages":[{"role":"user","content":"hi"}]}'` (adjust to the LLM Gateway's actual REST completions path in `llm-gateway/src/adapters/inbound/rest/router.rs`) → HTTP 200 with a `choices[0].message.content` equal to the `default.json` fixture's canned text, proving the gateway → stub wiring works end to end.
6. `task test:e2e:seed` run twice in a row — both runs exit 0 with no duplicate-user or duplicate-room errors (idempotency check).
7. `task test:e2e` — Playwright reports `1 passed` for `smoke.spec.ts`.
8. `task test:e2e:down` — stack tears down cleanly (`docker compose --profile test ps` shows no containers).
9. `task up` (the existing dev stack, unaffected ports 5432/8080/8081/3000) still starts cleanly afterward, confirming the test profile did not alter or conflict with the dev stack.
10. `task test:server && task test:gateway` — existing unit tests still pass unmodified (regression check that no Go/Rust source was touched).

## Completion criteria
- [ ] `web/playwright.config.ts` and `web/e2e/` (smoke spec, seed script, fixtures support module) exist and are wired to run against the test-profile stack.
- [ ] `llm-stub/` service exists with deterministic fixture-based completions and SSE streaming responses, plus its own unit tests.
- [ ] `docker-compose.yml` has a working, alternate-port `test` profile covering all six new services, fully additive to the existing dev services.
- [ ] `Taskfile.yml` has `test:e2e:up`, `test:e2e:seed`, `test:e2e`, `test:e2e:down` tasks.
- [ ] The seed routine is idempotent (safe to re-run without teardown).
- [ ] The smoke spec passes against the Step-4 BFF proxy path (register → create room → send message).
- [ ] All verification checks above pass.
