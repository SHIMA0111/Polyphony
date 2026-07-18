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
  // 15s assertion timeout (default 5s): under full-suite parallelism the
  // whole stack shares one machine with Kratos's argon2 hashing, the
  // streaming AI pipeline, and multiple browser contexts -- individually
  // fast assertions (post-registration navigation, WS-delivered messages,
  // AI replies) routinely exceeded 5s and flaked (wave-7 integration run).
  // Genuinely broken assertions still fail, just a little slower.
  expect: { timeout: 15_000 },
  // Cap workers below Playwright's cpus/2 default: the entire compose test
  // stack (Next SSR, Kratos hashing, Go API, gateway, three Postgres
  // instances) shares a Docker VM with far fewer CPUs than the host, and
  // 8 concurrent browser flows starve it -- UI registrations stalled past
  // even the 15s assertion timeout (wave-7 integration run). Override with
  // `--workers` for machines with a beefier Docker VM.
  workers: 4,
  reporter: [["html", { outputFolder: "e2e-report", open: "never" }]],
  globalSetup: "./e2e/seed/seed.ts",
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3001",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      // Step 57's rate-limiting spec runs in its own, strictly-later
      // project (below): its Case 2 recreates `api-e2e` with a deliberately
      // tightened AI-invoke limit, and when that window overlaps another
      // worker's AI send (attachments, private-mode, streaming, ...) that
      // spec's reply 429s and flakes (caught live by the wave-7 integration
      // run). Excluded here, run by the `rate-limiting` project after the
      // rest of the suite has finished.
      testIgnore: /regression\/rate-limiting\.spec\.ts/,
      use: {
        ...devices["Desktop Chrome"],
        // Step 44 (oauth-dex.spec.ts): dex's `issuer` is the Docker-internal
        // hostname `dex` (resolved automatically by Kratos server-side via
        // Docker's DNS), but the OIDC authorization redirect sends *this*
        // browser to that same hostname — which the host does not resolve
        // by default. This maps it to the e2e stack's dedicated `dex-e2e`
        // service's published host port (8095, not the dev `dex` service's
        // 5556 — see docker-compose.yml's `dex-e2e` block, added so the two
        // stacks' dex containers no longer collide on the same host port)
        // without requiring a manual `/etc/hosts` edit (see ory/README.md
        // for that equivalent, for anyone testing social login by hand
        // against the dev stack).
        launchOptions: {
          args: ["--host-resolver-rules=MAP dex:5556 127.0.0.1:8095"],
        },
      },
    },
    {
      // See the chromium project's testIgnore comment: this project exists
      // solely to serialize the rate-limiting spec *after* every other spec,
      // so its tightened-limit `api-e2e` recreate window can never overlap
      // another worker's AI sends. Run it standalone with
      // `bunx playwright test e2e/regression/rate-limiting.spec.ts --no-deps`
      // (without `--no-deps`, the chromium dependency project runs first,
      // ignoring CLI file filters).
      name: "rate-limiting",
      testMatch: /regression\/rate-limiting\.spec\.ts/,
      dependencies: ["chromium"],
      use: {
        ...devices["Desktop Chrome"],
      },
    },
  ],
})
