import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { fixtureBalance, fixtureZeroBalance } from "../api/handlers"
import type { TokenBalance } from "../types"
import { BalanceBadge } from "./BalanceBadge"

/**
 * Component-level tests for `BalanceBadge`, covering:
 * - the formatted balance rendering from a mocked
 *   `GET /api/proxy/billing/balance` (the fixture from `../api/handlers.ts`);
 * - the low-balance visual treatment (`colorPalette="red"`) applying when
 *   `balance` is `0`;
 * - linking to `/billing/usage`.
 */
describe("BalanceBadge", () => {
  it("renders the formatted balance from GET /billing/balance", async () => {
    render(<BalanceBadge />)

    await waitFor(() =>
      expect(
        screen.getByText(fixtureBalance.balance.toLocaleString("en-US")),
      ).toBeInTheDocument(),
    )
  })

  it("links to /billing/usage", async () => {
    render(<BalanceBadge />)

    await waitFor(() =>
      expect(
        screen.getByText(fixtureBalance.balance.toLocaleString("en-US")),
      ).toBeInTheDocument(),
    )

    expect(screen.getByRole("link", { name: /view token usage history/i })).toHaveAttribute(
      "href",
      "/billing/usage",
    )
  })

  it("shows the low-balance treatment when balance is 0", async () => {
    server.use(
      http.get("/api/proxy/billing/balance", () => {
        return HttpResponse.json<TokenBalance>(fixtureZeroBalance)
      }),
    )

    render(<BalanceBadge />)

    await waitFor(() => expect(screen.getByText("0")).toBeInTheDocument())

    const link = screen.getByRole("link", { name: /view token usage history/i })
    expect(link.querySelector('[data-low-balance="true"]')).toBeInTheDocument()
  })

  it("does not apply the low-balance treatment for a positive balance", async () => {
    render(<BalanceBadge />)

    await waitFor(() =>
      expect(
        screen.getByText(fixtureBalance.balance.toLocaleString("en-US")),
      ).toBeInTheDocument(),
    )

    const link = screen.getByRole("link", { name: /view token usage history/i })
    expect(link.querySelector('[data-low-balance="false"]')).toBeInTheDocument()
  })
})
