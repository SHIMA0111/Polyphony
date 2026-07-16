import { authRequest } from "@/lib/http-client"

/**
 * Calls `POST /api/auth/logout` (Step 4's BFF route handler), which clears
 * the httpOnly session cookie.
 */
export function logout(): Promise<{ ok: true }> {
  return authRequest<{ ok: true }>("/logout", { method: "POST" })
}
