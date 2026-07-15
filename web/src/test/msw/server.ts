import { setupServer } from "msw/node"
import { handlers } from "./handlers"

/**
 * Node MSW server instance used by Vitest (`src/test/setup.ts` starts/stops
 * it around the whole suite). Import this in individual tests to call
 * `server.use(...)` for per-test handler overrides (e.g. simulating an error
 * response).
 */
export const server = setupServer(...handlers)
