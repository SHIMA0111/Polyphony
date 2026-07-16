"use client"

import { useMutation } from "@tanstack/react-query"
import { toaster } from "@/components/ui/toaster"
import { getErrorMessage } from "@/lib/get-error-message"
import { createBillingPortalSession } from "../api/create-billing-portal-session"

/**
 * Starts a Stripe Billing Portal session for the current user and, on
 * success, performs a full browser navigation to the returned `portal_url`
 * (a Stripe-hosted page covering plan cancellation and payment-method
 * updates — see `docs/tasks/step53.md`'s Out of scope).
 *
 * On failure, surfaces the error via Step 17's `toaster`.
 */
export function useCreateBillingPortalSession() {
  return useMutation({
    mutationFn: () => createBillingPortalSession(),
    onSuccess: (data) => {
      window.location.href = data.portal_url
    },
    onError: (err) => {
      toaster.create({
        type: "error",
        title: "Could not open billing portal",
        description: getErrorMessage(err, "Please try again."),
      })
    },
  })
}
