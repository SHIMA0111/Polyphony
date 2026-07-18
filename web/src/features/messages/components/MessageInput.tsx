"use client"

import { useState, useRef, useCallback, useEffect } from "react"
import Link from "next/link"
import { Box, Button, Flex, IconButton, Separator, Spacer, Text } from "@chakra-ui/react"
import { ArrowUp, ImagePlus, Lock, LockOpen, Sparkles } from "lucide-react"
import { estimateTokens } from "@/features/messages/api/estimate-tokens"
import type { Message, ModelInfo } from "@/features/messages/types"
import { useAttachmentStaging } from "@/features/messages/hooks/use-attachment-staging"
import { Tooltip } from "@/components/ui/tooltip"
import { AttachmentChip } from "./AttachmentChip"
import { ModelSelector } from "./ModelSelector"

/** Debounce delay, in ms, before firing a token estimate request after the
 * draft/model/visible-messages inputs settle — matches this file's existing
 * plain-`setTimeout` style rather than pulling in a debounce dependency. */
const TOKEN_ESTIMATE_DEBOUNCE_MS = 400

/** Copy shown when "Send with AI" is disabled because images are staged but
 * the selected model can't see them. */
const VISION_UNSUPPORTED_MESSAGE =
  "The selected model can't see images — pick a vision-capable model"

/** `<input type="file" accept="...">`'s accept list, matching
 * `use-attachment-staging.ts`'s `ALLOWED_ATTACHMENT_MIME_TYPES`. */
const ACCEPTED_IMAGE_MIME_TYPES = "image/png,image/jpeg,image/webp,image/gif"

interface MessageInputProps {
  /** The room attachments are uploaded into (`POST
   * /rooms/:roomId/attachments/upload-url`); also used to scope the staging
   * hook's state to this room. */
  roomId: string
  onSend: (content: string, attachmentIds: string[]) => Promise<void>
  onSendWithAI: (
    content: string,
    model: string,
    attachmentIds: string[],
    isPrivate: boolean,
  ) => Promise<void>
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
  /**
   * Set by `ChatRoom`/`useChatRoom` when the most recent "Send with AI" was
   * rejected with HTTP 402 (insufficient token balance); rendered as an
   * inline error line beneath the button row with a link to `/billing/usage`.
   * `undefined`/`null` (the default) renders nothing, so every other caller
   * of this component is unaffected.
   */
  aiError?: string | null
}

const EMPTY_MESSAGES: Message[] = []

export function MessageInput({
  roomId,
  onSend,
  onSendWithAI,
  models,
  disabled,
  canInvokeAI = true,
  messages = EMPTY_MESSAGES,
  aiError,
}: MessageInputProps) {
  const [input, setInput] = useState("")
  const [isSending, setIsSending] = useState(false)
  // Only ever set by the user explicitly picking a model in `ModelSelector`;
  // the *effective* model (derived below) falls back to the first available
  // model without needing a mount-time effect to seed this state.
  const [explicitModel, setExplicitModel] = useState<ModelInfo | null>(null)
  const [estimatedTokens, setEstimatedTokens] = useState<number | null>(null)
  const [isDropActive, setIsDropActive] = useState(false)
  // Locally dismisses the `aiError` prop once the user starts typing again
  // or attempts another send, so a resolved error doesn't linger on screen
  // even though `useChatRoom` only clears its own `aiError` state at the
  // *start* of the next `handleSendWithAI` call.
  const [aiErrorDismissed, setAiErrorDismissed] = useState(false)
  // Tracks the most recent `aiError` value this component has reacted to, so
  // a *new* rejection can un-dismiss the error line during render (see
  // `displayedAiError`'s derivation below) without a
  // `useEffect`-that-calls-`setState` (which trips
  // `react-hooks/set-state-in-effect`).
  const [lastSeenAiError, setLastSeenAiError] = useState(aiError)
  // Step 47: private AI mode toggle. Opt-in per message (not sticky) --
  // reset to `false` after every "Send with AI" call, mirroring the
  // existing `setInput("")` reset in `handleSendWithAI`. Has no effect on
  // the plain "Send" path.
  const [isPrivate, setIsPrivate] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  // Guards against an older, slower estimate response overwriting a newer
  // one that already resolved (no built-in request cancellation for a plain
  // `fetch`-backed call here).
  const estimateRequestIdRef = useRef(0)

  const {
    attachments,
    addFiles,
    remove: removeAttachment,
    reset: resetAttachments,
    retry: retryAttachment,
  } = useAttachmentStaging(roomId)

  // Derived, not stored: falls back to the first available model the
  // instant `models` loads, without a "set default model when models are
  // loaded" mount effect (which would otherwise trip
  // react-hooks/set-state-in-effect).
  const effectiveModel = explicitModel ?? models[0] ?? null

  // A new (truthy) `aiError` always un-dismisses — it represents a fresh
  // rejection, not the one just dismissed. Adjusting state during render
  // (rather than in a `useEffect`) per React's "you can update state right
  // while rendering" pattern: guarded by comparing against `lastSeenAiError`
  // so this only fires once per actual `aiError` change, not on every
  // render, and never triggers `react-hooks/set-state-in-effect`.
  if (aiError !== lastSeenAiError) {
    setLastSeenAiError(aiError)
    if (aiError) {
      setAiErrorDismissed(false)
    }
  }

  const displayedAiError = aiErrorDismissed ? null : (aiError ?? null)

  const handleInputChange = useCallback((value: string) => {
    setInput(value)
    setAiErrorDismissed(true)
  }, [])

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
    if (!effectiveModel) return

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

      estimateTokens(effectiveModel.id, payload)
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
  }, [input, effectiveModel, messages])

  const doneAttachmentIds = attachments
    .filter((a) => a.status === "done" && a.attachmentId)
    .map((a) => a.attachmentId as string)
  const hasUploadingAttachment = attachments.some((a) => a.status === "uploading")
  // "Images staged" for vision-gating purposes means real (non-rejected)
  // staged files, not entries that failed client-side validation and were
  // never even sent to the server.
  const hasStagedImages = attachments.some((a) => a.status !== "error")
  const modelSupportsImages = effectiveModel?.supports_image_input ?? false
  const visionGated = hasStagedImages && !modelSupportsImages

  const handleSend = useCallback(async () => {
    const content = input.trim()
    if (!content || isSending || hasUploadingAttachment) return
    setAiErrorDismissed(true)
    setIsSending(true)
    try {
      await onSend(content, doneAttachmentIds)
      setInput("")
      resetAttachments()
    } catch {
      // The mutation's own `onError` already appended a retryable "failed"
      // bubble to the transcript and surfaced a failure toast — restore the
      // typed content here so it isn't lost, rather than letting the
      // rejection go uncaught. Staged attachments are left in place too, so
      // retrying the send doesn't require re-uploading them.
      setInput(content)
    } finally {
      setIsSending(false)
    }
  }, [input, isSending, hasUploadingAttachment, doneAttachmentIds, onSend, resetAttachments])

  const handleSendWithAI = useCallback(async () => {
    const content = input.trim()
    if (
      !content ||
      isSending ||
      !effectiveModel ||
      hasUploadingAttachment ||
      visionGated
    ) {
      return
    }
    setAiErrorDismissed(true)
    setIsSending(true)
    try {
      await onSendWithAI(content, effectiveModel.id, doneAttachmentIds, isPrivate)
      setInput("")
      resetAttachments()
      // Private mode is opt-in per message, not sticky: reset to off on a
      // successful send, mirroring `setInput("")`/`resetAttachments()`
      // above. Left as-is on failure (like the typed content) so a retried
      // send doesn't silently lose the user's private-mode choice.
      setIsPrivate(false)
    } catch {
      // See `handleSend`'s catch above: restore the content instead of
      // losing it, the toast/failed-bubble is already handled by the
      // mutation itself.
      setInput(content)
    } finally {
      setIsSending(false)
    }
  }, [
    input,
    isSending,
    effectiveModel,
    hasUploadingAttachment,
    visionGated,
    doneAttachmentIds,
    isPrivate,
    onSendWithAI,
    resetAttachments,
  ])

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

  const handlePaste = (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const items = e.clipboardData?.items
    if (!items) return

    const imageFiles: File[] = []
    for (const item of items) {
      if (item.kind === "file" && item.type.startsWith("image/")) {
        const file = item.getAsFile()
        if (file) imageFiles.push(file)
      }
    }
    // Plain text paste is untouched: no `preventDefault()` here, and no
    // image files means nothing further to do.
    if (imageFiles.length > 0) {
      addFiles(imageFiles)
    }
  }

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setIsDropActive(false)
    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) addFiles(files)
  }

  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) {
      addFiles(Array.from(e.target.files))
    }
    // Reset so selecting the exact same file again still fires onChange.
    e.target.value = ""
  }

  const isDisabled = !input.trim() || isSending || disabled || hasUploadingAttachment
  // Unlike the plain-send button, the AI-send button also needs a model to
  // invoke — `isDisabled` alone doesn't check for one, since plain send has
  // no such requirement. Without this, a room with no models configured (or
  // between mount and `models` loading) would render the AI-send button
  // clickable and let `handleSendAI` below silently no-op on its own
  // `!effectiveModel` early return.
  const isSendWithAIDisabled = isDisabled || visionGated || !effectiveModel

  return (
    <Box bg="bg/80" backdropFilter="blur(8px)" pb={4} pt={2}>
      <Flex direction="column" maxW="3xl" mx="auto" px={4} gap={2}>
        {/* Floating card input */}
        <Box
          bg="bg"
          borderWidth="1px"
          borderColor={isDropActive ? "blue.400" : "border"}
          rounded="2xl"
          shadow="lg"
          overflow="hidden"
          onDragOver={(e) => {
            e.preventDefault()
            setIsDropActive(true)
          }}
          onDragLeave={() => setIsDropActive(false)}
          onDrop={handleDrop}
        >
          {attachments.length > 0 && (
            <Flex gap={2} px={4} pt={4} overflowX="auto">
              {attachments.map((attachment) => (
                <AttachmentChip
                  key={attachment.id}
                  attachment={attachment}
                  onRemove={removeAttachment}
                  onRetry={retryAttachment}
                />
              ))}
            </Flex>
          )}

          {/* Textarea area */}
          <Box px={4} pt={4} pb={3}>
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => handleInputChange(e.target.value)}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
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
            <input
              ref={fileInputRef}
              type="file"
              accept={ACCEPTED_IMAGE_MIME_TYPES}
              multiple
              hidden
              onChange={handleFileInputChange}
            />
            <Tooltip content="Attach image">
              <IconButton
                aria-label="Attach image"
                variant="ghost"
                size="sm"
                h={8}
                minW={8}
                rounded="lg"
                disabled={disabled || isSending}
                onClick={() => fileInputRef.current?.click()}
              >
                <ImagePlus size={16} />
              </IconButton>
            </Tooltip>

            {effectiveModel && models.length > 0 && (
              <ModelSelector
                models={models}
                selectedModel={effectiveModel}
                onModelSelect={setExplicitModel}
              />
            )}
            {canInvokeAI && (
              <Tooltip content="Only you will see this exchange with the AI">
                <IconButton
                  aria-label={
                    isPrivate ? "Private mode on" : "Private mode off"
                  }
                  aria-pressed={isPrivate}
                  variant={isPrivate ? "solid" : "ghost"}
                  colorPalette={isPrivate ? "purple" : "gray"}
                  size="sm"
                  h={8}
                  minW={8}
                  rounded="lg"
                  disabled={disabled || isSending}
                  onClick={() => setIsPrivate((prev) => !prev)}
                >
                  {isPrivate ? <Lock size={16} /> : <LockOpen size={16} />}
                </IconButton>
              </Tooltip>
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
              <Tooltip content={VISION_UNSUPPORTED_MESSAGE} disabled={!visionGated}>
                <Box as="span" display="inline-flex" tabIndex={0}>
                  <Button
                    size="sm"
                    onClick={handleSendWithAI}
                    disabled={isSendWithAIDisabled}
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

        {displayedAiError && (
          <Text role="alert" textAlign="center" fontSize="xs" color="fg.error">
            {displayedAiError} Visit{" "}
            <Link href="/billing/usage" style={{ textDecoration: "underline" }}>
              Usage
            </Link>{" "}
            to check your balance.
          </Text>
        )}

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
