# Step 8: Rust gateway operational hardening

## Meta
- **Type**: refactor
- **Components**: llm-gateway
- **Wave**: 2 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 3: Rust gateway domain/ports reshape (streaming+multimodal-ready)
- **Unlocks**: Step 14: Rust gateway test harness: router oneshot + wiremock provider tests | Step 19: gRPC contract: proto definitions + Rust tonic inbound adapter | Step 25: Rust: Anthropic provider adapter | Step 26: Rust: Gemini provider adapter
- **Size**: 1 PR (a few hours for one AI agent)

## Context

The LLM Gateway currently has no operational safety net: `OpenAIProvider` reads `OPENAI_BASE_URL` straight from `std::env` inside the adapter (`llm-gateway/src/adapters/outbound/openai.rs`), the `reqwest::Client` only sets a 10s *connect* timeout with no total request timeout and no retry on transient failures, `main.rs` calls `axum::serve(...).await` with no shutdown hook, there is no request-id/tracing correlation across the HTTP stack, and `GET /health` is the only liveness signal (no distinction between "process is up" and "dependencies are actually usable"). Phase 8 in `phases.md` calls for retries, health checks, and hardening before gRPC and additional providers (Anthropic, Gemini) are added; this step delivers the non-gRPC half of that (gRPC itself is Step 19) so that Steps 25/26 can add new provider adapters against a stable, config-driven, retry-aware foundation instead of copy-pasting ad-hoc env reads and timeout logic.

## Goal

After this PR, the gateway is shaped like a production service: all HTTP-client tuning (timeouts, retry/backoff) and per-provider base URLs live in a single consolidated `Config` loaded once in `main.rs` and passed explicitly into adapters — no adapter reads `std::env` directly anymore. Outbound requests to providers retry with bounded exponential backoff on `429`/`5xx` responses. The process shuts down gracefully on `Ctrl+C` or `SIGTERM`, allowing in-flight requests to complete. Every HTTP request is tagged with an `X-Request-Id` (generated if absent, propagated to the response) and wrapped in a `tower-http` trace span so `tracing` logs correlate to a single request. A new `GET /ready` endpoint reports whether the gateway's dependencies (provider API keys, via `KeyStore`) are actually resolvable, distinct from the existing `GET /health` liveness check.

## Scope

- [x] Consolidate configuration in `llm-gateway/src/config.rs`:
  - [x] Add `HttpClientConfig { connect_timeout: Duration, request_timeout: Duration, max_retries: u32, retry_base_delay: Duration }`.
  - [x] Add `ProviderConfig { base_url: String }` (per-provider base URL only — API keys stay resolved via `KeyStore`, never via `Config`).
  - [x] Extend `Config` with `pub http: HttpClientConfig` and `pub openai: ProviderConfig`; load `OPENAI_BASE_URL` here (moved out of the adapter) plus new `LLM_GATEWAY_CONNECT_TIMEOUT_SECS`, `LLM_GATEWAY_REQUEST_TIMEOUT_SECS`, `LLM_GATEWAY_MAX_RETRIES`, `LLM_GATEWAY_RETRY_BASE_DELAY_MS` env vars, each with a sensible default.
  - [x] Add unit tests for `Config::from_env()` default values and env-var overrides (follow the existing test style already used for `EnvKeyStore` in `llm-gateway/src/adapters/outbound/env_key.rs`).
- [x] Add a shared retry helper `llm-gateway/src/adapters/outbound/http_retry.rs` (new module, exported from `adapters/outbound/mod.rs`) so it can be reused by future provider adapters (Steps 25/26), not just OpenAI:
  - [x] `RetryPolicy` struct built from `HttpClientConfig` (`max_retries`, `base_delay`).
  - [x] `RetryPolicy::is_retryable(status: reqwest::StatusCode) -> bool` — true for `429` and any `5xx`.
  - [x] `RetryPolicy::backoff_delay(attempt: u32) -> Duration` — exponential backoff (`base_delay * 2^attempt`).
  - [x] An async `send_with_retry` function that takes a retryable closure returning `Result<reqwest::Response, reqwest::Error>`, resends up to `max_retries` times on a retryable status, sleeping `backoff_delay` (or the response's `Retry-After` header value, if present, on `429`) between attempts, and logs each retry via `tracing::warn!`.
  - [x] Unit tests for `is_retryable` and `backoff_delay` (pure functions — no network needed).
- [x] Rewire `OpenAIProvider` (whatever module path Step 3 leaves it at, e.g. `llm-gateway/src/adapters/outbound/openai.rs` or `openai/mod.rs` if Step 3 splits it into `openai/request.rs` + `openai/stream.rs`):
  - [x] `OpenAIProvider::new` takes `Arc<dyn KeyStore>` (unchanged) plus the new `HttpClientConfig` and `ProviderConfig` — delete the `std::env::var("OPENAI_BASE_URL")` call from the adapter.
  - [x] Build the `reqwest::Client` with both `.connect_timeout(http.connect_timeout)` and `.timeout(http.request_timeout)` (today only `connect_timeout` is set; there is no total-request timeout at all).
  - [x] Wrap the outbound `send()` call in `send_with_retry`, mapping an exhausted-retries `429` to `DomainError::RateLimited` and an exhausted-retries `5xx` to `DomainError::ProviderError` (reuse whatever error mapping Step 3 already established for non-2xx responses; only the retry wrapping is new here).
- [x] Update DI wiring in `llm-gateway/src/main.rs`:
  - [x] Pass `config.http.clone()` and `config.openai.clone()` into `OpenAIProvider::new(...)`.
  - [x] Pass the `key_store` into `CompletionService::new(...)` (see readiness item below) alongside the provider list.
  - [x] Add graceful shutdown: `axum::serve(listener, router).with_graceful_shutdown(shutdown_signal()).await` where `shutdown_signal()` is a small async fn that `tokio::select!`s on `tokio::signal::ctrl_c()` and, on `cfg(unix)`, `tokio::signal::unix::signal(SignalKind::terminate())`, logging which signal triggered shutdown via `tracing::info!`.
- [x] Add readiness support:
  - [x] Add `fn readiness(&self) -> Result<(), DomainError>` to `CompletionUseCase` (`llm-gateway/src/ports/inbound/completion.rs`).
  - [x] Implement it on `CompletionService` (`llm-gateway/src/domain/service.rs`): iterate registered providers and call `key_store.get_key(provider.provider_name())` for each, returning the first `DomainError::KeyNotFound` encountered, `Ok(())` otherwise. No network calls — this only confirms credentials are resolvable, keeping `/ready` fast.
  - [x] Give `CompletionService` a `key_store: Arc<dyn KeyStore>` field and update its constructor accordingly (update the existing `CompletionService::new` call in `main.rs` and the existing unit tests in `llm-gateway/src/domain/service.rs` — the `MockProvider`-based tests there will need a stub `KeyStore` passed in too).
  - [x] Add `pub async fn ready(...)` handler in `llm-gateway/src/adapters/inbound/rest/handlers.rs` returning `200 {"status":"ready"}` when `readiness()` is `Ok`, `503 {"status":"not_ready","error":...}` otherwise.
  - [x] Register `GET /ready` in `llm-gateway/src/adapters/inbound/rest/router.rs` next to the existing `/health` route.
- [x] Add request-id + tracing middleware in `llm-gateway/src/adapters/inbound/rest/router.rs` (or a new sibling `middleware.rs` if that keeps `router.rs` readable):
  - [x] Add `tower`, `tower-http` (features `trace`, `request-id`) and `uuid` (feature `v4`) to `llm-gateway/Cargo.toml`.
  - [x] A `MakeRequestId` impl that generates a UUIDv4 per request (tower-http does not ship a UUID generator itself, to avoid a mandatory `uuid` dependency, so this project supplies its own).
  - [x] Compose, via `tower::ServiceBuilder`, in this order: `SetRequestIdLayer` (generates/accepts `x-request-id`) → `TraceLayer::new_for_http()` with `make_span_with` including the request id, method, and URI in the span → `PropagateRequestIdLayer` (copies the header onto the response). This is the order documented by `tower-http`'s own request-id example and is required for the trace span to see the id and for the response to carry it back.
  - [x] Apply the composed layer to the router in `build_router`.
- [x] Update `docker-compose.yml`'s `llm-gateway` service block and `.env.example`'s `# === LLM Gateway (Rust) ===` section additively with the new env vars (`LLM_GATEWAY_CONNECT_TIMEOUT_SECS`, `LLM_GATEWAY_REQUEST_TIMEOUT_SECS`, `LLM_GATEWAY_MAX_RETRIES`, `LLM_GATEWAY_RETRY_BASE_DELAY_MS`), each with the same default as the Rust-side fallback so omitting them from `.env` is safe.
- [x] Add/extend rustdoc (`///`) on every new/changed public item (`HttpClientConfig`, `ProviderConfig`, `RetryPolicy`, `send_with_retry`, `readiness`, the `ready` handler, `shutdown_signal`) with `# Arguments` / `# Returns` / `# Errors` sections per `CLAUDE.md` conventions.

## Out of scope

- gRPC inbound adapter and proto definitions — Step 19.
- Anthropic / Gemini provider adapters — Steps 25 / 26 (this step only makes the shared retry/config plumbing reusable for them).
- A full wiremock/oneshot integration test harness exercising the retry loop end-to-end against mocked `429`/`500` provider responses — Step 14 adds that harness; this step verifies the retry *logic* with pure unit tests (`is_retryable`, `backoff_delay`) plus a manual smoke check (see Verification).
- Model metadata (token limits, pricing) mentioned in `phases.md` Phase 8 — not part of this step's `keyScope`.
- OpenTelemetry span export / collector wiring — this step only adds `tower-http` `TraceLayer` + request-id correlation inside existing `tracing` JSON logs, not an OTel exporter.
- Circuit breakers or provider health caching beyond the single synchronous `readiness()` key check.

## Implementation notes

- Real files to modify: `llm-gateway/src/config.rs`, `llm-gateway/src/main.rs`, `llm-gateway/src/adapters/outbound/openai.rs` (or its Step 3 split), `llm-gateway/src/adapters/outbound/mod.rs`, `llm-gateway/src/adapters/inbound/rest/router.rs`, `llm-gateway/src/adapters/inbound/rest/handlers.rs`, `llm-gateway/src/ports/inbound/completion.rs`, `llm-gateway/src/domain/service.rs`, `llm-gateway/Cargo.toml`, `docker-compose.yml`, `.env.example`. New file: `llm-gateway/src/adapters/outbound/http_retry.rs`.
- **conflictNotes**: this step touches the OpenAI adapter module and `main.rs` right after Step 3's domain/ports rewrite — rebase onto Step 3's branch structure (module path, `CompletionRequest`/`DomainError` shape) before editing rather than assuming today's file layout verbatim. Step 14 lands its test harness afterward and will add integration tests against the retry helper and `/ready`/`/health` handlers introduced here — do not pre-build that harness in this step.
- Today `OpenAIProvider::new` reads `OPENAI_BASE_URL` directly (`llm-gateway/src/adapters/outbound/openai.rs:38`) and only sets `.connect_timeout(...)` on the client (`:44`) with no total timeout and no retry — this is exactly the gap `keyScope` calls out ("no raw env reads in adapters", "reqwest client timeout from Config"). Move the env read to `Config::from_env()` and delete it from the adapter.
- `DomainError` (`llm-gateway/src/domain/error.rs`) is expected to already carry a `RateLimited` variant and richer error shape after Step 3's reshape (per the plan-wide architecture notes: "thiserror DomainError with source chains + RateLimited"). If, when this step is implemented, that variant is not yet present, add it as a minimal one-variant addition (plus the corresponding `AppError` → HTTP `429` mapping arm in `llm-gateway/src/adapters/inbound/rest/handlers.rs`, next to the existing `DomainError::Timeout → 504` and `DomainError::ProviderError → 502` arms) — do not redesign the error enum beyond that.
- `CompletionService::new` currently takes only `providers: Vec<Box<dyn LLMProvider>>` (`llm-gateway/src/domain/service.rs:22`). Adding a `key_store: Arc<dyn KeyStore>` parameter means updating every call site: the DI assembly in `main.rs` and the four existing unit tests in `domain/service.rs` (`test_routes_to_correct_provider`, `test_model_not_found`, `test_empty_messages_rejected`, `test_list_models_aggregates_all_providers`) each construct `CompletionService::new(vec![...])` — give them a trivial always-succeeding `KeyStore` test double (a small struct implementing `KeyStore::get_key` returning `Ok("test-key".into())`) rather than reaching for `EnvKeyStore`.
- Router changes: today's `build_router` (`llm-gateway/src/adapters/inbound/rest/router.rs`) is a bare `Router::new().route(...).with_state(state)`. Insert the `ServiceBuilder`-composed middleware stack via `.layer(...)` before `.with_state(state)` (state must be the last thing applied so the layers wrap the stateful router, matching axum 0.8's usual pattern).
- `HttpClientConfig` and `ProviderConfig` should derive `Clone` since both `main.rs` and (in the future) multiple provider adapters need owned copies.
- Env var naming: keep the `LLM_GATEWAY_` prefix for gateway-wide HTTP tuning (mirrors the existing `LLM_GATEWAY_PORT`), and keep `OPENAI_BASE_URL` / `OPENAI_API_KEY` as the per-provider prefix (mirrors `EnvKeyStore`'s `{PROVIDER}_API_KEY` convention in `llm-gateway/src/adapters/outbound/env_key.rs`) so Steps 25/26 can add `ANTHROPIC_BASE_URL`/`GEMINI_BASE_URL` the same way without touching the gateway-wide vars.
- `docker-compose.yml`'s `llm-gateway` service block (currently just `LLM_GATEWAY_PORT` and `OPENAI_API_KEY`) and `.env.example`'s `# === LLM Gateway (Rust) ===` section are edited additively only — do not reorder or remove existing lines, per the plan-wide convention that these two files receive alphabetized/additive edits across many steps.
- No `schema.sql` / Atlas changes — this step touches no database tables (`dbChanges` is empty).
- Follow existing rustdoc conventions already used throughout the crate (e.g. `llm-gateway/src/adapters/outbound/env_key.rs`, `llm-gateway/src/ports/outbound/key_store.rs`) — one-line summary, then `# Arguments`/`# Environment Variables`/`# Errors` sections as applicable.

## Verification

1. `cd llm-gateway && cargo build` — compiles cleanly with the new `tower`, `tower-http`, and `uuid` dependencies.
2. `cd llm-gateway && cargo test` — all existing and new unit tests pass, including: `Config::from_env` default/override tests, `RetryPolicy::is_retryable`/`backoff_delay` tests, the updated `domain/service.rs` tests (now constructing `CompletionService` with a stub `KeyStore`), and the existing `openai.rs`/`env_key.rs` tests.
3. `cd llm-gateway && cargo clippy` — no new warnings (matches `Taskfile.yml`'s `lint:gateway` task).
4. Start the gateway locally with a fake key: `cd llm-gateway && OPENAI_API_KEY=test-key cargo run` (default port 8081). In another terminal:
   - `curl -si http://localhost:8081/health` → `200`, body `{"status":"ok"}`.
   - `curl -si http://localhost:8081/ready` → `200`, body `{"status":"ready"}` (the key resolves via `KeyStore`, no network call made).
   - `curl -si http://localhost:8081/health -H 'X-Request-Id: test-req-1'` → response includes `x-request-id: test-req-1` (propagated); a bare `curl -si http://localhost:8081/health` (no header) still returns an `x-request-id` header (server-generated UUID), and the gateway's JSON log line for that request includes the same `request_id`.
5. Restart the gateway *without* `OPENAI_API_KEY` set: `cd llm-gateway && cargo run`, then `curl -si http://localhost:8081/ready` → `503`, body containing `"status":"not_ready"` — confirms `/ready` genuinely distinguishes from `/health`, which should still return `200`.
6. Graceful shutdown: start the gateway (`OPENAI_API_KEY=test-key cargo run &`), note its PID, run `kill -TERM <pid>`; the process logs a shutdown message and exits with status `0` within a couple of seconds (check `echo $?` after `wait <pid>`), rather than being killed mid-request. Repeat with `Ctrl+C` (SIGINT) in an interactive `cargo run` session.
7. Retry/backoff smoke check: point the gateway at a throwaway local stub that returns `429` twice then `200`, and confirm the gateway succeeds (proving the retry loop runs) rather than surfacing an error after the first `429`:
   ```
   OPENAI_BASE_URL=http://localhost:9091 OPENAI_API_KEY=test-key cargo run &
   python3 - <<'EOF' &
   import http.server
   class H(http.server.BaseHTTPRequestHandler):
       hits = 0
       def do_POST(self):
           H.hits += 1
           if H.hits < 3:
               self.send_response(429); self.send_header('Content-Type','application/json'); self.send_header('Retry-After','1'); self.end_headers()
               self.wfile.write(b'{"error":{"message":"rate limited"}}')
           else:
               body = b'{"id":"x","model":"gpt-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}'
               self.send_response(200); self.send_header('Content-Type','application/json'); self.end_headers(); self.wfile.write(body)
   http.server.HTTPServer(('localhost', 9091), H).serve_forever()
   EOF
   curl -si http://localhost:8081/completions -H 'Content-Type: application/json' \
     -d '{"model":"gpt-5.2","messages":[{"role":"user","content":"hi"}]}'
   ```
   Expect the final `curl` to return `200` with the mocked completion body, and the gateway's logs to show two `tracing::warn!` retry lines before success.
8. `cd .. && task lint:gateway` and `task test:gateway` (from repo root, per `Taskfile.yml`) both succeed, confirming the step integrates with the project's standard task runner.

## Completion criteria

- [x] `Config` is the single source of truth for HTTP timeouts, retry policy, and the OpenAI base URL; no adapter reads `std::env` directly.
- [x] Outbound OpenAI requests retry with bounded exponential backoff on `429`/`5xx`, honoring `Retry-After` when present, and give up after `max_retries` with a mapped `DomainError`.
- [x] The gateway shuts down gracefully on `Ctrl+C` and `SIGTERM`, logging which signal triggered it.
- [x] Every request is tagged with an `X-Request-Id` (generated if absent) that is propagated to the response and appears in the corresponding `tracing` log span.
- [x] `GET /ready` reports `503` when a registered provider's API key cannot be resolved via `KeyStore`, and `200` otherwise, independent of `GET /health` (which always reports `200` once the process is up).
- [x] All new/changed public items have rustdoc comments following `CLAUDE.md` conventions.
- [x] All verification checks above pass.
