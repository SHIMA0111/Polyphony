# Step 35: Web WebSocket client + live cache merge

## Meta
- **Type**: feature
- **Components**: web
- **Wave**: 5 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 15: Server WebSocket endpoint + realtime delivery | Step 29: Web send pipeline UX: optimistic send, AI thinking, retry, infinite scroll
- **Unlocks**: Step 47: Web: private AI mode UI | Step 54: Web: streaming AI rendering | Step 57: E2E regression: realtime AI core — send, model select, context controls, rate limiting
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Phase 2 of `phases.md` requires messages to be delivered to all room members in real time, and the plan's architecture notes commit the web frontend to TanStack Query with "the Query cache is the single merge point for WS events and streaming chunks." Step 15 lands the server-side WebSocket endpoint and ticket-issuing mechanism; Step 29 lands the TanStack Query-based send pipeline (optimistic updates, infinite scroll pagination) that this step's live events must merge into without producing duplicate or out-of-order messages. Today `web/src/features/messages/components/ChatRoom.tsx` only fetches once via `useEffect` + `apiClient.listMessages` and appends locally on send — there is no live channel at all. This step closes that gap with a reconnecting WebSocket hook that feeds the same cache Step 29 established.

## Goal
A `useRoomSocket` hook (or equivalently named feature hook) opens a ticket-authenticated WebSocket connection per room, automatically reconnects with exponential backoff on drop, and merges every inbound room/message event into the TanStack Query cache using `queryClient.setQueryData`, de-duplicating against messages already present from the optimistic-send/REST/infinite-scroll paths introduced in Step 29. A small connection-status indicator is rendered in the chat room header. The hook is a complete no-op (does not attempt a real socket connection) when the app is running in MSW mock mode, so mock-mode dev/tests are unaffected. `ChatRoom.tsx` is touched only to mount the hook and render the indicator — one line each.

## Scope
- [x] Add `useRoomSocket(roomId: string)` hook in `web/src/features/messages/hooks/use-room-socket.ts`:
  - [x] Fetches a short-lived WS ticket from the authenticated endpoint Step 15 exposes (inspect `server/internal/interface/handler/websocket_handler.go` — Step 15's `POST /ws/ticket` endpoint — and its route registration — likely reached through the Next.js BFF proxy at `web/src/app/api/proxy/[...path]/route.ts` — for the exact path/response shape) before opening the socket; never sends the JWT/session cookie directly on the WS URL.
  - [x] Opens a native `WebSocket` to the gateway's WS upgrade path (again per Step 15's actual route), passing the ticket as a query param.
  - [x] Implements reconnect with exponential backoff + jitter (e.g. base 500ms, doubling, capped at ~30s), restarting the backoff counter on a successful `onopen`.
  - [x] Cleans up (closes socket, clears timers) on unmount or `roomId` change.
  - [x] Exposes a connection status (`"connecting" | "connected" | "reconnecting" | "offline"`) for the indicator to render.
  - [x] Is fully inert (does not construct a `WebSocket` at all, returns a static `"offline"`/disabled status) when the app is running under MSW/mock mode — reuse whatever mock-mode flag exists at this point (the old `IS_MOCK`/`NEXT_PUBLIC_MOCK_API` runtime mock was deleted by Step 9; if no MSW-era env flag such as `NEXT_PUBLIC_API_MOCKING` exists by this wave, add a small `isMockMode()` helper in `web/src/config/env.ts` reading such a flag and use it here).
- [x] Add a cache-merge utility, e.g. `web/src/features/messages/lib/merge-message-event.ts`, that:
  - [x] Parses/validates the inbound WS event payload against the shape Step 15 actually emits (inspect Step 15's `wsEventFrame` in `server/internal/interface/handler/websocket_handler.go` and the `EventType` constants in `server/internal/domain/event/hub.go` for the JSON contract — `{"type": "message_created"|"message_updated", "room_id": ..., "message": {...}}` frames).
  - [x] Calls `queryClient.setQueryData` on the exact messages query key Step 29 established for the room (inspect the `useMessages`/infinite-query hook Step 29 added under `web/src/features/messages/` for the query key shape, e.g. `["rooms", roomId, "messages"]`).
  - [x] De-dupes by message `id` against everything already in the cached pages (optimistic entries already reconciled by Step 29, REST-fetched pages, previously-merged WS events) — on a `created` event for an id already present, no-op or reconcile status only; on `updated`, patch the existing entry in place; never append a second copy of the same id.
  - [x] Preserves page/sequence ordering (append new messages to the newest page only; never re-sort the whole cache).
- [x] Add a minimal connection-status indicator component, e.g. `web/src/features/messages/components/ConnectionStatus.tsx`, built from `@chakra-ui/react` primitives per `.claude/rules/chakra-ui.md` (e.g. a small `Badge`/dot + `Tooltip` — reuse the existing `web/src/components/ui/tooltip.tsx` snippet, do not add a new one), showing connected/reconnecting/offline states; hidden or shown as inert in mock mode.
- [x] Mount the hook and indicator in `web/src/features/messages/components/ChatRoom.tsx` with a one-line hook call and a one-line indicator render in the existing header `Flex` — no other changes to `ChatRoom.tsx`'s send/regenerate logic.
- [x] Unit tests for `use-room-socket.ts` (e.g. `web/src/features/messages/hooks/use-room-socket.test.ts`) using a hand-rolled fake `WebSocket`/`vi.stubGlobal` (or `mock-socket` if a WS-mocking dependency is already present in `web/package.json`; do not add a new heavyweight dependency for this alone) covering: successful connect, message-event merge/dedupe, reconnect-with-backoff after an abnormal close, no-op behavior in mock mode.
- [x] Unit tests for `merge-message-event.ts` covering create/update/duplicate/out-of-order-arrival cases against a seeded query cache.

## Out of scope
- Server-side WebSocket endpoint, ticket issuance, and broadcast/hub logic (Step 15).
- Optimistic send, AI "thinking" indicator, retry UI, and infinite-scroll pagination mechanics (Step 29) — this step only consumes the query key/cache shape Step 29 defines, it does not change the send pipeline.
- Message item menus (edit/delete/exclude-from-ai toggle) and the message input component (Step 38, same wave — disjoint files by convention).
- Private AI mode delivery/visibility filtering (Step 47) and streaming token rendering (Step 54) — this step only wires generic message create/update events into the cache; per-token streaming chunks and private-only delivery are handled by those later steps reusing this hook's connection.
- Redis-backed multi-instance fan-out (already a server-side concern, Phase 10/Step 31) — this step is transport-agnostic on the client and requires no change if the server swaps `MessageHub` implementations.
- Any new WebSocket-aware Playwright E2E spec (covered by Step 57, which depends transitively on this step).

## Implementation notes
- **Wave/file ownership**: this step is confined to a new hook + cache-merge utility + status indicator component, plus a one-line mount in `ChatRoom.tsx`. Step 38 (same wave 5) owns message item menus and `MessageInput.tsx` changes — do not touch those files beyond the single hook-mount line in `ChatRoom.tsx`'s header, and coordinate if a merge conflict appears there.
- **Ticket flow**: per the plan's AUTH DATA-PLANE PLAN, the WS ticket is a short-lived, server-signed token from an authenticated endpoint, independent of whichever `AuthService` is active (SimpleJWT now, Kratos later) — fetch it through the same `/api/proxy/*` BFF path other authenticated data calls use (use Step 4's `apiRequest` from `web/src/lib/http-client.ts`, which already targets `/api/proxy/*` — `web/src/lib/api.ts` no longer exists after Step 9). Do not persist the ticket; fetch a fresh one on every (re)connect attempt.
- **Query client**: reuse the shared `QueryClient` instance established by the wave 1-2 TanStack Query setup (Step 9's `web/src/lib/query-client.ts` / the `QueryProvider` in `web/src/app/query-provider.tsx`) rather than creating a second instance — `useQueryClient()` inside the hook is the correct way to reach it.
- **Existing state to replace/extend**: `ChatRoom.tsx` currently holds messages in local `useState` (`useState<Message[]>`) populated via `apiClient.listMessages`/`sendMessage`/`sendAIMessage` (see current `web/src/features/messages/components/ChatRoom.tsx`). By this step, Step 29 is expected to have already moved that state into TanStack Query (`useQuery`/`useInfiniteQuery` + mutations); this step's job is purely to feed live events into that same cache, not to reintroduce local state. If Step 29's migration left any local `useState` for messages, treat that as a Step 29 gap to flag rather than something to silently work around.
- **Event/type shapes**: mirror the JSON emitted by the server's `MessageHub`/WS handler exactly — define a discriminated union type (e.g. `RoomSocketEvent`) in `web/src/features/messages/types/ws-events.ts` (or fold it into `web/src/features/messages/types.ts`, Step 9's per-feature types module, if that reads better) rather than using `any`/loose typing, per the project's English-only, typed-code conventions.
- **Chakra v3**: use Compound Components / style props per `.claude/rules/chakra-ui.md`; check `chakra-ui:get_component_props`/`get_component_example` via the MCP server if the exact `Badge`/status-dot API is unclear rather than guessing.
- **No DB/server changes**: this step is web-only; `dbChanges` is empty.
- **GoDoc/rustdoc**: not applicable (TypeScript). Follow the existing code style in `web/src/features/messages/` (no barrel files, direct imports, `"use client"` at top of client components/hooks that use browser APIs).

## Verification
1. [x] `cd web && bun install && bun run lint` — passes with no new lint errors.
2. [x] `cd web && bun run test` (or the project's configured Vitest command, e.g. `bunx vitest run`) — new tests for `use-room-socket.ts` and `merge-message-event.ts` pass, and no existing test is broken.
3. `docker compose up -d db migrate api llm-gateway web` (from repo root) — stack starts cleanly.
4. Manual two-client real-time check: log in as two different users (or two browser profiles) in the same room via `http://localhost:3000`; send a message from client A and confirm it appears in client B's chat within a couple of seconds with no page reload, and the connection indicator on both clients shows "connected".
5. Reconnect check: `docker compose stop api`, observe the indicator transition to "reconnecting" within the backoff window on both clients (no uncaught console errors); `docker compose start api` and confirm the indicator returns to "connected" and subsequently sent messages are delivered again without a manual page refresh.
6. Dedup check: with two clients open, send a message from client A and confirm it renders exactly once in client A's own view (no duplicate from the optimistic entry + the WS echo of the same message).
7. Mock-mode check: run the web app with mock mode enabled (`NEXT_PUBLIC_MOCK_API=true` or whatever flag this wave's MSW migration uses) and confirm no WebSocket connection attempt is made (no network tab WS entry, no console errors) and the connection indicator renders its inert/mock state.

## Completion criteria
- [x] `use-room-socket.ts` hook implemented with ticket-auth connect, exponential-backoff reconnect, and typed event handling.
- [x] `merge-message-event.ts` cache-merge utility implemented with id-based dedupe and update-in-place semantics, targeting the exact query key Step 29 defined.
- [x] Connection status indicator component implemented with Chakra v3 primitives and mounted in `ChatRoom.tsx` via a one-line change.
- [x] Hook is verifiably inert under MSW/mock mode.
- [x] Unit tests added for both the hook and the merge utility and passing.
- [x] `ChatRoom.tsx` diff is limited to the hook mount + indicator render (no send/regenerate logic changes).
- [x] All verification checks above pass. (Wave-5 integration review, live compose stack: #3 ✓ stack starts cleanly; #4 ✓ a message sent by another member appeared in a live browser client within ~1s with no reload, indicator "Connected"; #5 ✓ `docker compose stop api` flipped the indicator to "Reconnecting…" with no uncaught console exceptions, and `start api` restored "Connected" + subsequent delivery; #7 ✓ under `NEXT_PUBLIC_MOCK_API=true` no WS/ticket request is made and the indicator renders "Offline". **#6 was fixed in a follow-up review round**: a message sent from the client's own UI rendered twice — the WS echo of the new message raced ahead of the POST response, `mergeMessageEvent` prepended the server copy, and `use-send-message.ts`'s (and `use-send-ai-message.ts`'s) `onSuccess` then swapped the optimistic entry into a second copy of the same id. Fixed by having each `onSuccess` check `findMessageInPages` for the server id first: if the WS echo already merged it, the optimistic entry is removed via `removeFromNewestPage` instead of replaced; otherwise the original swap runs unchanged. Covered by new vitest cases in `use-send-message.test.tsx`/`use-send-ai-message.test.tsx` that simulate the echo-before-onSuccess race via `mergeMessageEvent`.)
