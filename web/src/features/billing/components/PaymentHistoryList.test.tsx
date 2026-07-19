import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import type { Payment, PaymentHistoryPage } from "../types"
import { PaymentHistoryList } from "./PaymentHistoryList"

/**
 * Mirrors `PaymentHistoryList.tsx`'s own amount formatting so assertions
 * don't hardcode locale output — reads the minor-unit exponent from the
 * formatter's own `resolvedOptions()` rather than assuming `/ 100`, since
 * that's not true for every currency (e.g. JPY has 0 decimal digits, KWD
 * has 3), except for the Stripe special cases below (ISK, UGX): ISO 4217
 * treats them as zero-decimal, but Stripe always represents amounts in
 * these two currencies with 2 decimal digits regardless.
 */
const STRIPE_EXPONENT_OVERRIDES: Record<string, number> = {
  isk: 2,
  ugx: 2,
}

function formatAmount(amountCents: number, currency: string): string {
  const formatter = new Intl.NumberFormat(undefined, { style: "currency", currency })
  const exponent =
    STRIPE_EXPONENT_OVERRIDES[currency.toLowerCase()] ??
    formatter.resolvedOptions().maximumFractionDigits ??
    2
  // `getByText`'s whitespace-collapsing normalizer only runs on the DOM's
  // own text, not on this expected string (see `matches.js`'s
  // `getDefaultNormalizer`) — some currency formats (e.g. KWD) separate the
  // symbol from the amount with a non-breaking space, which the normalizer
  // collapses to a regular space, so this must match that too.
  return formatter.format(amountCents / 10 ** exponent).replace(/\u00a0/g, " ")
}

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

  it("formats zero-decimal (JPY) and three-decimal (KWD) amounts correctly", async () => {
    const jpyPayment: Payment = {
      id: "pay-jpy",
      kind: "token_purchase",
      amount_cents: 500,
      currency: "jpy",
      tokens_credited: 50_000,
      status: "succeeded",
      stripe_reference_id: "cs_test_jpy",
      created_at: "2026-01-05T00:00:00Z",
    }
    const kwdPayment: Payment = {
      id: "pay-kwd",
      kind: "token_purchase",
      amount_cents: 1_500,
      currency: "kwd",
      tokens_credited: 150_000,
      status: "succeeded",
      stripe_reference_id: "cs_test_kwd",
      created_at: "2026-01-04T00:00:00Z",
    }

    server.use(
      http.get("/api/proxy/billing/payments", () => {
        return HttpResponse.json<PaymentHistoryPage>({
          payments: [jpyPayment, kwdPayment],
          next_cursor: null,
        })
      }),
    )

    render(<PaymentHistoryList />)

    await waitFor(() => expect(screen.getAllByText(/Token top-up/)).toHaveLength(2))

    // JPY is zero-decimal: 500 minor units is ¥500, not ¥5.00.
    expect(
      screen.getByText(formatAmount(jpyPayment.amount_cents, jpyPayment.currency)),
    ).toBeInTheDocument()
    // KWD uses 3 decimal digits: 1500 minor units is KD 1.500, not KD 15.00.
    expect(
      screen.getByText(formatAmount(kwdPayment.amount_cents, kwdPayment.currency)),
    ).toBeInTheDocument()
  })

  it("formats Stripe's ISK/UGX special cases as 2-decimal despite Intl treating them as zero-decimal", async () => {
    const iskPayment: Payment = {
      id: "pay-isk",
      kind: "token_purchase",
      amount_cents: 500,
      currency: "isk",
      tokens_credited: 50_000,
      status: "succeeded",
      stripe_reference_id: "cs_test_isk",
      created_at: "2026-01-07T00:00:00Z",
    }
    const ugxPayment: Payment = {
      id: "pay-ugx",
      kind: "token_purchase",
      amount_cents: 1_500,
      currency: "ugx",
      tokens_credited: 150_000,
      status: "succeeded",
      stripe_reference_id: "cs_test_ugx",
      created_at: "2026-01-06T00:00:00Z",
    }

    server.use(
      http.get("/api/proxy/billing/payments", () => {
        return HttpResponse.json<PaymentHistoryPage>({
          payments: [iskPayment, ugxPayment],
          next_cursor: null,
        })
      }),
    )

    render(<PaymentHistoryList />)

    await waitFor(() => expect(screen.getAllByText(/Token top-up/)).toHaveLength(2))

    // Without the override, `Intl` would treat ISK as zero-decimal and
    // render 500 minor units as ISK 500 -- 100x the correct amount. Stripe's
    // own convention divides by 100 regardless, same as USD.
    expect(
      screen.getByText(formatAmount(iskPayment.amount_cents, iskPayment.currency)),
    ).toBeInTheDocument()
    expect(
      screen.getByText(formatAmount(ugxPayment.amount_cents, ugxPayment.currency)),
    ).toBeInTheDocument()
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
