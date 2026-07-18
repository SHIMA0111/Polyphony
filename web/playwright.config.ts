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
      use: {
        ...devices["Desktop Chrome"],
        // Step 44 (oauth-dex.spec.ts): dex's `issuer` is the Docker-internal
        // hostname `dex` (resolved automatically by Kratos server-side via
        // Docker's DNS), but the OIDC authorization redirect sends *this*
        // browser to that same hostname — which the host does not resolve
        // by default. This maps it to dex's published host port without
        // requiring a manual `/etc/hosts` edit (see ory/README.md for that
        // equivalent, for anyone testing social login by hand).
        launchOptions: {
          args: ["--host-resolver-rules=MAP dex:5556 127.0.0.1:5556"],
        },
      },
    },
  ],
})
