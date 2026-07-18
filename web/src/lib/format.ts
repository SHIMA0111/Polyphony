/**
 * Shared date/time formatting helper.
 *
 * {@link formatDateTimeUtc} pins `timeZone: "UTC"` and the `"en-US"`
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
 * "Jul 14, 2026, 3:45 PM UTC"), for the places that show a full timestamp
 * rather than just a calendar date.
 *
 * Named `...Utc`, not `...Local`, because the value is pinned to UTC rather
 * than the viewer's own timezone (see the module doc comment above for why)
 * — and the rendered string always ends with an explicit "UTC" suffix so
 * that pin is visible to the reader rather than being mistaken for their
 * local time. `dateStyle`/`timeStyle` cannot be combined with
 * `timeZoneName` in `Intl.DateTimeFormat` (it throws `RangeError`), so the
 * suffix is appended as a literal instead of requested from the formatter.
 */
export function formatDateTimeUtc(iso: string): string {
  const formatted = new Date(iso).toLocaleString("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  })
  return `${formatted} UTC`
}
