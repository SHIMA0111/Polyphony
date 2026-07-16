# Phase 18: Context Summarization

Tracks `phases.md`'s Phase 18 scope. `docs/tasks/step50.md` ("Server:
context summarization with cached summaries") delivered the entire
feature — server-side overflow detection/summarization/caching plus the
web-side "summary in use" badge — in wave 6; this file did not previously
exist (per `CLAUDE.md`'s progress-tracking convention, created here by
Step 58, the wave-8 regression step that unlocks on Step 50 and is the
first to end-to-end-verify its client-visible effect against the real
compose stack).

## Scope (delivered by Step 50; see `docs/tasks/step50.md` for the full
## per-file checklist)

- [x] Go API: detect token-limit overflow during context building
      (`MessageUsecase.assembleAIContext`,
      `server/internal/usecase/message/context.go`), comparing
      `ai.LLMGateway.EstimateTokens`'s estimate of the full verbatim context
      against `ai.ResolveContextWindow(models, model) -
      reservedOutputTokens`.
- [x] On overflow: summarize the older-public bucket of history via
      `ai.LLMGateway.Complete` (`BuildSummarizationPrompt`), cache the
      result in `message_context_summaries` (one row per room, keyed by
      `room_id`, replaced on a new covering summary).
- [x] The cached/freshly-computed summary is prepended as a single
      `system`-role `ChatMessage`, with only the newest
      `summaryRecentTailCount = 10` messages (plus any of the requester's
      own private messages, which are never summarized/cached) sent
      verbatim alongside it.
- [x] Web Frontend: indicate when summaries are being used —
      `MessageBubble.tsx` renders a `colorPalette="purple"` `Badge`
      ("Summarized history") wrapped in the existing `Tooltip` snippet next
      to any AI message whose `used_context_summary` is `true`, unit-tested
      in `MessageList.test.tsx`/`MessageBubble.test.tsx`.

## DB Changes (delivered by Step 50)

- [x] `message_context_summaries` — summary cache (`room_id` PK, `model`,
      `covered_up_to_sequence`, `summary_text`, `token_count`,
      `created_at`/`updated_at`).

## Wave 8 (Step 58) end-to-end verification

Step 50's own verification ran against mocked repositories/unit tests only
(per its own scope). Step 58's `web/e2e/regression/advanced-ai/
summarization.spec.ts` is the first to exercise the whole feature against
the real compose stack:

- [x] `web/e2e/support/seed-long-history.ts`: deterministically builds a
      room whose accumulated history (via a bounded loop of plain, non-AI
      sends) crosses Step 50's actual, discovered summarization threshold
      (`ai.ResolveContextWindow` minus `reservedOutputTokens`, self-verified
      with a 1.5x margin via `POST /tokens/estimate` before returning) —
      see that file's header comment for the full threshold derivation.
- [x] `summarization.spec.ts` proves, against the real stack: the seeded
      room's final AI-triggering send actually sets
      `ai_message.used_context_summary: true` in the REST response, a real
      `message_context_summaries` row is written for the room (verified via
      a direct `docker compose exec ... psql` query, not just the client
      response), the "Summarized history" badge renders once the room is
      opened fresh in the browser, and a fresh short room's AI reply does
      NOT show the badge (proving it is conditional, not always-on).
- [x] No web-side gap was found: the badge Step 50 already wired
      (`MessageBubble.tsx`, not `MessageList.tsx` — a wave-8 successor
      relocated per-message indicator rendering there; field names as
      shipped: `used_context_summary` on the REST/WS-finalized message,
      `summary_used` on the per-chunk `token_chunk` WS frame) needed no
      changes for this step's spec to pass.

## Excluded (per phases.md and step50.md)

- None (Phase 18 itself has no excluded scope); Step 58's own regression
  suite additionally excludes any new summarization UI beyond the existing
  badge (no settings toggle, no summary-content preview, no per-room
  summarization controls) — see `docs/tasks/step58.md`'s Out of scope.
