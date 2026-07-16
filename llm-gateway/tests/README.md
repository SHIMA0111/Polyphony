# `llm-gateway` integration test harness

This directory holds network-free, deterministic integration tests, split into test
binaries by what they exercise:

- **`router_test.rs`** — drives the fully assembled `axum::Router` (from
  `adapters::inbound::rest::router::build_router`) through
  `tower::util::ServiceExt::oneshot` against a `StubUseCase` test double that
  implements `CompletionUseCase` directly. No bound TCP port is used; requests and
  responses are built and inspected in-process. Use this file's pattern to add cases
  for new routes, new `DomainError` → HTTP status mappings, or new request/response DTO
  shapes.
- **`grpc_test.rs`** — drives the gRPC inbound adapter
  (`adapters::inbound::grpc::serve_grpc`) end-to-end: a real `tonic` server, backed by
  the same kind of `StubUseCase` test double as `router_test.rs`, is spun up on a fixed
  high test port and exercised via a connected `tonic::transport::Channel`, covering
  `CompletionService`, `ModelsService`, and the standard `grpc.health.v1.Health`
  service. All assertions run inside a single `#[tokio::test]` sharing one server
  instance, so the fixed port is never raced by cargo's parallel test execution — keep
  new gRPC-adapter cases in that same function rather than adding a second server on a
  second port. Use this file's pattern for new gRPC methods/services.
- **`openai_provider_test.rs`**, **`anthropic_provider_test.rs`**,
  **`gemini_provider_test.rs`** — each drives its respective outbound provider
  (`OpenAIProvider` / `AnthropicProvider` / `GeminiProvider`) against a
  `wiremock::MockServer` standing in for that provider's real API host, with the
  provider constructed via explicit config injection (`HttpClientConfig` /
  `ProviderConfig` pointed at the mock server's URI). No real network call ever leaves
  the test process. `gemini_provider_test.rs` additionally exercises `GeminiProvider`
  wrapping an inner `OpenAIProvider` where relevant (Gemini's OpenAI-compatible
  endpoint), so it imports `OpenAIProvider` too. Use these files' shared
  construction/assertion style — or a sibling file following the same pattern — for any
  new outbound provider adapter.

## Conventions every file follows

- **No process environment mutation.** Every test constructs its dependencies (router
  state, provider config, key store) via plain function/constructor arguments. Do not
  add `std::env::set_var`/`remove_var` calls to new tests here — that pattern does not
  scale once tests across multiple providers run concurrently. (The *existing*
  `#[cfg(test)]` unit tests in `env_key.rs` and elsewhere are a separate, older pattern
  and are intentionally left as-is.)
- **`max_retries: 0` in test `HttpClientConfig`s**, unless a test is specifically
  exercising retry behavior (which belongs in `adapters/outbound/http_retry.rs`'s own
  unit tests, not here). This keeps `429`/`5xx` response tests deterministic: wiremock
  receives exactly one request instead of `1 + max_retries`.
- **Wide timing margins for timeout tests.** When asserting a `DomainError::Timeout`
  mapping, configure a short client timeout and a `wiremock` response delay that is
  several times longer, rather than tightening the margin — this avoids flakiness from
  scheduler jitter in CI.

## Extending this harness

Steps that add new provider HTTP behavior or a streaming endpoint should add new
`#[tokio::test]` functions to the relevant existing file (or, for a brand-new provider, a
new `tests/<provider>_provider_test.rs` file mirroring `openai_provider_test.rs`'s
structure) instead of introducing a different mocking library or testing approach. New
gRPC methods or services follow the same rule against `grpc_test.rs`.
