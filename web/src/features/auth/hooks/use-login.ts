"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { submitLoginFlow, type LoginFlowValues } from "../api/login-flow"
import type { UiContainer } from "../utils/kratos-flow"

/**
 * Submits a Kratos login flow via {@link submitLoginFlow}.
 *
 * On success (`{ ok: true }`, meaning Kratos established the session),
 * invalidates the cached `["auth", "session"]` query so `useSession`
 * refetches the now-authenticated session, and navigates to `/rooms`.
 *
 * On a validation/credential failure (`{ ok: false, flow }`), no side
 * effect runs here — the mutation still resolves (it never throws for this
 * case), and the caller (`LoginForm`) is expected to inspect the result and
 * re-render with the returned `flow` itself.
 */
export function useLogin() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: ({
      flow,
      values,
    }: {
      flow: UiContainer
      values: LoginFlowValues
    }) => submitLoginFlow(flow, values),
    onSuccess: async (result) => {
      if (!result.ok) return
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] })
      router.push("/rooms")
    },
  })
}
