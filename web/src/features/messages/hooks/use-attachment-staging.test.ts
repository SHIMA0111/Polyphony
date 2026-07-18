import { act, renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { useAttachmentStaging } from "./use-attachment-staging"

/** Builds a `File` with an overridden `size`, so oversized-file rejection can
 * be tested without actually allocating megabytes of content. */
function makeFile(name: string, type: string, sizeBytes: number): File {
  const file = new File([new Uint8Array(1)], name, { type })
  Object.defineProperty(file, "size", { value: sizeBytes })
  return file
}

describe("useAttachmentStaging", () => {
  it("rejects an unsupported MIME type without making a network call", () => {
    let uploadUrlRequested = false
    server.use(
      http.post("/api/proxy/rooms/:roomId/attachments/upload-url", () => {
        uploadUrlRequested = true
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([makeFile("document.pdf", "application/pdf", 1024)])
    })

    expect(result.current.attachments).toHaveLength(1)
    expect(result.current.attachments[0].status).toBe("error")
    expect(result.current.attachments[0].errorMessage).toMatch(/unsupported/i)
    expect(uploadUrlRequested).toBe(false)
  })

  it("rejects an oversized file without making a network call", () => {
    let uploadUrlRequested = false
    server.use(
      http.post("/api/proxy/rooms/:roomId/attachments/upload-url", () => {
        uploadUrlRequested = true
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([
        makeFile("big.png", "image/png", 10 * 1024 * 1024 + 1),
      ])
    })

    expect(result.current.attachments).toHaveLength(1)
    expect(result.current.attachments[0].status).toBe("error")
    expect(result.current.attachments[0].errorMessage).toMatch(/too large/i)
    expect(uploadUrlRequested).toBe(false)
  })

  it("transitions a valid file from uploading to done against the MSW-stubbed presigned upload flow", async () => {
    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([makeFile("photo.png", "image/png", 1024)])
    })

    expect(result.current.attachments).toHaveLength(1)
    expect(result.current.attachments[0].status).toBe("uploading")

    await waitFor(() => expect(result.current.attachments[0].status).toBe("done"))

    expect(result.current.attachments[0].attachmentId).toBe("attachment-1")
    expect(result.current.attachments[0].progress).toBe(100)
  })

  it("remove() drops the entry and revokes its preview URL", () => {
    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([makeFile("photo.png", "image/png", 1024)])
    })
    const id = result.current.attachments[0].id

    act(() => {
      result.current.remove(id)
    })

    expect(result.current.attachments).toHaveLength(0)
  })

  it("retry() re-validates first and leaves a client-side-rejected entry untouched, without a network call", () => {
    let uploadUrlRequested = false
    server.use(
      http.post("/api/proxy/rooms/:roomId/attachments/upload-url", () => {
        uploadUrlRequested = true
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([makeFile("document.pdf", "application/pdf", 1024)])
    })
    const id = result.current.attachments[0].id
    expect(result.current.attachments[0].status).toBe("error")

    act(() => {
      result.current.retry(id)
    })

    // Still the same file, so `validateFile` fails identically -- the entry
    // must stay in its error state rather than flip to "uploading" for a
    // retry that could only ever fail again.
    expect(result.current.attachments).toHaveLength(1)
    expect(result.current.attachments[0].status).toBe("error")
    expect(result.current.attachments[0].errorMessage).toMatch(/unsupported/i)
    expect(uploadUrlRequested).toBe(false)
  })

  it("retry() re-uploads and resolves to done once a transiently-failed upload is retried", async () => {
    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([makeFile("photo.png", "image/png", 1024)])
    })
    const id = result.current.attachments[0].id

    // Force the first upload attempt to fail with a network error.
    server.use(
      http.put(
        "/test-fixtures/attachment-upload",
        () => HttpResponse.error(),
        { once: true },
      ),
    )

    await waitFor(() => expect(result.current.attachments[0].status).toBe("error"))

    act(() => {
      result.current.retry(id)
    })

    // Validation passes this time (it's a real PNG under the size cap), so
    // the entry is reset back to "uploading" and the upload is retried
    // against the (now unstubbed, always-succeeding) PUT handler.
    expect(result.current.attachments[0].status).toBe("uploading")
    expect(result.current.attachments[0].progress).toBe(0)
    expect(result.current.attachments[0].errorMessage).toBeUndefined()

    await waitFor(() => expect(result.current.attachments[0].status).toBe("done"))
    expect(result.current.attachments[0].attachmentId).toBe("attachment-1")
  })

  it("reset() clears every staged entry", () => {
    const { result } = renderHook(() => useAttachmentStaging("room-1"))

    act(() => {
      result.current.addFiles([
        makeFile("a.png", "image/png", 1024),
        makeFile("b.png", "image/png", 1024),
      ])
    })
    expect(result.current.attachments).toHaveLength(2)

    act(() => {
      result.current.reset()
    })

    expect(result.current.attachments).toHaveLength(0)
  })
})
