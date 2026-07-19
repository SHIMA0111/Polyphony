import { afterEach, describe, expect, it, vi } from "vitest"
import { UploadAttachmentError, uploadAttachment } from "./upload-attachment"

/**
 * A minimal `XMLHttpRequest` stand-in that records the handlers/config
 * `uploadAttachment` wires up and lets a test fire them directly --
 * deterministic where waiting out a real 60s stall (or relying on MSW's XHR
 * interceptor to honor `.timeout`) would not be.
 */
class FakeXHR {
  static instances: FakeXHR[] = []

  timeout = 0
  status = 0
  upload: { onprogress: ((event: ProgressEvent) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  onabort: (() => void) | null = null
  ontimeout: (() => void) | null = null

  open = vi.fn()
  setRequestHeader = vi.fn()
  send = vi.fn()
  abort = vi.fn(() => {
    this.onabort?.()
  })

  constructor() {
    FakeXHR.instances.push(this)
  }
}

function makeFile(): File {
  return new File([new Uint8Array(1)], "photo.png", { type: "image/png" })
}

describe("uploadAttachment", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    FakeXHR.instances = []
  })

  it("sets a finite xhr.timeout and rejects with UploadAttachmentError when it fires", async () => {
    // `globalThis.XMLHttpRequest` is read-only in jsdom -- `vi.stubGlobal`
    // (same mechanism `use-room-socket.test.ts` uses for `WebSocket`) is
    // required rather than a plain assignment.
    vi.stubGlobal("XMLHttpRequest", FakeXHR)

    const promise = uploadAttachment(makeFile(), "https://example.com/upload", () => {})
    const xhr = FakeXHR.instances[0]

    expect(xhr.timeout).toBeGreaterThan(0)
    expect(xhr.timeout).toBe(60_000)

    xhr.ontimeout?.()

    await expect(promise).rejects.toBeInstanceOf(UploadAttachmentError)
    await expect(promise).rejects.toMatchObject({ status: 0 })
    await expect(promise).rejects.toThrow(/timed out/i)
  })

  it("still resolves normally on a 2xx response (timeout wiring doesn't interfere)", async () => {
    vi.stubGlobal("XMLHttpRequest", FakeXHR)

    const promise = uploadAttachment(makeFile(), "https://example.com/upload", () => {})
    const xhr = FakeXHR.instances[0]
    xhr.status = 200
    xhr.onload?.()

    await expect(promise).resolves.toBeUndefined()
  })
})
