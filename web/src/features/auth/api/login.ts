import { authRequest } from "@/lib/http-client"

export interface LoginInput {
  email: string
  password: string
}

/**
 * Calls `POST /api/auth/login` (Step 4's BFF route handler), which exchanges
 * credentials with the Go API and sets the httpOnly session cookie. The
 * response body never contains the token itself.
 */
export function login(input: LoginInput): Promise<{ ok: true }> {
  return authRequest<{ ok: true }>("/login", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
