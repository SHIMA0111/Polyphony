# Step 25: Rust: Anthropic provider adapter

## Meta
- **Type**: feature
- **Components**: llm-gateway
- **Wave**: 4 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 8: Rust gateway operational hardening | Step 14: Rust gateway test harness: router oneshot + wiremock provider tests
- **Unlocks**: Step 34: Model metadata (token limits + pricing) across gateway/server/web | Step 39: Vision multimodal plumbing: gateway content-parts mapping + Go DTOs | Step 43: Rust: streaming SSE for all providers + stream endpoint
- **Size**: 1 PR (a few hours for one AI agent)

## Context

`phases.md` Phase 6 ("Multi-provider (Anthropic)") calls for a second AI provider so rooms are no longer locked to OpenAI. Today `llm-gateway` has exactly one outbound adapter, `OpenAIProvider` (`llm-gateway/src/adapters/outbound/openai.rs`), registered with a single hard-coded line in `main.rs`. By the time this step runs, Step 8 has moved all HTTP-client tuning (timeouts, retry/backoff) and per-provider base URLs into `Config`/`HttpClientConfig`/`ProviderConfig` (no adapter reads `std::env` directly) and added a shared `send_with_retry`/`RetryPolicy` helper in `llm-gateway/src/adapters/outbound/http_retry.rs`, and Step 14 has built the wiremock + `tower::oneshot` test harness (`llm-gateway/tests/router_test.rs`, `llm-gateway/tests/openai_provider_test.rs`) that this step's tests must extend rather than reinvent. This step adds `AnthropicProvider` as the second `LLMProvider` implementation, proving the hexagonal boundary (`ports::outbound::provider::LLMProvider`) actually supports multiple providers cleanly, and exercising the Anthropic Messages API's two structural differences from OpenAI's Chat Completions API: a top-level `system` field (not a `system`-role message) and a required (not optional) `max_tokens`.

## Goal

After this PR, `llm-gateway` has a second working `LLMProvider` adapter, `AnthropicProvider`, that calls Anthropic's Messages API (`POST /v1/messages`) end-to-end through the existing `/completions` and `/models` REST endpoints: domain `ChatMessage`/`Role` values are mapped to Anthropic's request shape (system messages hoisted to the top-level `system` field, `max_tokens` defaulted when the domain request omits it), Anthropic's response (`content` blocks, `stop_reason`, `usage.input_tokens`/`output_tokens`) is mapped back to the shared `CompletionResponse`/`Choice`/`Usage` domain types, and Anthropic's error responses and HTTP statuses (401/403, 429 with `Retry-After`, 5xx/529) are mapped to the existing `DomainError` variants using the same retry/backoff plumbing Step 8 built for OpenAI. The adapter is registered in `main.rs` with one additive line, its API key resolves via the existing `KeyStore` port (`ANTHROPIC_API_KEY`), and a new wiremock integration test file proves all of the above without any real network call or real API key.

## Scope

- [x] Create the `llm-gateway/src/adapters/outbound/anthropic/` module, mirroring the per-provider `request.rs`/`stream.rs` submodule split Step 3 established for the OpenAI adapter (check `llm-gateway/src/adapters/outbound/openai.rs` — or `openai/mod.rs` + `openai/request.rs` + `openai/stream.rs` if Step 3 already split it — and copy that exact file layout, do not invent a different shape):
  - [x] `anthropic/mod.rs` — `AnthropicProvider` struct (client, base URL, API key resolved via `KeyStore`, `HttpClientConfig`/`ProviderConfig` injected per Step 8's config-injection pattern — no `std::env::var` calls inside the adapter), its constructor, and the `LLMProvider` trait impl (`complete`, `models`, `provider_name`, plus whatever `stream`-related method Step 3 added to the trait — see the stream.rs note below).
  - [x] `anthropic/request.rs` — Anthropic-specific DTOs (`AnthropicRequest`, `AnthropicMessage`, `AnthropicContentBlock`, `AnthropicResponse`, `AnthropicUsage`, `AnthropicErrorResponse`) and the `to_anthropic_request` / `from_anthropic_response` mapping functions, plus role-mapping helpers.
  - [x] `anthropic/stream.rs` — a minimal placeholder satisfying whatever streaming port Step 3 added to `LLMProvider` (`ports/outbound/provider.rs`) so the trait impl compiles; return a clearly-labeled "not implemented" `DomainError` rather than attempting real SSE parsing (that is Step 43's scope) — confirm the exact trait method signature in `llm-gateway/src/ports/outbound/provider.rs` before writing this and match it exactly.
  - [x] Register the new module: add `pub mod anthropic;` to `llm-gateway/src/adapters/outbound/mod.rs`.
- [x] Implement request mapping in `anthropic/request.rs`:
  - [x] Partition `CompletionRequest.messages` by `Role`: messages with `Role::System` are concatenated (joined with `"\n\n"` if there is more than one) into the Anthropic request's top-level `system: Option<String>` field and excluded from the `messages` array entirely — Anthropic's Messages API does not accept a `system`-role message inside `messages`.
  - [x] Map `Role::User` → `"user"`, `Role::Assistant` → `"assistant"`. Map `Role::Tool` to `"user"` as a documented best-effort fallback (rustdoc must state this is a known limitation — full Anthropic tool-use/tool-result content-block support is not implemented here).
  - [x] `max_tokens` is required by Anthropic's API (unlike OpenAI's optional field). Add a documented `DEFAULT_MAX_TOKENS` constant (e.g. `4096`) used whenever `CompletionRequest.max_tokens` is `None`, with a rustdoc `# Errors`/note explaining Anthropic returns `400 invalid_request_error` if `max_tokens` is omitted.
  - [x] Pass `temperature` through unchanged when present (`Option<f32>`, same as OpenAI).
  - [x] Anthropic's response `content` is an array of blocks (`{"type": "text", "text": "..."}`); concatenate all `text`-type blocks into a single string for the domain `Choice.message.content` (there is exactly one `Choice`, `index: 0`, since Anthropic's Messages API is not multi-choice).
  - [x] Map Anthropic's `stop_reason` (`"end_turn"`, `"max_tokens"`, `"stop_sequence"`, `"tool_use"`) straight through as the domain `Choice.finish_reason: String` (no need to re-encode as an enum — `finish_reason` is already a plain `String` in `domain::model::Choice`).
  - [x] Map `usage.input_tokens` → `Usage.prompt_tokens`, `usage.output_tokens` → `Usage.completion_tokens`, and `Usage.total_tokens = input_tokens + output_tokens` (Anthropic's usage object has no `total_tokens` field, unlike OpenAI's).
- [x] Implement error/usage mapping to domain `DomainError` in `anthropic/mod.rs`'s `complete()` (reusing Step 8's `http_retry::send_with_retry`/`RetryPolicy` exactly as `OpenAIProvider` does — do not reimplement retry logic):
  - [x] `401`/`403` → `DomainError::ProviderError` with Anthropic's `error.message` surfaced in the text (parse the response body as `AnthropicErrorResponse { type: String, error: { type: String, message: String } }`, falling back to the raw body if it doesn't parse).
  - [x] `429` (with `Retry-After` header, same pattern as the OpenAI adapter/Step 8) → `DomainError::RateLimited` after retries are exhausted.
  - [x] `500`–`599` (including Anthropic's `529 overloaded_error`, which falls within the `RetryPolicy::is_retryable` 5xx range already implemented in Step 8 — no special-casing needed) → `DomainError::ProviderError` after retries are exhausted.
  - [x] Network timeout → `DomainError::Timeout` (reuse the existing `reqwest::Error::is_timeout()` check already used in `openai.rs`).
  - [x] A `200 OK` response whose body doesn't deserialize into `AnthropicResponse` → `DomainError::ProviderError` (parse failure) without panicking.
- [x] Wire up `ANTHROPIC_API_KEY` and DI registration:
  - [x] `AnthropicProvider::new` resolves its key via `key_store.get_key("anthropic")` (the existing `KeyStore` port and `EnvKeyStore`'s `{PROVIDER}_API_KEY` convention already produce `ANTHROPIC_API_KEY` for provider name `"anthropic"` — no `KeyStore`/`EnvKeyStore` changes needed).
  - [x] Extend `Config` (`llm-gateway/src/config.rs`) with `pub anthropic: ProviderConfig`, loading `ANTHROPIC_BASE_URL` (default `"https://api.anthropic.com"`) the same way Step 8 loads `OPENAI_BASE_URL` into `config.openai`.
  - [x] Register `AnthropicProvider` in `llm-gateway/src/main.rs` with one additive block analogous to the existing `OpenAIProvider` construction, then push it into the `providers` vec passed to `CompletionService::new(...)` alongside `OpenAIProvider` — this is the only change to `main.rs`.
  - [x] Implement `models()` returning at least `claude-opus-4-6`, `claude-sonnet-4-6`, and `claude-haiku-4-6` (`ModelInfo { id, name, provider: "anthropic", owned_by: "anthropic" }`) — reuse the `claude-opus-4-6` id already referenced by the existing `MockProvider`-based tests in `llm-gateway/src/domain/service.rs` (`test_routes_to_correct_provider`, `test_list_models_aggregates_all_providers`) for consistency.
  - [x] `provider_name()` returns `"anthropic"`.
- [x] Add the Anthropic API's required headers to every outbound request: `x-api-key: <resolved key>`, `anthropic-version: 2023-06-01`, `content-type: application/json` (Anthropic does not use `Authorization: Bearer`, unlike OpenAI).
- [x] Add unit tests in `anthropic/request.rs` (same style as the existing `#[cfg(test)] mod tests` in `openai.rs`) for: system-message hoisting (single and multiple system messages), `max_tokens` defaulting when `None`, role mapping (`Role::User`/`Role::Assistant`/`Role::Tool`), and response mapping (`content` block concatenation, `usage` field mapping).
- [x] Add a new wiremock integration test file `llm-gateway/tests/anthropic_provider_test.rs`, following the exact pattern established by Step 14's `llm-gateway/tests/openai_provider_test.rs` (construct the provider via config injection pointed at `wiremock::MockServer::start().await`'s URI — never `std::env::set_var`), covering:
  - [x] `200 OK` success response (a realistic Anthropic Messages API JSON fixture) → asserts mapped `CompletionResponse` fields (id, model, concatenated content, `stop_reason` as `finish_reason`, `usage.prompt_tokens`/`completion_tokens`/`total_tokens`).
  - [x] Request-shape assertion: the JSON body wiremock receives has a top-level `system` field and no `system`-role entry inside `messages`, and always has `max_tokens` present even when the domain `CompletionRequest.max_tokens` is `None`.
  - [x] `401 Unauthorized` → `DomainError::ProviderError` with the Anthropic error message surfaced.
  - [x] `429 Too Many Requests` (with a `Retry-After` header) → `DomainError::RateLimited` after retries.
  - [x] `500 Internal Server Error` → `DomainError::ProviderError` after retries.
  - [x] A `200 OK` response with a body that doesn't match `AnthropicResponse`'s shape → `DomainError::ProviderError` (parse failure), no panic.
  - [x] A wiremock response delayed longer than the provider's configured client timeout → `DomainError::Timeout`.
- [x] Add/extend a `/models` case (or extend Step 14's `router_test.rs` `GET /models` case) so it can be manually verified that `claude-opus-4-6` etc. appear in the aggregated model list once `AnthropicProvider` is registered (a full new router-test case is optional — the existing `StubUseCase`-based router tests are provider-agnostic; this is primarily a manual verification item, see Verification).
- [x] Additive env/config plumbing: add `ANTHROPIC_API_KEY` to `docker-compose.yml`'s `llm-gateway` service `environment:` block and to `.env.example`'s `# === LLM Gateway (Rust) ===` section, following the existing `OPENAI_API_KEY` line exactly (append, do not reorder existing lines — Step 26 will add its own analogous `GEMINI_API_KEY` line in the same wave).
- [x] Add rustdoc (`///`) on every new public item (`AnthropicProvider`, its constructor, DTOs, mapping functions, `DEFAULT_MAX_TOKENS`) with `# Arguments`/`# Errors` sections per `CLAUDE.md` conventions.

## Out of scope

- Gemini provider adapter — Step 26 (parallel, same wave; disjoint files).
- Real Anthropic SSE streaming — Step 43 owns `anthropic/stream.rs`'s real implementation; this step only adds a placeholder that satisfies the trait signature.
- Vision / multimodal content-part mapping (`MessageContent::Parts`) — Step 39 owns `anthropic/request.rs`'s content-part extensions.
- Model metadata (token limits, pricing) beyond the plain `ModelInfo{id,name,provider,owned_by}` shape already used by OpenAI — Step 34.
- Full Anthropic tool-use / tool-result content-block support — `Role::Tool` gets a documented best-effort fallback mapping only, not real tool-call round-tripping.
- Go API / Web UI provider & model selection in room settings (`ai_provider`/`ai_model` columns, model dropdown) — these are `phases.md` Phase 6/7 items on the Go/Web side, not part of this gateway-only `keyScope`.
- gRPC transport for the Anthropic adapter — the REST/gRPC coexistence work is Step 19, already landed before this step in the dependency graph; no gRPC-specific work is added here.
- Retry/backoff/timeout/graceful-shutdown/request-id logic itself — already built generically in Step 8; this step only reuses it.

## Implementation notes

- Mirror the OpenAI adapter's structure exactly. Before this step, `llm-gateway/src/adapters/outbound/openai.rs` (pre-Step-3/8) has: a provider struct with `client`/`base_url`/`api_key` fields, a `new(key_store: Arc<dyn KeyStore>) -> Result<Self, DomainError>` constructor, private DTOs (`OpenAIRequest`/`OpenAIMessage`/`OpenAIResponse`/`OpenAIChoice`/`OpenAIUsage`/`OpenAIErrorResponse`/`OpenAIErrorDetail`), `role_to_string`/`string_to_role` helpers, `to_openai_request`/`from_openai_response` mapping functions, and an `impl LLMProvider for OpenAIProvider` block. By the time this step runs, Steps 3/8 will have split this into a `openai/` directory with `request.rs`/`stream.rs` submodules and changed the constructor to take `HttpClientConfig`/`ProviderConfig` instead of reading `OPENAI_BASE_URL` from the environment — **read the actual merged file layout and constructor signature before writing `anthropic/`, and match it exactly** rather than assuming the pre-Step-8 shape shown above.
- Domain types to map to/from live in `llm-gateway/src/domain/model.rs`: `Role` (`System`/`User`/`Assistant`/`Tool`), `ChatMessage { role, content }`, `CompletionRequest { model, messages, temperature, max_tokens }`, `CompletionResponse { id, model, choices, usage }`, `Choice { index, message, finish_reason }`, `Usage { prompt_tokens, completion_tokens, total_tokens }`, `ModelInfo { id, name, provider, owned_by }`. Do not modify these — Anthropic mapping is adapter-local, same as OpenAI's.
- `DomainError` lives in `llm-gateway/src/domain/error.rs`; as of pre-Step-3 it has `ProviderError(String)`, `InvalidRequest(String)`, `ModelNotFound(String)`, `KeyNotFound(String)`, `Timeout`. Step 8's notes indicate a `RateLimited` variant is expected to exist by the time this step runs (added by Step 3's reshape) — reuse it; do not add a new variant for Anthropic-specific concerns (map everything onto the existing set, per the "adding a provider only requires implementing `LLMProvider`" principle in `phases.md`).
- `LLMProvider` port lives in `llm-gateway/src/ports/outbound/provider.rs`; pre-Step-3 it has `complete`, `models`, `provider_name`. Confirm whether Step 3 added an async `models()` signature and/or a `stream`/`CompletionChunk`-returning method before implementing `AnthropicProvider` — match whatever the merged trait actually requires, do not guess.
- `KeyStore` port (`llm-gateway/src/ports/outbound/key_store.rs`) and `EnvKeyStore` (`llm-gateway/src/adapters/outbound/env_key.rs`) need no changes: `EnvKeyStore::get_key("anthropic")` already resolves `ANTHROPIC_API_KEY` via its existing `{PROVIDER}_API_KEY` uppercasing convention.
- Reuse Step 8's `llm-gateway/src/adapters/outbound/http_retry.rs` (`RetryPolicy`, `send_with_retry`) for all retry/backoff — do not write a second retry loop. Reuse Step 8's `Config`/`HttpClientConfig`/`ProviderConfig` (`llm-gateway/src/config.rs`) for timeouts and the new `ANTHROPIC_BASE_URL`-backed `ProviderConfig`.
- `main.rs` DI assembly (`llm-gateway/src/main.rs`) currently constructs `OpenAIProvider`, wraps it in `CompletionService::new(vec![Box::new(openai_provider)])`. Add `AnthropicProvider` construction right after it and push both into the same `providers` vec — this is the one-line-per-provider additive pattern the `conflictNotes` calls out, kept trivial specifically so Step 26 (Gemini, same wave) can do the analogous thing without a merge conflict.
- Anthropic Messages API reference (for the DTOs, no external docs needed): endpoint `POST {base_url}/v1/messages`; request body `{"model": "...", "max_tokens": <required int>, "messages": [{"role": "user"|"assistant", "content": "..."}], "system": "...", "temperature": <optional float>}`; success response `{"id": "...", "type": "message", "role": "assistant", "model": "...", "content": [{"type": "text", "text": "..."}], "stop_reason": "end_turn", "usage": {"input_tokens": N, "output_tokens": M}}`; error response `{"type": "error", "error": {"type": "invalid_request_error", "message": "..."}}`.
- Test double / harness reuse: `llm-gateway/tests/openai_provider_test.rs` (added by Step 14) is the canonical wiremock pattern to copy into `llm-gateway/tests/anthropic_provider_test.rs` — same `wiremock::MockServer::start().await` / `Mock::given(...).respond_with(...).mount(...)` shape, just pointed at `/v1/messages` instead of `/v1/chat/completions` and asserting Anthropic's request/response JSON shape instead of OpenAI's.
- No `schema.sql` / Atlas changes — `dbChanges` is empty; this step touches no database tables. `phases.md`'s Phase 6 `ai_provider`/`ai_model` columns on `rooms` are Go-side work explicitly out of scope here (see Out of scope).
- `docker-compose.yml`'s `llm-gateway` service block and `.env.example`'s `# === LLM Gateway (Rust) ===` section are edited additively only (append `ANTHROPIC_API_KEY` next to the existing `OPENAI_API_KEY` line) — per the plan-wide convention that these two files receive additive edits across many steps; do not reorder existing lines.
- **Conflict notes**: this is a brand-new module (`adapters/outbound/anthropic/`) with no overlapping file ownership with Step 26 (Gemini adds its own `adapters/outbound/gemini/` in the same wave). The only shared file is `main.rs`, and both steps' changes to it are one small additive DI-registration block each (trivial to merge/rebase in either order), plus both append one line each to `docker-compose.yml`/`.env.example`. `adapters/outbound/mod.rs` gets one additive `pub mod anthropic;` line (Step 26 adds its own `pub mod gemini;` line) — same low-conflict shape.
- All code, comments, doc comments, and test names in English, per `CLAUDE.md`.

## Verification

1. `cd llm-gateway && cargo build` — compiles cleanly with the new `anthropic` module.
2. `cd llm-gateway && cargo test` (equivalently `task test:gateway` from repo root) — all existing tests plus the new `anthropic/request.rs` unit tests and `tests/anthropic_provider_test.rs` integration tests pass.
3. `cd llm-gateway && cargo test --test anthropic_provider_test -- --nocapture` — shows each of the enumerated 200/request-shape/401/429/500/malformed-body/timeout cases passing individually.
4. `cd llm-gateway && cargo clippy --all-targets -- -D warnings` — clean, no new warnings (matches `Taskfile.yml`'s `lint:gateway` task).
5. `cd llm-gateway && cargo fmt --check` — new files correctly formatted.
6. Manual end-to-end smoke test against a local stub (no real Anthropic key needed): start a throwaway Python HTTP server on a spare port that returns a valid Anthropic Messages API JSON body for `POST /v1/messages`, then:
   ```
   ANTHROPIC_API_KEY=test-key ANTHROPIC_BASE_URL=http://localhost:9092 OPENAI_API_KEY=test-key cargo run &
   curl -s http://localhost:8081/models | grep -q claude-opus-4-6 && echo "anthropic models present"
   curl -si http://localhost:8081/completions -H 'Content-Type: application/json' \
     -d '{"model":"claude-opus-4-6","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"hi"}]}'
   ```
   Expect `GET /models` to include `claude-opus-4-6`/`claude-sonnet-4-6`/`claude-haiku-4-6` entries with `"provider":"anthropic"`, and the `POST /completions` call to route to `AnthropicProvider` (confirmed via the gateway's JSON log line showing `provider = "anthropic"`) and return `200` with a mapped `CompletionResponse` body.
7. `cd .. && task lint:gateway && task test:gateway` (from repo root) both succeed.

## Completion criteria

- [x] `AnthropicProvider` implements `LLMProvider` and is registered in `main.rs` alongside `OpenAIProvider` with one additive block.
- [x] System-role messages are hoisted into Anthropic's top-level `system` field and never sent inside the `messages` array.
- [x] `max_tokens` is always present in outbound Anthropic requests, defaulting to a documented constant when the domain request omits it.
- [x] Anthropic responses (`content` blocks, `stop_reason`, `usage.input_tokens`/`output_tokens`) are correctly mapped to the shared `CompletionResponse`/`Choice`/`Usage` domain types.
- [x] Anthropic error statuses (401/403, 429, 5xx/529, timeout, malformed body) map to the existing `DomainError` variants, reusing Step 8's retry/backoff helper.
- [x] `ANTHROPIC_API_KEY` resolves via the existing `KeyStore` port with no adapter-side `std::env` reads.
- [x] `llm-gateway/tests/anthropic_provider_test.rs` covers success, request-shape, 401, 429, 500, malformed-body, and timeout scenarios against a `wiremock::MockServer`.
- [x] All new public items have rustdoc comments following `CLAUDE.md` conventions.
- [x] All verification checks above pass.
