"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toaster } from "@/components/ui/toaster"
import { getErrorMessage } from "@/lib/get-error-message"
import { createCheckoutSession } from "../api/create-checkout-session"
import { getBalanceQueryOptions } from "../api/get-balance"
import { writeCheckoutMarker } from "../lib/checkout-marker"
import type { BillingPlan, TokenBalance } from "../types"

/**
 * Starts a Stripe Checkout Session for a `BillingPlan` and, on success,
 * performs a full browser navigation to the returned `checkout_url` —
 * Stripe Checkout is a hosted page, so this deliberately does not attempt an
 * in-app redirect or pull in `@stripe/stripe-js`.
 *
 * Immediately before that navigation, writes a `CheckoutMarker` (see
 * `../lib/checkout-marker.ts`) so the post-Checkout success page knows
 * whether to poll the subscription (`plan.interval === "month"`) or the
 * token balance (a one-time pack never changes the subscription) — for the
 * latter, the marker snapshots the *current* cached balance (`["billing",
 * "balance"]`, kept warm by the always-mounted `BalanceBadge`) as
 * `priorBalance`, since Checkout Session creation itself never changes it.
 *
 * On failure, surfaces the error via Step 17's `toaster` (matching
 * `LoginForm.tsx`'s error-surfacing convention) instead of throwing past the
 * caller.
 */
export function useCreateCheckoutSession() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (plan: BillingPlan) => createCheckoutSession(plan),
    onSuccess: (data, plan) => {
      writeCheckoutMarker(
        plan.interval === "month"
          ? { kind: "subscription" }
          : {
              kind: "token_purchase",
              priorBalance:
                queryClient.getQueryData<TokenBalance>(getBalanceQueryOptions().queryKey)
                  ?.balance ?? 0,
            },
      )
      window.location.href = data.checkout_url
    },
    onError: (err) => {
      toaster.create({
        type: "error",
        title: "Could not start checkout",
        description: getErrorMessage(err, "Please try again."),
      })
    },
  })
}
