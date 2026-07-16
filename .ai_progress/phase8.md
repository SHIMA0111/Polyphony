# Phase 8: gRPC + LLM Gateway Hardening

**Goal**: Switch Go API ↔ LLM Gateway communication to gRPC, add retries and health checks.

**Delivered by**: `docs/tasks/step8.md` (Rust gateway operational hardening), `docs/tasks/step19.md` (gRPC contract:
proto definitions + Rust tonic inbound adapter), `docs/tasks/step28.md` (Go: gRPC LLM client swap + retry +
health). See those files for the full per-file implementation notes.

---

## Step 8: Rust gateway operational hardening

- [x] Consolidated `llm-gateway/src/config.rs` (`HttpClientConfig`, `ProviderConfig`, connect/request timeouts,
      retry count/base delay, all env-driven)
- [x] Shared retry helper `llm-gateway/src/adapters/outbound/http_retry.rs` (`RetryPolicy`, `send_with_retry`),
      reused by every later provider adapter (Steps 25/26)
- [x] `OpenAIProvider` rewired onto the shared timeout/retry config
- [x] Readiness endpoint (`/ready`, distinct from `/health`) and graceful-shutdown signal handling
- [x] Request-id + tracing middleware in the REST router
- [x] New env vars (`LLM_GATEWAY_CONNECT_TIMEOUT_SECS`, `LLM_GATEWAY_REQUEST_TIMEOUT_SECS`,
      `LLM_GATEWAY_MAX_RETRIES`, `LLM_GATEWAY_RETRY_BASE_DELAY_MS`) added to `docker-compose.yml`/`.env.example`

## Step 19: gRPC contract (proto + Rust tonic inbound adapter)

- [x] `server/proto/llmgateway/v1/completion.proto` — `CompletionService.Complete`, `ChatRole`/`ChatMessage`/
      `CompletionRequest`/`Choice`/`Usage`/`CompletionResponse`
- [x] `server/proto/llmgateway/v1/models.proto` — `ModelsService.ListModels`, `ModelPricing`/`ModelInfo` (including
      `context_window`/`pricing`/`supports_image_input`, per-1M-token USD pricing as the canonical unit)
- [x] `tonic`/`prost`/`tonic-health` added to `llm-gateway/Cargo.toml`; tonic gRPC server in
      `llm-gateway/src/adapters/inbound/grpc/`, mapping domain types to/from generated protobuf types without the
      domain layer importing tonic/prost
- [x] gRPC health service wired via `tonic-health`

## Step 28: Go gRPC client swap + retry + health

- [x] Go client stubs generated into `server/internal/interface/gateway/grpc/llmgatewaypb/` (`buf`/`protoc`,
      reproducible via `server/buf.gen.yaml`/`buf.yaml`)
- [x] `server/internal/interface/gateway/grpc_client.go` — `GRPCClient` implementing `ai.LLMGateway`, with retry +
      exponential backoff and a health-check call
- [x] `Config.LLMGatewayTransport` (`rest` default | `grpc`), `LLMGatewayGRPCAddr`,
      `LLMGatewayGRPCMaxRetries`/`BaseBackoff`; transport switch wired in `container.go`'s `NewContainer` — both
      implementations satisfy the same `ai.LLMGateway` interface, zero changes needed in downstream usecases
- [x] `GRPCClient.Close()` wired into the container's graceful-shutdown path
- [x] New env vars added additively to `docker-compose.yml`'s `api` service and `.env.example` (`rest` stays the
      compose default)
- [x] Unit tests for both transports; `llm-gateway/tests/grpc_test.rs` exercises the gRPC server end-to-end via a
      real in-process client

## Verification run in this worktree (Step 60)

- [x] `cd llm-gateway && cargo build --all-targets && cargo clippy --all-targets -- -D warnings && cargo test`
      (includes `grpc_test.rs`: `test_grpc_completion_models_and_health_services`)
- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [ ] Manual `LLM_GATEWAY_TRANSPORT=grpc` end-to-end smoke test against the live compose stack — skipped
      (post-merge integration review)
