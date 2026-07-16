# Phase 11: Full RBAC

**Goal**: 5-tier permissions with reader/guest/member/admin/master.

**Deviation (see `phases.md` → Deviations from this plan, item 1)**: Phase 11 was **not** implemented as a
follow-on expansion of a 3-role Phase 4. Both phases were delivered together, from the start, by
`docs/tasks/step13.md` ("Go 5-tier RBAC domain + authorization middleware + ModelUsecase") as a single typed
5-tier `room.Role` enum (`Reader → Guest → Member → Admin → Master`) backed by one RBAC middleware. There is no
separate Phase-11-only diff to track — see `.ai_progress/phase4.md` for the complete, shared checklist (it covers
exactly the same merged work: the `Role`/`Action` types, the `rbac.go` `Authorize`/`RequireRole` primitive, the
`schema.sql` `CHECK` constraint on all five role values, and the full capability-matrix test coverage).

This file exists only to satisfy the one-phase-one-file convention and to record the deviation explicitly at the
Phase 11 entry point, per Step 60's docs-sync scope.

## Verification

See `.ai_progress/phase4.md`'s verification section — identical, since both phases were verified by the same
`go build`/`go vet`/`go test`/`go test -tags=integration` run against `docs/tasks/step13.md`'s merged code.
