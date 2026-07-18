import { setupWorker } from "msw/browser"
import { authHandlers } from "@/features/auth/api/handlers"
import { roomsHandlers } from "@/features/rooms/api/handlers"
import { messagesHandlers } from "@/features/messages/api/handlers"
import { membersHandlers } from "@/features/members/api/handlers"
import { billingHandlers } from "@/features/billing/api/handlers"
import { groupsHandlers } from "@/features/groups/api/handlers"

/**
 * Browser MSW worker sharing the same handlers as the Node `server` used by
 * Vitest. Exported for future interactive/dev-mode mocking; it is not started
 * anywhere in the app bootstrap yet — a caller must invoke `worker.start()`
 * explicitly once that wiring is added.
 */
export const worker = setupWorker(
  ...authHandlers,
  ...roomsHandlers,
  ...messagesHandlers,
  ...membersHandlers,
  ...billingHandlers,
  ...groupsHandlers,
)
