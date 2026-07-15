import { setupWorker } from "msw/browser"
import { handlers } from "./handlers"

/**
 * Browser MSW worker sharing the same handlers as the Node `server` used by
 * Vitest. Exported for future interactive/dev-mode mocking; it is not started
 * anywhere in the app bootstrap yet — a caller must invoke `worker.start()`
 * explicitly once that wiring is added.
 */
export const worker = setupWorker(...handlers)
