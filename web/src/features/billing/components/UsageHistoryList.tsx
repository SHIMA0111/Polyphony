"use client"

import { Badge, Box, Button, Flex, Skeleton, Table, Text } from "@chakra-ui/react"
import { formatDateTimeUtc } from "@/lib/format"
import { useUsageHistory } from "../hooks/use-usage-history"
import type { TransactionType } from "../types"

/** `Badge` `colorPalette` per `TransactionType`. */
const TYPE_COLOR_PALETTE: Record<TransactionType, string> = {
  consumption: "orange",
  charge: "green",
  adjustment: "purple",
}

/** Number of skeleton rows shown while the first page is loading. */
const SKELETON_ROW_COUNT = 5

/**
 * Formats a signed token amount with an explicit `+`/`-` prefix, matching
 * the ledger convention documented on `TokenTransaction.amount` (negative
 * for consumption, positive for charge/adjustment) — plain token counts,
 * never a currency formatter.
 */
function formatSignedAmount(amount: number): string {
  const sign = amount > 0 ? "+" : amount < 0 ? "-" : ""
  return `${sign}${Math.abs(amount).toLocaleString("en-US")}`
}

/**
 * Paginated table of the current user's token usage/transaction history
 * (`useUsageHistory()`, `GET /billing/transactions`), rendered by
 * `app/(main)/billing/usage/page.tsx`.
 *
 * Columns: date, type (`consumption`/`charge`/`adjustment`, colored per
 * {@link TYPE_COLOR_PALETTE}), room (the raw `room_id`, or "—" for
 * transactions not tied to a room — no room-name resolution is attempted,
 * matching the "display what the API actually returns" convention already
 * used for member ids elsewhere in this codebase), signed amount, and the
 * running `balance_after`. A "Load more" button fetches the next page while
 * `hasNextPage` is true.
 */
export function UsageHistoryList() {
  const {
    data,
    isPending,
    isError,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useUsageHistory()

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
        <Text color="fg.error">Failed to load usage history.</Text>
      </Box>
    )
  }

  const transactions = data.pages.flatMap((page) => page.transactions)

  if (transactions.length === 0) {
    return (
      <Box
        borderWidth="1px"
        borderStyle="dashed"
        borderColor="border"
        rounded="lg"
        p={6}
        textAlign="center"
      >
        <Text color="fg.muted">No usage yet</Text>
      </Box>
    )
  }

  return (
    <Flex direction="column" gap={4}>
      <Table.Root size="sm" variant="line">
        <Table.Header>
          <Table.Row>
            <Table.ColumnHeader>Date</Table.ColumnHeader>
            <Table.ColumnHeader>Type</Table.ColumnHeader>
            <Table.ColumnHeader>Room</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Amount</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Balance</Table.ColumnHeader>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {transactions.map((txn) => (
            <Table.Row key={txn.id}>
              <Table.Cell color="fg.muted" fontSize="sm">
                {formatDateTimeUtc(txn.created_at)}
              </Table.Cell>
              <Table.Cell>
                <Badge variant="subtle" colorPalette={TYPE_COLOR_PALETTE[txn.type]}>
                  {txn.type}
                </Badge>
              </Table.Cell>
              <Table.Cell color="fg.muted" fontSize="sm">
                {txn.room_id ?? "—"}
              </Table.Cell>
              <Table.Cell
                textAlign="end"
                fontWeight="medium"
                color={txn.amount > 0 ? "green.fg" : txn.amount < 0 ? "red.fg" : "fg"}
              >
                {formatSignedAmount(txn.amount)}
              </Table.Cell>
              <Table.Cell textAlign="end" color="fg.muted">
                {txn.balance_after.toLocaleString("en-US")}
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
