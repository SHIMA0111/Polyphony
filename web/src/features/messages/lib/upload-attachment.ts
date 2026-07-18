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
 * Thrown by {@link uploadAttachment} when `signal` is aborted before the
 * upload completes. Distinguished from {@link UploadAttachmentError} so
 * callers can treat a deliberate cancellation (e.g. `remove()`ing a staged
 * attachment mid-upload) as a silent no-op rather than a failure to surface.
 */
export class UploadAttachmentAbortError extends Error {
  constructor() {
    super("Attachment upload was aborted")
    this.name = "UploadAttachmentAbortError"
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
 * Resolves once the response status is in the `2xx` range; rejects with a
 * typed {@link UploadAttachmentError} otherwise (including for a network
 * error, where `status` is `0`), or a {@link UploadAttachmentAbortError} if
 * `signal` is aborted first.
 *
 * @param signal - Optional `AbortSignal`; aborting it calls `xhr.abort()` and
 * rejects with {@link UploadAttachmentAbortError} instead of resolving or
 * rejecting with the usual HTTP/network outcome. Not threaded through
 * `requestUploadUrl` -- the presign call itself is cheap and short-lived, so
 * there is nothing worth cancelling there.
 */
export function uploadAttachment(
  file: File,
  uploadUrl: string,
  onProgress: (percent: number) => void,
  signal?: AbortSignal,
): Promise<void> {
  return new Promise((resolve, reject) => {
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

    if (signal) {
      if (signal.aborted) {
        // Already aborted before the request was ever sent -- skip `send()`
        // entirely rather than starting an upload just to immediately abort
        // it.
        reject(new UploadAttachmentAbortError())
        return
      }
      signal.addEventListener("abort", () => {
        xhr.abort()
        reject(new UploadAttachmentAbortError())
      })
    }

    xhr.send(file)
  })
}
