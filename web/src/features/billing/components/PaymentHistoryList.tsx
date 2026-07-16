"use client"

import { Badge, Box, Button, Flex, Skeleton, Table, Text } from "@chakra-ui/react"
import { formatCurrency, formatUtcDate } from "@/lib/format"
import { usePaymentHistory } from "../hooks/use-payment-history"
import type { Payment, PaymentStatus } from "../types"

/** `Badge` `colorPalette` per `PaymentStatus`. */
const STATUS_COLOR_PALETTE: Record<PaymentStatus, string> = {
  succeeded: "green",
  pending: "orange",
  failed: "red",
  refunded: "gray",
}

/** Number of skeleton rows shown while the first page is loading. */
const SKELETON_ROW_COUNT = 5

/**
 * Derives a human-readable row label from `kind` + `tokens_credited` — Step
 * 49's `PaymentRecordResponse` has no free-text description field.
 */
function describePayment(payment: Payment): string {
  const kindLabel = payment.kind === "subscription" ? "Subscription renewal" : "Token top-up"
  return `${kindLabel} (+${payment.tokens_credited.toLocaleString("en-US")} tokens)`
}

/**
 * Paginated table of the current user's past payments
 * (`usePaymentHistory()`, `GET /billing/payments`), rendered by
 * `app/(main)/billing/history/page.tsx`.
 *
 * Columns: date, a description derived from `kind`/`tokens_credited` (no
 * receipt/invoice link — Stripe-hosted receipts remain reachable via the
 * billing portal instead, per Step 49's contract), amount, and a status
 * `Badge`. A "Load more" button fetches the next page while `hasNextPage`
 * is true.
 */
export function PaymentHistoryList() {
  const {
    data,
    isPending,
    isError,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = usePaymentHistory()

  if (isPending) {
    return (
      <Flex direction="column" gap={2}>
        {Array.from({ length: SKELETON_ROW_COUNT }).map((_, i) => (
          <Skeleton key={i} h="10" rounded="md" />
        ))}
      </Flex>
    )
  }

  if (isError) {
    return (
      <Box borderWidth="1px" borderColor="border" rounded="lg" p={6} textAlign="center">
        <Text color="fg.error">Failed to load payment history.</Text>
      </Box>
    )
  }

  const payments = data.pages.flatMap((page) => page.payments)

  if (payments.length === 0) {
    return (
      <Box
        borderWidth="1px"
        borderStyle="dashed"
        borderColor="border"
        rounded="lg"
        p={6}
        textAlign="center"
      >
        <Text color="fg.muted">No payments yet</Text>
      </Box>
    )
  }

  return (
    <Flex direction="column" gap={4}>
      <Table.Root size="sm" variant="line">
        <Table.Header>
          <Table.Row>
            <Table.ColumnHeader>Date</Table.ColumnHeader>
            <Table.ColumnHeader>Description</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Amount</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Status</Table.ColumnHeader>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {payments.map((payment) => (
            <Table.Row key={payment.id}>
              <Table.Cell color="fg.muted" fontSize="sm">
                {formatUtcDate(payment.created_at)}
              </Table.Cell>
              <Table.Cell>{describePayment(payment)}</Table.Cell>
              <Table.Cell textAlign="end" fontWeight="medium">
                {formatCurrency(payment.amount_cents, payment.currency)}
              </Table.Cell>
              <Table.Cell textAlign="end">
                <Badge
                  variant="subtle"
                  colorPalette={STATUS_COLOR_PALETTE[payment.status]}
                  // Own, directly-assertable test hook (see `BalanceBadge`'s
                  // `data-low-balance` for the same rationale).
                  data-status={payment.status}
                >
                  {payment.status}
                </Badge>
              </Table.Cell>
            </Table.Row>
          ))}
        </Table.Body>
      </Table.Root>

      {hasNextPage && (
        <Flex justify="center">
          <Button
            variant="outline"
            size="sm"
            onClick={() => fetchNextPage()}
            loading={isFetchingNextPage}
          >
            Load more
          </Button>
        </Flex>
      )}
    </Flex>
  )
}
