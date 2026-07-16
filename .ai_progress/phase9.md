# Phase 9: Ory Kratos Adoption

**Goal**: Migrate auth from SimpleJWT to Ory Kratos.

**Delivered by**: `docs/tasks/step11.md` (Infra: Ory Kratos services in docker-compose), `docs/tasks/step20.md`
(Server: Kratos session auth swap + identity migration), `docs/tasks/step30.md` (Web: Kratos-driven login/registration
+ auth data-plane flip). See those files for the full per-file implementation notes.

---

## Step 11: Ory Kratos services in docker-compose

- [x] `ory/kratos/kratos.yml` — Kratos config (DSN, public/admin base URLs, cookie/cipher secrets, identity schema,
      password method, courier via `mailslurper`)
- [x] `ory/kratos/identity.schema.json` — `traits.email`/`traits.username` identity schema
- [x] `docker-compose.yml`: `kratos-db`, `kratos-migrate`, `kratos`, `mailslurper` services added additively,
      alphabetically ordered, with health-gated `depends_on` chains
- [x] `.env.example`: `# === Ory Kratos ===` section (`KRATOS_DB_*`, `KRATOS_DSN`, `KRATOS_PUBLIC_URL`,
      `KRATOS_ADMIN_URL`, `KRATOS_COOKIE_SECRET`, `KRATOS_CIPHER_SECRET`)
- [x] `ory/README.md` documenting the new services

## Step 20: Kratos session auth swap + identity migration

- [x] `schema.sql`: `users.kratos_identity_id UUID NULL UNIQUE`; migration via `task migrate:generate --
      add_users_kratos_identity_id`
- [x] `domain/user.User.KratosIdentityID`; `UserRepository.GetByKratosIdentityID`/`SetKratosIdentityID`
- [x] `server/internal/interface/auth/kratos.go` — `KratosAuthService` implementing `domainauth.AuthService`
      (hand-rolled Kratos REST calls, no full SDK dependency)
- [x] `middleware.JWTAuth` extended to fall back to a named session cookie when no `Authorization` header is present
- [x] CORS: `AllowCredentials: true` added (origin never `"*"`, required for credentialed cross-origin cookies)
- [x] `Config.AuthMode` (`simple_jwt` default | `kratos`), `KratosPublicURL`/`AdminURL`/`CookieName`; DI branch in
      `container.go` constructing whichever `AuthService` implementation, transparent to downstream usecases
- [x] `server/cmd/kratosmigrate/main.go` — one-off CLI migrating existing SimpleJWT users into Kratos identities,
      reusing the existing argon2/PHC password hash directly (no forced password reset)

## Step 30: Web Kratos-driven login/registration + auth data-plane flip

- [x] `web/src/app/api/kratos/[...path]/route.ts` — catch-all proxy forwarding cookies/body to Kratos's public API,
      re-appending every `Set-Cookie` value
- [x] `web/src/app/api/proxy/[...path]/route.ts` rewritten to forward the raw `Cookie` header instead of minting
      `Authorization: Bearer`; Step 4's JWT-minting `/api/auth/*` routes deleted
- [x] `web/src/lib/kratos-cookie.ts` (`KRATOS_SESSION_COOKIE_NAME`); `middleware.ts` checks for it instead of
      `access_token`
- [x] `web/src/features/auth/utils/kratos-flow.ts` — shared `UiNode`/`UiContainer` types,
      `toRelativeKratosAction`, per-field/flow-level message helpers
- [x] Login/registration/logout/session flow modules (`api/login-flow.ts`, `registration-flow.ts`, `logout-flow.ts`,
      `get-session.ts`) driving Kratos's self-service browser flows
- [x] `LoginForm`/`RegisterForm` rewritten to render Kratos `ui.nodes[]` with `react-hook-form` + client-side `zod`
      validation layered on top, surfacing both flow-level and per-field Kratos error messages

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [ ] `docker compose config -q` against the full Kratos service block / live Kratos flow walkthrough — requires
      the compose stack; skipped (post-merge integration review)
