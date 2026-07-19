"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import {
  submitRegistrationFlow,
  type RegistrationFlowValues,
} from "../api/registration-flow"
import type { UiContainer } from "../utils/kratos-flow"

/**
 * Submits a Kratos registration flow via {@link submitRegistrationFlow}.
 *
 * On success (`{ ok: true }`, meaning Kratos created the account and
 * established the session), invalidates the cached `["auth", "session"]`
 * query so `useSession` refetches the now-authenticated session, and
 * navigates to `/rooms`.
 *
 * On a validation failure (`{ ok: false, flow }`, e.g. "email already in
 * use"), no side effect runs here — the mutation still resolves (it never
 * throws for this case), and the caller (`RegisterForm`) is expected to
 * inspect the result and re-render with the returned `flow` itself.
 */
export function useRegister() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: ({
      flow,
      values,
    }: {
      flow: UiContainer
      values: RegistrationFlowValues
    }) => submitRegistrationFlow(flow, values),
    onSuccess: async (result) => {
      if (!result.ok) return
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] })
      router.push("/rooms")
    },
  })
}
