"use client"

import { useState, useRef, useCallback, useEffect } from "react"
import { Box, Button, Flex, Separator, Spacer, Text } from "@chakra-ui/react"
import { ArrowUp, Sparkles } from "lucide-react"
import { Tooltip } from "@/components/ui/tooltip"
import { ModelSelector, type Model } from "./ModelSelector"

interface MessageInputProps {
  onSend: (content: string) => Promise<void>
  onSendWithAI: (content: string, model: string) => Promise<void>
  models: Model[]
  disabled?: boolean
}

export function MessageInput({
  onSend,
  onSendWithAI,
  models,
  disabled,
}: MessageInputProps) {
  const [input, setInput] = useState("")
  const [isSending, setIsSending] = useState(false)
  const [selectedModel, setSelectedModel] = useState<Model | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Set default model when models are loaded
  useEffect(() => {
    if (models.length > 0 && !selectedModel) {
      setSelectedModel(models[0])
    }
  }, [models, selectedModel])

  // Auto-resize textarea
  useEffect(() => {
    const textarea = textareaRef.current
    if (textarea) {
      textarea.style.height = "auto"
      const newHeight = Math.min(textarea.scrollHeight, 120)
      textarea.style.height = `${newHeight}px`
    }
  }, [input])

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content || isSending) return
    setIsSending(true)
    try {
      await onSend(content)
      setInput("")
    } catch {
      // The mutation's own `onError` already appended a retryable "failed"
      // bubble to the transcript and surfaced a failure toast — restore the
      // typed content here so it isn't lost, rather than letting the
      // rejection go uncaught.
      setInput(content)
    } finally {
      setIsSending(false)
    }
  }, [input, isSending, onSend])

  const handleSendWithAI = useCallback(async () => {
    const content = input.trim()
    if (!content || isSending || !selectedModel) return
    setIsSending(true)
    try {
      await onSendWithAI(content, selectedModel.id)
      setInput("")
    } catch {
      // See `handleSend`'s catch above: restore the content instead of
      // losing it, the toast/failed-bubble is already handled by the
      // mutation itself.
      setInput(content)
    } finally {
      setIsSending(false)
    }
  }, [input, isSending, selectedModel, onSendWithAI])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.nativeEvent.isComposing) {
      if (e.metaKey || e.ctrlKey) {
        e.preventDefault()
        handleSendWithAI()
      } else if (!e.shiftKey) {
        e.preventDefault()
        handleSend()
      }
    }
  }

  const isDisabled = !input.trim() || isSending || disabled
  // The AI send button has its own, stricter disabled condition: sending
  // with AI additionally requires a selected model (handleSendWithAI itself
  // no-ops without one), so the button must reflect that -- otherwise it
  // renders as clickable while a click would silently do nothing whenever
  // `models` hasn't loaded a selection yet.
  const isAISendDisabled = isDisabled || !selectedModel

  return (
    <Box bg="bg/80" backdropFilter="blur(8px)" pb={4} pt={2}>
      <Flex direction="column" maxW="3xl" mx="auto" px={4} gap={2}>
        {/* Floating card input */}
        <Box
          bg="bg"
          borderWidth="1px"
          borderColor="border"
          rounded="2xl"
          shadow="lg"
          overflow="hidden"
        >
          {/* Textarea area */}
          <Box px={4} pt={4} pb={3}>
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask me anything..."
              disabled={disabled || isSending}
              rows={1}
              style={{
                width: "100%",
                background: "transparent",
                resize: "none",
                outline: "none",
                border: "none",
                color: "var(--chakra-colors-fg)",
                fontSize: "15px",
                lineHeight: "1.625",
                fontFamily: "inherit",
              }}
            />
          </Box>

          {/* Separator */}
          <Separator borderColor="border.muted" />

          {/* Button row */}
          <Flex px={3} py={2} align="center" gap={2}>
            {selectedModel && models.length > 0 && (
              <ModelSelector
                models={models}
                selectedModel={selectedModel}
                onModelSelect={setSelectedModel}
              />
            )}
            <Spacer />
            <Button
              size="sm"
              variant="outline"
              onClick={handleSend}
              disabled={isDisabled}
              loading={isSending && !disabled}
              h={8}
              px={3}
              fontSize="xs"
              fontWeight="medium"
              gap={1.5}
              rounded="lg"
            >
              <ArrowUp size={14} />
              Send
            </Button>
            <Tooltip
              content="Select a model to send with AI"
              disabled={!!selectedModel}
            >
              {/*
                A `disabled` native button doesn't fire pointer or focus
                events in most browsers, so a Tooltip wrapping it directly
                would never trigger — not on mouse hover, and not on
                keyboard focus (Tab). Wrapping the Button in a focusable
                (`tabIndex={0}`) `span` gives the tooltip an always-
                interactive element to anchor to, so "Select a model to send
                with AI" is reachable both by hovering and by tabbing to it,
                even while the button itself is disabled.
              */}
              <Box as="span" display="inline-flex" tabIndex={0}>
                <Button
                  size="sm"
                  onClick={handleSendWithAI}
                  disabled={isAISendDisabled}
                  h={8}
                  px={3}
                  fontSize="xs"
                  fontWeight="medium"
                  gap={1.5}
                  rounded="lg"
                  colorPalette="blue"
                  bg="linear-gradient(to right, var(--chakra-colors-blue-500), var(--chakra-colors-blue-600))"
                  color="white"
                  _hover={{ opacity: 0.9 }}
                >
                  <Sparkles size={14} />
                  Send with AI
                </Button>
              </Box>
            </Tooltip>
          </Flex>
        </Box>

        {/* Hint text */}
        <Text textAlign="center" fontSize="xs" color="fg.muted">
          Enter to send, Shift+Enter for new line, Ctrl+Enter to send with AI
        </Text>
      </Flex>
    </Box>
  )
}
