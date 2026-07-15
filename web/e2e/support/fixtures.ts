/**
 * Fixed E2E fixture identity and room, shared by the seed routine
 * (`e2e/seed/seed.ts`) and every spec that wants a pre-seeded, logged-in
 * user instead of registering its own (future steps: 30, 37, 38, 44, 45,
 * 47, 48, 52, 53, 54, and the regression suites 56-59).
 *
 * These values are constants (not generated per run) precisely so the seed
 * routine can be idempotent: re-running it against a test database that
 * already has this user/room must be a no-op, not a duplicate-creation
 * error.
 */
export const FIXTURE_USER = {
  email: "e2e-fixture@polyphony.test",
  username: "e2e-fixture",
  password: "e2e-fixture-password-123",
} as const

/** Name of the room the seed routine ensures exists for the fixture user. */
export const FIXTURE_ROOM_NAME = "E2E Fixture Room"
