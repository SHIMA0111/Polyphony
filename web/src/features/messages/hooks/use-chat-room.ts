"use client"

import { useCallback, useMemo, useState } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { ApiRequestError } from "@/lib/http-client"
import { useCurrentUser } from "@/features/auth/hooks/use-current-user"
import { useMembers } from "@/features/members/hooks/use-members"
import { useRoom } from "@/features/rooms/hooks/use-room"
import { useMessages } from "@/features/messages/hooks/use-messages"
import { useModels } from "@/features/messages/hooks/use-models"
import { useSendMessage } from "@/features/messages/hooks/use-send-message"
import {
  useSendAIMessage,
  takeFailedAISendIntent,
} from "@/features/messages/hooks/use-send-ai-message"
import { useRegenerateAIMessage } from "@/features/messages/hooks/use-regenerate-ai-message"
import { attachToMessage } from "@/features/messages/api/attach-to-message"
import { listAttachments } from "@/features/messages/api/list-attachments"
import { messageAttachmentsQueryKey } from "@/features/messages/hooks/use-message-attachments"
import { flattenMessagePages } from "@/features/messages/lib/flatten-message-pages"
import { removeFromNewestPage, type MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import type { Message, ModelInfo } from "@/features/messages/types"
import type { Room } from "@/features/rooms/types"

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
 * A failure linking any individual attachment (or refreshing the list
 * afterward) is logged and swallowed rather than failing the whole send:
 * by the time this runs, the message's text content has already been sent
 * successfully, so surfacing a hard error here would misleadingly suggest
 * the entire send failed.
 */
async function linkAttachments(
  queryClient: QueryClient,
  roomId: string,
  messageId: string,
  attachmentIds: string[],
): Promise<void> {
  if (attachmentIds.length === 0) return

  await Promise.all(
    attachmentIds.map((attachmentId) =>
      attachToMessage(roomId, messageId, attachmentId).catch((err: unknown) => {
        console.error("Failed to link attachment to message", err)
      }),
    ),
  )

  try {
    const res = await listAttachments(roomId, messageId)
    queryClient.setQueryData(
      messageAttachmentsQueryKey(roomId, messageId),
      res.attachments,
    )
  } catch (err) {
    console.error("Failed to refresh attachments after linking", err)
  }
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
const EMPTY_SENDER_USERNAMES: Record<string, string> = {}

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
   * The signed-in viewer's own local user id, sourced from
   * `useCurrentUser()` (`GET /users/me`), or `null` before that query
   * resolves (or while signed out). Passed down through
   * `MessageList`/`MessageGroup` so a human message group can be labeled
   * "You" for the viewer's own messages instead of always showing "You" for
   * every human sender — see `MessageGroup`'s `resolveSenderLabel`.
   *
   * Deliberately *not* `useSession().data?.identity.id`: under
   * `AUTH_MODE=kratos`, that's Kratos's own identity UUID, which is never
   * equal to the local `users.id` that `Message.sender_id` is keyed by (see
   * `CurrentUser`'s doc comment in `@/features/auth/types.ts`) — comparing
   * against it would leave every one of the viewer's own messages mislabeled
   * with their username instead of "You".
   */
  currentUserId: string | null
  /**
   * Map from member `user_id` to `username`, derived from
   * `useMembers(roomId)`. Passed down alongside `currentUserId` so
   * `MessageGroup` can label a human group that isn't the viewer's own by
   * the sender's actual username rather than mislabeling it "You" too. A
   * sender with no entry here (e.g. they have since left the room) falls
   * back to a short form of their id in `MessageGroup`.
   */
  senderUsernames: Record<string, string>
  /**
   * Set to {@link INSUFFICIENT_BALANCE_MESSAGE} when the most recent
   * `handleSendWithAI` *or* `handleRegenerate` call was rejected with HTTP
   * 402, `null` otherwise (including after any other kind of failure, which
   * the relevant mutation's own toast already surfaces -- see
   * `useSendAIMessage`/`useRegenerateAIMessage`'s docstrings). Cleared at the
   * start of every subsequent `handleSendWithAI`/`handleRegenerate` call so a
   * resolved-then-retried attempt doesn't leave a stale error on screen.
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
  const currentUserQuery = useCurrentUser()
  const membersQuery = useMembers(roomId)

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

  // Not included in `isLoading` above: the current-user/member-list queries
  // are supplementary display data for `MessageGroup`'s sender label (see
  // `currentUserId`/`senderUsernames`'s own doc comments), not data the
  // chat transcript itself needs to render -- gating the whole page's
  // loading state on them would delay the transcript for no benefit, since
  // `MessageBubble` already tolerates its own `useCurrentUser()` call
  // resolving a moment after first paint the same way.
  const currentUserId = currentUserQuery.data?.id ?? null
  const senderUsernames = useMemo(() => {
    const members = membersQuery.data?.members
    if (!members || members.length === 0) return EMPTY_SENDER_USERNAMES
    const map: Record<string, string> = {}
    for (const member of members) {
      map[member.user_id] = member.username
    }
    return map
  }, [membersQuery.data])

  const handleSend = useCallback(
    async (content: string, attachmentIds: string[] = []) => {
      const message = await sendMessageMutation.mutateAsync(content)
      await linkAttachments(queryClient, roomId, message.id, attachmentIds)
    },
    [sendMessageMutation, queryClient, roomId],
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
        const res = await sendAIMessageMutation.mutateAsync({
          content,
          model,
          private: isPrivate,
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
        // `BillingUsecase.RecordUsage`). For the non-streaming/attachment
        // branch (`stream: false`) this is already post-debit, since
        // `RecordUsage` runs synchronously before that response resolves.
        // For the default streaming branch it is *not* yet post-debit --
        // billing only happens once the stream finishes in the background,
        // well after this 202 response -- so `use-room-socket.ts`'s
        // `onmessage` handler additionally invalidates the same query on
        // the terminating `message_updated` finalize event (L1 post-review
        // finding), which is what actually reflects the post-debit number
        // for a streaming send.
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
          await linkAttachments(queryClient, roomId, res.user_message.id, attachmentIds)
          try {
            await regenerateMutation.mutateAsync({
              aiMessageId: res.ai_message.id,
              humanMessageId: res.user_message.id,
              model,
            })
          } catch (error) {
            // Mirrors `handleRegenerate`'s own catch below: a 402 is
            // surfaced via the same inline `aiError` alert as a direct send
            // rejection; any other error is already surfaced by
            // `useRegenerateAIMessage`'s own `onError` toast, and
            // `MessageBubble` also reflects a persisted `status: "failed"`
            // AI message via its own styling either way, so there is
            // nothing further to do here.
            if (error instanceof ApiRequestError && error.status === 402) {
              setAiError(INSUFFICIENT_BALANCE_MESSAGE)
            }
          }
        }
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
    [sendAIMessageMutation, queryClient, roomId, regenerateMutation],
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

      setAiError(null)
      try {
        await regenerateMutation.mutateAsync({
          aiMessageId,
          humanMessageId: aiMessage.in_response_to_message_id,
        })
      } catch (error) {
        // M1 post-review finding: a regenerate hitting 402/502 used to be a
        // silent no-op here. A 402 is now surfaced the same way a direct
        // send's is (the inline `aiError` alert); any other error is already
        // surfaced by `useRegenerateAIMessage`'s own `onError` toast, and
        // `MessageBubble` also reflects a persisted `status: "failed"` AI
        // message via its own styling either way, so there is nothing
        // further to do here.
        if (error instanceof ApiRequestError && error.status === 402) {
          setAiError(INSUFFICIENT_BALANCE_MESSAGE)
        }
      }
    },
    [messages, regenerateMutation],
  )

  const handleRetry = useCallback(
    async (messageId: string, content: string) => {
      // Drop the stale failed optimistic entry first so the mutation's own
      // `onMutate` (which appends a *new* optimistic entry with a fresh id)
      // doesn't leave both the old failed bubble and the new "sending"
      // bubble on screen at once.
      queryClient.setQueryData<MessagesInfiniteData>(
        ["rooms", roomId, "messages"],
        (old) => removeFromNewestPage(old, messageId),
      )

      // A failed AI send leaves its human echo's optimistic id recorded in
      // the `QueryClient`-backed retry-intent map (set in
      // `useSendAIMessage`'s own `onError`, see its docstring and
      // `takeFailedAISendIntent`'s) -- consulting it here is what makes a
      // retry of an AI send actually retry as an AI send (with the original
      // model/stream/private), instead of this method previously always
      // falling back to the plain-send mutation regardless of how the
      // message was originally sent -- which also silently downgraded a
      // failed private send's retry to a public one. A failed *plain* send
      // has no entry here, so it falls through to the plain-send branch
      // exactly as before. Stored in the `QueryClient` rather than a
      // component-local ref so it survives a remount of this hook.
      const aiIntent = takeFailedAISendIntent(queryClient, roomId, messageId)

      // Mirrors `handleRegenerate`'s error handling: both mutations' own
      // `onError` already roll back the optimistic entry (to `status:
      // "failed"` again) and surface a toast, so there is nothing further
      // to do here beyond the same 402-specific `aiError` alert
      // `handleSendWithAI`/`handleRegenerate` also set. Deliberately not
      // re-thrown, unlike `handleSendWithAI`, so `onRetry` callers can fire
      // this without needing their own catch.
      try {
        if (aiIntent) {
          await sendAIMessageMutation.mutateAsync({
            content,
            model: aiIntent.model,
            stream: aiIntent.stream,
            private: aiIntent.private,
          })
        } else {
          await sendMessageMutation.mutateAsync(content)
        }
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 402) {
          setAiError(INSUFFICIENT_BALANCE_MESSAGE)
        }
      }
    },
    [queryClient, roomId, sendMessageMutation, sendAIMessageMutation],
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
    currentUserId,
    senderUsernames,
    aiError,
  }
}
