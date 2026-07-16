# Phase 14: Private AI Mode

**Goal**: Option to make AI responses visible only to the sender.

**Delivered by**: `docs/tasks/step41.md` (Server: private AI mode), `docs/tasks/step47.md` (Web: private AI mode
UI). See those files for the full per-file implementation notes.

---

## Step 41: Server private AI mode

- [x] `server/schema.sql`: `messages.visibility VARCHAR(20) NOT NULL DEFAULT 'public'` + `CHECK` constraint;
      migration via `task migrate:generate`
- [x] `domain/message.MessageVisibility` (`public`/`private`) + `Message.Visibility` field
- [x] `MessageRepository.ListByRoom`/`GetByID`/`ListByRoomUpTo` extended with a `requestingUserID` parameter;
      Postgres queries filter `visibility = 'public' OR sender_id = $requestingUserID`
- [x] `MessageUsecase.SendAIMessage` accepts a `private bool`; private events delivered via `MessageHub`'s
      per-user-targeted method (Step 15) instead of the room-broadcast method
- [x] `SendAIMessageRequest.Private` / `MessageResponse.Visibility` DTO fields
- [x] All `List`/`Send` handler call sites updated to pass the authenticated user ID as `requestingUserID`
- [x] Unit + integration tests covering visibility filtering and targeted WS delivery

## Step 47: Web private AI mode UI

- [x] `Message` type extended with `visibility`
- [x] Private-mode toggle in the AI controls row of the message composer (`MessageInput.tsx`), gated to the AI send
      path only
- [x] `useSendAIMessage` propagates the flag to `POST /rooms/:id/messages/ai`'s `private` field; MSW handler updated
      to echo it
- [x] Optimistic local insert tagged with the selected `visibility` so the badge appears immediately, pre-WS
      reconciliation
- [x] `MessageList` renders a "Private" badge + distinguishing bubble treatment for `visibility === "private"`
      messages (no sender-identity comparison needed — the server never delivers another user's private message to
      this client)
- [x] WS event merge (`merge-message-event.ts`) passes `visibility` through untouched; defensive no-op guard drops
      (and logs) any event whose `sender_id` mismatches the current user while `visibility === "private"`
- [x] Unit tests: `MessageInput` toggle state/reset, `MessageList` badge rendering, `merge-message-event` pass-through
      + defensive-guard cases
- [x] `web/e2e/private-mode.spec.ts` — two-user private-mode isolation spec (written per the harness's conventions)

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [ ] `web/e2e/private-mode.spec.ts` / `web/e2e/regression/advanced-ai/private-mode.spec.ts` against the live
      compose stack — requires the full E2E stack; skipped (post-merge integration review)
