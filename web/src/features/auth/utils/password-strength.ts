/**
 * Dependency-free password-strength heuristic used to drive
 * `PasswordStrengthMeter` (`@/components/ui/password-input`) on the register
 * form.
 *
 * Returns an integer on a `0..4` scale (matching `PasswordStrengthMeter`'s
 * default `max={4}`), incrementing by one for each of the following signals
 * present in `password`:
 * - length is at least 8 characters;
 * - length is at least 12 characters;
 * - contains both an uppercase and a lowercase letter;
 * - contains at least one digit or symbol (any non-letter character).
 *
 * This is a coarse UX signal, not a security control — it does not check
 * against dictionaries/breach lists and intentionally avoids pulling in a
 * third-party library (e.g. `zxcvbn`) for a small in-repo heuristic.
 *
 * @param password - The candidate password, as currently typed.
 * @returns An integer clamped to the `0..4` range.
 */
export function getPasswordStrength(password: string): number {
  let score = 0

  if (password.length >= 8) score += 1
  if (password.length >= 12) score += 1
  if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score += 1
  if (/[^a-zA-Z]/.test(password)) score += 1

  return Math.min(score, 4)
}
