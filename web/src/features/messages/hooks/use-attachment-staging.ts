"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { requestUploadUrl } from "../api/request-upload-url"
import { UploadAttachmentAbortedError, uploadAttachment } from "../lib/upload-attachment"

/**
 * MIME types Step 12's `AttachmentUsecase.RequestUpload` allow-lists
 * server-side (`server/internal/usecase/attachment/usecase.go`), mirrored
 * here so an unsupported file is rejected before it ever reaches the
 * network.
 */
export const ALLOWED_ATTACHMENT_MIME_TYPES = [
  "image/png",
  "image/jpeg",
  "image/webp",
  "image/gif",
] as const

/**
 * Byte cap mirroring `AttachmentUsecase.MaxAttachmentSizeBytes` (10 MiB),
 * enforced client-side so an oversized file is rejected before upload.
 */
export const MAX_ATTACHMENT_SIZE_BYTES = 10 * 1024 * 1024

export type StagedAttachmentStatus = "uploading" | "done" | "error"

/** One file staged for attachment to the next sent message. */
export interface StagedAttachment {
  /** Client-generated id, stable for the lifetime of this staged entry
   * (distinct from `attachmentId`, which only exists once the server has
   * created an `Attachment` row for it). */
  id: string
  file: File
  /** `URL.createObjectURL(file)` -- revoked on `remove`/`reset`/unmount. */
  previewUrl: string
  status: StagedAttachmentStatus
  /** Upload progress percentage, `0`-`100`. Meaningless once `status` is
   * `"done"` or `"error"` (left at its last value). */
  progress: number
  /** Set once `requestUploadUrl` resolves; the id `attachToMessage` needs at
   * send time. `undefined` until then, and for a rejected (`"error"`) file
   * that never even requested a ticket. */
  attachmentId?: string
  /** Human-readable failure reason, set only when `status === "error"`. */
  errorMessage?: string
}

export interface UseAttachmentStagingResult {
  attachments: StagedAttachment[]
  /** Validates and stages each file, kicking off its upload in the
   * background (see the internal effect below). A file that fails
   * client-side validation is staged with `status: "error"` and never
   * triggers a network call. */
  addFiles: (files: File[]) => void
  /** Removes a staged entry (of any status) and revokes its preview URL.
   * Aborts its in-flight upload first, if any. */
  remove: (id: string) => void
  /** Clears every staged entry, revoking all of their preview URLs. Aborts
   * every in-flight upload first. */
  reset: () => void
  /** Re-attempts the upload for an `"error"` entry. Re-runs client-side
   * validation first: if the file still fails it (unsupported type,
   * oversized), the entry is left in its error state -- retrying the
   * network call would only fail again identically. Only a transient
   * network/upload failure actually triggers a fresh upload. */
  retry: (id: string) => void
}

/**
 * Validates a single file against the client-side mirror of Step 12's
 * server-side allow-list/size-cap, returning a human-readable rejection
 * reason or `null` if the file is acceptable.
 */
function validateFile(file: File): string | null {
  if (
    !ALLOWED_ATTACHMENT_MIME_TYPES.includes(
      file.type as (typeof ALLOWED_ATTACHMENT_MIME_TYPES)[number],
    )
  ) {
    return "Unsupported file type"
  }
  if (file.size > MAX_ATTACHMENT_SIZE_BYTES) {
    return "File is too large (max 10 MB)"
  }
  return null
}

/**
 * Manages the client-side list of pending attachments before a message is
 * sent: staging (with client-side MIME/size validation), background
 * upload (presign + direct-to-S3 `PUT`, tracked via `progress`), removal,
 * retry, and full reset -- all against `roomId`'s
 * `POST /rooms/:roomId/attachments/upload-url` endpoint.
 *
 * The actual upload is driven by an internal effect that watches for
 * newly-staged (`status: "uploading"`, not-yet-started) entries rather than
 * kicking the upload off directly inside `addFiles`: this keeps `addFiles`
 * itself a pure, synchronous state update (friendlier to React 19 Strict
 * Mode's double-invocation of event handlers in development) while a
 * `startedRef` guard ensures each entry's upload is only ever kicked off
 * once, even if the effect re-runs before that upload finishes.
 */
export function useAttachmentStaging(roomId: string): UseAttachmentStagingResult {
  const [attachments, setAttachments] = useState<StagedAttachment[]>([])
  const startedRef = useRef<Set<string>>(new Set())
  // Mirrors `attachments` so the unmount-only cleanup effect below can read
  // the latest preview URLs without depending on `attachments` itself (which
  // would otherwise re-run that effect's cleanup on every progress update).
  const attachmentsRef = useRef<StagedAttachment[]>([])
  useEffect(() => {
    attachmentsRef.current = attachments
  }, [attachments])

  // One `AbortController` per in-flight upload, keyed by the staged entry's
  // client-generated `id`. Consulted (and torn down) by `remove`/`reset`/the
  // unmount cleanup below so a file staged mid-upload doesn't keep uploading
  // -- and can't call back into `setAttachments` -- after it's been dropped.
  const controllersRef = useRef<Map<string, AbortController>>(new Map())

  const addFiles = useCallback((files: File[]) => {
    const newEntries: StagedAttachment[] = files.map((file) => {
      const id = crypto.randomUUID()
      const previewUrl = URL.createObjectURL(file)
      const validationError = validateFile(file)

      return validationError
        ? {
            id,
            file,
            previewUrl,
            status: "error",
            progress: 0,
            errorMessage: validationError,
          }
        : { id, file, previewUrl, status: "uploading", progress: 0 }
    })

    setAttachments((prev) => [...prev, ...newEntries])
  }, [])

  const remove = useCallback((id: string) => {
    controllersRef.current.get(id)?.abort()
    controllersRef.current.delete(id)
    setAttachments((prev) => {
      const target = prev.find((a) => a.id === id)
      if (target) URL.revokeObjectURL(target.previewUrl)
      return prev.filter((a) => a.id !== id)
    })
    startedRef.current.delete(id)
  }, [])

  const reset = useCallback(() => {
    controllersRef.current.forEach((controller) => controller.abort())
    controllersRef.current.clear()
    setAttachments((prev) => {
      prev.forEach((a) => URL.revokeObjectURL(a.previewUrl))
      return []
    })
    startedRef.current.clear()
  }, [])

  const retry = useCallback((id: string) => {
    const target = attachmentsRef.current.find((a) => a.id === id)
    if (!target) return

    const validationError = validateFile(target.file)
    if (validationError) {
      // Re-validate first: a client-side rejection (unsupported type,
      // oversized) will fail identically again, so leave the entry in its
      // error state rather than flipping it to "uploading" and burning a
      // network round-trip that can only fail the same way.
      setAttachments((prev) =>
        prev.map((a) => (a.id === id ? { ...a, errorMessage: validationError } : a)),
      )
      return
    }

    startedRef.current.delete(id)
    setAttachments((prev) =>
      prev.map((a) =>
        a.id === id
          ? {
              ...a,
              status: "uploading",
              progress: 0,
              attachmentId: undefined,
              errorMessage: undefined,
            }
          : a,
      ),
    )
  }, [])

  // Drives the actual presign + upload for every "uploading" entry that
  // hasn't started yet -- covers both freshly-added files and `retry()`
  // (which resets an entry back to "uploading" and clears `startedRef` for
  // it).
  useEffect(() => {
    const pending = attachments.filter(
      (a) => a.status === "uploading" && !startedRef.current.has(a.id),
    )
    if (pending.length === 0) return

    for (const item of pending) {
      startedRef.current.add(item.id)
      // Presigning is cheap and untied to any particular attempt, so only
      // the actual upload PUT is cancellable -- `requestUploadUrl` below
      // deliberately doesn't receive this signal.
      const controller = new AbortController()
      controllersRef.current.set(item.id, controller)

      void (async () => {
        try {
          const ticket = await requestUploadUrl(roomId, item.file.type, item.file.size)
          setAttachments((prev) =>
            prev.map((a) =>
              a.id === item.id ? { ...a, attachmentId: ticket.attachment_id } : a,
            ),
          )

          await uploadAttachment(
            item.file,
            ticket.upload_url,
            (percent) => {
              setAttachments((prev) =>
                prev.map((a) => (a.id === item.id ? { ...a, progress: percent } : a)),
              )
            },
            controller.signal,
          )

          setAttachments((prev) =>
            prev.map((a) =>
              a.id === item.id ? { ...a, status: "done", progress: 100 } : a,
            ),
          )
        } catch (err) {
          if (err instanceof UploadAttachmentAbortedError) {
            // Deliberate cancellation from remove()/reset()/unmount -- the
            // entry is already gone (or about to be), so there's no error
            // state to surface and nothing left to update.
            return
          }
          setAttachments((prev) =>
            prev.map((a) =>
              a.id === item.id
                ? {
                    ...a,
                    status: "error",
                    errorMessage: err instanceof Error ? err.message : "Upload failed",
                  }
                : a,
            ),
          )
        } finally {
          controllersRef.current.delete(item.id)
        }
      })()
    }
  }, [attachments, roomId])

  // Abort every in-flight upload and revoke every remaining preview URL on
  // unmount (e.g. navigating away mid-upload) -- `remove`/`reset` already
  // handle the non-unmount cases.
  useEffect(() => {
    const controllers = controllersRef.current
    return () => {
      controllers.forEach((controller) => controller.abort())
      attachmentsRef.current.forEach((a) => URL.revokeObjectURL(a.previewUrl))
    }
  }, [])

  return { attachments, addFiles, remove, reset, retry }
}
