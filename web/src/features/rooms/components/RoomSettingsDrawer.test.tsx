import { HttpResponse, http } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import type { RoomRole } from "@/features/members/types"
import { RoomSettingsDrawer } from "./RoomSettingsDrawer"
import type { ForkJob, Room, RoomForkResponse } from "../types"

const { useRouterMock, pushMock } = vi.hoisted(() => {
  const pushMock = vi.fn()
  return { useRouterMock: vi.fn(() => ({ push: pushMock })), pushMock }
})

vi.mock("next/navigation", () => ({
  useRouter: useRouterMock,
  // `Provider` (via `src/test/render.tsx`) wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's SSR
  // styles; jsdom never streams, so a no-op is all component tests need
  // (see `MemberListItem.test.tsx`'s identical mock).
  useServerInsertedHTML: vi.fn(),
}))

/**
 * Covers Step 36's Scope requirement: `RoomSettingsDrawer`'s role-gated
 * visibility matrix (reader/guest/member read-only with no AI-settings/
 * cutoff/delete controls; admin editable but no delete; master gets
 * everything including delete), the AI-model picker's preselection from the
 * room's existing `ai_model`, and the delete confirmation dialog requiring
 * an explicit confirm click before `deleteRoom` fires.
 */
const baseRoom: Room = {
  id: "room-1",
  name: "General",
  description: "General discussion room",
  owner_id: "user-1",
  role: "master",
  ai_context_cutoff_at: null,
  ai_provider: "OpenAI",
  ai_model: "gpt-5",
  forked_from_room_id: null,
  is_archived: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

function roomWithRole(role: RoomRole): Room {
  return { ...baseRoom, role }
}

describe("RoomSettingsDrawer", () => {
  it.each<RoomRole>(["reader", "guest", "member"])(
    "shows read-only fields and no editable/AI-settings/cutoff/delete controls for a %s viewer",
    async (role) => {
      render(
        <RoomSettingsDrawer
          open
          onOpenChange={vi.fn()}
          room={roomWithRole(role)}
          role={role}
        />,
      )

      expect(await screen.findByText("General")).toBeInTheDocument()
      expect(
        screen.getByText("General discussion room"),
      ).toBeInTheDocument()
      expect(screen.getByText("No cutoff set")).toBeInTheDocument()

      expect(screen.queryByLabelText("Room name")).not.toBeInTheDocument()
      expect(screen.queryByLabelText("Description")).not.toBeInTheDocument()
      expect(
        screen.queryByLabelText("AI default model"),
      ).not.toBeInTheDocument()
      expect(
        screen.queryByLabelText("Cutoff date and time"),
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole("button", { name: "Set cutoff to now" }),
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole("button", { name: "Delete room" }),
      ).not.toBeInTheDocument()
    },
  )

  it("shows editable rename/description/AI-settings/cutoff but no delete action for an admin viewer", async () => {
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={vi.fn()}
        room={roomWithRole("admin")}
        role="admin"
      />,
    )

    expect(await screen.findByLabelText("Room name")).toHaveValue("General")
    expect(screen.getByLabelText("Description")).toHaveValue(
      "General discussion room",
    )
    expect(screen.getByLabelText("AI default model")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Set cutoff to now" }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Delete room" }),
    ).not.toBeInTheDocument()
  })

  it("shows the delete action for a master viewer", async () => {
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={vi.fn()}
        room={roomWithRole("master")}
        role="master"
      />,
    )

    expect(
      await screen.findByRole("button", { name: "Delete room" }),
    ).toBeInTheDocument()
  })

  it("preselects the room's existing ai_model and saves it to the settings endpoint", async () => {
    let capturedBody: unknown
    server.use(
      http.patch(
        "/api/proxy/rooms/:roomId/settings",
        async ({ request, params }) => {
          capturedBody = await request.json()
          return HttpResponse.json({
            ...baseRoom,
            id: String(params.roomId),
          })
        },
      ),
    )

    const user = userEvent.setup()
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={vi.fn()}
        room={roomWithRole("master")}
        role="master"
      />,
    )

    const select = await screen.findByLabelText("AI default model")
    await waitFor(() => expect(select).toHaveValue("gpt-5"))

    const saveButtons = screen.getAllByRole("button", { name: "Save" })
    // The AI-default section's own "Save" button -- the first "Save" belongs
    // to the room-details section.
    await user.click(saveButtons[1])

    await waitFor(() =>
      expect(capturedBody).toEqual({ ai_provider: "OpenAI", ai_model: "gpt-5" }),
    )
  })

  it("does not delete the room until the confirmation dialog's confirm button is clicked", async () => {
    let deleteCalled = false
    server.use(
      http.delete("/api/proxy/rooms/:roomId", () => {
        deleteCalled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={onOpenChange}
        room={roomWithRole("master")}
        role="master"
      />,
    )

    await user.click(
      await screen.findByRole("button", { name: "Delete room" }),
    )

    expect(await screen.findByText("Delete this room?")).toBeInTheDocument()
    expect(deleteCalled).toBe(false)

    await user.click(
      await screen.findByRole("button", { name: "Yes, delete room" }),
    )

    await waitFor(() => expect(deleteCalled).toBe(true))
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/rooms"))
  })
})

/**
 * Covers Step 52's Scope requirement: the "Fork room" section is gated by
 * the same `canManage` boolean as the rename/AI-settings/cutoff sections
 * (visible for admin/master, hidden for reader/guest/member -- distinct
 * from the master-only "Danger zone"), triggering it drives an inline
 * progress view via `GET /rooms/:roomId/fork-jobs/:jobId` polling, and the
 * view resolves to either an "Open forked room" link (`completed`) or an
 * inline error with a working "Try again" action (`failed`).
 */
describe("RoomSettingsDrawer fork room section", () => {
  function mockFork(job: ForkJob) {
    server.use(
      http.post("/api/proxy/rooms/:roomId/fork", () => {
        return HttpResponse.json<RoomForkResponse>(
          {
            job,
            new_room: {
              ...baseRoom,
              id: job.new_room_id,
              forked_from_room_id: baseRoom.id,
              is_archived: job.status !== "completed",
            },
          },
          { status: 202 },
        )
      }),
      http.get("/api/proxy/rooms/:roomId/fork-jobs/:jobId", () => {
        return HttpResponse.json<ForkJob>(job)
      }),
    )
  }

  it.each<RoomRole>(["reader", "guest", "member"])(
    "hides the fork section for a %s viewer",
    async (role) => {
      render(
        <RoomSettingsDrawer
          open
          onOpenChange={vi.fn()}
          room={roomWithRole(role)}
          role={role}
        />,
      )

      expect(await screen.findByText("General")).toBeInTheDocument()
      expect(
        screen.queryByRole("button", { name: "Fork this room" }),
      ).not.toBeInTheDocument()
    },
  )

  it.each<RoomRole>(["admin", "master"])(
    "shows the fork section for a %s viewer",
    async (role) => {
      render(
        <RoomSettingsDrawer
          open
          onOpenChange={vi.fn()}
          room={roomWithRole(role)}
          role={role}
        />,
      )

      expect(
        await screen.findByRole("button", { name: "Fork this room" }),
      ).toBeInTheDocument()
    },
  )

  it("triggers a fork and renders the progress view while the job runs", async () => {
    mockFork({
      id: "job-1",
      source_room_id: "room-1",
      new_room_id: "room-1-fork",
      status: "running",
      total_messages: 1000,
      copied_messages: 320,
      error_message: null,
      created_at: "2026-01-03T00:00:00Z",
      updated_at: "2026-01-03T00:00:01Z",
    })

    const user = userEvent.setup()
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={vi.fn()}
        room={roomWithRole("master")}
        role="master"
      />,
    )

    await user.click(
      await screen.findByRole("button", { name: "Fork this room" }),
    )

    expect(
      await screen.findByText("Copying messages… 320 / 1000"),
    ).toBeInTheDocument()
  })

  it("renders an 'Open forked room' link once the job completes", async () => {
    mockFork({
      id: "job-2",
      source_room_id: "room-1",
      new_room_id: "room-1-fork",
      status: "completed",
      total_messages: 42,
      copied_messages: 42,
      error_message: null,
      created_at: "2026-01-03T00:00:00Z",
      updated_at: "2026-01-03T00:00:02Z",
    })

    const user = userEvent.setup()
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={vi.fn()}
        room={roomWithRole("master")}
        role="master"
      />,
    )

    await user.click(
      await screen.findByRole("button", { name: "Fork this room" }),
    )

    const openLink = await screen.findByRole("link", {
      name: "Open forked room",
    })
    expect(openLink).toHaveAttribute("href", "/rooms/room-1-fork")
  })

  it("renders the error message and a working 'Try again' button once the job fails", async () => {
    let forkCallCount = 0
    server.use(
      http.post("/api/proxy/rooms/:roomId/fork", () => {
        forkCallCount += 1
        return HttpResponse.json<RoomForkResponse>(
          {
            job: {
              id: `job-${forkCallCount}`,
              source_room_id: "room-1",
              new_room_id: "room-1-fork",
              status: "failed",
              total_messages: 10,
              copied_messages: 3,
              error_message: "Copy failed: destination room disappeared",
              created_at: "2026-01-03T00:00:00Z",
              updated_at: "2026-01-03T00:00:02Z",
            },
            new_room: {
              ...baseRoom,
              id: "room-1-fork",
              forked_from_room_id: baseRoom.id,
              is_archived: true,
            },
          },
          { status: 202 },
        )
      }),
      http.get("/api/proxy/rooms/:roomId/fork-jobs/:jobId", ({ params }) => {
        return HttpResponse.json<ForkJob>({
          id: String(params.jobId),
          source_room_id: "room-1",
          new_room_id: "room-1-fork",
          status: "failed",
          total_messages: 10,
          copied_messages: 3,
          error_message: "Copy failed: destination room disappeared",
          created_at: "2026-01-03T00:00:00Z",
          updated_at: "2026-01-03T00:00:02Z",
        })
      }),
    )

    const user = userEvent.setup()
    render(
      <RoomSettingsDrawer
        open
        onOpenChange={vi.fn()}
        room={roomWithRole("master")}
        role="master"
      />,
    )

    await user.click(
      await screen.findByRole("button", { name: "Fork this room" }),
    )

    expect(
      await screen.findByText("Copy failed: destination room disappeared"),
    ).toBeInTheDocument()

    const tryAgainButton = await screen.findByRole("button", {
      name: "Try again",
    })
    await user.click(tryAgainButton)

    await waitFor(() => expect(forkCallCount).toBe(2))
  })
})
