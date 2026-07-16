import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import type { PaymentHistoryPage } from "../types"
import { PaymentHistoryList } from "./PaymentHistoryList"

/**
 * Component-level tests for `PaymentHistoryList`, covering:
 * - rows rendering from a mocked `GET /api/proxy/billing/payments` (the
 *   fixture from `../api/handlers.ts`);
 * - "Load more" fetching the next page using the returned `next_cursor`;
 * - the empty state rendering when the list is empty.
 */
describe("PaymentHistoryList", () => {
  it("renders payment rows from GET /billing/payments", async () => {
    render(<PaymentHistoryList />)

    await waitFor(() =>
      expect(screen.getByText(/Subscription renewal/)).toBeInTheDocument(),
    )

    expect(screen.getByText(/Token top-up/)).toBeInTheDocument()
    expect(screen.getByText(/\+500,000 tokens/)).toBeInTheDocument()
    expect(screen.getByText(/\+100,000 tokens/)).toBeInTheDocument()

    // Both fixture rows are "succeeded".
    const succeededBadges = screen.getAllByText("succeeded")
    expect(succeededBadges).toHaveLength(2)
    expect(succeededBadges[0]).toHaveAttribute("data-status", "succeeded")
  })

  it("fetches the next page via Load more using next_cursor", async () => {
    const user = userEvent.setup()
    render(<PaymentHistoryList />)

    await waitFor(() =>
      expect(screen.getAllByText(/Subscription renewal/)).toHaveLength(1),
    )

    await user.click(screen.getByRole("button", { name: "Load more" }))

    await waitFor(() =>
      expect(screen.getAllByText(/Subscription renewal/)).toHaveLength(2),
    )

    // The second (final) page has no further cursor, so "Load more" is gone.
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument()
  })

  it("renders the empty state when there are no payments", async () => {
    server.use(
      http.get("/api/proxy/billing/payments", () => {
        return HttpResponse.json<PaymentHistoryPage>({
          payments: [],
          next_cursor: null,
        })
      }),
    )

    render(<PaymentHistoryList />)

    await waitFor(() => expect(screen.getByText("No payments yet")).toBeInTheDocument())
  })
})
