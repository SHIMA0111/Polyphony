"use client"

import { useMutation } from "@tanstack/react-query"
import { toaster } from "@/components/ui/toaster"
import { createCheckoutSession } from "../api/create-checkout-session"
import { useBalance } from "./use-balance"
import { writeCheckoutMarker } from "../utils/checkout-marker"
import type { BillingPlan } from "../types"

/**
 * Starts a Stripe Checkout Session for a `BillingPlan` and, on success,
 * performs a full browser navigation to the returned `checkout_url` —
 * Stripe Checkout is a hosted page, so this deliberately does not attempt an
 * in-app redirect or pull in `@stripe/stripe-js`.
 *
 * Immediately before that navigation, writes a `checkout-marker.ts` marker
 * so `checkout/success/page.tsx` knows which query to poll once Stripe
 * redirects back: `plan.interval === "month"` marks a subscription
 * (`{ kind: "subscription" }`); a one-time token pack marks
 * `{ kind: "token_purchase", priorBalance }`, capturing the balance
 * (`useBalance()`'s currently cached value, defaulting to `0` if it hasn't
 * loaded yet) *before* the purchase so the success page can detect the
 * credit by watching for the balance to exceed it.
 *
 * On failure, surfaces the error via Step 17's `toaster` (matching
 * `LoginForm.tsx`'s error-surfacing convention) instead of throwing past the
 * caller.
 */
export function useCreateCheckoutSession() {
  const { data: balance } = useBalance()

  return useMutation({
    mutationFn: (plan: BillingPlan) => createCheckoutSession(plan),
    onSuccess: (data, plan) => {
      writeCheckoutMarker(
        plan.interval === "month"
          ? { kind: "subscription" }
          : { kind: "token_purchase", priorBalance: balance?.balance ?? 0 },
      )
      window.location.href = data.checkout_url
    },
    onError: (err) => {
      toaster.create({
        type: "error",
        title: "Could not start checkout",
        description: err instanceof Error ? err.message : "Please try again.",
      })
    },
  })
}
