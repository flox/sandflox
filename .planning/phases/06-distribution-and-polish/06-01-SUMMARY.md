---
phase: 06-distribution-and-polish
plan: 01
subsystem: distribution
tags: [flox, floxhub, nix, flox-publish, flox-install, buildGoModule]

# Dependency graph
requires:
  - phase: 01-go-scaffold-policy-engine-build-validation
    provides: "Nix build expression (.flox/pkgs/sandflox.nix) with buildGoModule, fileset.toSource, -trimpath"
  - phase: 05-subcommands
    provides: "Complete Go binary with all subcommands (validate, status, elevate)"
provides:
  - "sandflox published to FloxHub as 8BitTacoSupreme/sandflox"
  - "flox install 8BitTacoSupreme/sandflox works in any Flox environment"
  - "Hermetic Nix build verified (fileset.toSource + -trimpath)"
affects: []

# Tech tracking
tech-stack:
  added: [flox-publish, floxhub-catalog]
  patterns: [flox-build-publish-install-pipeline]

key-files:
  created: []
  modified:
    - ".flox/pkgs/sandflox.nix"
    - ".flox/env.json"

key-decisions:
  - "Fixed env.json by git-restoring to local path format (removed FloxHub owner field that blocked flox build)"
  - "Added ../../templates to Nix fileset.unions for go:embed template files"
  - "Removed stale .flox/env.lock (FloxHub artifact not to be confused with manifest.lock)"

patterns-established:
  - "flox build/publish/install pipeline: build from .flox/pkgs/*.nix, publish to FloxHub, install via catalog"

requirements-completed: [DIST-02, DIST-03, DIST-05]

# Metrics
duration: 12min
completed: 2026-04-17
---

# Phase 6 Plan 01: Distribution and Polish Summary

**sandflox published to FloxHub via `flox publish` and verified installable via `flox install 8BitTacoSupreme/sandflox` with functional --help and all subcommands**

## Performance

- **Duration:** 12 min
- **Started:** 2026-04-17T12:25:00Z
- **Completed:** 2026-04-17T12:49:37Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Fixed env.json blocker (removed FloxHub owner field) so `flox build` works with local path environment
- Fixed Nix fileset to include `../../templates` directory for `//go:embed templates/*.tmpl` patterns
- Built sandflox via `flox build sandflox` producing a static Go binary at `result-sandflox/bin/sandflox`
- Published sandflox to FloxHub via `flox publish sandflox` under `8BitTacoSupreme` catalog
- Verified `flox install 8BitTacoSupreme/sandflox` works in a fresh Flox environment (human-verified)
- Confirmed `sandflox --help` shows all expected flags (-debug, -net, -policy, -profile, -requisites)
- Verified DIST-05: Nix expression contains `fileset.toSource` and `-trimpath` for hermetic reproducible builds

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix env.json, push to remote, build and publish to FloxHub** - `562ae0b` (fix)
2. **Task 2: Verify flox install in fresh environment** - checkpoint:human-verify (no commit, verification-only)

**Plan metadata:** (pending -- this commit)

## Files Created/Modified
- `.flox/pkgs/sandflox.nix` - Added `../../templates` to fileset.unions for go:embed support
- `.flox/env.json` - Restored to local path format (removed `"owner"` field)

## Decisions Made
- Fixed env.json by `git restore` to revert from FloxHub-linked format to local path format -- the FloxHub owner field prevents `flox build` from working
- Added `../../templates` directory to Nix fileset.unions -- the Go binary uses `//go:embed templates/*.tmpl` and Nix builds failed without these files in the source derivation
- Removed `.flox/env.lock` (FloxHub lockfile artifact) -- not to be confused with `.flox/env/manifest.lock` which IS committed

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added templates directory to Nix fileset**
- **Found during:** Task 1 (flox build)
- **Issue:** `flox build sandflox` failed with `shell.go:24:12 pattern templates/entrypoint.sh.tmpl no matching files found` because Nix fileset only included `.go` files and `go.mod`
- **Fix:** Added `../../templates` to `lib.fileset.unions` in `.flox/pkgs/sandflox.nix`
- **Files modified:** `.flox/pkgs/sandflox.nix`
- **Verification:** `flox build sandflox` succeeds, binary is functional
- **Committed in:** `562ae0b`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Essential fix for build to succeed. Templates were added in Phase 3 but the Nix expression was written in Phase 1 before templates existed. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviation above.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all distribution pipeline steps are fully wired and verified.

## Next Phase Readiness
- This is the final phase (Phase 6 of 6). All v1 requirements are complete.
- sandflox is available for installation via `flox install 8BitTacoSupreme/sandflox`
- All 34 v1 requirements satisfied across 6 phases

## Self-Check: PASSED

- FOUND: .flox/pkgs/sandflox.nix
- FOUND: commit 562ae0b
- FOUND: 06-01-SUMMARY.md

---
*Phase: 06-distribution-and-polish*
*Completed: 2026-04-17*
