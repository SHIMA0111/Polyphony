import type { APIRequestContext } from "@playwright/test"

/**
 * Base URL of the test-profile Go API, reachable from the host — matches
 * `e2e/seed/seed.ts`'s `E2E_API_URL` constant exactly. `request` (a
 * standalone Playwright `APIRequestContext`, not tied to any browser
 * `page`) has no `baseURL` configured for it in `playwright.config.ts`
 * (that config's `use.baseURL` is the *web* app at `localhost:3001`), so
 * every call below uses a fully-qualified URL against this constant instead
 * of a relative path.
 */
const E2E_API_URL = process.env.E2E_API_URL ?? "http://localhost:8090"

/**
 * The model `seedLongHistoryRoom`'s seed loop and self-verification
 * estimate both target. Must match `DEFAULT_AI_MODEL`'s e2e-stack default
 * (`docker-compose.yml`'s `api-e2e` service does not override
 * `DEFAULT_AI_MODEL`, so the Go API falls back to
 * `server/internal/infrastructure/config/config.go`'s
 * `defaultDefaultAIModel`, `"gpt-5-mini"`), since the final AI-triggering
 * send a calling spec issues after seeding never specifies a model either.
 */
const MODEL = "gpt-5-mini"

/**
 * # Step 50's actual summarization threshold (discovered, not guessed)
 *
 * There is no env-tunable threshold and no e2e-only override anywhere in
 * `docker-compose.yml`/`server/internal/infrastructure/config/config.go` —
 * confirmed by grepping `-i "summar"` across both, which turns up nothing.
 * The real trigger, read from the merged Step 50 code
 * (`server/internal/usecase/message/context.go`'s `assembleAIContext`,
 * `server/internal/usecase/message/usecase.go`), is:
 *
 * 1. Only the newest `defaultContextMessages = 50` messages
 *    (`usecase.go`) are ever fetched as candidate context for an AI send —
 *    `CONTEXT_MESSAGE_WINDOW` below mirrors that constant, since seeding
 *    more than 50 messages could never change whether the *next* AI call's
 *    context overflows.
 * 2. Of those, the newest `summaryRecentTailCount = 10` (`context.go`) are
 *    always kept verbatim; the remainder splits into an older-private and
 *    an older-public bucket, only the latter of which is ever eligible for
 *    summarization.
 * 3. The full verbatim concatenation of all three buckets (`verbatim()` in
 *    `context.go`) is estimated via `ai.LLMGateway.EstimateTokens` and
 *    compared against `budget = ai.ResolveContextWindow(models, model) -
 *    reservedOutputTokens`, where `reservedOutputTokens = 1024`
 *    (`context.go`). Only once `estimate > budget` is the older-public
 *    bucket replaced by a cached/freshly-computed summary.
 * 4. `ai.ResolveContextWindow` reports `gpt-5-mini`'s context window as
 *    272,000 tokens (`llm-gateway/src/adapters/outbound/openai/mod.rs`'s
 *    hardcoded model table, source of truth for both the Go API's
 *    best-effort `ListModels` call and the Rust gateway's own token
 *    estimator).
 *
 * So: `budget = 272_000 - 1024 = 270_976` tokens. This constant is that
 * number, documented so a future change to Step 50's threshold or the
 * gateway's context-window table fails loudly here (via the "never crossed
 * the threshold" error below) instead of silently producing a spec that no
 * longer exercises summarization at all.
 */
const SUMMARIZATION_TOKEN_BUDGET = 272_000 - 1_024

/**
 * Comfortable margin over `SUMMARIZATION_TOKEN_BUDGET` the seed loop targets
 * before it considers the room "long enough" to self-verify against, so
 * small estimator drift (the heuristic character-based estimate the Go API
 * asks the gateway for is not byte-identical between this helper's locally
 * constructed probe messages and the server's own `assembleAIContext` call)
 * can never accidentally land just under the real trigger.
 */
const MARGIN = 1.5

/** The token count the seed loop's self-check must reach or exceed. */
const TARGET_TOKENS = Math.ceil(SUMMARIZATION_TOKEN_BUDGET * MARGIN)

/**
 * Mirrors `defaultContextMessages` (`server/internal/usecase/message/
 * usecase.go`): only the newest this-many messages are ever fetched as
 * context, so the self-verification estimate below is built from at most
 * this many filler messages, matching what the real budget check would
 * actually see.
 */
const CONTEXT_MESSAGE_WINDOW = 50

/**
 * Hard cap on the number of plain messages this helper will ever send,
 * regardless of whether the token estimate has crossed `TARGET_TOKENS` yet.
 * Exists purely so a miscalibrated threshold/filler size fails fast with a
 * descriptive error (see the loop below) instead of silently sending
 * hundreds of messages forever.
 */
const MAX_MESSAGES = 300

/** How many messages to send between each self-verification estimate call. */
const ESTIMATE_CHECK_BATCH = 5

/**
 * Length (in characters) of each seeded filler message. Chosen so that
 * `CONTEXT_MESSAGE_WINDOW` (50) messages of this size alone estimate to
 * comfortably more than `TARGET_TOKENS`: the gateway's heuristic estimator
 * (`llm-gateway/src/domain/token_estimator.rs`) computes
 * `ceil(chars / 4) + 4` tokens per message, so 50 messages of 33,000
 * characters each estimate to roughly
 * `50 * (33_000 / 4 + 4) ≈ 412,700` tokens — above `TARGET_TOKENS`
 * (`≈ 406,464`) with headroom, while staying well under Postgres's
 * unbounded `TEXT` column (no server-side content length cap exists;
 * `SendMessageRequest.Content` is only checked non-empty,
 * `message_handler.go`).
 */
const FILLER_CHAR_COUNT = 33_000

/** Repeating phrase padded/truncated to exactly `FILLER_CHAR_COUNT` characters. */
const FILLER_MESSAGE = "Polyphony e2e summarization regression filler content. "
  .repeat(Math.ceil(FILLER_CHAR_COUNT / 57))
  .slice(0, FILLER_CHAR_COUNT)

interface TokenResponse {
  access_token: string
  token_type: string
}

interface RoomResponse {
  id: string
  name: string
}

interface TokenEstimateResponse {
  model: string
  estimated_tokens: number
}

export interface SeedLongHistoryRoomOptions {
  /**
   * Email of an already-registered user to reuse instead of registering a
   * fresh one. Must be paired with `userPassword`. When omitted (the
   * common case), a brand-new, uniquely-named user is registered instead —
   * this helper must be safe to call many times without interfering with
   * other specs' state, so it never reuses the shared Step 10 fixture
   * user/room by default.
   */
  userEmail?: string
  /** Password for `userEmail`. Required whenever `userEmail` is given. */
  userPassword?: string
  /** Room name to use instead of a generated, run-unique default. */
  roomName?: string
}

export interface SeedLongHistoryRoomResult {
  /** The seeded room's id. */
  roomId: string
  /** The seeded room's name (generated or `opts.roomName`). */
  roomName: string
  /** Number of plain filler messages actually sent. */
  messageCount: number
  /**
   * The final self-verification estimate (via `POST /tokens/estimate`) over
   * the newest `min(messageCount, CONTEXT_MESSAGE_WINDOW)` filler messages —
   * guaranteed to be `>= TARGET_TOKENS` (a `1.5x` margin over Step 50's
   * actual, documented summarization threshold) when this function returns
   * successfully.
   */
  estimatedTokens: number
}

/** Registers a brand-new, uniquely-named user directly against the Go API and returns a bearer access token. */
async function registerFreshUser(
  request: APIRequestContext,
  runId: string,
): Promise<{ email: string; password: string; accessToken: string }> {
  const email = `seedlh-${runId}@polyphony.test`
  // Underscores only, <=32 chars: registerSchema's username regex
  // (`/^[a-zA-Z0-9_]+$/`) — matches every other spec's convention, even
  // though this helper registers via direct REST (not the client-validated
  // form), so nothing here strictly requires it, but keeping the same shape
  // avoids any server-side validation surprises.
  const username = `seedlh_${runId}`.slice(0, 32)
  const password = "seedlh-test-password-123"

  const accessToken = await registerOrLoginUser(request, email, username, password)
  return { email, password, accessToken }
}

/**
 * Registers `email`/`username`/`password`, falling back to logging in if the
 * account already exists (HTTP non-2xx from `/auth/register`, mirroring
 * `e2e/seed/seed.ts`'s `ensureFixtureUser` idempotency pattern) — in
 * practice this only matters for the timestamp+random-collision edge case,
 * since every caller of this helper generates a fresh, run-unique identity.
 */
async function registerOrLoginUser(
  request: APIRequestContext,
  email: string,
  username: string,
  password: string,
): Promise<string> {
  const registerRes = await request.post(`${E2E_API_URL}/auth/register`, {
    data: { email, username, password },
  })
  if (registerRes.ok()) {
    const body = (await registerRes.json()) as TokenResponse
    return body.access_token
  }

  return loginUser(request, email, password)
}

/** Logs in `email`/`password` against the Go API and returns a bearer access token. */
async function loginUser(request: APIRequestContext, email: string, password: string): Promise<string> {
  const loginRes = await request.post(`${E2E_API_URL}/auth/login`, {
    data: { email, password },
  })
  if (!loginRes.ok()) {
    throw new Error(
      `seedLongHistoryRoom: failed to register or log in ${email} (login: HTTP ${loginRes.status()})`,
    )
  }
  const body = (await loginRes.json()) as TokenResponse
  return body.access_token
}

/**
 * Builds a room whose accumulated visible history is long enough to force
 * Step 50's server-side context summarization on the next AI-triggering
 * send, without needing dozens of real LLM round-trips: seeds a bounded
 * loop of plain (non-AI) `POST /rooms/:roomId/messages` calls with a fixed,
 * ~33,000-character filler payload each, self-verifying via `POST
 * /tokens/estimate` (Step 27) after every {@link ESTIMATE_CHECK_BATCH}-sized
 * batch that the estimated token count across the newest
 * {@link CONTEXT_MESSAGE_WINDOW} messages has crossed {@link TARGET_TOKENS}
 * (a `1.5x` margin over Step 50's actual, documented summarization
 * threshold — see this file's header comment for how that threshold was
 * discovered from the merged code).
 *
 * Uses direct REST calls against `E2E_API_URL` (default
 * `http://localhost:8090`) — the same pattern `e2e/seed/seed.ts` already
 * establishes — rather than driving a browser, since seeding dozens of
 * large messages through the UI would be far slower and is unnecessary: no
 * assertion in this step's specs depends on the seed messages themselves
 * being rendered.
 *
 * @param request - A Playwright `APIRequestContext` (the standalone
 *   `request` test fixture, not `page.request`): this helper talks directly
 *   to the Go API rather than through the web app's `/api/proxy` BFF route,
 *   so it works independent of any browser `page`/session.
 * @param opts - See {@link SeedLongHistoryRoomOptions}.
 * @returns See {@link SeedLongHistoryRoomResult}.
 * @throws If registration/login, room creation, a message send, or a token
 *   estimate call fails, or if `MAX_MESSAGES` is reached without the
 *   estimate ever crossing {@link TARGET_TOKENS} (signaling that Step 50's
 *   threshold or the gateway's context-window table has changed since the
 *   constants above were last calibrated).
 */
export async function seedLongHistoryRoom(
  request: APIRequestContext,
  opts: SeedLongHistoryRoomOptions = {},
): Promise<SeedLongHistoryRoomResult> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`

  let accessToken: string
  if (opts.userEmail && opts.userPassword) {
    accessToken = await loginUser(request, opts.userEmail, opts.userPassword)
  } else {
    const fresh = await registerFreshUser(request, runId)
    accessToken = fresh.accessToken
  }

  const authHeaders = { Authorization: `Bearer ${accessToken}` }
  const roomName = opts.roomName ?? `Long History Room ${runId}`

  const roomRes = await request.post(`${E2E_API_URL}/rooms`, {
    headers: authHeaders,
    data: {
      name: roomName,
      description: "Seeded long-history room for Step 58's summarization regression spec.",
    },
  })
  if (!roomRes.ok()) {
    throw new Error(`seedLongHistoryRoom: failed to create room: HTTP ${roomRes.status()}`)
  }
  const room = (await roomRes.json()) as RoomResponse
  const roomId = room.id

  let messageCount = 0
  let estimatedTokens = 0
  let crossedThreshold = false

  while (messageCount < MAX_MESSAGES) {
    const sendRes = await request.post(`${E2E_API_URL}/rooms/${roomId}/messages`, {
      headers: authHeaders,
      data: { content: `${FILLER_MESSAGE} [seed #${messageCount + 1}]` },
    })
    if (!sendRes.ok()) {
      throw new Error(
        `seedLongHistoryRoom: failed to seed message #${messageCount + 1} in room ${roomId}: HTTP ${sendRes.status()}`,
      )
    }
    messageCount++

    if (messageCount % ESTIMATE_CHECK_BATCH !== 0) continue

    const windowSize = Math.min(messageCount, CONTEXT_MESSAGE_WINDOW)
    const estimateRes = await request.post(`${E2E_API_URL}/tokens/estimate`, {
      headers: authHeaders,
      data: {
        model: MODEL,
        messages: Array.from({ length: windowSize }, () => ({
          role: "user",
          content: FILLER_MESSAGE,
        })),
      },
    })
    if (!estimateRes.ok()) {
      throw new Error(
        `seedLongHistoryRoom: POST /tokens/estimate failed after seeding ${messageCount} messages: HTTP ${estimateRes.status()}`,
      )
    }
    const estimateBody = (await estimateRes.json()) as TokenEstimateResponse
    estimatedTokens = estimateBody.estimated_tokens

    if (estimatedTokens >= TARGET_TOKENS) {
      crossedThreshold = true
      break
    }
  }

  if (!crossedThreshold) {
    throw new Error(
      `seedLongHistoryRoom: seeded ${messageCount} messages (cap ${MAX_MESSAGES}) but the estimated token count across the newest ${CONTEXT_MESSAGE_WINDOW}-message window only reached ${estimatedTokens}, never crossing the ${TARGET_TOKENS}-token target (a 1.5x margin over Step 50's documented ${SUMMARIZATION_TOKEN_BUDGET}-token summarization threshold). Step 50's threshold or the gateway's context-window table may have changed -- recalibrate this file's constants.`,
    )
  }

  return { roomId, roomName, messageCount, estimatedTokens }
}
