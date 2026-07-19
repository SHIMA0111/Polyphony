"use client"

import { useCallback, useMemo, useState } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { toaster } from "@/components/ui/toaster"
import { ApiRequestError } from "@/lib/http-client"
import { useRoom } from "@/features/rooms/hooks/use-room"
import { useMessages } from "@/features/messages/hooks/use-messages"
import { useModels } from "@/features/messages/hooks/use-models"
import { useSendMessage } from "@/features/messages/hooks/use-send-message"
import {
  takeFailedAISendIntent,
  useSendAIMessage,
} from "@/features/messages/hooks/use-send-ai-message"
import { useRegenerateAIMessage } from "@/features/messages/hooks/use-regenerate-ai-message"
import { attachToMessage } from "@/features/messages/api/attach-to-message"
import { listAttachments } from "@/features/messages/api/list-attachments"
import { messageAttachmentsQueryKey } from "@/features/messages/api/use-message-attachments"
import { flattenMessagePages } from "@/features/messages/lib/flatten-message-pages"
import { removeFromNewestPage, type MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import type { Message, ModelInfo } from "@/features/messages/types"
import type { Room } from "@/features/rooms/types"

/** Return value of {@link linkAttachments} -- see its docstring. */
interface LinkAttachmentsResult {
  /** Number of `attachmentIds` that failed to link (out of the total attempted). */
  failedCount: number
}

/**
 * Links each of `attachmentIds` (staged, uploaded, and resolved to a real
 * `attachment_id` by `use-attachment-staging.ts`) to `messageId` via
 * `attachToMessage` (Step 12's endpoint), then seeds
 * `useMessageAttachments`'s query cache with the resulting
 * `AttachmentWithUrl[]` (fetched once via `listAttachments`) so
 * `MessageAttachments` renders the just-sent message's thumbnails the
 * instant it mounts, rather than waiting on that hook's own independent
 * fetch to kick off and resolve.
 *
 * A failure linking any individual attachment is logged (never thrown) and
 * counted in the returned {@link LinkAttachmentsResult.failedCount}: by the
 * time this runs, the message's text content has already been sent
 * successfully, so this function itself must not reject and fail the whole
 * send -- callers are responsible for surfacing `failedCount > 0` to the
 * user (e.g. a toaster error) and, for an AI send, deciding whether a
 * partial failure still warrants a Vision-aware regenerate. A failure
 * refreshing the attachment list afterward is logged and swallowed the same
 * way, but does not count toward `failedCount` -- the attachments *did*
 * link successfully; only this function's own cache-priming fetch failed,
 * and `useMessageAttachments`'s independent fetch still picks them up.
 */
async function linkAttachments(
  queryClient: QueryClient,
  roomId: string,
  messageId: string,
  attachmentIds: string[],
): Promise<LinkAttachmentsResult> {
  if (attachmentIds.length === 0) return { failedCount: 0 }

  const results = await Promise.all(
    attachmentIds.map((attachmentId) =>
      attachToMessage(roomId, messageId, attachmentId)
        .then(() => true)
        .catch((err: unknown) => {
          console.error("Failed to link attachment to message", err)
          return false
        }),
    ),
  )
  const failedCount = results.filter((ok) => !ok).length

  try {
    const res = await listAttachments(roomId, messageId)
    queryClient.setQueryData(
      messageAttachmentsQueryKey(roomId, messageId),
      res.attachments,
    )
  } catch (err) {
    console.error("Failed to refresh attachments after linking", err)
  }

  return { failedCount }
}

/**
 * Shows a toaster error for a `linkAttachments` result with `failedCount >
 * 0`. Shared by `handleSend` and `handleSendWithAI` so both surface the
 * same copy rather than drifting independently.
 */
function notifyAttachmentLinkFailures(failedCount: number): void {
  if (failedCount === 0) return
  toaster.create({
    type: "error",
    title: "Attachment error",
    description: `${failedCount} attachment(s) could not be attached.`,
  })
}

/**
 * Copy shown by `MessageInput`'s inline error line when an AI send is
 * rejected with HTTP 402 (`domain.ErrInsufficientBalance`, see
 * `server/internal/usecase/billing/usecase.go`'s `CheckBalance`).
 */
export const INSUFFICIENT_BALANCE_MESSAGE = "Insufficient token balance."

/** Stable identity fallbacks so `useCallback`/`useMemo` deps below don't
 * change on every render while a query has no data yet. */
const EMPTY_MESSAGES: Message[] = []
const EMPTY_MODELS: ModelInfo[] = []

export interface UseChatRoomResult {
  room: Room | undefined
  messages: Message[]
  models: ModelInfo[]
  isLoading: boolean
  /** The AI message id currently being regenerated, or `null`. */
  isRegenerating: string | null
  /** Whether an older page of history is available via `fetchNextPage`. */
  hasNextPage: boolean
  /** Whether the next (older) page is currently being fetched. */
  isFetchingNextPage: boolean
  /** Fetches the next older page of message history. */
  fetchNextPage: () => Promise<unknown>
  /** Number of currently loaded pages; used to anchor scroll position across a load. */
  pageCount: number
  /** Sends a plain message, optionally linking already-uploaded
   * `attachmentIds` (from `MessageInput`'s staging) to it once the message
   * is persisted. */
  handleSend: (content: string, attachmentIds?: string[]) => Promise<void>
  /** Sends a message with an AI response, optionally linking
   * `attachmentIds` and then regenerating the AI reply so it sees them (see
   * `linkAttachments`'s docstring and this function's own body for why a
   * single call can't do both). `isPrivate` (Step 47) sets both the human
   * message and the AI reply's `visibility` to `"private"` -- see
   * `SendAIMessageRequest.Private`. */
  handleSendWithAI: (
    content: string,
    model: string,
    attachmentIds?: string[],
    isPrivate?: boolean,
  ) => Promise<void>
  handleRegenerate: (aiMessageId: string) => Promise<void>
  /** Re-sends a failed human message's original content, replacing its
   * failed optimistic entry so no duplicate bubble is left behind. */
  handleRetry: (messageId: string, content: string) => Promise<void>
  /**
   * Set to {@link INSUFFICIENT_BALANCE_MESSAGE} when the most recent
   * `handleSendWithAI` call was rejected with HTTP 402, `null` otherwise
   * (including after any other kind of send failure, which the mutation's
   * own toast already surfaces). Cleared at the start of every subsequent
   * `handleSendWithAI` call so a resolved-then-retried send doesn't leave a
   * stale error on screen.
   */
  aiError: string | null
}

/**
 * Data-orchestration hook for the chat room page: composes step 9's
 * per-feature TanStack Query hooks (room, messages, models) with the
 * send/send-with-AI/regenerate mutations and derives the loading and
 * regenerating state `ChatRoom` needs to render.
 *
 * Kept separate from `ChatRoom.tsx` so the component itself stays a thin
 * composition of `ChatRoomHeader` + `MessageList` + `MessageInput`.
 *
 * @param roomId - The room to load and interact with.
 */
export function useChatRoom(roomId: string): UseChatRoomResult {
  const queryClient = useQueryClient()
  const [aiError, setAiError] = useState<string | null>(null)

  const roomQuery = useRoom(roomId)
  const messagesQuery = useMessages(roomId)
  const modelsQuery = useModels()

  const sendMessageMutation = useSendMessage(roomId)
  const sendAIMessageMutation = useSendAIMessage(roomId)
  const regenerateMutation = useRegenerateAIMessage(roomId)

  const room = roomQuery.data
  const pages = messagesQuery.data?.pages
  const messages = useMemo(
    () => (pages ? flattenMessagePages(pages) : EMPTY_MESSAGES),
    [pages],
  )
  const models = modelsQuery.data ?? EMPTY_MODELS
  const isLoading =
    roomQuery.isPending || messagesQuery.isPending || modelsQuery.isPending

  const handleSend = useCallback(
    async (content: string, attachmentIds: string[] = []) => {
      const message = await sendMessageMutation.mutateAsync(content)
      const { failedCount } = await linkAttachments(queryClient, roomId, message.id, attachmentIds)
      notifyAttachmentLinkFailures(failedCount)
    },
    [sendMessageMutation, queryClient, roomId],
  )

  /**
   * Core of an AI send that may carry staged attachments: invokes
   * `useSendAIMessage`, and -- when `attachmentIds` is non-empty -- links
   * each attachment to the resulting human message and then regenerates the
   * AI reply so it actually sees them (see the inline comments below for why
   * a single call can't do both). Shared by `handleSendWithAI` (the initial
   * send) and `handleRetry` (replaying a failed attachment send) so the
   * link + regenerate sequence and its failure semantics live in exactly one
   * place.
   */
  const sendAIMessageWithAttachments = useCallback(
    async (
      content: string,
      model: string | undefined,
      attachmentIds: string[],
      isPrivate: boolean,
    ) => {
      const res = await sendAIMessageMutation.mutateAsync({
        content,
        model,
        private: isPrivate,
        attachmentIds,
        // Attachment sends opt out of streaming: the regenerate call
        // below must target a settled AI message, not one whose stream is
        // still in flight -- see `SendAIMessageInput.stream`'s doc
        // comment for the finalize-vs-regenerate clobbering race this
        // avoids.
        stream: attachmentIds.length === 0,
      })
      // Refresh the top-bar balance promptly after a successful AI send,
      // rather than waiting for `useBalance`'s background poll — a send
      // debits the room owner's balance server-side (see
      // `BillingUsecase.RecordUsage`).
      await queryClient.invalidateQueries({ queryKey: ["billing", "balance"] })

      if (attachmentIds.length > 0) {
        // `res.ai_message` was generated *before* any attachment could be
        // linked to `res.user_message` — attachments cannot be linked to a
        // message that doesn't exist yet. Linking them now and then
        // regenerating is the only sequence that satisfies both
        // `RegenerateAIMessage`'s precondition (an AI-typed message must
        // already exist immediately after the human message it targets,
        // see `server/internal/usecase/message/usecase.go`) and actually
        // gets the attachments in front of the model: no existing endpoint
        // both creates a message and includes attachments linked to that
        // same message in the same outbound completion request.
        const { failedCount } = await linkAttachments(
          queryClient,
          roomId,
          res.user_message.id,
          attachmentIds,
        )
        notifyAttachmentLinkFailures(failedCount)

        // Regenerating against a model that still can't see any of the
        // attachments the user just staged would only reproduce the exact
        // same (already-persisted) text-only reply for a second time --
        // pure wasted cost with no chance of a different, Vision-aware
        // outcome, so skip it when every link failed. A *partial* failure
        // still regenerates: the model sees whichever attachments did
        // link.
        if (failedCount < attachmentIds.length) {
          try {
            await regenerateMutation.mutateAsync({
              aiMessageId: res.ai_message.id,
              humanMessageId: res.user_message.id,
              model,
            })
          } catch {
            // Mirrors `handleRegenerate`'s own swallow below: the mutation's
            // rejection already reflects as a persisted `status: "failed"`
            // AI message via `MessageBubble`'s own styling, so there is
            // nothing further to do here.
          }
        }
      }
    },
    [sendAIMessageMutation, queryClient, roomId, regenerateMutation],
  )

  const handleSendWithAI = useCallback(
    async (
      content: string,
      model: string,
      attachmentIds: string[] = [],
      isPrivate = false,
    ) => {
      setAiError(null)
      try {
        await sendAIMessageWithAttachments(content, model, attachmentIds, isPrivate)
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 402) {
          // Distinguish "the AI declined to answer" from "the request was
          // never even allowed to run": surface a specific, actionable error
          // to `MessageInput` instead of the generic failed-send toast
          // `useSendAIMessage`'s own `onError` already shows.
          setAiError(INSUFFICIENT_BALANCE_MESSAGE)
        }
        // Re-throw regardless of status so `MessageInput`'s own catch still
        // restores the typed content and `useSendAIMessage`'s `onError`
        // still rolls back the optimistic entries — this only adds the
        // 402-specific `aiError` state on top of that existing handling.
        throw error
      }
    },
    [sendAIMessageWithAttachments],
  )

  const handleRegenerate = useCallback(
    async (aiMessageId: string) => {
      // Resolve the target human message directly from the AI message's
      // `in_response_to_message_id` link (a Step 7 server addition) instead
      // of scanning `messages` backwards by array position — the previous
      // approach could resolve the wrong human message whenever the AI
      // message wasn't immediately preceded by its own human message (e.g.
      // after an interleaved system/failed entry).
      const aiMessage = messages.find((m) => m.id === aiMessageId)
      if (!aiMessage?.in_response_to_message_id) {
        // Should not happen for an AI message created via `SendAIMessage`
        // (see `server/internal/usecase/message/usecase.go`); no-op rather
        // than guess at a fallback target.
        return
      }

      try {
        await regenerateMutation.mutateAsync({
          aiMessageId,
          humanMessageId: aiMessage.in_response_to_message_id,
        })
      } catch {
        // The mutation's rejection is enough for callers that want to
        // observe it (e.g. via `regenerateMutation.isError`); `MessageBubble`
        // already reflects a persisted `status: "failed"` AI message via its
        // own styling, so there is nothing further to do here.
      }
    },
    [messages, regenerateMutation],
  )

  const handleRetry = useCallback(
    async (messageId: string, content: string) => {
      // A failed message that originated from an AI send has an entry here
      // (set in `useSendAIMessage`'s own `onError`, see
      // `failedAISendIntentQueryKey`'s docstring); anything else (a plain
      // send's failure) has none, and falls back to a plain resend below --
      // its original intent already *was* plain, so there is nothing to
      // recover. Restoring `stream`/`private` here (not just `model`) is
      // what keeps a retried private send private and a retried
      // non-streaming send non-streaming, instead of silently reverting to
      // this mutation's public/streaming defaults.
      const intent = takeFailedAISendIntent(queryClient, roomId, messageId)

      // Drop the stale failed optimistic entry first so the mutation's own
      // `onMutate` (which appends a *new* optimistic entry with a fresh id)
      // doesn't leave both the old failed bubble and the new "sending"
      // bubble on screen at once.
      queryClient.setQueryData<MessagesInfiniteData>(
        ["rooms", roomId, "messages"],
        (old) => removeFromNewestPage(old, messageId),
      )

      try {
        if (intent?.attachmentIds && intent.attachmentIds.length > 0) {
          // The original send had staged attachments -- replaying the bare
          // AI-send mutation below would silently drop them (the attachments
          // were never part of that mutation's own request body; they are
          // linked in afterward, see `sendAIMessageWithAttachments`). Replay
          // the same link + regenerate workflow `handleSendWithAI` used
          // instead, so the retry re-links the *original* attachments.
          await sendAIMessageWithAttachments(
            content,
            intent.model,
            intent.attachmentIds,
            intent.private,
          )
        } else if (intent) {
          await sendAIMessageMutation.mutateAsync({
            content,
            model: intent.model,
            stream: intent.stream,
            private: intent.private,
          })
        } else {
          await sendMessageMutation.mutateAsync(content)
        }
      } catch {
        // The mutation's own `onError` already reflects the failure (a new
        // `status: "failed"` entry, plus a toast) and — for the AI path —
        // re-populates the retry-intent map for the newly-failed message id;
        // there is nothing further to do here, mirroring `handleRegenerate`'s
        // identical catch-and-ignore above.
      }
    },
    [queryClient, roomId, sendMessageMutation, sendAIMessageMutation, sendAIMessageWithAttachments],
  )

  const isRegenerating = regenerateMutation.isPending
    ? (regenerateMutation.variables?.aiMessageId ?? null)
    : null

  return {
    room,
    messages,
    models,
    isLoading,
    isRegenerating,
    hasNextPage: messagesQuery.hasNextPage,
    isFetchingNextPage: messagesQuery.isFetchingNextPage,
    fetchNextPage: messagesQuery.fetchNextPage,
    pageCount: pages?.length ?? 0,
    handleSend,
    handleSendWithAI,
    handleRegenerate,
    handleRetry,
    aiError,
  }
}
