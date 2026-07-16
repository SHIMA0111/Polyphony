"use client"

import { useMutation } from "@tanstack/react-query"
import { toaster } from "@/components/ui/toaster"
import { getErrorMessage } from "@/lib/get-error-message"
import { createCheckoutSession } from "../api/create-checkout-session"
import type { BillingPlan } from "../types"

/**
 * Starts a Stripe Checkout Session for a `BillingPlan` and, on success,
 * performs a full browser navigation to the returned `checkout_url` —
 * Stripe Checkout is a hosted page, so this deliberately does not attempt an
 * in-app redirect or pull in `@stripe/stripe-js`.
 *
 * On failure, surfaces the error via Step 17's `toaster` (matching
 * `LoginForm.tsx`'s error-surfacing convention) instead of throwing past the
 * caller.
 */
export function useCreateCheckoutSession() {
  return useMutation({
    mutationFn: (plan: BillingPlan) => createCheckoutSession(plan),
    onSuccess: (data) => {
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
