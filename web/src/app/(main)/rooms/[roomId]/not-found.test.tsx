import { describe, expect, it } from "vitest"
import { render, screen } from "@/test/render"
import RoomNotFound from "./not-found"

/**
 * Component-level test for the `not-found.tsx` route file rendered when
 * `app/(main)/rooms/[roomId]/page.tsx` calls `notFound()` for a room ID that
 * doesn't exist or was deleted. Exercises the rendered content directly
 * (the App Router's own routing/`notFound()` dispatch is Next.js's
 * responsibility, covered end-to-end by the Playwright spec in `web/e2e/`).
 */
describe("RoomNotFound", () => {
  it("renders a heading and a link back to /rooms", () => {
    render(<RoomNotFound />)

    expect(
      screen.getByRole("heading", { name: "Room not found" }),
    ).toBeInTheDocument()

    const link = screen.getByRole("link", { name: "Back to rooms" })
    expect(link).toHaveAttribute("href", "/rooms")
  })
})
