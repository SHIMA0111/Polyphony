import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { server } from "@/test/msw/server"
import { render, screen, waitFor } from "@/test/render"
import { fixtureAttachmentWithUrl } from "@/features/messages/api/handlers"
import type { AttachmentWithUrl, Message } from "@/features/messages/types"
import { MessageAttachments } from "./MessageAttachments"

const baseMessage: Message = {
  id: "message-1",
  room_id: "room-1",
  sender_id: "user-1",
  content: "Look at this",
  type: "human",
  status: "completed",
  sequence: 1,
  in_response_to_message_id: null,
  is_deleted: false,
  exclude_from_ai: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

function mockAttachments(attachments: AttachmentWithUrl[]) {
  server.use(
    http.get(
      "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
      () => {
        return HttpResponse.json<{ attachments: AttachmentWithUrl[] }>({
          attachments,
        })
      },
    ),
  )
}

describe("MessageAttachments", () => {
  it("renders nothing when the message has no attachments", async () => {
    let fetched = false
    server.use(
      http.get(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        () => {
          fetched = true
          return HttpResponse.json<{ attachments: AttachmentWithUrl[] }>({
            attachments: [],
          })
        },
      ),
    )

    render(<MessageAttachments message={baseMessage} />)

    // Give the (empty-resolving) query a chance to settle before asserting
    // no thumbnail ever appears.
    await waitFor(() => expect(fetched).toBe(true))
    expect(screen.queryAllByRole("img")).toHaveLength(0)
  })

  it("renders a single thumbnail", async () => {
    mockAttachments([fixtureAttachmentWithUrl])

    render(<MessageAttachments message={baseMessage} />)

    await waitFor(() =>
      expect(screen.getAllByRole("img")).toHaveLength(1),
    )
  })

  it("renders multiple thumbnails", async () => {
    mockAttachments([
      fixtureAttachmentWithUrl,
      { ...fixtureAttachmentWithUrl, id: "attachment-2", view_url: "https://minio.example.test/second.png" },
    ])

    render(<MessageAttachments message={baseMessage} />)

    await waitFor(() =>
      expect(screen.getAllByRole("img")).toHaveLength(2),
    )
  })

  it("opens the lightbox with the full-size image when a thumbnail is clicked", async () => {
    mockAttachments([fixtureAttachmentWithUrl])
    const user = userEvent.setup()

    render(<MessageAttachments message={baseMessage} />)

    const thumbnail = await screen.findByRole("img", { name: "Message attachment" })
    await user.click(thumbnail)

    const dialogImages = await screen.findAllByRole("img", {
      name: "Full-size attachment",
    })
    expect(dialogImages).toHaveLength(1)
    expect(dialogImages[0]).toHaveAttribute("src", fixtureAttachmentWithUrl.view_url)
  })

  it("never queries attachments for a client-only optimistic message", () => {
    let requested = false
    server.use(
      http.get(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        () => {
          requested = true
          return HttpResponse.json({ attachments: [] })
        },
      ),
    )

    render(
      <MessageAttachments
        message={{ ...baseMessage, id: "optimistic-abc123" }}
      />,
    )

    expect(requested).toBe(false)
  })
})
