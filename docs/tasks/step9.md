# Step 9: Web data layer migration: TanStack Query, RSC prefetch, per-feature API modules

## Meta
- **Type**: refactor
- **Components**: web
- **Wave**: 2 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 1: Go server foundation: DI/router split, structured logging, lint, config (provides `GET /users/me`, this step's session source) | Step 4: Web BFF auth + data-plane proxy: httpOnly cookies, middleware guard, typed HTTP client | Step 5: Web vitest + RTL unit-test harness + MSW mock layer
- **Unlocks**: Step 16: Web persistent sidebar chat layout | Step 17: Web forms, toaster, and App Router error surfaces | Step 18: Web message list rendering upgrade + ChatRoom decomposition | Step 34: Model metadata (token limits + pricing) across gateway/server/web
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Today `web/src/lib/api.ts` is a single 250-line file holding both `RealApiClient` and `MockApiClient` behind one `ApiClientInterface`, `web/src/types/api.ts` is a single shared types file, and every component (`ChatRoom.tsx`, `RoomList.tsx`, `CreateRoomForm.tsx`, `LoginForm.tsx`, `RegisterForm.tsx`) fetches data itself via `useState` + `useEffect` + `apiClient.*` calls, with a `NEXT_PUBLIC_MOCK_API` flag switching to canned data. This does not scale to the rest of the plan: later steps need a shared query cache that WebSocket events and streaming chunks can write into (per the plan-wide architecture notes), Server Components need to prefetch data before first paint, and Step 5's MSW mocks need a natural per-feature home instead of one shared client. This step finishes the `api.ts` split that Step 4 began (Step 4 introduced the httpOnly-cookie BFF and the `/api/proxy/*` data-plane proxy; the old token-based `RealApiClient`/`MockApiClient` plumbing is what remains to be replaced), moving the whole web app onto TanStack Query with RSC prefetch/hydration, and deletes the mock client entirely now that MSW (Step 5) is the sanctioned way to fake network responses in tests.

## Goal
After this PR, `web/src/lib/api.ts`, `web/src/lib/mock-data.ts`, `web/src/types/api.ts`, `web/src/components/mock-badge.tsx`, and `web/src/hooks/use-auth.ts` no longer exist. In their place, `features/auth`, `features/rooms`, and `features/messages` each own a `types.ts`, an `api/` directory (fetch functions + `queryOptions`/mutation factories built on the Step 4 typed HTTP client), and a `hooks/` directory (`useRooms`, `useRoom`, `useCreateRoom`, `useDeleteRoom`, `useMessages`, `useSendMessage`, `useSendAIMessage`, `useRegenerateAIMessage`, `useModels`, `useSession`, `useLogin`, `useRegister`, `useLogout`). A shared `QueryClient` is provided app-wide from `web/src/app/query-provider.tsx`, and `/rooms` and `/rooms/[roomId]` are Server Components that prefetch their data and hand it to client components through `HydrationBoundary`. Step 5's MSW handlers are split across the three features' `api/handlers.ts` modules and reassembled in the shared MSW bootstrap. `RoomList`, `ChatRoom`, `CreateRoomForm`, `LoginForm`, and `RegisterForm` are rewired to the new hooks with no remaining `apiClient`/`IS_MOCK` references anywhere in `web/src`. The auth/session hook has RTL+MSW test coverage for the first time, exercised against the cookie-based flow.

## Scope
- [x] Add `@tanstack/react-query` and `@tanstack/react-query-devtools` to `web/package.json` (`bun add`).
- [x] Create `web/src/lib/query-client.ts` exporting `getQueryClient()`: a `makeQueryClient()` factory (sane `staleTime`, e.g. 60s, so RSC-prefetched data isn't immediately refetched on mount) plus the standard Next.js App Router split — always a fresh `QueryClient` on the server (`isServer` from `@tanstack/react-query`), a module-level singleton reused across renders in the browser.
- [x] Create `web/src/app/query-provider.tsx` (`"use client"`): wraps children in `QueryClientProvider` using `useState(() => getQueryClient())`, and mounts `ReactQueryDevtools` only when `process.env.NODE_ENV !== "production"`.
- [x] Wire `QueryProvider` into `web/src/app/layout.tsx` alongside the existing Chakra `Provider`; drop the `<MockBadge />` render and its import.
- [x] Create `web/src/lib/http-client.server.ts` (`import "server-only"`): a server-only fetcher for RSC prefetch that forwards the incoming request's `cookie` header (via `headers()` from `next/headers`) to the Step-4 proxy at an internal origin (`process.env.APP_INTERNAL_URL`, default `http://localhost:3000`), reusing the proxy's existing cookie→Bearer translation instead of duplicating it.
- [x] Create `web/src/features/auth/types.ts` (`Session`/`User` shape returned by Step 1's `GET /users/me` whoami endpoint, consumed via `/api/proxy/users/me`) and delete the corresponding interfaces from `web/src/types/api.ts`.
- [x] Create `web/src/features/auth/api/{get-session,login,register,logout}.ts` (fetch fns + `getSessionQueryOptions()` query-options factory taking an optional fetcher, defaulting to the Step-4 client `httpClient`) and `web/src/features/auth/hooks/{use-session,use-login,use-register,use-logout}.ts`.
- [x] Create `web/src/features/rooms/types.ts` (`Room`) and `web/src/features/rooms/api/{get-rooms,get-room,create-room,delete-room}.ts` + `web/src/features/rooms/hooks/{use-rooms,use-room,use-create-room,use-delete-room}.ts`.
- [x] Create `web/src/features/messages/types.ts` (`Message`, `MessageType`, `MessageStatus`, `MessagePage`, `AIMessageResponse`, `ModelInfo`, `ModelListResponse`) and `web/src/features/messages/api/{get-messages,send-message,send-ai-message,regenerate-ai-message,get-models}.ts` + `web/src/features/messages/hooks/{use-messages,use-send-message,use-send-ai-message,use-regenerate-ai-message,use-models}.ts`.
- [x] Delete `web/src/lib/api.ts`, `web/src/lib/mock-data.ts`, `web/src/types/api.ts`, `web/src/components/mock-badge.tsx`, `web/src/hooks/use-auth.ts`.
- [x] Rewrite `web/src/app/(main)/rooms/page.tsx` as an `async` Server Component that calls `getQueryClient()`, `await queryClient.prefetchQuery(getRoomsQueryOptions(serverHttpClient.get))`, and renders `<HydrationBoundary state={dehydrate(queryClient)}><RoomList /></HydrationBoundary>`.
- [x] Rewrite `web/src/app/(main)/rooms/[roomId]/page.tsx` the same way, prefetching room, messages, and models in parallel (`Promise.all`) before rendering `<ChatRoom roomId={roomId} />` inside the `HydrationBoundary`.
- [x] Rewrite `web/src/features/rooms/components/RoomList.tsx` to read rooms via `useRooms()` (no local `rooms`/`isLoading` state, no `fetchRooms`/`useEffect`); keep the loading skeleton driven by `query.isPending`.
- [x] Rewrite `web/src/features/rooms/components/CreateRoomForm.tsx` to call `useCreateRoom()` directly (mutation invalidates the `["rooms"]` query on success) instead of receiving an `onCreated` callback that mutates parent state; remove the now-unused `onCreated` prop from `RoomList`.
- [x] Rewrite `web/src/features/messages/components/ChatRoom.tsx` to source `room`, `messages`, `models` from `useRoom(roomId)`, `useMessages(roomId)`, `useModels()`, and to call `useSendMessage(roomId)`, `useSendAIMessage(roomId)`, `useRegenerateAIMessage(roomId)` mutations instead of `apiClient.*` + manual `setMessages` splicing; keep the component's JSX/behavior otherwise unchanged (full decomposition is Step 18's job).
- [x] Rewrite `web/src/features/auth/components/LoginForm.tsx` and `RegisterForm.tsx` to call `useLogin()`/`useRegister()` mutations instead of `useAuth()`; keep the existing local `error`/inline-`Box` error display as-is (toaster-based error surfaces are Step 17's job) and drive the submit button's `loading` state from `mutation.isPending`.
- [x] Wire the logout button in `RoomList.tsx` to `useLogout()`.
- [x] Locate Step 5's MSW handler module(s) and MSW bootstrap (per Step 5's scope: `web/src/test/msw/handlers.ts`, `web/src/test/msw/server.ts`, `web/src/test/msw/browser.ts`, wired via `web/src/test/setup.ts`) and split the handlers into `web/src/features/auth/api/handlers.ts`, `web/src/features/rooms/api/handlers.ts`, `web/src/features/messages/api/handlers.ts`, each exporting an array of `msw` `http.*` handlers scoped to that feature's `/api/proxy/*` endpoints; update the shared bootstrap to `setupServer(...authHandlers, ...roomsHandlers, ...messagesHandlers)` (or the worker equivalent) instead of importing one monolithic handlers file. Delete the monolithic handlers file once empty.
- [x] Delete the `MockApiClient` class and the `IS_MOCK`/`NEXT_PUBLIC_MOCK_API` branch entirely (it is now covered by `web/src/features/*/api/handlers.ts` in tests) — do not touch `docker-compose.yml`/`Taskfile.yml`; the now-unused `NEXT_PUBLIC_MOCK_API` build arg there is harmless and out of scope for this step.
- [x] Add `web/src/features/auth/hooks/use-session.test.tsx` (RTL + MSW, wrapped in a test `QueryClientProvider`): the auth/session hook test deferred from Step 5, exercised against the cookie-based flow — covers authenticated (`200` with the `/users/me` body) and unauthenticated (`401`) responses from the mocked session endpoint (MSW handler pinned to `GET /api/proxy/users/me`, per the session-source note in the Types section of the Implementation notes).
- [x] Add at least one hook test per feature demonstrating the MSW migration works end to end: `web/src/features/rooms/hooks/use-rooms.test.tsx` (list renders from a mocked `GET /api/proxy/rooms`) and `web/src/features/messages/hooks/use-messages.test.tsx` (mocked `GET /api/proxy/rooms/:roomId/messages`).
- [x] Add a component-level test that a mutation invalidates and refetches correctly, e.g. `web/src/features/rooms/components/CreateRoomForm.test.tsx` asserting the rooms list query is invalidated after a successful create.
- [x] Run `bun run lint` and fix any resulting issues; run the vitest suite added by Step 5 and confirm all new/updated tests pass.

## Out of scope
- Any change to `docker-compose.yml` or `Taskfile.yml` (ownership of those files is assigned to other steps per the plan's hot-file convention).
- Rewriting `LoginForm`/`RegisterForm` error UX onto a toaster, or any App Router `error.tsx`/`loading.tsx` boundaries — Step 17.
- Decomposing `ChatRoom.tsx` into smaller components, virtualized/infinite message lists, or cursor-based pagination via `useInfiniteQuery` — Step 18. This step keeps the existing single-page `listMessages(roomId, undefined, 100)` fetch shape, just moved onto `useQuery`.
- The persistent sidebar/room-switcher layout — Step 16.
- Any WebSocket wiring or streaming; this step only leaves the query cache in a shape (stable `queryKey`s, `setQueryData`-friendly message arrays) that a future WS event handler or SSE stream can write into.
- Adding Anthropic/Gemini model metadata, token limits, or pricing to `ModelInfo` — Step 34.
- Changes to the Go server or LLM Gateway; this is a web-only step.
- Any change to the Step-4 cookie/session mechanics, the middleware guard, or the `/api/proxy/[...path]` route handler itself — this step only *consumes* them.

## Implementation notes

### Assumed Step 4 / Step 5 contracts
This step is written assuming Step 4 and Step 5 landed with the following shape. If the actual file names differ, adapt the import paths below accordingly — the contracts that matter are: **(a)** all client-side data calls go through `/api/proxy/*` with the session cookie attached automatically by the browser, and **(b)** a shared MSW handler set exists and is bootstrapped for the vitest/RTL environment.
- A typed client-side HTTP wrapper, assumed at `web/src/lib/http-client.ts`, exporting an object such as `httpClient` with `get<T>(path)`, `post<T>(path, body)`, `patch<T>(path, body)`, `delete<T>(path)` methods that call relative `/api/proxy${path}` URLs (the browser attaches the httpOnly session cookie automatically; the proxy route attaches the `Authorization: Bearer` header server-side).
- `web/src/middleware.ts` guarding `/rooms` and `/rooms/*` — no changes needed here, but it is why RSC prefetch failures (401) should be allowed to throw and let the middleware's redirect already have handled the unauthenticated case upstream.
- Step 5's MSW setup under `web/src/test/msw/` (`handlers.ts` + `server.ts` using `setupServer` for Node/vitest + `browser.ts`, wired into `web/src/test/setup.ts`), with handlers pinned to `/api/proxy/*` paths, and RTL utilities (a custom `render` wrapping the Chakra `Provider`) at `web/src/test/render.tsx`.

### QueryClient (new)
`web/src/lib/query-client.ts` — follow the standard TanStack Query Next.js App Router pattern:
```ts
import { QueryClient, isServer } from "@tanstack/react-query"

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { staleTime: 60 * 1000 },
    },
  })
}

let browserQueryClient: QueryClient | undefined

export function getQueryClient() {
  if (isServer) return makeQueryClient()
  if (!browserQueryClient) browserQueryClient = makeQueryClient()
  return browserQueryClient
}
```

### Server-only fetcher for RSC prefetch (new)
`web/src/lib/http-client.server.ts`:
```ts
import "server-only"
import { headers } from "next/headers"

const APP_INTERNAL_URL = process.env.APP_INTERNAL_URL ?? "http://localhost:3000"

async function request<T>(path: string): Promise<T> {
  const incoming = await headers()
  const res = await fetch(`${APP_INTERNAL_URL}/api/proxy${path}`, {
    headers: { cookie: incoming.get("cookie") ?? "" },
    cache: "no-store",
  })
  if (!res.ok) throw new Error(`Request failed: ${res.status}`)
  if (res.status === 204) return undefined as T
  return res.json()
}

export const serverHttpClient = {
  get: <T>(path: string) => request<T>(path),
}
```
This deliberately re-enters the app's own `/api/proxy/*` route instead of re-deriving the Bearer token from the cookie, so the cookie→Bearer translation stays in exactly one place (Step 4's proxy route handler).

### Per-feature `api/` module pattern (canonical example)
Each query-capable resource exports a fetcher and a `queryOptions()` factory parameterized by fetcher, so the same factory prefetches on the server and fetches on the client:
```ts
// web/src/features/rooms/api/get-rooms.ts
import { queryOptions } from "@tanstack/react-query"
import { httpClient } from "@/lib/http-client"
import type { Room } from "../types"

type Fetcher = <T>(path: string) => Promise<T>

export function getRoomsQueryOptions(fetcher: Fetcher = httpClient.get) {
  return queryOptions({
    queryKey: ["rooms"] as const,
    queryFn: () => fetcher<Room[]>("/rooms"),
  })
}
```
```ts
// web/src/features/rooms/hooks/use-rooms.ts
"use client"
import { useQuery } from "@tanstack/react-query"
import { getRoomsQueryOptions } from "../api/get-rooms"

export function useRooms() {
  return useQuery(getRoomsQueryOptions())
}
```
```ts
// web/src/app/(main)/rooms/page.tsx
import { HydrationBoundary, dehydrate } from "@tanstack/react-query"
import { getQueryClient } from "@/lib/query-client"
import { serverHttpClient } from "@/lib/http-client.server"
import { getRoomsQueryOptions } from "@/features/rooms/api/get-rooms"
import { RoomList } from "@/features/rooms/components/RoomList"

export default async function RoomsPage() {
  const queryClient = getQueryClient()
  await queryClient.prefetchQuery(getRoomsQueryOptions(serverHttpClient.get))
  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <RoomList />
    </HydrationBoundary>
  )
}
```
Apply the same shape to `getRoomQueryOptions(roomId, fetcher)` (`GET /rooms/:roomId`), `getMessagesQueryOptions(roomId, fetcher)` (`GET /rooms/:roomId/messages?limit=100`, mirroring the existing `apiClient.listMessages(roomId, undefined, 100)` call in `ChatRoom.tsx`, reversed for display exactly as today), `getModelsQueryOptions(fetcher)` (`GET /models`), and `getSessionQueryOptions(fetcher)` (`GET /users/me`, Step 1's whoami endpoint, via the proxy; a `401` maps to a signed-out state, not an error). Query keys: `["rooms"]`, `["rooms", roomId]`, `["rooms", roomId, "messages"]`, `["models"]`, `["auth", "session"]` — keep these stable; later WS/streaming steps append to the `["rooms", roomId, "messages"]` cache entry via `queryClient.setQueryData`.

### Mutations
Mutations are client-only (no server prefetch variant needed). Example:
```ts
// web/src/features/rooms/api/create-room.ts
import { httpClient } from "@/lib/http-client"
import type { Room } from "../types"

export function createRoom(input: { name: string; description: string }) {
  return httpClient.post<Room>("/rooms", input)
}
```
```ts
// web/src/features/rooms/hooks/use-create-room.ts
"use client"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { createRoom } from "../api/create-room"

export function useCreateRoom() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createRoom,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["rooms"] }),
  })
}
```
Apply the same shape to `useDeleteRoom` (invalidate `["rooms"]`), `useLogin`/`useRegister` (on success, `invalidateQueries({ queryKey: ["auth", "session"] })` then `useRouter().push("/rooms")`, replacing the `router.push` calls currently in `web/src/hooks/use-auth.ts`), and `useLogout` (invalidate `["auth", "session"]`, `router.push("/login")`). For `useSendMessage`/`useSendAIMessage`/`useRegenerateAIMessage`, prefer `queryClient.setQueryData(["rooms", roomId, "messages"], (old: Message[] = []) => [...])` over a full invalidate so the append-only UX in `ChatRoom.tsx` (today's `setMessages((prev) => [...prev, msg])`) is preserved — `useRegenerateAIMessage` replaces the matching message by id (`old.map((m) => (m.id === messageId ? updated : m))`), matching today's behavior in `ChatRoom.tsx`'s `handleRegenerate`.

### Component rewiring
- `web/src/features/rooms/components/RoomList.tsx`: replace `useState<Room[]>`/`fetchRooms`/`useEffect` with `const { data: rooms = [], isPending } = useRooms()`; replace the local `logout` from `useAuth()` with `useLogout().mutate`; drop `handleRoomCreated`/`onCreated` prop plumbing to `CreateRoomForm`.
- `web/src/features/rooms/components/CreateRoomForm.tsx`: drop the `onCreated` prop; call `const { mutateAsync: createRoom, isPending } = useCreateRoom()` and `await createRoom({ name, description })` in `handleSubmit`, closing the dialog and resetting fields in `onSuccess`/after `await`.
- `web/src/features/messages/components/ChatRoom.tsx`: replace the `fetchData`/`Promise.all` effect with `useRoom(roomId)`, `useMessages(roomId)`, `useModels()`; combine `isPending` across the three for the existing full-page spinner; replace `handleSend`/`handleSendWithAI`/`handleRegenerate` bodies with the corresponding mutation's `mutateAsync`, keeping the same function signatures so `MessageInput`/`MessageList`/`ModelSelector` props are untouched.
- `web/src/features/auth/components/LoginForm.tsx` / `RegisterForm.tsx`: replace `useAuth()` with `useLogin()`/`useRegister()`; call `await login({ email, password })` / `await register({ email, username, password })` inside the existing `try`/`catch`, using `mutation.isPending` for the submit button's `loading` prop instead of local `isSubmitting` state.
- `web/src/app/layout.tsx`: wrap `<Provider>{children}</Provider>` with `<QueryProvider>` (outermost), and delete the `<MockBadge />` line and its import.

### Types
Split `web/src/types/api.ts` along feature lines: `AuthResponse`/`User` → `web/src/features/auth/types.ts` (the session source is Step 1's authenticated `GET /users/me`, called through the data-plane proxy as `/api/proxy/users/me` — it returns `{ id, email, username, created_at }`, so type `User` against exactly that and let `useSession` expose it directly; an unauthenticated call surfaces as the proxy's `401`, which `useSession` maps to a signed-out state rather than an error. No BFF-side session route or JWT decoding is needed. Step 30 later rewrites `get-session.ts` against Kratos `whoami` keeping the same `["auth", "session"]` query key); `Room` → `web/src/features/rooms/types.ts`; `MessageType`, `MessageStatus`, `Message`, `MessagePage`, `AIMessageResponse`, `ModelInfo`, `ModelListResponse` → `web/src/features/messages/types.ts`. `ApiError` can be dropped if unused after the migration (the Step-4 `httpClient` should already normalize errors).

### Chakra UI conventions
No new Chakra components are introduced by this step; keep all existing `@chakra-ui/react` imports and Compound Component usage in `RoomList.tsx`/`ChatRoom.tsx`/`CreateRoomForm.tsx` exactly as they are — only the data-fetching plumbing changes, per `.claude/rules/chakra-ui.md`.

### Conflict notes
This step finishes the `api.ts` split Step 4 began — Step 4 owns `web/src/lib/http-client.ts`, `web/src/middleware.ts`, and `web/src/app/api/proxy/[...path]/route.ts`; this step must not modify those, only import from `http-client.ts`. `ChatRoom.tsx` is edited here for data-fetching only; its JSX structure is intentionally left alone for Step 18 to decompose, and `MessageList`/`MessageInput` ownership per the plan-wide notes doesn't rotate to this step. Every later web step that touches the query cache (WebSocket merge in later waves, streaming chunks) must reuse the `queryKey`s defined here rather than inventing new ones.

## Verification
1. `cd web && bun install` — installs `@tanstack/react-query`/`@tanstack/react-query-devtools` cleanly.
2. `cd web && bun run lint` — passes with no errors, and `grep -rn "apiClient\|IS_MOCK\|@/lib/api\|@/types/api\|@/hooks/use-auth" src` returns no matches.
3. `cd web && bunx vitest run` (or whatever script Step 5 wired into `package.json`, e.g. `bun run test`) — all existing tests plus the new `use-session.test.tsx`, `use-rooms.test.tsx`, `use-messages.test.tsx`, and `CreateRoomForm.test.tsx` pass.
4. `cd web && bun run build` — the production build succeeds, confirming the Server Components in `app/(main)/rooms/page.tsx` and `app/(main)/rooms/[roomId]/page.tsx` compile and the `serverHttpClient` module (marked `server-only`) is never pulled into a client bundle.
5. `docker compose up -d db api llm-gateway web` then, after logging in through the UI at `http://localhost:3000/login` (or via the BFF login route Step 4 exposes), open browser devtools: the initial HTML response for `/rooms` already contains the room list (view-source shows the dehydrated query state), confirming RSC prefetch worked; opening a room shows messages/models without an additional client-side loading spinner flash on first paint.
6. In the same running app, create a room via the UI and confirm the list updates without a full page reload (mutation-driven cache invalidation), then send a message and an AI message in a room and confirm both appear without a page reload (mutation-driven `setQueryData`).

## Completion criteria
- [x] `web/src/lib/api.ts`, `web/src/lib/mock-data.ts`, `web/src/types/api.ts`, `web/src/components/mock-badge.tsx`, `web/src/hooks/use-auth.ts` are deleted and nothing imports them.
- [x] `web/src/features/{auth,rooms,messages}` each have `types.ts`, `api/`, and `hooks/` as scoped above.
- [x] `web/src/app/(main)/rooms/page.tsx` and `web/src/app/(main)/rooms/[roomId]/page.tsx` are Server Components using `getQueryClient()` + `prefetchQuery` + `HydrationBoundary`.
- [x] `RoomList.tsx`, `ChatRoom.tsx`, `CreateRoomForm.tsx`, `LoginForm.tsx`, `RegisterForm.tsx` use the new hooks exclusively; no component holds its own fetched-data `useState`/`useEffect` pair for server data.
- [x] Step 5's MSW handlers are split into `web/src/features/{auth,rooms,messages}/api/handlers.ts` and the shared MSW bootstrap assembles them; the old monolithic handlers file no longer exists.
- [x] `use-session.test.tsx` (new, cookie-based) plus at least one hook test per feature and one mutation-invalidation component test exist and pass.
- [x] All verification checks above pass. (1-4 run and pass in this worktree; 5-6 require the full `docker compose` stack on fixed ports and are left for the post-merge integration review — see skippedComposeChecks.)
