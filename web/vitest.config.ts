import path from "node:path"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vitest/config"

/**
 * Vitest configuration for the web frontend's unit/component test suite.
 *
 * Mirrors `tsconfig.json`'s `compilerOptions.paths` (`"@/*": ["./src/*"]`) via a
 * manual `resolve.alias` entry, since Vitest does not read `tsconfig.json` path
 * mappings automatically without an extra plugin.
 */
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
})
