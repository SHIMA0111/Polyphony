"use client"

import { useQuery } from "@tanstack/react-query"
import { getModelsQueryOptions } from "../api/get-models"

/** Available LLM models, sourced from `GET /api/proxy/models`. */
export function useModels() {
  return useQuery(getModelsQueryOptions())
}
