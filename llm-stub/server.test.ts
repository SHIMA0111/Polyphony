import { describe, expect, test } from "bun:test"

import { extractFixtureName, fetchHandler, mergeCompletionResponse } from "./server"

describe("extractFixtureName", () => {
  test("falls back to 'default' when there is no fixture marker", () => {
    expect(
      extractFixtureName([{ role: "user", content: "hello there" }]),
    ).toBe("default")
  })

  test("falls back to 'default' when there is no user message at all", () => {
    expect(
      extractFixtureName([{ role: "assistant", content: "hi" }]),
    ).toBe("default")
  })

  test("extracts the name from an explicit [[fixture:NAME]] marker", () => {
    expect(
      extractFixtureName([
        { role: "user", content: "[[fixture:rate-limited]] please fail" },
      ]),
    ).toBe("rate-limited")
  })

  test("uses the last user message, ignoring earlier ones", () => {
    expect(
      extractFixtureName([
        { role: "user", content: "[[fixture:first]] ignored" },
        { role: "assistant", content: "ok" },
        { role: "user", content: "[[fixture:second]] used" },
      ]),
    ).toBe("second")
  })
})

describe("mergeCompletionResponse", () => {
  test("assigns the deterministic id and echoes the request model", () => {
    const merged = mergeCompletionResponse(
      "default",
      { choices: [], usage: {} },
      "gpt-5-mini",
    )
    expect(merged.id).toBe("chatcmpl-e2e-default")
    expect(merged.model).toBe("gpt-5-mini")
  })

  test("server-assigned id/model win over anything present in the fixture", () => {
    const merged = mergeCompletionResponse(
      "default",
      { id: "should-be-overwritten", model: "should-be-overwritten" },
      "gpt-5.2",
    )
    expect(merged.id).toBe("chatcmpl-e2e-default")
    expect(merged.model).toBe("gpt-5.2")
  })
})

describe("POST /v1/chat/completions", () => {
  function request(body: unknown): Request {
    return new Request("http://localhost/v1/chat/completions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
  }

  test("non-streaming: returns the default fixture merged with id/model", async () => {
    const res = await fetchHandler(
      request({
        model: "gpt-5-mini",
        messages: [{ role: "user", content: "hi" }],
      }),
    )
    expect(res.status).toBe(200)
    expect(res.headers.get("Content-Type")).toBe("application/json")

    const json = (await res.json()) as {
      id: string
      model: string
      choices: Array<{ message: { role: string; content: string } }>
    }
    expect(json.id).toBe("chatcmpl-e2e-default")
    expect(json.model).toBe("gpt-5-mini")
    expect(json.choices[0].message.role).toBe("assistant")
  })

  test("streaming: returns the default SSE fixture terminated by [DONE]", async () => {
    const res = await fetchHandler(
      request({
        model: "gpt-5-mini",
        messages: [{ role: "user", content: "hi" }],
        stream: true,
      }),
    )
    expect(res.status).toBe(200)
    expect(res.headers.get("Content-Type")).toBe("text/event-stream")

    const text = await res.text()
    expect(text).toContain("data: [DONE]")
    expect(text).toContain('"role":"assistant"')
  })

  test("unknown fixture name returns 404 with a JSON error body", async () => {
    const res = await fetchHandler(
      request({
        model: "gpt-5-mini",
        messages: [{ role: "user", content: "[[fixture:does-not-exist]] hi" }],
      }),
    )
    expect(res.status).toBe(404)
    const json = (await res.json()) as { error: { message: string } }
    expect(json.error.message).toContain("does-not-exist")
  })

  test("unknown streaming fixture name returns 404 with a JSON error body", async () => {
    const res = await fetchHandler(
      request({
        model: "gpt-5-mini",
        messages: [{ role: "user", content: "[[fixture:does-not-exist]] hi" }],
        stream: true,
      }),
    )
    expect(res.status).toBe(404)
    const json = (await res.json()) as { error: { message: string } }
    expect(json.error.message).toContain("does-not-exist")
  })

  test("a path-traversal fixture marker is rejected with 400 before any file access", async () => {
    const res = await fetchHandler(
      request({
        model: "gpt-5-mini",
        messages: [{ role: "user", content: "[[fixture:../package]] hi" }],
      }),
    )
    expect(res.status).toBe(400)
    const json = (await res.json()) as { error: { message: string } }
    expect(json.error.message).toContain("../package")
  })
})

describe("GET /health", () => {
  test("returns 200 with { status: 'ok' }", async () => {
    const res = await fetchHandler(new Request("http://localhost/health"))
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ status: "ok" })
  })
})
