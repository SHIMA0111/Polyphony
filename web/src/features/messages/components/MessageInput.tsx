"use client"

import { useState, useRef, useCallback, useEffect } from "react"
import { Box, Button, Flex, Separator, Spacer, Text } from "@chakra-ui/react"
import { ArrowUp, Sparkles } from "lucide-react"
import { Tooltip } from "@/components/ui/tooltip"
import { estimateTokens } from "@/features/messages/api/estimate-tokens"
import type { Message, ModelInfo } from "@/features/messages/types"
import { ModelSelector } from "./ModelSelector"

/** Debounce delay, in ms, before firing a token estimate request after the
 * draft/model/visible-messages inputs settle — matches this file's existing
 * plain-`setTimeout` style rather than pulling in a debounce dependency. */
const TOKEN_ESTIMATE_DEBOUNCE_MS = 400

interface MessageInputProps {
  onSend: (content: string) => Promise<void>
  onSendWithAI: (content: string, model: string) => Promise<void>
  models: ModelInfo[]
  disabled?: boolean
  /**
   * Whether the viewer may invoke AI in this room (Step 37's `RoomRole`
   * gating: `guest` and below cannot). Defaults to `true` so every existing
   * caller that doesn't pass this prop is unaffected. When `false`, the
   * "Send with AI" button is omitted entirely rather than left visible and
   * disabled — the control must not be visible, not just inert, since a
   * guest attempting AI invocation is independently rejected server-side.
   */
  canInvokeAI?: boolean
  /**
   * The currently-visible message transcript, used to compute the live
   * token estimate below the composer. Optional (defaults to an empty
   * transcript) so existing callers/tests that don't care about the meter
   * don't need to pass anything.
   */
  messages?: Message[]
}

const EMPTY_MESSAGES: Message[] = []

export function MessageInput({
  onSend,
  onSendWithAI,
  models,
  disabled,
  canInvokeAI = true,
  messages = EMPTY_MESSAGES,
}: MessageInputProps) {
  const [input, setInput] = useState("")
  const [isSending, setIsSending] = useState(false)
  const [selectedModel, setSelectedModel] = useState<ModelInfo | null>(null)
  const [estimatedTokens, setEstimatedTokens] = useState<number | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  // Guards against an older, slower estimate response overwriting a newer
  // one that already resolved (no built-in request cancellation for a plain
  // `fetch`-backed call here).
  const estimateRequestIdRef = useRef(0)

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

  // Debounced live token estimate: recomputed whenever the draft, selected
  // model, or visible message list changes. Advisory only — a failed
  // estimate is logged and swallowed rather than blocking or disabling
  // send (see `estimateTokens`'s docstring).
  useEffect(() => {
    if (!selectedModel) return

    const timeoutId = setTimeout(() => {
      const requestId = ++estimateRequestIdRef.current

      // Defends against any not-yet-reconciled optimistic/WS cache entry
      // that might transiently carry `is_deleted: true` even though the
      // server already omits soft-deleted rows from `GET
      // /rooms/:roomId/messages` (see `message-cache.ts`'s any-page
      // helpers).
      const payload = messages
        .filter((m) => !m.is_deleted && !m.exclude_from_ai)
        .map((m) => ({
          role: (m.type === "human" ? "user" : "assistant") as
            | "user"
            | "assistant",
          content: m.content,
        }))

      const draft = input.trim()
      if (draft) {
        payload.push({ role: "user", content: draft })
      }

      estimateTokens(selectedModel.id, payload)
        .then((res) => {
          if (estimateRequestIdRef.current === requestId) {
            setEstimatedTokens(res.estimated_tokens)
          }
        })
        .catch((err: unknown) => {
          // Advisory feature only — never blocks or disables send.
          console.error("Failed to estimate tokens", err)
        })
    }, TOKEN_ESTIMATE_DEBOUNCE_MS)

    return () => clearTimeout(timeoutId)
  }, [input, selectedModel, messages])

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
            {canInvokeAI && (
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
            )}
          </Flex>
        </Box>

        {/* Hint text */}
        <Text textAlign="center" fontSize="xs" color="fg.muted">
          Enter to send, Shift+Enter for new line, Ctrl+Enter to send with AI
        </Text>

        {/* Live, debounced token estimate (Step 38) — advisory only, never
            blocks send. */}
        {estimatedTokens !== null && (
          <Text textAlign="center" fontSize="2xs" color="fg.muted">
            ~{estimatedTokens} tokens
          </Text>
        )}
      </Flex>
    </Box>
  )
}
