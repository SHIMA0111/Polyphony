import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen } from "@/test/render"
import type { StagedAttachment } from "@/features/messages/hooks/use-attachment-staging"
import { AttachmentChip } from "./AttachmentChip"

function makeAttachment(overrides: Partial<StagedAttachment> = {}): StagedAttachment {
  return {
    id: "staged-1",
    file: new File(["x"], "photo.png", { type: "image/png" }),
    previewUrl: "blob:mock-preview-url-0",
    status: "done",
    progress: 100,
    ...overrides,
  }
}

describe("AttachmentChip", () => {
  it("renders a progress indicator while uploading", () => {
    render(
      <AttachmentChip
        attachment={makeAttachment({ status: "uploading", progress: 42 })}
        onRemove={vi.fn()}
        onRetry={vi.fn()}
      />,
    )

    expect(screen.getByRole("progressbar")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Retry upload" })).not.toBeInTheDocument()
  })

  it("renders neither a progress indicator nor a retry affordance once done", () => {
    render(
      <AttachmentChip
        attachment={makeAttachment({ status: "done" })}
        onRemove={vi.fn()}
        onRetry={vi.fn()}
      />,
    )

    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Retry upload" })).not.toBeInTheDocument()
  })

  it("renders a retry affordance for an error entry and fires onRetry with its id", async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    render(
      <AttachmentChip
        attachment={makeAttachment({ status: "error", errorMessage: "File is too large" })}
        onRemove={vi.fn()}
        onRetry={onRetry}
      />,
    )

    await user.click(screen.getByRole("button", { name: "Retry upload" }))

    expect(onRetry).toHaveBeenCalledWith("staged-1")
  })

  it("fires onRemove with the attachment's id when the remove button is clicked", async () => {
    const user = userEvent.setup()
    const onRemove = vi.fn()
    render(
      <AttachmentChip attachment={makeAttachment()} onRemove={onRemove} onRetry={vi.fn()} />,
    )

    await user.click(screen.getByRole("button", { name: "Remove attachment" }))

    expect(onRemove).toHaveBeenCalledWith("staged-1")
  })
})
