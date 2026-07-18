"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { requestUploadUrl } from "../api/request-upload-url"
import { UploadAttachmentAbortError, uploadAttachment } from "../lib/upload-attachment"
import { getErrorMessage } from "@/lib/get-error-message"

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
  /** Removes a staged entry (of any status), aborting its in-flight upload
   * (if any) and revoking its preview URL. */
  remove: (id: string) => void
  /** Clears every staged entry, aborting any in-flight uploads and revoking
   * all of their preview URLs. */
  reset: () => void
  /** Re-attempts an `"error"` entry: re-validates the file client-side
   * first, and only re-uploads (resetting `progress`/`attachmentId`/
   * `errorMessage` and flipping `status` back to `"uploading"`) if it still
   * passes. A file that fails client-side validation (unsupported type,
   * too large) fails identically on every retry, so re-running that check
   * here -- rather than unconditionally re-uploading -- skips a network
   * round trip that could only ever be rejected again; the entry is left
   * untouched in its `"error"` state in that case. */
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
  // One `AbortController` per in-flight upload, keyed by the staged entry's
  // client-generated `id`. `remove()`/`reset()`/unmount abort and delete the
  // matching controller(s) so a cancelled upload's `XMLHttpRequest` is
  // actually torn down instead of continuing in the background after its
  // staged entry (and any UI reflecting its progress) has disappeared.
  const abortControllersRef = useRef<Map<string, AbortController>>(new Map())
  // Mirrors `attachments` so the unmount-only cleanup effect below (and
  // `retry`, which needs synchronous access to a staged entry's `file`) can
  // read the latest state without depending on `attachments` itself (which
  // would otherwise re-run that effect's cleanup on every progress update).
  const attachmentsRef = useRef<StagedAttachment[]>([])
  useEffect(() => {
    attachmentsRef.current = attachments
  }, [attachments])

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
    abortControllersRef.current.get(id)?.abort()
    abortControllersRef.current.delete(id)
    setAttachments((prev) => {
      const target = prev.find((a) => a.id === id)
      if (target) URL.revokeObjectURL(target.previewUrl)
      return prev.filter((a) => a.id !== id)
    })
    startedRef.current.delete(id)
  }, [])

  const reset = useCallback(() => {
    abortControllersRef.current.forEach((controller) => controller.abort())
    abortControllersRef.current.clear()
    setAttachments((prev) => {
      prev.forEach((a) => URL.revokeObjectURL(a.previewUrl))
      return []
    })
    startedRef.current.clear()
  }, [])

  const retry = useCallback((id: string) => {
    const target = attachmentsRef.current.find((a) => a.id === id)
    if (!target) return

    // Re-validate first: a client-side rejection (unsupported type, too
    // large) would only fail identically on a network retry, so if the file
    // still doesn't pass, leave the entry in its error state untouched
    // rather than flipping it to "uploading" for a doomed request.
    if (validateFile(target.file)) return

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
      const controller = new AbortController()
      abortControllersRef.current.set(item.id, controller)

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
          if (err instanceof UploadAttachmentAbortError) {
            // A deliberate cancellation (remove()/reset()/unmount) -- the
            // staged entry has already been dropped from state (or the
            // whole hook is unmounting), so there is no error state to
            // surface; treat it as a no-op.
            return
          }
          setAttachments((prev) =>
            prev.map((a) =>
              a.id === item.id
                ? {
                    ...a,
                    status: "error",
                    errorMessage: getErrorMessage(err, "Upload failed"),
                  }
                : a,
            ),
          )
        } finally {
          abortControllersRef.current.delete(item.id)
        }
      })()
    }
  }, [attachments, roomId])

  // Abort every remaining in-flight upload and revoke every remaining
  // preview URL on unmount (e.g. navigating away mid-upload) -- `remove`/
  // `reset` already handle the non-unmount cases.
  useEffect(() => {
    // Captured once, at mount: this is the same `Map` instance for the
    // hook's entire lifetime (never reassigned, only mutated), so reading
    // it into a local here -- rather than dereferencing `.current` again
    // inside the cleanup below -- just satisfies
    // `react-hooks/exhaustive-deps`'s "ref may have changed by the time
    // cleanup runs" warning without changing behavior.
    const abortControllers = abortControllersRef.current
    return () => {
      abortControllers.forEach((controller) => controller.abort())
      abortControllers.clear()
      attachmentsRef.current.forEach((a) => URL.revokeObjectURL(a.previewUrl))
    }
  }, [])

  return { attachments, addFiles, remove, reset, retry }
}
