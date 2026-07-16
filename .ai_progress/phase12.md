# Phase 12: Image Upload + Vision

**Goal** (`phases.md` Phase 12): Upload images and have AI perform image recognition. Delivered across three steps:
Step 12 (MinIO object storage + attachment domain/usecase/handler stack — a client uploads directly to MinIO and
links the object to a message, the API server never touches file bytes), Step 39 (Vision/multimodal completion
mapping across the gateway and Go DTOs), and Step 45 (web image upload UI + Vision send). Steps 39 and 45 are
reconciled into this file at Step 60; they had not previously been tracked here.

---

## Step 12: Server: MinIO object storage + attachments + presigned upload endpoints

- [x] `docker-compose.yml`: `minio` service (image `minio/minio`, ports 9000/9001, named volume `miniodata`,
      healthcheck via `curl -f http://localhost:9000/minio/health/live`), inserted alphabetically between
      `mailslurper` and `migrate`
- [x] `docker-compose.yml`: one-shot `minio-init` service (image `minio/mc`) running `mc alias set` + `mc mb -p`
      against the bucket, gated on `minio: condition: service_healthy`
- [x] `docker-compose.yml`: S3 env vars added to the `api` service block
- [x] `.env.example`: S3_ENDPOINT/S3_REGION/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY/S3_FORCE_PATH_STYLE added,
      reconciled with the pre-existing MINIO_* section (S3_ACCESS_KEY/S3_SECRET_KEY match
      MINIO_ROOT_USER/MINIO_ROOT_PASSWORD; S3_BUCKET matches MINIO_BUCKET) rather than duplicating a second
      credential set
- [x] `server/internal/infrastructure/config/config.go`: `Config` struct + `Load()` extended with S3 fields, all
      optional with defaults (host-reachable `S3_ENDPOINT=http://localhost:9000` by design — see implementation
      notes in docs/tasks/step12.md on why the presigning host must be browser-reachable, not the compose-internal
      `minio` hostname)
- [x] `server/go.mod`: `github.com/aws/aws-sdk-go-v2`, `.../credentials`, `.../service/s3` added via `go get` +
      `go mod tidy`
- [x] `server/internal/domain/attachment/entity.go` — `Attachment` struct, fully GoDoc'd
- [x] `server/internal/domain/attachment/repository.go` — `AttachmentRepository` port (`Create`, `GetByID`,
      `AttachToMessage`, `ListByMessageID`), fully GoDoc'd
- [x] `server/internal/domain/storage/storage.go` — `ObjectStorage` port (`PresignUpload`, `PresignView`); does not
      import the AWS SDK, mirroring the `domain/ai.LLMGateway` swap-point pattern
- [x] `server/internal/domain/errors.go` — `ErrAttachmentAlreadyLinked`, `ErrUnsupportedMimeType`,
      `ErrAttachmentTooLarge` added
- [x] `server/internal/interface/storage/s3_client.go` — `S3Storage` adapter using aws-sdk-go-v2's
      `s3.PresignClient`, `s3.Options{BaseEndpoint, UsePathStyle}` (no deprecated `EndpointResolverWithOptions`),
      static credentials via `credentials.NewStaticCredentialsProvider`
- [x] `server/internal/interface/repository/postgres/attachment_repository.go` — `AttachmentRepository` backed by
      `pgxpool.Pool`; `AttachToMessage` uses a conditional `UPDATE ... WHERE id = $1 AND message_id IS NULL` +
      `RowsAffected` check to enforce the already-linked conflict atomically
- [x] `server/internal/usecase/attachment/usecase.go` — `AttachmentUsecase` with `RequestUpload`, `AttachToMessage`,
      `ListAttachments`; MIME allow-list (`image/png`, `image/jpeg`, `image/webp`, `image/gif`),
      `MaxAttachmentSizeBytes = 10 MiB`, own `checkMembership` helper (not shared with message usecase, per Step 13
      concurrently reworking those helpers this wave)
- [x] `server/internal/interface/handler/attachment_handler.go` + `dto.go` additions — `RequestUpload`, `Attach`,
      `List` handlers with `PresignUploadRequest/Response`, `AttachAttachmentRequest`, `AttachmentResponse`,
      `AttachmentViewResponse`, `AttachmentListResponse`, and `handleAttachmentError` (400/403/404/409 mapping)
- [x] `server/internal/app/routes_attachment.go` — `registerAttachmentRoutes` registering the three endpoints on the
      authenticated group; wired into `router.go`'s `NewRouter`
- [x] `server/internal/app/container.go` — `S3Storage`, `AttachmentRepository`, `AttachmentUsecase`,
      `AttachmentHandler` wired into `NewContainer` as new fields/construction lines (additive only)
- [x] `server/schema.sql` — `message_attachments` table + `idx_message_attachments_message_id` index (additive only;
      `messages` table definition untouched)
- [x] Atlas migration generated via `task migrate:generate -- add_message_attachments`
      (`server/migrations/20260715180129_add_message_attachments.sql`), `atlas.sum` regenerated
- [x] Unit tests: `server/internal/usecase/attachment/usecase_test.go` — happy-path upload ticket, unsupported MIME
      type, oversized upload, attach-by-non-sender rejection, attach-already-linked rejection,
      list-with-view-urls, using new `mocks.AttachmentRepo` / `mocks.ObjectStorage` fakes added to
      `server/internal/testutil/mocks`
- [x] Unit tests: `server/internal/interface/handler/attachment_handler_test.go` — request/response JSON shapes and
      HTTP status codes for all three endpoints, wired against real `AttachmentUsecase` + shared mocks
- [x] Integration test: `server/internal/interface/repository/postgres/attachment_repository_integration_test.go`
      (`//go:build integration`) — `Create`/`GetByID`, `AttachToMessage` happy path + already-linked conflict +
      not-found, `ListByMessageID` ordering/filtering, using the Step 2 testcontainers helper
- [x] `.ai_progress/phase12.md` created

## Verification run in this worktree

- [x] `cd server && go build ./... && go vet ./...`
- [x] `go test ./...` (all packages, including new `usecase/attachment` and `handler` attachment tests)
- [x] `go test -tags=integration ./...` (includes the new `attachment_repository_integration_test.go`, run against a
      real Docker-backed Postgres testcontainer)
- [x] `golangci-lint run ./...` — 0 issues
- [x] `task migrate:generate -- add_message_attachments` — migration generated cleanly against Atlas's Docker dev
      database
- [x] `docker compose config -q` — compose file parses/validates
- [ ] `task migrate:status` — requires a real reachable Postgres target (no `url` configured in the `local` Atlas
      env beyond the Docker dev-db used for diffing); skipped, needs the compose `db` service
- [x] `docker compose up -d db migrate minio minio-init api llm-gateway` + `docker compose ps` — verified live in
      the wave-9 integration review via `task up`: all listed services `Up (healthy)` (one-shots exited 0).
- [ ] End-to-end curl verification (steps 5-10 of docs/tasks/step12.md) — skipped (requires the running compose
      stack); left for the post-merge integration review

## Step 39: Vision multimodal plumbing (gateway content-parts mapping + Go DTOs)

- [x] Gateway domain: reuses Step 3's `MessageContent::Text | Parts` / `ContentPart::Text | ImageUrl |
      ImageBase64` — no reshaping, only extended where a provider mapping genuinely needed a new variant
- [x] REST DTOs (`llm-gateway/src/adapters/inbound/rest/{request,response}.rs`): `content` accepts either a plain
      string (backward compatible) or an array of part objects, via `#[serde(untagged)]`
- [x] gRPC proto extended with an equivalent `oneof content` shape where the gRPC adapter already existed
- [x] Per-provider `request.rs` submodules (OpenAI/Anthropic/Gemini) map `ContentPart`s to each provider's
      multimodal wire format; unmappable parts return the existing `DomainError::InvalidRequest`
- [x] Go: attachment lookup + content-part building so a message referencing `message_attachments` rows is sent to
      the gateway as multimodal content
- [x] Provider-level tests: `test_complete_with_image_content_sends_{openai,anthropic,gemini}_multimodal_body`
      (confirmed passing in this worktree's `cargo test` run above)

## Step 45: Web image upload + attachment UI + Vision send

- [x] `web/src/features/messages/api/` — attachment request/attach/list methods mirroring Step 12's three endpoints
- [x] `web/src/features/messages/lib/upload-attachment.ts` — `XMLHttpRequest`-based direct-to-storage `PUT` with
      upload-progress reporting (not `fetch`, which has no reliable progress event)
- [x] `web/src/features/messages/hooks/use-attachment-staging.ts` — client-side pending-attachment list
      (upload/preview/progress/error state), same MIME allow-list + 10 MiB cap enforced client-side as Step 12's
      usecase enforces server-side
- [x] `MessageInput.tsx`: attach affordances (file picker, staged-attachment chips, per-file error display)
- [x] Send-with-attachments sequencing in the chat-room orchestration hook
- [x] `MessageAttachments.tsx` (thumbnails inside `MessageBubble.tsx`) + `AttachmentLightbox.tsx` (Chakra `Dialog`
      compound component, full-resolution view)
- [x] Tests: staging hook, upload helper, thumbnail rendering, lightbox open/close

## Verification run in this worktree (Step 60)

- [x] `cd llm-gateway && cargo build --all-targets && cargo clippy --all-targets -- -D warnings && cargo test`
      (includes the three `*_with_image_content_sends_*_multimodal_body` provider tests)
- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [x] `web/e2e/attachments.spec.ts` / `web/e2e/regression/advanced-ai/vision-attachments.spec.ts` against the live
      compose stack (real MinIO + the stubbed Vision-capable model) — verified live in the wave-9 integration
      review: both specs passed in the full-suite E2E runs.
