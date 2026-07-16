import { defineConfig, devices } from "@playwright/test"

/**
 * Playwright configuration for the E2E harness.
 *
 * Specs run against the isolated Docker Compose `test` profile stack (see
 * `docker-compose.yml` and `docs/tasks/step10.md`), never against the
 * developer's own `task up` dev stack. `globalSetup` seeds a fixed fixture
 * user/room (see `e2e/seed/seed.ts`) before any spec runs; specs that need a
 * fresh, unshared identity (e.g. `smoke.spec.ts`) register their own user
 * instead of relying on the seed.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: [["html", { outputFolder: "e2e-report", open: "never" }]],
  globalSetup: "./e2e/seed/seed.ts",
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3001",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
})
