import { setupServer } from "msw/node"
import { authHandlers } from "@/features/auth/api/handlers"
import { roomsHandlers } from "@/features/rooms/api/handlers"
import { messagesHandlers } from "@/features/messages/api/handlers"
import { membersHandlers } from "@/features/members/api/handlers"

/**
 * Node MSW server instance used by Vitest (`src/test/setup.ts` starts/stops
 * it around the whole suite). Import this in individual tests to call
 * `server.use(...)` for per-test handler overrides (e.g. simulating an error
 * response).
 *
 * Handlers are assembled from each feature's own `api/handlers.ts` module
 * (Step 9 split of the original monolithic `src/test/msw/handlers.ts`) so
 * each feature owns its own MSW fixtures alongside its `api/`/`hooks/`
 * modules.
 */
export const server = setupServer(
  ...authHandlers,
  ...roomsHandlers,
  ...messagesHandlers,
  ...membersHandlers,
)
