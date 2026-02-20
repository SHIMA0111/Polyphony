"use client"

import { useState } from "react"
import { Badge, Box, Button, Flex, Popover, Portal, Text } from "@chakra-ui/react"
import { Check, ChevronDown } from "lucide-react"

export interface Model {
  id: string
  name: string
  provider: string
}

interface ModelSelectorProps {
  models: Model[]
  selectedModel: Model
  onModelSelect: (model: Model) => void
}

export function ModelSelector({
  models,
  selectedModel,
  onModelSelect,
}: ModelSelectorProps) {
  const [open, setOpen] = useState(false)

  const handleSelect = (model: Model) => {
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
                {models.map((model) => {
                  const isSelected = model.id === selectedModel.id

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
                        <Text fontSize="sm" fontWeight="medium">
                          {model.name}
                        </Text>
                      </Flex>
                      {isSelected && (
                        <Box color="blue.500" flexShrink={0}>
                          <Check size={14} />
                        </Box>
                      )}
                    </Box>
                  )
                })}
              </Flex>
            </Popover.Body>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}
