import globalSetup from "./seed"

/**
 * Standalone CLI entrypoint for running the E2E seed routine outside of
 * Playwright's `globalSetup` lifecycle (used by the `test:e2e:seed`
 * Taskfile task to seed the fixture user/room without also running specs).
 *
 * Kept separate from `seed.ts` because Playwright loads that file's default
 * export as `globalSetup` under Node with a CJS transpile (`web/package.json`
 * has no `"type": "module"`) — a top-level `import.meta.main` guard there
 * would be a `SyntaxError` in that context. This file is only ever invoked
 * directly via `bun run e2e/seed/cli.ts`, never imported by Playwright, so it
 * can use `bun`'s process APIs freely.
 */
globalSetup()
  .then(() => {
    console.log("E2E seed complete.")
  })
  .catch((err) => {
    console.error("E2E seed failed:", err)
    process.exit(1)
  })
