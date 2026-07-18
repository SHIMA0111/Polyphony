import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import type { TokenTransactionPage } from "../types"
import { UsageHistoryList } from "./UsageHistoryList"

/**
 * Component-level tests for `UsageHistoryList`, covering:
 * - rows rendering from a mocked `GET /api/proxy/billing/transactions`
 *   (the fixture from `../api/handlers.ts`, including a signed amount and a
 *   room-less transaction rendering "—" instead of resolving a room name);
 * - "Load more" fetching the next page using the returned `next_cursor`;
 * - the empty state rendering when the list is empty.
 */
describe("UsageHistoryList", () => {
  it("renders transaction rows from GET /billing/transactions", async () => {
    render(<UsageHistoryList />)

    await waitFor(() =>
      expect(screen.getByText("-300")).toBeInTheDocument(),
    )

    // Signed amounts: negative (consumption) and positive (charge).
    expect(screen.getByText("+5,000")).toBeInTheDocument()

    // Transaction types render as distinct badges.
    expect(screen.getByText("consumption")).toBeInTheDocument()
    expect(screen.getByText("charge")).toBeInTheDocument()

    // Room id displayed verbatim for a room-scoped transaction...
    expect(screen.getByText("room-1")).toBeInTheDocument()
    // ...and "—" for a transaction with no room_id (e.g. a top-up).
    expect(screen.getAllByText("—").length).toBeGreaterThan(0)
  })

  it("fetches the next page via Load more using next_cursor", async () => {
    const user = userEvent.setup()
    render(<UsageHistoryList />)

    await waitFor(() =>
      expect(screen.getByText("-300")).toBeInTheDocument(),
    )

    expect(screen.queryByText("adjustment")).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Load more" }))

    await waitFor(() =>
      expect(screen.getByText("adjustment")).toBeInTheDocument(),
    )

    // First page's rows remain rendered alongside the newly-loaded page.
    expect(screen.getByText("charge")).toBeInTheDocument()
    // The second (final) page has no further cursor, so "Load more" is gone.
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument()
  })

  it("renders the empty state when there is no usage history", async () => {
    server.use(
      http.get("/api/proxy/billing/transactions", () => {
        return HttpResponse.json<TokenTransactionPage>({
          transactions: [],
          next_cursor: null,
        })
      }),
    )

    render(<UsageHistoryList />)

    await waitFor(() => expect(screen.getByText("No usage yet")).toBeInTheDocument())
  })
})
