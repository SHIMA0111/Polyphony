import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import type { BatchInviteByGroupResult, Group } from "@/features/groups/types"
import { InviteDialog } from "./InviteDialog"

/**
 * Component tests covering only Step 46's addition to `InviteDialog`: the
 * "Invite a group" section. The existing invite-by-username and
 * generate-link sections are Step 37's territory and already untested at
 * the component level (this is the first dedicated test file for
 * `InviteDialog`), so this file is scoped to the new third section only.
 *
 * `GroupPicker` is stubbed out here — its own selection mechanics
 * (rendering groups from `GET /api/proxy/groups`, the `/groups` empty-state
 * link, and calling `onSelect`) are already covered by
 * `GroupPicker.test.tsx` — rather than driven through its real `Menu.Root`
 * in this file: a `Menu` nested inside this `Dialog.Root` briefly carries a
 * transient `pointer-events: none` while opening/closing (the same class of
 * nested-overlay interaction already documented on
 * `TransferOwnershipDialog`), which reads as flaky under jsdom's synthetic
 * timers even though it is a real-browser non-issue. Stubbing the picker
 * isolates `InviteDialog`'s own wiring (selected group + role -> mutation
 * body, success -> invited-count/skip-reason summary) from that unrelated
 * concern.
 */
const { fixtureGroup } = vi.hoisted(() => ({
  fixtureGroup: {
    id: "group-1",
    owner_id: "user-1",
    name: "Team A",
    description: "A test group",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } satisfies Group,
}))

vi.mock("@/features/groups/components/GroupPicker", () => ({
  GroupPicker: ({ onSelect }: { onSelect: (group: Group) => void }) => (
    <button type="button" onClick={() => onSelect(fixtureGroup)}>
      Select Team A (stub)
    </button>
  ),
}))

describe("InviteDialog - Invite a group section", () => {
  it("renders a GroupPicker inside the Invite a group section", async () => {
    const user = userEvent.setup()
    render(<InviteDialog roomId="room-1" />)

    await user.click(screen.getByRole("button", { name: "Invite" }))

    expect(screen.getByText("Invite a group")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Select Team A (stub)" }),
    ).toBeInTheDocument()
  })

  it("submitting with a selected group and role calls batch-invite-by-group with the right body", async () => {
    let capturedBody: unknown
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/invitations/batch-by-group",
        async ({ request }) => {
          capturedBody = await request.json()
          return HttpResponse.json<BatchInviteByGroupResult>({
            invited: [],
            skipped: [],
          })
        },
      ),
    )

    const user = userEvent.setup()
    render(<InviteDialog roomId="room-1" />)

    await user.click(screen.getByRole("button", { name: "Invite" }))
    await user.click(screen.getByRole("button", { name: "Select Team A (stub)" }))

    // Three role `<select>`s exist in this dialog (invite-by-username,
    // generate-link, invite-a-group); the group section's is the last one.
    const roleSelects = screen.getAllByRole("combobox")
    await user.selectOptions(roleSelects[roleSelects.length - 1], "admin")

    await user.click(screen.getByRole("button", { name: "Invite group" }))

    await waitFor(() =>
      expect(capturedBody).toEqual({ group_id: "group-1", role: "admin" }),
    )
  })

  it("renders the invited-count/skip-reason summary after a successful batch invite", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/invitations/batch-by-group", () => {
        return HttpResponse.json<BatchInviteByGroupResult>({
          invited: [
            {
              id: "invitation-1",
              room_id: "room-1",
              inviter_id: "user-1",
              invitee_id: "user-3",
              invite_code: "code-1",
              role: "member",
              status: "pending",
              expires_at: "2026-02-01T00:00:00Z",
              created_at: "2026-01-01T00:00:00Z",
            },
          ],
          skipped: [
            { user_id: "user-4", username: "carol", reason: "already invited" },
          ],
        })
      }),
    )

    const user = userEvent.setup()
    render(<InviteDialog roomId="room-1" />)

    await user.click(screen.getByRole("button", { name: "Invite" }))
    await user.click(screen.getByRole("button", { name: "Select Team A (stub)" }))
    await user.click(screen.getByRole("button", { name: "Invite group" }))

    expect(await screen.findByText("Invited 1 member(s).")).toBeInTheDocument()
    expect(screen.getByText("carol: already invited")).toBeInTheDocument()
  })

  it("rejects a fractional expires-in-hours value client-side without calling the server", async () => {
    let called = false
    server.use(
      http.post("/api/proxy/rooms/:roomId/invitations/batch-by-group", () => {
        called = true
        return HttpResponse.json<BatchInviteByGroupResult>({ invited: [], skipped: [] })
      }),
    )

    const user = userEvent.setup()
    render(<InviteDialog roomId="room-1" />)

    await user.click(screen.getByRole("button", { name: "Invite" }))
    await user.click(screen.getByRole("button", { name: "Select Team A (stub)" }))
    await user.type(screen.getByPlaceholderText("e.g., 168"), "1.5")
    await user.click(screen.getByRole("button", { name: "Invite group" }))

    expect(
      await screen.findByText(
        "Expiration must be a whole number of hours between 1 and 720.",
      ),
    ).toBeInTheDocument()
    expect(called).toBe(false)
  })
})
