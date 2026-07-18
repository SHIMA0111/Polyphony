"use client"

import { useQuery } from "@tanstack/react-query"
import { getBalanceQueryOptions } from "../api/get-balance"

/** Refetch interval for the top-bar balance widget, in milliseconds. */
const BALANCE_REFETCH_INTERVAL_MS = 15_000

/**
 * The current user's token balance, sourced from
 * `GET /api/proxy/billing/balance`.
 *
 * Polls every {@link BALANCE_REFETCH_INTERVAL_MS} so the top-bar
 * `BalanceBadge` reflects usage from other sessions/rooms without a manual
 * refresh; a successful AI send additionally invalidates the
 * `["billing", "balance"]` query directly (see
 * `web/src/features/messages/hooks/use-chat-room.ts`'s `handleSendWithAI`)
 * so the widget updates immediately after *this* session's own send, rather
 * than waiting for the next poll tick.
 */
export function useBalance() {
  return useQuery({
    ...getBalanceQueryOptions(),
    refetchInterval: BALANCE_REFETCH_INTERVAL_MS,
  })
}
