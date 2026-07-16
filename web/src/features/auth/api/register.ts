import { authRequest } from "@/lib/http-client"

export interface RegisterInput {
  email: string
  username: string
  password: string
}

/**
 * Calls `POST /api/auth/register` (Step 4's BFF route handler), which
 * creates the account with the Go API and sets the httpOnly session cookie.
 */
export function register(input: RegisterInput): Promise<{ ok: true }> {
  return authRequest<{ ok: true }>("/register", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
