/**
 * Thrown by {@link uploadAttachment} for any non-2xx response, or a genuine
 * network failure, from the presigned S3 `PUT`.
 */
export class UploadAttachmentError extends Error {
  /** HTTP status code of the upload response, or `0` for a network error
   * (no response was ever received). */
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = "UploadAttachmentError"
    this.status = status
  }
}

/**
 * Thrown by {@link uploadAttachment} when its `signal` is aborted before the
 * upload settles (e.g. the staged attachment was removed, the staging hook
 * was reset, or the component unmounted while the `PUT` was in flight).
 * Deliberately a distinct class from {@link UploadAttachmentError} so
 * callers can tell a caller-initiated cancellation apart from a genuine
 * upload failure and treat it as a no-op rather than an error to surface.
 */
export class UploadAttachmentAbortedError extends Error {
  constructor() {
    super("Attachment upload aborted")
    this.name = "UploadAttachmentAbortedError"
  }
}

/**
 * Uploads `file`'s raw bytes directly to a presigned S3 `upload_url` (from
 * `requestUploadUrl`) via a `PUT` request, reporting progress as `onProgress`
 * is called with a `0`-`100` integer percentage.
 *
 * Deliberately implemented with `XMLHttpRequest` rather than `fetch`: the
 * Fetch API has no reliable, widely-supported way to observe upload progress
 * for a request body (`ReadableStream` request bodies with progress are not
 * consistently supported across browsers), while `XMLHttpRequest.upload`'s
 * `progress` event has been stable for this exact use case for years.
 *
 * `signal`, if given, cancels the in-flight `PUT` via `xhr.abort()` when
 * aborted (including if it is already aborted when this is called, in which
 * case the request is never even sent) and rejects with
 * {@link UploadAttachmentAbortedError} rather than a generic network error.
 *
 * Resolves once the response status is in the `2xx` range; rejects with a
 * typed {@link UploadAttachmentError} otherwise (including for a network
 * error, where `status` is `0`), or {@link UploadAttachmentAbortedError} for
 * a caller-initiated cancellation.
 */
export function uploadAttachment(
  file: File,
  uploadUrl: string,
  onProgress: (percent: number) => void,
  signal?: AbortSignal,
): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new UploadAttachmentAbortedError())
      return
    }

    const xhr = new XMLHttpRequest()
    xhr.open("PUT", uploadUrl)
    xhr.setRequestHeader("Content-Type", file.type)

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        onProgress(Math.round((event.loaded / event.total) * 100))
      }
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
      } else {
        reject(
          new UploadAttachmentError(
            xhr.status,
            `Attachment upload failed with HTTP ${xhr.status}`,
          ),
        )
      }
    }

    xhr.onerror = () => {
      reject(new UploadAttachmentError(0, "Network error during attachment upload"))
    }

    xhr.onabort = () => {
      reject(new UploadAttachmentAbortedError())
    }

    signal?.addEventListener("abort", () => xhr.abort())

    xhr.send(file)
  })
}
