import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { ModelInfo, ModelListResponse } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["models"]` query
 * (`GET /api/proxy/models`). Unwraps the raw `{ models: ModelInfo[] }`
 * envelope so cached/hook data is a plain `ModelInfo[]`, ready to hand
 * straight to `ModelSelector`/`MessageInput`.
 */
export function getModelsQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["models"] as const,
    queryFn: async (): Promise<ModelInfo[]> => {
      const res = await fetcher<ModelListResponse>("/models")
      return res.models
    },
  })
}
