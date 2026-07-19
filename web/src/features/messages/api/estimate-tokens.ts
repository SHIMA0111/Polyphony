import { apiRequest } from "@/lib/http-client"
import type { EstimateChatMessage, TokenEstimateResponse } from "../types"

/**
 * Calls `POST /api/proxy/tokens/estimate` (Step 27's Go proxy in front of
 * the LLM Gateway's character-based token heuristic). Unlike `GET /models`,
 * this route requires an authenticated caller — `apiRequest` already
 * forwards the browser's Kratos session cookie through the BFF proxy, so no
 * extra auth wiring is needed here.
 *
 * `messages` may be empty (a valid input representing an empty draft with
 * no prior context), in which case the response is just the model's fixed
 * overhead.
 */
export function estimateTokens(
  model: string,
  messages: EstimateChatMessage[],
): Promise<TokenEstimateResponse> {
  return apiRequest<TokenEstimateResponse>("/tokens/estimate", {
    method: "POST",
    body: JSON.stringify({ model, messages }),
  })
}
