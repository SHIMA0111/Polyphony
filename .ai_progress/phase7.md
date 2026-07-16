# Phase 7: Multi-provider (Gemini) + Model Selection UI

**Goal**: 3-provider support, intuitive model selection UI.

**Delivered by**: `docs/tasks/step26.md` (Rust: Gemini provider adapter), `docs/tasks/step34.md` (Model metadata —
token limits + pricing — across gateway/server/web). See those files for the full per-file implementation notes.

---

## Step 26: Gemini provider adapter (Rust)

- [x] `llm-gateway/src/adapters/outbound/gemini/` module (`mod.rs` + `request.rs` + `stream.rs`, mirroring the
      OpenAI/Anthropic submodule split); `stream()` is a non-streaming placeholder pending Step 43
- [x] `Config.gemini: ProviderConfig` (`GEMINI_BASE_URL`, default `https://generativelanguage.googleapis.com`)
- [x] DI: `GeminiProvider` registered alongside OpenAI/Anthropic in `main.rs`'s `CompletionService` provider list
- [x] `models()` returns `gemini-3-pro`, `gemini-3-flash`, `gemini-2.5-flash` (all `gemini-`-prefixed, no ID
      collisions with `gpt-`/`claude-` IDs — asserted by a model-registry uniqueness test)
- [x] `GEMINI_API_KEY` wired into `.env.example` and `docker-compose.yml`
- [x] Unit tests + wiremock integration test (`llm-gateway/tests/gemini_provider_test.rs`); rustdoc on every public
      item

## Step 34: Model metadata (token limits + pricing) across the stack

- [x] Canonical shape (per-1M-token USD pricing): `ModelInfo.context_window: Option<u32>`, `pricing:
      Option<ModelPricing>`, `supports_image_input: Option<bool>` in `llm-gateway/src/domain/model.rs`; populated
      with real published figures for every OpenAI/Anthropic/Gemini model
- [x] REST DTO (`ModelInfoDto`) and gRPC proto (`models.proto`) carry the same fields; `convert.rs` kept in sync
- [x] Go domain: `ai.ModelInfo` extended with `ContextWindow`, `InputPricePerMillionTokens`,
      `OutputPricePerMillionTokens`, `SupportsImageInput` (absent/`null` → zero value)
- [x] Go REST + gRPC gateway clients both decode/flatten the new wire fields identically
- [x] `ModelResponse`/`ModelListResponse` DTOs + `ModelHandler.List` passthrough
- [x] Web: `ModelInfo` type extended (`web/src/features/messages/types.ts`); `useModels()` query hook replaces the
      ad-hoc `apiClient.listModels()` call
- [x] Web: `ModelSelector.tsx` rewritten to consume `useModels()`, grouped by provider, rendering context window,
      per-1M pricing, and an image-capability indicator
- [x] Tests: gateway unit tests (non-zero fixture values propagate through `list_models()`), Go handler test for the
      new JSON fields, MSW-backed `ModelSelector` component tests (grouping, pricing rendering)

## Verification run in this worktree (Step 60)

- [x] `cd llm-gateway && cargo build --all-targets && cargo clippy --all-targets -- -D warnings && cargo test`
- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run` (199/199 unit/component tests
      pass, including `ModelSelector`)
- [ ] Manual verification against real Gemini credentials / live compose stack — skipped (post-merge integration
      review)
