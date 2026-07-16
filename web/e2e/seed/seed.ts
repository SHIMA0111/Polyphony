import { FIXTURE_ROOM_NAME, FIXTURE_USER, HYDRA_DEMO_FIXTURE_USER } from "../support/fixtures"

/** Base URL of the test-profile Go API, reachable from the host. */
const E2E_API_URL = process.env.E2E_API_URL ?? "http://localhost:8090"

const HEALTH_POLL_TIMEOUT_MS = 30_000
const HEALTH_POLL_INTERVAL_MS = 1_000

interface TokenResponse {
  access_token: string
  token_type: string
}

interface RoomResponse {
  id: string
  name: string
  description: string
  owner_id: string
  created_at: string
  updated_at: string
}

/**
 * Polls `GET {E2E_API_URL}/health` until it responds 200, or throws once
 * `HEALTH_POLL_TIMEOUT_MS` has elapsed.
 *
 * `docker compose up -d` returns before `api-e2e` has finished booting (DB
 * connection, migrations having just run in `migrate-e2e`), so callers must
 * not assume the API is reachable immediately after the stack "starts".
 */
async function waitForHealth(): Promise<void> {
  const deadline = Date.now() + HEALTH_POLL_TIMEOUT_MS
  let lastError: unknown

  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${E2E_API_URL}/health`)
      if (res.ok) return
      lastError = new Error(`/health returned HTTP ${res.status}`)
    } catch (err) {
      lastError = err
    }
    await new Promise((resolve) => setTimeout(resolve, HEALTH_POLL_INTERVAL_MS))
  }

  throw new Error(
    `E2E API at ${E2E_API_URL} did not become healthy within ${HEALTH_POLL_TIMEOUT_MS}ms: ${String(lastError)}`,
  )
}

/**
 * Registers the fixture user, falling back to login if the account already
 * exists (the Go API returns HTTP 409 for a duplicate email/username, but
 * any non-2xx response from `/auth/register` is treated the same way here
 * so the seed is resilient to other "already exists" shapes).
 *
 * @returns The fixture user's access token.
 */
async function ensureFixtureUser(): Promise<string> {
  const registerRes = await fetch(`${E2E_API_URL}/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: FIXTURE_USER.email,
      username: FIXTURE_USER.username,
      password: FIXTURE_USER.password,
    }),
  })

  if (registerRes.ok) {
    const body = (await registerRes.json()) as TokenResponse
    return body.access_token
  }

  const loginRes = await fetch(`${E2E_API_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: FIXTURE_USER.email,
      password: FIXTURE_USER.password,
    }),
  })

  if (!loginRes.ok) {
    throw new Error(
      `failed to register or log in fixture user (register: ${registerRes.status}, login: ${loginRes.status})`,
    )
  }

  const body = (await loginRes.json()) as TokenResponse
  return body.access_token
}

/**
 * Registers `HYDRA_DEMO_FIXTURE_USER`, falling back to login if the account
 * already exists -- same idempotency pattern as {@link ensureFixtureUser},
 * for `oauth-hydra.spec.ts` (Step 56). No access token is needed by that
 * spec (it logs in through the rendered UI to establish a real Kratos
 * browser session), so unlike {@link ensureFixtureUser} this helper does not
 * return one.
 */
async function ensureHydraDemoFixtureUser(): Promise<void> {
  const registerRes = await fetch(`${E2E_API_URL}/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: HYDRA_DEMO_FIXTURE_USER.email,
      username: HYDRA_DEMO_FIXTURE_USER.username,
      password: HYDRA_DEMO_FIXTURE_USER.password,
    }),
  })

  if (registerRes.ok) {
    return
  }

  const loginRes = await fetch(`${E2E_API_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email: HYDRA_DEMO_FIXTURE_USER.email,
      password: HYDRA_DEMO_FIXTURE_USER.password,
    }),
  })

  if (!loginRes.ok) {
    throw new Error(
      `failed to register or log in Hydra demo fixture user (register: ${registerRes.status}, login: ${loginRes.status})`,
    )
  }
}

/**
 * Ensures the fixture room exists for the fixture user, creating it only if
 * a room with `FIXTURE_ROOM_NAME` is not already present in the user's room
 * list — this is what makes re-running the seed against an already-seeded
 * database safe.
 *
 * @param accessToken - Bearer token for the fixture user.
 */
async function ensureFixtureRoom(accessToken: string): Promise<void> {
  const authHeaders = { Authorization: `Bearer ${accessToken}` }

  const listRes = await fetch(`${E2E_API_URL}/rooms`, { headers: authHeaders })
  if (!listRes.ok) {
    throw new Error(`failed to list rooms: HTTP ${listRes.status}`)
  }
  const rooms = (await listRes.json()) as RoomResponse[]
  if (rooms.some((room) => room.name === FIXTURE_ROOM_NAME)) {
    return
  }

  const createRes = await fetch(`${E2E_API_URL}/rooms`, {
    method: "POST",
    headers: { ...authHeaders, "Content-Type": "application/json" },
    body: JSON.stringify({
      name: FIXTURE_ROOM_NAME,
      description: "Seeded room for Playwright E2E fixtures.",
    }),
  })
  if (!createRes.ok) {
    throw new Error(`failed to create fixture room: HTTP ${createRes.status}`)
  }
}

/**
 * Idempotent E2E seed routine: waits for the test-profile API to be healthy,
 * then ensures the fixture user, fixture room, and Hydra demo fixture user
 * (`HYDRA_DEMO_FIXTURE_USER`, Step 56) exist. Safe to run repeatedly (e.g.
 * `task test:e2e:seed` run twice, or Playwright's
 * `globalSetup` running on every `task test:e2e` invocation without a
 * teardown in between).
 *
 * Exported as the default export so Playwright can use it directly as
 * `globalSetup` (see `playwright.config.ts`). This module intentionally has
 * no standalone CLI entrypoint of its own: Playwright's Node-based loader
 * transpiles `globalSetup` modules to CJS (`web/package.json` has no
 * `"type": "module"`), where Bun's `import.meta.main` guard is a
 * `SyntaxError`. The standalone `bun run` entrypoint lives in `./cli.ts`
 * instead (see the `test:e2e:seed` Taskfile task), which only ever runs
 * directly under Bun and is never imported by Playwright.
 */
export default async function globalSetup(): Promise<void> {
  await waitForHealth()
  const accessToken = await ensureFixtureUser()
  await ensureFixtureRoom(accessToken)
  await ensureHydraDemoFixtureUser()
}
