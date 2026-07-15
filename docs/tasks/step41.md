# Step 41: Server: private AI mode

## Meta
- **Type**: feature
- **Components**: server
- **Wave**: 5 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 15: Server WebSocket endpoint + realtime delivery, Step 23: Server: AI context control (soft delete, exclude flag, cutoff, context builder)
- **Unlocks**: Step 47: Web: private AI mode UI, Step 50: Server: context summarization with cached summaries
- **Size**: 1 PR (a few hours for one AI agent)

## Context

`phases.md` Phase 14 ("Private AI Mode") specifies that a user must be able to ask the AI a question inside a shared room without other members seeing the exchange — the human question and the AI answer should be visible only to the requester, delivered over WebSocket only to that user's connections. Today (`server/internal/domain/message/entity.go`) every message is implicitly room-wide: `MessageRepository.ListByRoom`/`GetByID` return rows to any room member, and `MessageUsecase.SendAIMessage` (`server/internal/usecase/message/usecase.go`) always persists messages that every member can read. This step adds the `visibility` dimension end-to-end — schema, repository filtering, usecase plumbing, targeted WS delivery, and context-builder isolation — so private exchanges never leak to other members through any read path (REST list, single-message fetch, WS broadcast, or AI context).

## Goal

After this PR, `POST /rooms/:roomId/messages/ai` accepts an optional `private` flag. When set, both the resulting human message and AI message are persisted with `visibility = 'private'` and are excluded from `ListByRoom`/`GetByID` results and from AI context building for every user except the original requester; the corresponding WebSocket events are delivered only to that requester's connections (using the per-user targeted delivery already exposed by the `MessageHub` interface landed in Step 15), never broadcast to the room. Public messages behave exactly as before. All new behavior is covered by usecase- and repository-level tests.

## Scope

- [ ] Add `messages.visibility` column: `server/schema.sql` — `visibility VARCHAR(20) NOT NULL DEFAULT 'public'` on the `messages` table, plus a `CHECK (visibility IN ('public', 'private'))` constraint. Run `task migrate:generate` (dir `server`, wraps `atlas migrate diff <name> --env local`) to emit the versioned migration file under `server/migrations/` and update `server/migrations/atlas.sum`.
- [ ] Domain: `server/internal/domain/message/entity.go` — add a `MessageVisibility` string type with `MessageVisibilityPublic`/`MessageVisibilityPrivate` constants (mirroring the existing `MessageType`/`MessageStatus` pattern) and a `Visibility MessageVisibility` field on `Message`.
- [ ] Repository interface: `server/internal/domain/message/repository.go` — extend `ListByRoom` and `GetByID` (and `ListByRoomUpTo`, used by context building) to accept a `requestingUserID string` parameter so implementations can filter out other users' private messages. Update GoDoc on each method to state the new visibility contract (private messages are returned only when `requestingUserID` is their owner; otherwise they are treated as if they do not exist).
- [ ] Postgres impl: `server/internal/interface/repository/postgres/message_repository.go` — thread `requestingUserID` through `ListByRoom`, `GetByID`, `ListByRoomUpTo` and add `AND (visibility = 'public' OR sender_id = $requestingUserID)` to each query; update `messageColumns`/`scanMessage` to include/scan `visibility`; update `Create` to insert it.
- [ ] Usecase: `server/internal/usecase/message/usecase.go`
  - Extend `SendAIMessage` with a `private bool` parameter. When true, set `Visibility: domainmessage.MessageVisibilityPrivate` on both the human message (via an internal private-aware path, since `SendMessage` is also called for plain human sends) and the AI message.
  - For the AI message specifically, since `SenderID` is otherwise always `nil`, set `SenderID: &userID` on private AI messages so the single `visibility = 'public' OR sender_id = $requestingUserID` predicate works uniformly for both message rows without adding another column. Document this deviation from the "AI messages have nil SenderID" convention with a code comment at the assignment site.
  - Update all repository call sites (`ListByRoom`, `GetByID`, `ListByRoomUpTo`) in `SendAIMessage`, `RegenerateAIMessage`, `ListMessages`, and the context-building helper introduced in Step 23 to pass the acting `userID` as `requestingUserID`, so a private exchange is invisible to every other room member on every read path, including AI context assembly for other users' subsequent requests.
  - `RegenerateAIMessage` must reject regeneration of another user's private exchange with `domain.ErrForbidden` (fetch with the acting user as `requestingUserID`; a `domain.ErrNotFound` from the filtered lookup means "not visible to this user", which for regeneration purposes should be surfaced as forbidden since the room membership check already passed).
- [ ] WS delivery: publish the `SendAIResult` events using the `MessageHub` interface introduced in Step 15. Locate its per-user targeted-delivery method (the plan commits to per-user delivery being present in the interface "from day one" — search the interface, e.g. `grep -rn "MessageHub" server/internal` to find its current location/method names) and use it instead of the room-broadcast method whenever `msg.Visibility == domainmessage.MessageVisibilityPrivate`, so private message events reach only the sender's active WebSocket connections. Public messages keep using the existing room-broadcast method.
- [ ] Handler/DTO: `server/internal/interface/handler/dto.go` — add `Private bool `json:"private"`` (optional, default `false`) to `SendAIMessageRequest`, and add `Visibility string `json:"visibility"`` to `MessageResponse`. `server/internal/interface/handler/message_handler.go` — pass `req.Private` into `usecase.SendAIMessage`, and populate `Visibility` in `toMessageResponse`.
- [ ] Update `List`/`Send` handlers and any other `usecase` call sites to pass the authenticated user ID as the new `requestingUserID` repository/usecase argument (the handler already has `userID := middleware.GetUserID(c)` available).
- [ ] Tests:
  - `server/internal/domain/message/entity_test.go` — add visibility constant assertions following the existing `TestMessageTypeConstants` pattern.
  - `server/internal/usecase/message/usecase_test.go` — extend the mock repository (`mockMsgRepo`) to honor `requestingUserID` filtering; add cases: (a) private `SendAIMessage` sets `Visibility=private` and `SenderID` on the AI message; (b) a second user's `ListMessages`/`ListByRoom` on the same room does not include the private pair; (c) the owning user's `ListMessages` does include it; (d) `RegenerateAIMessage` by a non-owner on a private exchange returns `domain.ErrForbidden`; (e) context building (Step 23's helper) excludes another user's private messages when assembling context for a different requester's public/private request.
  - `server/internal/interface/handler/message_handler_test.go` — add a case asserting `SendAI` with `"private": true` returns `visibility: "private"` in both message responses, and that `List` called by another mock user omits the private pair.
  - Repository-level test: if `server/internal/interface/repository/postgres/` has (or gains, via Step 23's testcontainers setup) a testcontainers-backed integration test file, add one verifying the SQL-level filter directly (private row invisible to non-owner, visible to owner); if no such harness exists yet at this point, note this as a follow-up in the PR description rather than inventing a new testcontainers harness in this step.

## Out of scope

- Web UI toggle for private mode and rendering private messages distinctly in the message list (Step 47).
- Context summarization caching and any interaction between private visibility and cached summaries (Step 50) — this step only ensures the Step-23 context builder's per-request filtering respects visibility; summary caching itself is untouched.
- RBAC-based restrictions on who may use private mode (e.g. reader/guest tiers) — private mode is available to any room member who can already send AI messages; no new role gate is introduced.
- Redis-backed `MessageHub` (`RedisHub`) — this step only depends on the `MessageHub` interface and its in-process implementation from Step 15; the Redis swap is Step 31 and is transparent to this step's code.
- Changes to `room.ai_model`/model-resolution logic — that is Step 24's separate helper file; this step's only coordination point with Step 24 is the `SendAIMessage` call site (a one-line addition of the `private` argument alongside Step 24's model-resolution argument).
- New model resolution, streaming, or Vision-related fields on messages.

## Implementation notes

- **Schema**: edit only `server/schema.sql`'s `messages` table (this is the sole wave-5 step permitted to touch `messages` per the plan's conflict convention). Do not touch other tables. After editing, run `task migrate:generate` from the repo root (it `cd`s into `server` and runs `atlas migrate diff <name> --env local`); commit the generated file under `server/migrations/` and the updated `server/migrations/atlas.sum`. If the migration name auto-generated by the `now`-based default is undesirable, pass an explicit name: `task migrate:generate -- add_message_visibility`.
- **Existing patterns to follow**:
  - `MessageType`/`MessageStatus` in `server/internal/domain/message/entity.go` are the template for `MessageVisibility`.
  - `messageColumns` / `scanMessage` in `server/internal/interface/repository/postgres/message_repository.go` is the single place column lists are defined for `Create`, `GetByID`, `ListByRoom`, and `ListByRoomUpTo` — extend it once and all four queries pick it up.
  - `handleMessageError` in `server/internal/interface/handler/message_handler.go` already maps `domain.ErrForbidden`/`domain.ErrNotFound`; no new error types are needed — reuse `domain.ErrForbidden` for the regenerate-of-another-user's-private-message case.
  - Mock repositories for tests: extend Step 2's shared `server/internal/testutil/mocks` package (the old per-file `mockMsgRepo`-style duplicates were consolidated there) rather than adding a new hand-rolled mock.
- **API shape**:
  - `POST /rooms/:roomId/messages/ai` request body gains `private` (bool, optional, default `false`) alongside existing `content`/`model`.
  - `MessageResponse` (used by `Send`, `List`, `SendAI`, `RegenerateAI` responses) gains `visibility` (string: `"public"` or `"private"`).
- **Conflict/coordination notes** (per the plan's execution contract): Step 24 (same wave) owns model-resolution logic in its own new helper file plus the settings handler; this step owns `SendAIMessage`'s privacy plumbing and all read-path filters. The two steps' only overlapping edit is the `SendAIMessage` call site inside `message_handler.go`'s `SendAI` — keep that a one-line, additive change (passing both the resolved model and the `private` flag) so the two PRs merge cleanly regardless of order.
- **GoDoc**: every new/changed exported symbol (`MessageVisibility`, its constants, the new `requestingUserID` parameters, the DTO fields) needs a GoDoc comment describing what/why/constraints per `CLAUDE.md` conventions — state explicitly in each repository method's doc that private messages are invisible to non-owners rather than merely "filtered."
- **Logging**: use `slog` (already the project-wide logger) if you add any log line around private-message WS delivery; include `room_id` and the acting `user_id` (never log private message content).

## Verification

1. `cd server && go build ./...` — compiles cleanly.
2. `cd server && go test ./...` (or `task test:server` from repo root) — all tests pass, including the new visibility-focused cases in `usecase_test.go`, `entity_test.go`, and `message_handler_test.go`.
3. `docker compose up -d db && task migrate:apply` (or `task up`) — migration applies without error; `docker compose exec db psql -U polyphony -d polyphony -c "\d messages"` shows the new `visibility` column with a `NOT NULL DEFAULT 'public'` and a check constraint.
4. With the stack up (`task up`), as two different authenticated users A and B who are both members of the same room:
   - `curl -X POST http://localhost:8080/rooms/<roomId>/messages/ai -H "Authorization: Bearer <A-token>" -d '{"content":"secret question","private":true}'` returns `201` with both messages having `"visibility":"private"`.
   - `curl http://localhost:8080/rooms/<roomId>/messages -H "Authorization: Bearer <B-token>"` does NOT include either of those two messages.
   - The same list call with A's token DOES include them.
5. `cd server && go vet ./...` — no new issues.
6. Manual/WS check (or a Go test against the in-process hub if one exists from Step 15): connect two WS clients as A and B to the room's realtime endpoint, send a private AI message as A, and confirm only A's connection receives the corresponding message events; B's connection receives nothing for that exchange.

## Completion criteria

- [ ] `messages.visibility` column exists in `server/schema.sql` with a migration generated via `task migrate:generate` and applied successfully.
- [ ] `MessageVisibility` type/constants and `Message.Visibility` field added per existing entity conventions.
- [ ] `ListByRoom`, `GetByID`, `ListByRoomUpTo` (interface, Postgres implementation, and all usecase call sites) filter out other users' private messages.
- [ ] `SendAIMessage` supports a `private` flag that marks both the human and AI message rows private, with the AI message's `SenderID` set to the requester for filtering purposes (documented in code).
- [ ] `RegenerateAIMessage` returns `domain.ErrForbidden` when a non-owner targets another user's private exchange.
- [ ] Private message WS events are delivered only to the sender via the `MessageHub`'s per-user targeted delivery; public messages remain room-broadcast.
- [ ] `SendAIMessageRequest.private` and `MessageResponse.visibility` are wired through the handler and DTOs.
- [ ] The Step-23 context builder excludes other users' private messages when assembling AI context for a different requester.
- [ ] New/updated unit tests cover: private send, cross-user list exclusion, owner list inclusion, non-owner regenerate rejection, and context-builder exclusion.
- [ ] All verification checks above pass.
