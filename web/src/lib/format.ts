/**
 * Shared date/time/currency formatting helpers (M5 post-review dedup): before
 * this module existed, roughly a dozen components each hand-rolled their own
 * near-identical `toLocaleDateString`/`toLocaleString` wrapper.
 *
 * {@link formatUtcDate} and {@link formatDateTimeUtc} always pin
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
 * Formats an ISO timestamp as a UTC-pinned date *and* time, with a trailing
 * "UTC" marker (e.g. "Jul 14, 2026, 3:45 PM UTC"), for the places that show
 * a full timestamp rather than just a calendar date.
 *
 * Named `...Utc` (not `...Local`, this function's previous name) precisely
 * because it is the opposite of local: `timeZone: "UTC"` is pinned
 * deliberately for hydration safety (see this module's own doc comment
 * above), so the rendered time is never the viewer's actual local time. The
 * previous name invited a reader to assume otherwise; the explicit "UTC"
 * suffix makes the same point unmissable in the rendered output itself, not
 * just the function name.
 *
 * Spelled out as explicit `month`/`day`/`year`/`hour`/`minute` options
 * (reproducing `dateStyle: "medium"` + `timeStyle: "short"`'s exact output)
 * rather than those two style shorthands directly: `Intl.DateTimeFormat`
 * throws a `TypeError` if `dateStyle`/`timeStyle` are combined with
 * `timeZoneName`, and `timeZoneName` is what appends the "UTC" marker.
 */
export function formatDateTimeUtc(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  })
}

/**
 * Minor-unit exponent overrides for currencies where Stripe's own minor-unit
 * convention disagrees with ISO 4217 -- and therefore with
 * `Intl.NumberFormat`, which follows ISO 4217. Stripe bills both ISK
 * (Icelandic króna) and UGX (Ugandan shilling) as ordinary 2-decimal
 * currencies (see https://docs.stripe.com/currencies#special-cases) even
 * though ISO 4217, and so `Intl`, lists both as zero-decimal. Without this
 * override, `amountMinorUnits` for these two currencies would be divided by
 * `10 ** 0` (i.e. not divided at all), overstating the displayed amount by
 * 100x. Keyed by lowercase ISO 4217 code, matching this function's own
 * `currency` parameter convention.
 */
const STRIPE_EXPONENT_OVERRIDES: Record<string, number> = {
  isk: 2,
  ugx: 2,
}

/**
 * Formats an integer minor-currency-unit amount (Stripe convention) as a
 * localized currency string. Never render a raw minor-unit value or a
 * hand-rolled `/ 100` division without this.
 *
 * The minor-unit exponent is NOT hardcoded to 2 (cents): most currencies use
 * 2 decimal places, but Stripe (and ISO 4217) also has zero-decimal
 * currencies like `"jpy"` (minor unit === major unit) and three-decimal
 * currencies like `"kwd"`. The exponent is read back from
 * `Intl.NumberFormat`'s own `resolvedOptions().maximumFractionDigits` so it
 * always agrees with the `style: "currency"` formatting below, except for
 * {@link STRIPE_EXPONENT_OVERRIDES}'s two currencies, which are checked
 * first and win over the `Intl`-derived value.
 *
 * @param amountMinorUnits - The amount in minor currency units (e.g. cents
 *   for `"usd"`, whole yen for `"jpy"`, fils for `"kwd"`).
 * @param currency - ISO 4217 currency code (e.g. "usd").
 */
export function formatCurrency(amountMinorUnits: number, currency: string): string {
  const formatter = new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
  })
  // `style: "currency"` always populates `maximumFractionDigits` at runtime;
  // the `| undefined` in its type is inherited from the shared
  // `Intl.ResolvedNumberFormatOptions` shape (other `style`s can omit it).
  // The `?? 2` fallback matches this codebase's pre-existing (cents)
  // assumption and only matters if that invariant is ever violated.
  const intlExponent = formatter.resolvedOptions().maximumFractionDigits ?? 2
  const exponent = STRIPE_EXPONENT_OVERRIDES[currency.toLowerCase()] ?? intlExponent
  return formatter.format(amountMinorUnits / 10 ** exponent)
}
