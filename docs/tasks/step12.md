# Step 12: Server: MinIO object storage + attachments + presigned upload endpoints

## Meta
- **Type**: feature
- **Components**: server, infra
- **Wave**: 3 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 1: Go server foundation: DI/router split, structured logging, lint, config; Step 2: Go test infrastructure: testcontainers helper + shared mocks; Step 6: Infra tooling baseline: compose healthchecks, Taskfile, env staging, migration convention; Step 7: Go event-driven messaging core: MessageHub, atomic sequences, response linkage
- **Unlocks**: Step 39: Vision multimodal plumbing: gateway content-parts mapping + Go DTOs
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Phase 12 of `phases.md` ("Image Upload + Vision") requires object storage before AI Vision requests can reference uploaded images. This step lands only the storage/attachment half: a local S3-compatible object store (MinIO), a Go-side `ObjectStorage` port with a real adapter, and the HTTP surface (presigned upload, presigned view, attach-to-message) that lets a client upload an image directly to MinIO and link it to a message — without the API server ever touching file bytes. Step 39 will later consume `message_attachments` rows to build multimodal completion requests; this step's job is only to make attachment objects uploadable, linkable, and viewable.

## Goal
After this PR, `docker compose up` starts a `minio` service plus a one-shot bootstrap job that creates the attachments bucket. The Go API exposes three authenticated, room-membership-checked endpoints: request a presigned PUT URL for an upload, attach a previously-uploaded object to an existing message, and list a message's attachments with fresh presigned GET URLs. A new `message_attachments` table persists attachment metadata (S3 key, MIME type, size, optional message link). The S3 client is implemented with `aws-sdk-go-v2` behind a domain-level `ObjectStorage` interface so it can be swapped or mocked like `ai.LLMGateway`.

## Scope
- [ ] Add a `minio` service to `docker-compose.yml` (image `minio/minio`, `server /data --console-address ":9001"`, ports `9000` (S3 API) and `9001` (console), named volume `miniodata`, healthcheck via `curl -f http://localhost:9000/minio/health/live`), inserted alphabetically among the existing service blocks per the Step 6 convention.
- [ ] Add a one-shot `minio-init` service (image `minio/mc`) that runs `mc alias set` against `minio:9000` and `mc mb -p` to create the attachments bucket idempotently; `depends_on: minio: condition: service_healthy`.
- [ ] Add S3 env vars to `.env.example` and to the `api` service block in `docker-compose.yml`: `S3_ENDPOINT` (public/browser-reachable, default `http://localhost:9000`), `S3_REGION` (default `us-east-1`), `S3_BUCKET` (default `polyphony-attachments`), `S3_ACCESS_KEY` / `S3_SECRET_KEY` (default `minioadmin`/`minioadmin`), `S3_FORCE_PATH_STYLE` (default `true`). Reuse the same var names for `MINIO_ROOT_USER`/`MINIO_ROOT_PASSWORD` and the `minio-init` bucket name so one set of env vars configures everything.
- [ ] Extend `server/internal/infrastructure/config/config.go`'s `Config` struct and `Load()` with the S3 fields above (all optional with the defaults listed; none of DATABASE_URL/JWT_SECRET's "required" behavior applies to these).
- [ ] Add `github.com/aws/aws-sdk-go-v2`, `.../config`, `.../credentials`, and `.../service/s3` to `server/go.mod` (`go get` + `go mod tidy`).
- [ ] Add `server/internal/domain/attachment/entity.go` — `Attachment` struct (`ID`, `MessageID *string`, `S3Key`, `MimeType`, `SizeBytes int64`, `CreatedAt`) and `server/internal/domain/attachment/repository.go` — `AttachmentRepository` interface (`Create`, `GetByID`, `AttachToMessage`, `ListByMessageID`), GoDoc on every exported symbol, following the shape of `server/internal/domain/message/repository.go`.
- [ ] Add `server/internal/domain/storage/storage.go` — `ObjectStorage` interface (`PresignUpload(ctx, key, contentType string, expires time.Duration) (url string, err error)`, `PresignView(ctx, key string, expires time.Duration) (url string, err error)`). This is the swap point analogous to `ai.LLMGateway`; it must not import the AWS SDK.
- [ ] Add domain errors to `server/internal/domain/errors.go`: `ErrAttachmentAlreadyLinked`, `ErrUnsupportedMimeType`, `ErrAttachmentTooLarge`.
- [ ] Add `server/internal/interface/storage/s3_client.go` — `S3Storage` struct implementing `storage.ObjectStorage` via `aws-sdk-go-v2`'s `s3.PresignClient`, configured with static credentials (`credentials.NewStaticCredentialsProvider`) and `s3.Options{BaseEndpoint: aws.String(cfg.S3Endpoint), UsePathStyle: cfg.S3ForcePathStyle}` (do not use the deprecated custom `EndpointResolver`). `NewS3Storage(endpoint, region, bucket, accessKey, secretKey string, pathStyle bool) *S3Storage` mirrors the constructor style of `server/internal/interface/gateway/llm_client.go`.
- [ ] Add `server/internal/interface/repository/postgres/attachment_repository.go` implementing `attachment.AttachmentRepository` against `pgxpool.Pool`, following the query/scan/error patterns in `server/internal/interface/repository/postgres/message_repository.go` (wrap `pgx.ErrNoRows` as `domain.ErrNotFound`; `AttachToMessage` returns `domain.ErrAttachmentAlreadyLinked` if `message_id` is already set — enforce with a conditional `UPDATE ... WHERE id = $1 AND message_id IS NULL` and check `RowsAffected`).
- [ ] Add `server/internal/usecase/attachment/usecase.go` — `AttachmentUsecase{attachmentRepo, roomRepo, msgRepo, storage}` with:
  - `RequestUpload(ctx, userID, roomID, mimeType string, sizeBytes int64) (*UploadTicket, error)`: verify room membership (same pattern as `checkMembership` in `server/internal/usecase/message/usecase.go`), reject unsupported MIME types (allow-list: `image/png`, `image/jpeg`, `image/webp`, `image/gif`) with `ErrUnsupportedMimeType`, reject `sizeBytes` over a `const MaxAttachmentSizeBytes = 10 * 1024 * 1024` with `ErrAttachmentTooLarge`, generate a UUID-based `s3_key` (e.g. `attachments/{roomID}/{uuid}`), persist an `Attachment` row with `MessageID = nil`, call `storage.PresignUpload` with a 15-minute expiry, return `UploadTicket{AttachmentID, S3Key, UploadURL, ExpiresAt}`.
  - `AttachToMessage(ctx, userID, roomID, messageID, attachmentID string) (*attachment.Attachment, error)`: verify membership, load the message and confirm `msg.RoomID == roomID` and `msg.SenderID != nil && *msg.SenderID == userID` (only the message's own sender may attach), call `attachmentRepo.AttachToMessage`.
  - `ListAttachments(ctx, userID, roomID, messageID string) ([]AttachmentWithURL, error)`: verify membership, confirm the message belongs to the room, list attachments by `messageID`, call `storage.PresignView` per attachment with a 1-hour expiry.
- [ ] Add `server/internal/interface/handler/attachment_handler.go` with three handlers (`RequestUpload`, `Attach`, `List`) and corresponding DTOs in `server/internal/interface/handler/dto.go` (`PresignUploadRequest`, `PresignUploadResponse`, `AttachAttachmentRequest`, `AttachmentResponse`, `AttachmentListResponse`), following the error-mapping style of `handleMessageError` in `message_handler.go` (add a local `handleAttachmentError` mapping `ErrUnsupportedMimeType`/`ErrAttachmentTooLarge` → 400, `ErrForbidden` → 403, `ErrNotFound` → 404, `ErrAttachmentAlreadyLinked` → 409).
- [ ] Register the three routes through the Step 1 `internal/app` registrar pattern: add a new `server/internal/app/routes_attachment.go` file exporting a `registerAttachmentRoutes(e *echo.Echo, c *Container)` function (mirroring `routes_room.go`/`routes_message.go`) that registers `POST /rooms/:roomId/attachments/upload-url`, `POST /rooms/:roomId/messages/:messageId/attachments`, `GET /rooms/:roomId/messages/:messageId/attachments` on the authenticated group, and call it from `router.go`'s `NewRouter`. Wire `S3Storage`, `AttachmentRepository`, `AttachmentUsecase`, `AttachmentHandler` as new fields/construction lines in `server/internal/app/container.go`'s `NewContainer` alongside the existing repos/usecases/handlers (do not touch the shrunk `server/cmd/api/main.go`).
- [ ] Add `message_attachments` table to `server/schema.sql` (additive-only edit; do not touch the `messages` table definition):
  ```sql
  CREATE TABLE message_attachments (
      id UUID PRIMARY KEY,
      message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
      s3_key VARCHAR(512) NOT NULL,
      mime_type VARCHAR(100) NOT NULL,
      size_bytes BIGINT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );

  CREATE INDEX idx_message_attachments_message_id ON message_attachments(message_id);
  ```
  `message_id` is nullable because an attachment row is created at upload-request time, before it is linked to a message.
- [ ] Run `task migrate:generate -- add_message_attachments` to generate the Atlas migration file and update `atlas.sum` under `server/migrations/`.
- [ ] Unit tests: `server/internal/usecase/attachment/usecase_test.go` reusing Step 2's shared mocks (`server/internal/testutil/mocks`) for `RoomRepository`/`MessageRepository`, plus new mocks for `AttachmentRepository` and `storage.ObjectStorage` (a fake returning canned URLs — add them to the shared `mocks` package so later steps can reuse them), covering: happy-path upload ticket, unsupported MIME type, oversized upload, attach-by-non-sender rejection, attach-already-linked rejection, list-with-view-urls.
- [ ] Unit tests: `server/internal/interface/handler/attachment_handler_test.go` wiring the real `AttachmentUsecase` against the shared in-memory mocks from `server/internal/testutil/mocks` (same pattern the post-Step-2 handler tests use), covering request/response JSON shapes and HTTP status codes for all three endpoints.
- [ ] Integration test: `server/internal/interface/repository/postgres/attachment_repository_test.go` using the shared PostgreSQL testcontainers helper introduced in Step 2 (locate it under `server/internal` — reuse its container-bootstrap and schema-apply helpers rather than writing new ones), covering `Create`, `GetByID`, `AttachToMessage` (including the already-linked conflict path), and `ListByMessageID`.
- [ ] Update `.ai_progress/phase12.md` (create if absent) with a checklist mirroring this scope.

## Out of scope
- Vision/multimodal completion request mapping and gateway content-parts DTOs (Step 39).
- Any non-image MIME types (video, PDF, etc.) — explicitly excluded in `phases.md` Phase 12.
- CloudFront/CDN in front of object storage — excluded until Phase 21 (AWS), which is out of the agreed project scope entirely.
- Background garbage collection of orphaned S3 objects (e.g. upload tickets that are never attached, or attachments whose message was deleted) — not addressed by this step; `ON DELETE CASCADE` only cleans up the metadata row, not the MinIO object.
- Web frontend upload UI / image preview — a separate frontend step consumes these endpoints later.
- RBAC role enforcement beyond plain room membership — the typed 5-tier `room.Role` middleware lands in Step 13; this step reuses the existing membership check only.
- Rate limiting of upload requests — lands with Redis-backed rate limiting in a later wave.

## Implementation notes
- **Why the presigned URL host matters**: the Go API container and a browser/`curl` on the host resolve `minio` (the Docker Compose service name) differently — the API container can reach `http://minio:9000`, but a browser on the host cannot. Configure `S3_ENDPOINT` to the **host-reachable** URL (`http://localhost:9000` by default) and use that same client for presigning. This works because `PresignPutObject`/`PresignGetObject` only construct a signed URL string — they do not require the presigning caller (the API server) to actually connect to the endpoint. The API server never calls `PutObject`/`GetObject` directly in this design; all object bytes flow directly between the client and MinIO.
- **AWS SDK v2 endpoint configuration**: use the modern `s3.Options{BaseEndpoint: ..., UsePathStyle: ...}` functional option on `s3.NewFromConfig`, not the deprecated `EndpointResolverWithOptions`. MinIO requires `UsePathStyle: true` since it does not support virtual-hosted-style bucket addressing by default.
- Existing patterns to reuse: `server/internal/interface/gateway/llm_client.go` for the adapter-behind-domain-interface shape and constructor style; `server/internal/usecase/message/usecase.go`'s `checkMembership` for the membership-check pattern (verify via `roomRepo.GetMember(ctx, roomID, userID)`, mapping absence to `domain.ErrForbidden`); `server/internal/interface/repository/postgres/message_repository.go` for scan/error-wrapping conventions; `server/internal/interface/handler/message_handler.go` + `dto.go` for handler/DTO/error-mapping conventions; `server/internal/interface/handler/mock_test.go` for the shared in-memory mock style used in handler tests.
- `docker-compose.yml`/`.env.example` edits are additive only — do not reorder or modify the existing `db`, `migrate`, `api`, `llm-gateway`, `web` blocks beyond inserting the two new ones and adding S3 env vars to `api`.
- **Conflict note**: this step depends on Step 7 purely for **schema.sql merge order** — Step 7 also touches the `messages` table (response linkage columns), while this step only adds the new, disjoint `message_attachments` table with a `messages(id)` foreign key. Rebase on top of Step 7's `messages` table state before generating the migration; do not otherwise modify the `messages` table definition.
- Response shapes:
  - `POST /rooms/:roomId/attachments/upload-url` — request `{"mime_type": "image/png", "size_bytes": 123456}` → `201 {"attachment_id": "...", "s3_key": "...", "upload_url": "https://...", "expires_at": "2026-...Z"}`.
  - `POST /rooms/:roomId/messages/:messageId/attachments` — request `{"attachment_id": "..."}` → `200 {"id": "...", "message_id": "...", "s3_key": "...", "mime_type": "...", "size_bytes": ..., "created_at": "..."}`.
  - `GET /rooms/:roomId/messages/:messageId/attachments` → `200 {"attachments": [{"id": "...", ..., "view_url": "https://...", "created_at": "..."}]}`.
- Keep GoDoc comments on every new exported Go symbol per `CLAUDE.md` conventions; use `slog` for any logging added (none is strictly required for this step's happy paths, but log presign/storage errors before wrapping them).

## Verification
1. `cd server && go build ./... && go vet ./...` — builds and vets cleanly.
2. `task test:server` — all unit tests pass, including the new `usecase/attachment` and `handler` attachment tests and the testcontainers-backed `attachment_repository_test.go`.
3. `task migrate:generate -- add_message_attachments` followed by `task migrate:status` (from `server/`) — shows the new migration applied cleanly against a fresh schema.
4. `docker compose up -d db migrate minio minio-init api llm-gateway` then `docker compose ps` — `minio` shows healthy, `minio-init` exits 0, `api` is running.
5. Register a user, log in, and create a room via the existing `/auth/register`, `/auth/login`, `POST /rooms` endpoints to obtain `$TOKEN` and `$ROOM_ID`.
6. `curl -s -X POST http://localhost:8080/rooms/$ROOM_ID/attachments/upload-url -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"mime_type":"image/png","size_bytes":1024}'` — returns 201 with an `upload_url` whose host is `localhost:9000`.
7. `curl -s -X PUT "$UPLOAD_URL" -H "Content-Type: image/png" --data-binary @some-test-image.png -w '%{http_code}'` — returns `200`.
8. `curl -s -X POST http://localhost:8080/rooms/$ROOM_ID/messages -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"content":"here is an image"}'` to get `$MESSAGE_ID`, then `curl -s -X POST http://localhost:8080/rooms/$ROOM_ID/messages/$MESSAGE_ID/attachments -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{\"attachment_id\":\"$ATTACHMENT_ID\"}"` — returns 200 with `message_id` set.
9. `curl -s http://localhost:8080/rooms/$ROOM_ID/messages/$MESSAGE_ID/attachments -H "Authorization: Bearer $TOKEN"` — returns the attachment with a fresh `view_url`; `curl -s "$VIEW_URL" -o downloaded.png && diff downloaded.png some-test-image.png` — files are identical.
10. Attempt `POST /rooms/$ROOM_ID/attachments/upload-url` with `{"mime_type":"application/pdf","size_bytes":100}` — returns 400; with `{"mime_type":"image/png","size_bytes":99999999999}` — returns 400.

## Completion criteria
- [ ] `minio` and `minio-init` services run in `docker compose up` and the bucket is created automatically.
- [ ] `Config`, `ObjectStorage` port, `S3Storage` adapter, `AttachmentRepository`, `AttachmentUsecase`, `AttachmentHandler`, and the three routes exist and are wired in `main.go`.
- [ ] `message_attachments` table exists via an Atlas migration and only adds new schema (no edits to `messages`).
- [ ] All new exported Go symbols have GoDoc comments.
- [ ] All items in Scope are checked off.
- [ ] All verification checks above pass.
