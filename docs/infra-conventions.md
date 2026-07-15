# Infra Conventions

This document records the conventions for editing database migrations, `docker-compose.yml`, and
`Taskfile.yml` so that the many parallel infra-touching steps in `docs/tasks/` (10, 11, 12, 31, 44,
49, 55, ...) can extend the same few files without stepping on each other's diffs.

## Atlas migrations: one migration per PR

- `server/schema.sql` is the single declarative source of truth for the database schema (Atlas
  "declarative management"). Never hand-write files under `server/migrations/` directly — always
  generate them from a `schema.sql` diff.
- Each PR that changes `server/schema.sql` must generate **exactly one** new migration file, with a
  descriptive name passed explicitly:

  ```bash
  task migrate:generate -- add_room_invitations
  ```

  Never commit a migration generated with the default `auto_<timestamp>` name in a real PR — the
  default name only exists for quick local experiments and should be regenerated with a real name
  before the PR is opened.
- Before committing (or before `atlas migrate apply` runs against a real database), lint the new
  migration for destructive/unsafe changes:

  ```bash
  task migrate:lint
  ```

  This runs `atlas migrate lint --env local --latest 1` against the `local` environment already
  declared in `server/atlas.hcl` (`dev = "docker://postgres/17/dev?search_path=public"`,
  `migration.dir = "file://migrations"`). A non-zero exit means the migration needs a follow-up
  change (e.g. a safer multi-step column rename) before it can be merged.

  **Atlas Pro login required**: as of Atlas v0.38, `atlas migrate lint` requires an authenticated
  Atlas account (`atlas login`) — it exits 1 without one, even for a clean migration. Run
  `atlas login` once per machine (a free Atlas account is sufficient) before running
  `task migrate:lint`. If you cannot or do not want to create an Atlas account, review the
  generated migration SQL by hand instead (destructive statements like `DROP COLUMN`/`DROP TABLE`
  or a `NOT NULL` addition without a default are the main things `migrate lint` would flag) and
  skip the automated lint step for that PR.
- `task up` never runs `migrate:generate` as a side effect. Migration generation is always a
  deliberate, explicit step a developer runs after editing `schema.sql`.

## Recovering from an `atlas.sum` conflict after rebase

`server/migrations/atlas.sum` is a checksum manifest covering every file in `server/migrations/`.
Because every PR in this project adds its own migration file, rebasing a branch whose migration
directory has diverged from `main` will almost always produce a conflict in `atlas.sum` (and
sometimes a numbering collision between two migrations generated around the same time).

Do **not** hand-merge `atlas.sum` — it is a generated file and a manual edit will not match the
checksums Atlas expects. Instead:

1. Resolve any conflicting migration *file* first (rename/renumber if two PRs picked overlapping
   timestamps; keep both migrations' SQL intact).
2. Regenerate the checksum file mechanically:

   ```bash
   atlas migrate hash --env local
   ```

   Run this from `server/` (or use the equivalent explicit `--dir` flag) with no other schema
   changes pending — it only rehashes existing migration files, it does not create a new one.
3. If a new schema change genuinely needs to be re-diffed against the now-rebased history (rather
   than just rehashed), instead re-run `task migrate:generate -- <name>` to regenerate the
   migration and the checksum file together.
4. Commit the regenerated `server/migrations/atlas.sum` as its own step in the rebase; never leave
   a hand-edited version of this file in the tree.

## `docker-compose.yml`: alphabetical, additive service blocks

- The top-level `services:` block is kept in alphabetical order by service name.
- When a later step adds a new service (`dex`, `hydra`, `kratos`, `minio`, `redis`, ...), insert its
  block at its alphabetically correct position. Do not reorder or reformat existing service blocks
  to "make room" — that turns an additive diff into a file-wide rewrite and causes conflicts between
  parallel PRs editing different services.
- Every service that exposes an HTTP health endpoint should declare a `healthcheck` (see `api` and
  `llm-gateway` for the `wget --spider` pattern used against plain Alpine runtime images with no
  `curl` installed) and every dependent service should express that dependency via `depends_on` with
  an explicit `condition:` (`service_healthy` or `service_completed_successfully`), never the bare
  short-list form (`depends_on: [other-service]`), so the compose dependency graph stays a real
  health-gated chain instead of a "container started" race.

## `Taskfile.yml`: additive task blocks, grouped by section

- Tasks are grouped under comment-delimited sections (`Docker`, `Migration`, `Test`, `Lint & Format`,
  `Local Development`, ...). When adding a new task, add it inside the section it belongs to instead
  of appending a new section at the end of the file, unless it genuinely introduces a new category.
- Do not reformat or reorder existing tasks when adding a new one — append the new task block within
  its section, leaving surrounding tasks byte-identical, so parallel PRs adding unrelated tasks don't
  collide on the same lines.
- The top-level `dotenv: ['.env.local', '.env']` key makes every task — Docker-based and host-run
  alike — see the same environment variables without a manual `export`. `.env.local` is optional and
  gitignored; see the next section.
- **Precedence note**: go-task gives precedence to *earlier* entries in the `dotenv` list — the
  first file that defines a variable wins, later files do not override it. `.env.local` must
  therefore come first so its host-run overrides (`localhost` instead of Docker service hostnames)
  take effect; listing `.env` first would make `.env.local` silently unable to override anything.

## Host-run dev tasks: `.env.local`

- `task dev:server`, `task dev:gateway`, and `task dev:web` run their process directly on the host,
  where Docker service hostnames (`db`, `llm-gateway`) do not resolve.
- `.env.local.example` (tracked in git) documents the `localhost`-pointed overrides needed for this
  case. Copy it once per machine:

  ```bash
  cp .env.local.example .env.local
  ```

  `.env.local` itself is gitignored (covered by the blanket `.env.*` rule in `.gitignore`, with an
  explicit `!.env.local.example` exception keeping the template tracked) and is loaded automatically
  by the Taskfile's `dotenv` key, listed *before* `.env` so its values take precedence for
  host-run tasks (see the precedence note above).
