import { execFileSync } from "node:child_process"
import path from "node:path"

// NOTE: this module must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs (and the support modules they import) to
// CJS because web/package.json has no `"type": "module"`, so the ambient
// CJS `__dirname` is used directly.

/** `server/`, so `go run ./cmd/seed-tokens` below resolves relative to the
 * Go module root regardless of the shell's own working directory. */
const SERVER_DIR = path.resolve(__dirname, "../../../server")

/**
 * Credits `email`'s token balance directly against the e2e stack's Postgres
 * database (`db-e2e`, mapped to the host at `localhost:5433` — see
 * `docker-compose.yml`), by shelling out to `server/cmd/seed-tokens`: the
 * same `BillingRepository.CreditAndRecord` production code path the `task
 * billing:topup` dev convenience task already drives (see that CLI's own
 * doc comment), just pointed at the e2e database instead of the dev one.
 *
 * A brand-new user's `token_balances` row is lazily created at zero balance
 * (`BillingUsecase.GetOrCreateBalance` — see `billing.spec.ts`'s 402
 * coverage), so "Send with AI" against a freshly-registered user always
 * 402s unless topped up first. Crediting balance is the only way to
 * exercise a send -> attach -> regenerate (or similar AI-invoking) sequence
 * end to end without adding a dedicated (and much heavier,
 * out-of-scope-for-a-web-step) test-only balance-grant endpoint to the Go
 * API.
 *
 * Previously duplicated verbatim across every spec that needed it
 * (`attachments.spec.ts`, `private-mode.spec.ts`, `streaming.spec.ts`, and
 * several `regression/` specs) — extracted here so a future change to the
 * seeding mechanism only needs to happen once.
 */
export function creditTokenBalance(email: string, amount: number): void {
  const databaseUrl =
    process.env.E2E_SEED_DATABASE_URL ??
    "postgres://polyphony:polyphony@localhost:5433/polyphony?sslmode=disable"

  execFileSync(
    "go",
    ["run", "./cmd/seed-tokens", "-email", email, "-amount", String(amount)],
    {
      cwd: SERVER_DIR,
      env: { ...process.env, DATABASE_URL: databaseUrl },
      stdio: "pipe",
      // Playwright's own test timeout cannot interrupt a blocked Node event
      // loop (execFileSync is synchronous), so a hung seed command -- e.g.
      // the e2e Postgres never becoming reachable -- would otherwise stall
      // whichever spec calls this forever.
      timeout: 30_000,
    },
  )
}
