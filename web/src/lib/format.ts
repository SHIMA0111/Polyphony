/**
 * Shared date/time/currency formatting helpers (M5 post-review dedup): before
 * this module existed, roughly a dozen components each hand-rolled their own
 * near-identical `toLocaleDateString`/`toLocaleString` wrapper.
 *
 * {@link formatUtcDate} and {@link formatDateTimeLocal} always pin
 * `timeZone: "UTC"` and the `"en-US"` locale, so the rendered string is
 * byte-identical between the Next.js server render and the browser's
 * hydration render — a server/browser timezone (or, for a locale-sensitive
 * formatter, locale) mismatch otherwise produces a React hydration error.
 * {@link formatCurrency} intentionally does NOT pin a locale (passes
 * `undefined` to `Intl.NumberFormat`, matching the pre-existing behavior of
 * every call site this consolidates) — that's a pre-existing characteristic
 * of this codebase's currency formatting, not something introduced here.
 *
 * The one documented exception to the "always pin" rule is
 * `features/messages/utils/group-messages.ts`'s `dayLabel`, which
 * deliberately switches to the runtime's own locale/timezone only *after*
 * mount (see that function's own doc comment for why) and so intentionally
 * does not use these helpers.
 */

/** {@link formatUtcDate}'s default options when `opts` is omitted. */
const DEFAULT_UTC_DATE_OPTS: Intl.DateTimeFormatOptions = {
  month: "short",
  day: "numeric",
  year: "numeric",
}

/**
 * Formats an ISO timestamp as a UTC-pinned calendar date (e.g.
 * "Jul 14, 2026"), for the many places that show a message/payment/
 * subscription/etc. date without a time component.
 *
 * @param iso - The timestamp to format.
 * @param opts - Full override for the `Intl.DateTimeFormat` options (merged
 *   with `timeZone: "UTC"`, which always wins). Defaults to
 *   `{ month: "short", day: "numeric", year: "numeric" }`; pass an explicit
 *   options object (e.g. omitting `year`) for a different shape rather than
 *   relying on partial-override semantics.
 */
export function formatUtcDate(
  iso: string,
  opts: Intl.DateTimeFormatOptions = DEFAULT_UTC_DATE_OPTS,
): string {
  return new Date(iso).toLocaleDateString("en-US", { ...opts, timeZone: "UTC" })
}

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

/**
 * Formats an integer minor-currency-unit amount (Stripe convention -- cents,
 * not major units) as a localized currency string. Never render a raw cents
 * value or a hand-rolled `/ 100` division without this.
 *
 * @param cents - The amount in minor currency units (e.g. US cents).
 * @param currency - ISO 4217 currency code (e.g. "usd").
 */
export function formatCurrency(cents: number, currency: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
  }).format(cents / 100)
}
