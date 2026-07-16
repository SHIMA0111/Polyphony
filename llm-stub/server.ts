import { join } from "node:path"

/**
 * `llm-stub`: a minimal, fully deterministic OpenAI-compatible HTTP server
 * used only by the E2E Docker Compose test profile (see
 * `docker-compose.yml`'s `llm-stub` service and `docs/tasks/step10.md`).
 *
 * It implements just enough of the OpenAI Chat Completions wire format for
 * `llm-gateway/src/adapters/outbound/openai/` to talk to it as if it were
 * the real OpenAI API (`OPENAI_BASE_URL` pointed at this service), so
 * Playwright specs get canned, reproducible AI responses instead of
 * depending on a live provider account.
 */

const FIXTURES_DIR = join(import.meta.dir, "fixtures")

/** A single OpenAI-style chat message as sent by the LLM Gateway. */
interface ChatMessage {
  role: string
  content: string
}

/** Shape of the request body the LLM Gateway's OpenAI adapter posts. */
interface CompletionRequestBody {
  model: string
  messages: ChatMessage[]
  stream?: boolean
}

/** Matches a `[[fixture:NAME]]` marker at the start of a message's content. */
const FIXTURE_MARKER = /^\[\[fixture:([^\]]+)]]/

/**
 * Allowed shape for a fixture name, whether it comes from the `[[fixture:NAME]]`
 * marker or falls back to `"default"`. Enforced before any fixture name is
 * joined into a filesystem path (see {@link loadNonStreamingFixture} and
 * {@link loadStreamingFixture}), so a marker like `[[fixture:../../etc/passwd]]`
 * can never escape `FIXTURES_DIR` via path traversal.
 */
const VALID_FIXTURE_NAME = /^[a-zA-Z0-9_-]+$/

/**
 * Determines which fixture to serve for a completion request.
 *
 * Inspects the last message with `role: "user"` in the request body: if its
 * content starts with `[[fixture:NAME]]`, that fixture name is used;
 * otherwise (including when there is no user message at all) the response
 * falls back to `"default"`.
 *
 * @param messages - The request body's `messages` array.
 * @returns The fixture name to load (`"default"` unless explicitly overridden).
 */
export function extractFixtureName(messages: ChatMessage[]): string {
  const lastUserMessage = [...messages].reverse().find((m) => m.role === "user")
  if (!lastUserMessage) return "default"

  const match = FIXTURE_MARKER.exec(lastUserMessage.content)
  return match ? match[1] : "default"
}

/**
 * Loads a non-streaming completion fixture from `fixtures/{name}.json`.
 *
 * @param name - Fixture name (without extension).
 * @returns The parsed fixture body, or `null` if the fixture file does not exist.
 */
export async function loadNonStreamingFixture(
  name: string,
): Promise<Record<string, unknown> | null> {
  const file = Bun.file(join(FIXTURES_DIR, `${name}.json`))
  if (!(await file.exists())) return null
  return (await file.json()) as Record<string, unknown>
}

/**
 * Loads a streaming completion fixture's raw SSE text from
 * `fixtures/stream/{name}.sse`.
 *
 * @param name - Fixture name (without extension).
 * @returns The raw SSE body, or `null` if the fixture file does not exist.
 */
export async function loadStreamingFixture(name: string): Promise<string | null> {
  const file = Bun.file(join(FIXTURES_DIR, "stream", `${name}.sse`))
  if (!(await file.exists())) return null
  return await file.text()
}

/** Builds a JSON error response body shaped like OpenAI's `{ error: { message } }`. */
function errorResponse(status: number, message: string): Response {
  return new Response(JSON.stringify({ error: { message } }), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

/**
 * Merges a non-streaming fixture body with the `id`/`model` fields the
 * LLM Gateway expects on every response.
 *
 * The `default.json`/`NAME.json` fixtures deliberately omit `id` and
 * `model` (see `docs/tasks/step10.md`): the gateway does not accept
 * arbitrary provider-echoed ids, so this stub always assigns
 * `chatcmpl-e2e-<fixtureName>` and echoes back the request's own `model`
 * field, overriding anything a fixture might otherwise contain.
 *
 * @param fixtureName - Name of the fixture that was selected.
 * @param fixtureBody - Parsed fixture JSON (everything except `id`/`model`).
 * @param requestModel - The `model` field from the incoming request body.
 * @returns The complete response body to send back to the LLM Gateway.
 */
export function mergeCompletionResponse(
  fixtureName: string,
  fixtureBody: Record<string, unknown>,
  requestModel: string,
): Record<string, unknown> {
  return {
    ...fixtureBody,
    id: `chatcmpl-e2e-${fixtureName}`,
    model: requestModel,
  }
}

/**
 * Handles `POST /v1/chat/completions`: selects a fixture based on the
 * request's messages, then serves either the static JSON completion or the
 * canned SSE stream depending on `body.stream`.
 *
 * @param request - The incoming HTTP request.
 * @returns A `200` JSON/SSE response for a known fixture, a `404` JSON
 *   error body (so a missing fixture fails a spec loudly) for an unknown but
 *   validly-named one, a `400` JSON error body for an unparseable request, or
 *   a `400` JSON error body for a fixture name that fails
 *   {@link VALID_FIXTURE_NAME} (rejected before it ever reaches a path join,
 *   so it can't be used for path traversal).
 */
export async function handleChatCompletions(request: Request): Promise<Response> {
  let body: CompletionRequestBody
  try {
    body = (await request.json()) as CompletionRequestBody
  } catch {
    return errorResponse(400, "invalid JSON request body")
  }

  const fixtureName = extractFixtureName(body.messages ?? [])

  if (!VALID_FIXTURE_NAME.test(fixtureName)) {
    return errorResponse(400, `invalid fixture name: ${fixtureName}`)
  }

  if (body.stream === true) {
    const sse = await loadStreamingFixture(fixtureName)
    if (sse === null) {
      return errorResponse(404, `unknown streaming fixture: ${fixtureName}`)
    }
    return new Response(sse, {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    })
  }

  const fixtureBody = await loadNonStreamingFixture(fixtureName)
  if (fixtureBody === null) {
    return errorResponse(404, `unknown fixture: ${fixtureName}`)
  }

  return new Response(
    JSON.stringify(mergeCompletionResponse(fixtureName, fixtureBody, body.model)),
    { status: 200, headers: { "Content-Type": "application/json" } },
  )
}

/**
 * Top-level Bun `fetch` handler: routes `GET /health` and
 * `POST /v1/chat/completions`, and 404s everything else.
 *
 * @param request - The incoming HTTP request.
 * @returns The response for the matched route, or a generic 404.
 */
export async function fetchHandler(request: Request): Promise<Response> {
  const url = new URL(request.url)

  if (request.method === "GET" && url.pathname === "/health") {
    return new Response(JSON.stringify({ status: "ok" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })
  }

  if (request.method === "POST" && url.pathname === "/v1/chat/completions") {
    return handleChatCompletions(request)
  }

  return errorResponse(404, "not found")
}

const PORT = Number(process.env.PORT ?? 8092)

// Only bind a listening socket when this file is executed directly (`bun
// run server.ts` / the Dockerfile's `CMD`), not when `server.test.ts`
// imports it for unit testing.
if (import.meta.main) {
  Bun.serve({
    port: PORT,
    fetch: fetchHandler,
  })
  // eslint-disable-next-line no-console -- stub startup log, no logging framework in this tiny service
  console.log(`llm-stub listening on :${PORT}`)
}
