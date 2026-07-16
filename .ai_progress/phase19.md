# Phase 19: Streaming AI Responses (Step 54: web rendering)

Tracks `docs/tasks/step54.md`'s scope: the web-side last mile of Phase 19
("Streaming AI Responses"). The gateway `stream()` port and the Go
API's WS chunk forwarding (`token_chunk`/`message_updated` frames,
`POST /rooms/:roomId/messages/ai/stream`) were delivered by Step 51; this
step teaches the TanStack Query message cache and the AI message bubble to
consume those events, render them live, finalize on completion, and fall
back gracefully to the non-streaming path when no chunk events arrive.

## Scope

- [x] `web/src/features/messages/types/ws-events.ts`: extend
      `RoomSocketEventType`/the discriminated union with `"token_chunk"`
      (`ChunkPayload`, `ChunkSocketEvent`), and extend `isRoomSocketEvent`'s
      guard to accept it (it previously rejected any frame that wasn't
      `message_created`/`message_updated`).
- [x] `web/src/features/messages/types.ts`: add `"streaming"` to
      `MessageStatus` (a real wire value from Step 51's `202` response, not
      a client-only invention).
- [x] `web/src/features/messages/api/send-ai-message.ts`: add
      `sendAIMessageStream` (`POST /rooms/:roomId/messages/ai/stream`),
      keeping `sendAIMessage` (`POST /rooms/:roomId/messages/ai`) intact as
      the documented fallback (private AI mode, a future step, cannot use
      the streaming endpoint — it 400s `private: true`).
- [x] `web/src/features/messages/hooks/use-send-ai-message.ts`: switch the
      mutation's `mutationFn` to `sendAIMessageStream`; `onSuccess`/`onError`
      needed no changes (already generic over the response's message
      shape/status).
- [x] `web/src/features/messages/lib/merge-message-event.ts`: handle
      `token_chunk` events (`applyTokenChunk`) — append-to-existing,
      first-chunk-creates-placeholder, and idempotent no-op against an
      already-finalized (`completed`/`failed`) message.
- [x] `web/src/features/messages/components/MessageBubble.tsx`: three AI
      bubble states (thinking / live-streaming with a pulsing cursor /
      finalized), rendered as a content update within the same bubble (no
      remount) once the message's real id is established; `ThinkingBubble`
      now also covers `status: "streaming"` with no content yet.
      `Streaming…`/`Sending…` bottom-row status labels added alongside the
      existing timestamp.
- [x] `web/src/features/messages/components/MessageList.tsx`: added a
      stable `data-testid="message-list"` on the scroll container (used by
      `streaming.spec.ts` to attach a `MutationObserver` before the send is
      triggered) — no change to rendering/grouping logic.
- [x] `web/src/features/messages/api/handlers.ts`: default MSW handler +
      fixtures (`fixtureAiMessageStreaming`, `fixtureAiStreamResponse`) for
      `POST /rooms/:roomId/messages/ai/stream`.
- [x] Updated existing tests whose mocks targeted the now-unused
      `/messages/ai` path for the send-with-AI flow
      (`use-send-ai-message.test.tsx`, `use-chat-room.test.tsx`).
- [x] Tests: `merge-message-event.test.ts` (append-to-existing-placeholder,
      first-chunk-creates-placeholder, finalize-replaces-in-flight,
      idempotent finalize/chunk race), `use-room-socket.test.ts` (a
      `token_chunk` frame merges through the real WS pipeline),
      `use-send-ai-message.test.tsx` (full mutation -> chunks -> finalize
      lifecycle), `MessageBubble.test.tsx` (the three visual states).
- [x] `llm-stub/fixtures/stream/streaming-demo.sse` +
      `llm-stub/fixtures/streaming-demo.json`: an additive, many-chunk (22
      SSE events) fixture pair for the E2E stub, triggered by
      `[[fixture:streaming-demo]]` — `default.sse`'s 4 chunks traverse
      stub -> gateway -> Go -> WS fast enough (no pacing in the stub) that
      polling for multiple intermediate states would be flaky.
- [x] `web/e2e/streaming.spec.ts`: sends a message with AI in a fresh room,
      credits its balance via `server/cmd/seed-tokens` (same pattern as
      `attachments.spec.ts`), and asserts the thinking state, at least one
      genuinely partial streamed-content DOM state (captured via an in-page
      `MutationObserver` installed before the send, not test-runner
      polling), and a settled final state matching the fixture's text.
- [x] Update this progress file.

## Verification

- [x] `bun install` in `web/` — no new external animation/streaming
      dependency added.
- [x] `bun run lint` in `web/` — 0 errors (pre-existing warnings only,
      unrelated to this step).
- [x] `bunx tsc --noEmit` in `web/` — clean.
- [x] `bunx vitest run` in `web/` — 175/175 tests pass (full suite, no
      regressions).
- [x] `go build ./...` / `go vet ./...` / `go test ./...` in `server/` —
      unaffected (this step makes no server changes); all pass.
- [x] `bun test` in `llm-stub/` — 11/11 pass (new fixture files don't
      affect the stub's own unit tests, which don't enumerate fixtures).
- [x] `task test:e2e:up` + `bunx playwright test streaming.spec.ts`
      against the full compose test-profile stack — passes (wave-7
      integration review). Two integration fixes were required: web-e2e's
      `NEXT_PUBLIC_API_URL` build arg pointed the browser's WebSocket at
      the dev API's host port (8080) instead of api-e2e's (8090), and the
      LLM stub now paces its canned SSE events ~25ms apart
      (`STREAM_EVENT_DELAY_MS`) so intermediate streamed renders are
      actually observable.
- [x] Fallback exercise — the non-streaming path is exercised live by
      `private-mode.spec.ts` (private sends route through
      `POST /messages/ai`) and the attachment flow (`attachments.spec.ts`),
      which now also opts out of streaming (`SendAIMessageInput.stream:
      false`) so its immediate regenerate never races a still-in-flight
      stream's finalize (a clobbering race the wave-7 integration run
      caught live).
- [x] Full Playwright suite (`bunx playwright test`) green across
      consecutive runs against the compose test stack, including Step 47's
      private-mode badges sharing `MessageBubble.tsx` (wave-7 integration
      review).

## Out of scope (per step54.md)

- Any change to the LLM Gateway's SSE/streaming endpoint or `stream()` port
  (Step 51's dependency chain).
- Any change to the Go API's stream-consumption or WS-forwarding logic
  (Step 51 — treated as a frozen, read-only contract here).
- Privacy badges / the private-AI-mode toggle in `MessageBubble`/
  `MessageInput` (Step 47, same wave — disjoint region, not touched).
- Context summarization indicator UI / the "summary used" badge (Phase 18).
- Model selection UI (`ModelSelector.tsx`).
- Room-level AI cutoff settings UI (Step 36).
- Any new DB schema/migration (Phase 19 has none).
- Retry/backoff policy for dropped WebSocket connections mid-stream (Step
  35's reconnection logic — this step only reacts to events it receives).
