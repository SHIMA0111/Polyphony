# Phase 2: WebSocket + Real-time

**Goal** (`phases.md` Phase 2): Messages are delivered to all members in real-time. Delivered across three steps:
Step 7 (Go event-driven messaging core, the `MessageHub` port + atomic sequencing this phase's realtime delivery
is built on), Step 15 (the actual WebSocket endpoint), and Step 35 (the web WebSocket client — reconciled into this
file at Step 60; it had not previously been tracked here).

---

## Step 7: Go event-driven messaging core: MessageHub, atomic sequences, response linkage

- [x] `server/internal/domain/event/hub.go` — `EventType`, `RoomEvent`, `MessageHub` port, fully GoDoc'd
- [x] `server/internal/domain/event/inprocess_hub.go` — `InProcessHub` adapter (mutex-guarded registry, non-blocking
      buffered `Publish`, safe-to-call-once `unsubscribe`), fully GoDoc'd
- [x] `server/internal/domain/event/inprocess_hub_test.go` — unit tests: no-subscriber publish doesn't block/panic,
      subscriber receives event, `TargetUserIDs` filtering, unsubscribe stops delivery and is safe to call more than
      once, publish never blocks on a full subscriber channel
- [x] `domain/message.MessageRepository.ReserveSequenceRange(ctx, roomID, count) (int64, error)` replaces
      `GetNextSequence`; implemented in `postgres.MessageRepository` as a single
      `UPDATE room_sequences ... RETURNING next_sequence - $2` statement
- [x] `domain/message.Message.InResponseToMessageID *string` added; `postgres.MessageRepository` persists/scans
      `in_response_to_message_id`
- [x] `MessageUsecase` refactored:
  - [x] `NewMessageUsecase` takes a `hub event.MessageHub` (4th argument)
  - [x] `SendMessage` reserves 1 sequence via `ReserveSequenceRange` and publishes `EventMessageCreated` after
        `Create` succeeds
  - [x] `SendAIMessage` reserves 2 sequences atomically up front (`ReserveSequenceRange(ctx, roomID, 2)`), persists
        the human message via a shared `createHumanMessage` helper, sets `AIMessage.InResponseToMessageID` to the
        human message's ID, and publishes `EventMessageCreated` for both messages after their respective `Create`
        calls succeed (including the failed-placeholder path)
  - [x] `RegenerateAIMessage` publishes `EventMessageUpdated` after `UpdateAIResponse` succeeds
  - [x] GoDoc documents fire-and-forget broadcast semantics on the type and each method
- [x] `schema.sql`: `messages.in_response_to_message_id` (nullable, FK to `messages.id`, `ON DELETE SET NULL`) +
      `CONSTRAINT messages_room_sequence_unique UNIQUE (room_id, sequence)`
- [x] Atlas migration generated via `task migrate:generate -- add_message_response_link_and_sequence_unique`
      (`server/migrations/20260715164341_add_message_response_link_and_sequence_unique.sql`), `atlas.sum` regenerated
      by `atlas migrate diff`. `atlas migrate lint` requires Atlas Pro login (unavailable in this environment); the
      migration was hand-reviewed instead — it only adds a nullable column, a self-referencing FK, and a unique
      constraint, all additive/non-destructive.
- [x] `handler.MessageResponse` + `toMessageResponse` include `in_response_to_message_id`
- [x] DI: `server/internal/app/container.go` constructs `event.NewInProcessHub()` and wires it into
      `msgusecase.NewMessageUsecase`; exposed as `Container.MessageHub` for later steps (e.g. Step 15's WebSocket
      endpoint) to `Subscribe` on the same instance
- [x] `testutil/mocks.MessageRepo` updated: `GetNextSequence` replaced with `ReserveSequenceRange`; every
      `NewMessageUsecase(...)` call site in `usecase/message/usecase_test.go` and
      `interface/handler/message_handler_test.go` updated to pass `event.NewInProcessHub()`
- [x] `usecase/message/usecase_test.go`: new test asserting `SendAIMessage` produces adjacent sequences
      (`aiMsg.Sequence == humanMsg.Sequence + 1`) and `AIMessage.InResponseToMessageID == &humanMsg.ID`
- [x] `interface/repository/postgres/sequence_concurrency_integration_test.go` (build tag `integration`) rewritten
      as `TestReserveSequenceRangeConcurrency`: mixed-count (1 and 2) concurrent reservations against the same room,
      asserting zero duplicate/overlapping sequence numbers and an exact contiguous total
- [x] `interface/repository/postgres/message_repository_integration_test.go` (new, build tag `integration`):
      `TestMessages_UniqueRoomSequence` (duplicate `(room_id, sequence)` insert rejected with a unique-violation
      error) and `TestMessageRepository_InResponseToMessageIDRoundTrip` (nil for human, set for AI referencing the
      human message)
- [x] `.ai_progress/phase2.md` created (this file)

### Verification run this step

- [x] `cd server && go build ./...`
- [x] `cd server && go vet ./...`
- [x] `cd server && go test ./...` (Docker-free; `internal/interface/repository/postgres` has no non-integration
      test files, confirming the new tests are correctly gated behind `-tags=integration`)
- [ ] `docker compose up -d db && task migrate:apply` — requires the fixed-port compose stack; skipped per this
      run's constraints (post-merge integration review). Migration application was indirectly validated via
      `testutil/postgres.New`, which applies every file under `server/migrations/` (including the new one) against
      a fresh testcontainers PostgreSQL instance as part of every integration test run in this step.
- [x] `cd server && go test -tags=integration ./internal/interface/repository/postgres/... -run TestReserveSequenceRange -v`
- [x] `cd server && go test -tags=integration ./internal/interface/repository/postgres/... -run TestMessages_UniqueRoomSequence -v`
- [ ] Manual smoke check via `task up` + live HTTP calls — requires the compose stack; skipped per this run's
      constraints (post-merge integration review).

## Step 15: Server WebSocket endpoint + realtime delivery

- [x] `github.com/coder/websocket` added as a server dependency (`go get` + `go mod tidy`)
- [x] `server/internal/interface/wsticket/ticket.go` — `Issuer` type (`NewIssuer`, `Issue`, `Validate`): mints and
      validates short-lived HS256 JWT tickets independent of `domain/auth.AuthService`, fully GoDoc'd (including why
      it does not depend on `AuthService` and survives the Phase 9 Kratos swap unchanged)
- [x] `server/internal/interface/wsticket/ticket_test.go` — round-trip issue→validate, expired-ticket rejection,
      tampered-signature rejection, wrong-secret rejection
- [x] `server/internal/interface/handler/websocket_handler.go` — `WebSocketHandler` (`roomUsecase`, `hub`,
      `ticketIssuer`, `originPatterns`), `NewWebSocketHandler`, `IssueTicket` (`POST /ws/ticket`), `Handle`
      (`GET /rooms/:roomId/ws`, public route, manual ticket + membership auth, upgrades via `coder/websocket`,
      subscribes to `event.MessageHub`, forwards `RoomEvent`s as `wsEventFrame` JSON via `wsjson.Write` until the
      client disconnects or the channel closes), `wsEventFrame` wire type, fully GoDoc'd
- [x] `server/internal/interface/handler/websocket_handler_test.go` — `TestIssueTicket200`,
      `TestHandleWS_MissingTicket401`, `TestHandleWS_InvalidTicket401`, `TestHandleWS_NonMember403`,
      `TestHandleWS_RoomNotFound404`, and the integration-style `TestHandleWS_TwoClientsBroadcastAndTargeted`
      (real `httptest.NewServer`, two real `coder/websocket` clients, proves both broadcast and per-user-targeted
      delivery via a shared `event.NewInProcessHub()`)
- [x] `Config.WSTicketSecret` added (`WS_TICKET_SECRET` env var, defaults to `JWTSecret`), with
      `config_test.go` cases for both the default and the override
- [x] DI wiring in `server/internal/app/container.go`: `wsticket.NewIssuer(...)` constructed with a 60s TTL, reusing
      the same `messageHub` instance already passed into `msgusecase.NewMessageUsecase`; `handler.NewWebSocketHandler`
      wired with CORS-origin-derived `OriginPatterns` (`originPatternsFromCORS`); exposed as `Container.WebSocketHandler`
- [x] Route registration in `server/internal/app/routes_websocket.go` (new registrar, called from `NewRouter`):
      `POST /ws/ticket` on the existing authenticated group, `GET /rooms/:roomId/ws` as a public route on `e`
- [x] `.env.example` — optional, commented `WS_TICKET_SECRET` entry documented under "Go API Server"
- [x] `docker-compose.yml` — additive `WS_TICKET_SECRET` env line added to the `api` service (E2E profile blocks
      left untouched)
- [x] `.ai_progress/phase2.md` updated (this section)

### Verification run this step

- [x] `cd server && go build ./...`
- [x] `cd server && go vet ./... && gofmt -l .` (gofmt reports only the pre-existing, unrelated
      `internal/interface/gateway/llm_client.go`, untouched by this step)
- [x] `cd server && go test ./internal/interface/wsticket/... -v`
- [x] `cd server && go test ./internal/interface/handler/... -run TestIssueTicket -v` and `-run TestHandleWS -v`
- [x] `cd server && go test ./internal/interface/handler/... -run TestHandleWS -race -v`
- [x] `cd server && go test ./...` (full suite, no regressions)
- [ ] Manual end-to-end smoke check (`task up` + `curl`/`websocat` against `localhost:8080`) — requires the
      fixed-port compose stack; skipped per this run's constraints (post-merge integration review).

## Step 35: Web WebSocket client + live cache merge

- [x] `web/src/features/messages/hooks/use-room-socket.ts` — `useRoomSocket(roomId)`: connects to `GET
      /rooms/:roomId/ws` using a freshly-issued ticket (`POST /ws/ticket`), reconnects with backoff on abnormal
      close, no-ops in `NEXT_PUBLIC_MOCK_API=true` mode
- [x] `web/src/features/messages/lib/merge-message-event.ts` — merges incoming `message_created`/`message_updated`
      events into the TanStack Query message cache (create/update/dedupe/out-of-order-arrival handling)
- [x] `web/src/features/messages/components/ConnectionStatus.tsx` — connected/reconnecting/offline indicator
      (Chakra `Badge` + the existing shared `Tooltip` snippet, no new snippet added)
- [x] `ChatRoom.tsx`: one-line hook mount + one-line indicator render in the existing header, no other changes to
      send/regenerate logic
- [x] Unit tests: `use-room-socket.test.ts` (hand-rolled fake `WebSocket`, connect/merge/reconnect-backoff/mock-mode
      no-op cases), `merge-message-event.test.ts` (create/update/duplicate/out-of-order cases)

### Verification run this step (Step 60)

- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run` (includes
      `use-room-socket.test.ts` and `merge-message-event.test.ts`)
- [ ] Live WebSocket connection against the compose stack (`task test:e2e:up` + a real browser) — requires the
      full E2E stack; skipped (post-merge integration review)
