/**
 * Shared date/time formatting helper.
 *
 * {@link formatDateTimeLocal} pins `timeZone: "UTC"` and the `"en-US"`
 * locale, so the rendered string is byte-identical between the Next.js
 * server render and the browser's hydration render — a server/browser
 * timezone (or locale) mismatch otherwise produces a React hydration error.
 *
 * Minimal port of HEAD's `web/src/lib/format.ts` (which also exports
 * `formatUtcDate`/`formatCurrency`) — only the helper this tree's callers
 * need.
 */

/**
 * Formats an ISO timestamp as a UTC-pinned date *and* time (e.g.
 * "Jul 14, 2026, 3:45 PM"), for the places that show a full timestamp rather
 * than just a calendar date.
 */
export function formatDateTimeLocal(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  })
}
