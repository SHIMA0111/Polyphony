"use client"

import { useMemo, useState } from "react"
import { Badge, Box, Button, Flex, Popover, Portal, Text } from "@chakra-ui/react"
import { Check, ChevronDown, Image as ImageIcon } from "lucide-react"
import type { ModelInfo } from "../types"

/**
 * Alias kept for consumers importing `Model` from this module -- the type is
 * the shared `ModelInfo` (`web/src/features/messages/types.ts`), not a
 * parallel duplicate, to avoid drift between the two.
 */
export type Model = ModelInfo

interface ModelSelectorProps {
  models: ModelInfo[]
  selectedModel: ModelInfo
  onModelSelect: (model: ModelInfo) => void
}

/** Formats a token count for compact display, e.g. `272_000` -> `"272K"`. */
function formatContextWindow(tokens: number): string {
  if (tokens <= 0) return "?"
  if (tokens % 1_000_000 === 0) return `${tokens / 1_000_000}M`
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`
  return `${tokens}`
}

/**
 * Formats a USD per-1M-token price for compact display, e.g. `2.5` ->
 * `"$2.50"`. `0` (and any non-positive value) means "unknown" per
 * `ModelInfo`'s type contract, mirroring {@link formatContextWindow}'s
 * existing unknown convention.
 */
function formatPrice(price: number): string {
  if (price <= 0) return "?"
  return `$${price.toFixed(2)}`
}

/**
 * Builds the secondary metadata line shown under a model's name, e.g.
 * `"272K ctx · $1.25 / $10.00 per 1M"`. Omitted entirely when neither the
 * context window nor pricing is known (both are `0`).
 *
 * Exported so other model pickers (e.g.
 * `features/rooms/components/RoomSettingsDrawer.tsx`'s AI-default select)
 * can reuse the same formatting instead of re-deriving it.
 */
export function formatModelMeta(model: ModelInfo): string | null {
  const parts: string[] = []
  if (model.context_window > 0) {
    parts.push(`${formatContextWindow(model.context_window)} ctx`)
  }
  if (
    model.input_price_per_million_tokens > 0 ||
    model.output_price_per_million_tokens > 0
  ) {
    parts.push(
      `${formatPrice(model.input_price_per_million_tokens)} / ${formatPrice(model.output_price_per_million_tokens)} per 1M`,
    )
  }
  return parts.length > 0 ? parts.join(" · ") : null
}

/** Groups models by provider, preserving each group's first-seen order. */
function groupByProvider(models: ModelInfo[]): Map<string, ModelInfo[]> {
  const groups = new Map<string, ModelInfo[]>()
  for (const model of models) {
    const group = groups.get(model.provider)
    if (group) {
      group.push(model)
    } else {
      groups.set(model.provider, [model])
    }
  }
  return groups
}

export function ModelSelector({
  models,
  selectedModel,
  onModelSelect,
}: ModelSelectorProps) {
  const [open, setOpen] = useState(false)
  const groupedModels = useMemo(() => groupByProvider(models), [models])

  const handleSelect = (model: ModelInfo) => {
    onModelSelect(model)
    setOpen(false)
  }

  return (
    <Popover.Root
      open={open}
      onOpenChange={(e) => setOpen(e.open)}
      positioning={{ placement: "top-start" }}
      lazyMount
      unmountOnExit
    >
      <Popover.Trigger asChild>
        <Button
          variant="ghost"
          size="sm"
          h={8}
          px={2}
          fontSize="xs"
          fontWeight="medium"
          color="fg.muted"
          rounded="lg"
          gap={1}
        >
          {selectedModel.name}
          <ChevronDown size={12} opacity={0.5} />
        </Button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content w="xs" p={1.5}>
            <Popover.Body p={0}>
              <Flex direction="column" gap={0.5}>
                {Array.from(groupedModels.entries()).map(
                  ([provider, providerModels]) => (
                    <Box key={provider}>
                      <Text
                        fontSize="2xs"
                        color="fg.muted"
                        fontWeight="semibold"
                        textTransform="uppercase"
                        px={3}
                        pt={2}
                        pb={1}
                      >
                        {provider}
                      </Text>
                      {providerModels.map((model) => {
                        const isSelected = model.id === selectedModel.id
                        const meta = formatModelMeta(model)

                        return (
                          <Box
                            key={model.id}
                            as="button"
                            display="flex"
                            alignItems="center"
                            gap={3}
                            w="full"
                            rounded="md"
                            px={3}
                            py={2}
                            textAlign="left"
                            cursor="pointer"
                            bg={isSelected ? "bg.muted" : "transparent"}
                            _hover={{ bg: "bg.muted" }}
                            transition="backgrounds"
                            onClick={() => handleSelect(model)}
                          >
                            <Flex flex={1} align="center" gap={2}>
                              <Badge
                                size="sm"
                                variant="subtle"
                                colorPalette="gray"
                                fontSize="2xs"
                                px={1.5}
                              >
                                {model.provider}
                              </Badge>
                              <Flex direction="column" gap={0}>
                                <Flex align="center" gap={1}>
                                  <Text fontSize="sm" fontWeight="medium">
                                    {model.name}
                                  </Text>
                                  {model.supports_image_input && (
                                    <Box
                                      as="span"
                                      role="img"
                                      aria-label="Supports image input"
                                      color="fg.muted"
                                    >
                                      <ImageIcon size={12} />
                                    </Box>
                                  )}
                                </Flex>
                                {meta && (
                                  <Text fontSize="2xs" color="fg.muted">
                                    {meta}
                                  </Text>
                                )}
                              </Flex>
                            </Flex>
                            {isSelected && (
                              <Box color="blue.500" flexShrink={0}>
                                <Check size={14} />
                              </Box>
                            )}
                          </Box>
                        )
                      })}
                    </Box>
                  ),
                )}
              </Flex>
            </Popover.Body>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}
