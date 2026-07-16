import { http, HttpResponse } from "msw"
import type { User } from "../types"

/**
 * MSW request handlers for the auth feature, shared by the Node
 * `setupServer` (Vitest, see `src/test/msw/server.ts`) and the browser
 * `setupWorker` (`src/test/msw/browser.ts`).
 *
 * Covers the `/api/auth/*` BFF route handlers (Step 4: `login`/`register`/
 * `logout`, which never return the token itself) and the `/api/proxy/users/me`
 * session source consumed by `useSession`.
 */

export const fixtureUser: User = {
  id: "user-1",
  email: "user@example.com",
  username: "testuser",
  created_at: "2026-01-01T00:00:00Z",
}

export const authHandlers = [
  http.post("/api/auth/login", () => {
    return HttpResponse.json({ ok: true })
  }),

  http.post("/api/auth/register", () => {
    return HttpResponse.json({ ok: true })
  }),

  http.post("/api/auth/logout", () => {
    return HttpResponse.json({ ok: true })
  }),

  http.get("/api/proxy/users/me", () => {
    return HttpResponse.json<User>(fixtureUser)
  }),
]
