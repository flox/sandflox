---
phase: 03-shell-enforcement-artifacts
plan: 02
subsystem: shell-enforcement
tags: [go, exec, argv, sandbox-exec, bash-rcfile, bash-c, entrypoint, net-blocked-flag]

# Dependency graph
requires:
  - phase: 01-go-binary-foundation
    provides: ResolvedConfig, WriteCache, main.go orchestration, cache.go writers
  - phase: 02-kernel-enforcement-sbpl-sandbox-exec
    provides: buildSandboxExecArgv, execWithKernelEnforcement, exec_test.go argv tests
  - plan: 03-01
    provides: WriteShellArtifacts, GenerateEntrypoint, shellquote, embedded templates
provides:
  - buildSandboxExecArgv emits D-01 (bash --rcfile) and D-02 (bash -c source) argv payloads
  - execWithKernelEnforcement accepts and threads entrypointPath on both darwin and non-darwin
  - main.go calls WriteShellArtifacts after WriteCache, threads entrypointPath to exec path
  - Five updated Phase 2 argv-shape tests assert 16-element interactive / 16+N non-interactive shape
  - TestCacheWriteNetBlockedFlagToggle verifies SHELL-07 runtime gate toggle behavior
affects: [03-03, phase-04]

# Tech tracking
tech-stack:
  added: []
  patterns: [d01-rcfile-dispatch, d02-bash-c-dispatch, shellquote-in-argv-builder]

key-files:
  created: []
  modified:
    - exec_darwin.go
    - exec_other.go
    - exec_test.go
    - main.go
    - cache_test.go

key-decisions:
  - "Renamed TestBuildSandboxExecArgs_NoUserArgsDoesNotEmitDoubleDash to InteractiveUsesRcfileNotDashC -- old name no longer describes behavior after D-01 adds '--' before bash"
  - "net-blocked.flag writer already existed in cache.go from Phase 1 -- no modification needed, only added toggle test"
  - "main.go entrypointPath wiring done in Task 1 (minimal) to keep build green, then refined in Task 2 with WriteShellArtifacts insertion"

patterns-established:
  - "D-01/D-02 dispatch pattern: interactive uses bash --rcfile <ep> -i; non-interactive uses bash -c 'source <ep> && exec \"$@\"' --"
  - "shellquote(entrypointPath) in argv builder for path-safe bash -c payload construction"
  - "entrypointPath threading: computed in main.go, passed through execWithKernelEnforcement to buildSandboxExecArgv"

requirements-completed: [SHELL-01, SHELL-02, SHELL-04, SHELL-05, SHELL-06, SHELL-07]

# Metrics
duration: 5min
completed: 2026-04-17
---

# Phase 03 Plan 02: Runtime Wiring Summary

**Wired WriteShellArtifacts into main.go and rewired buildSandboxExecArgv for D-01 (bash --rcfile entrypoint.sh -i) and D-02 (bash -c 'source entrypoint.sh && exec "$@"') dispatch -- all five Phase 2 argv tests updated, net-blocked.flag toggle test added, zero regressions**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-17T00:32:55Z
- **Completed:** 2026-04-17T00:38:36Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- Rewired `buildSandboxExecArgv` to emit 16-element interactive argv (D-01: `bash --rcfile <entrypoint> -i`) and 16+N non-interactive argv (D-02: `bash -c 'source <ep> && exec "$@"' -- CMD...`)
- Updated `execWithKernelEnforcement` on both darwin and non-darwin stubs to accept `entrypointPath` parameter, keeping cross-platform builds green
- Inserted `WriteShellArtifacts(cacheDir, config)` call in main.go between WriteCache and emitDiagnostics (D-04: regenerate every run)
- Computed `entrypointPath := filepath.Join(cacheDir, "entrypoint.sh")` and threaded it through the exec pipeline
- All five Phase 2 argv-shape tests updated and passing against the new 16/16+N element shape
- Added `TestCacheWriteNetBlockedFlagToggle` verifying blocked<->unrestricted flag lifecycle

## Task Commits

Each task was committed atomically:

1. **Task 1: Rewire buildSandboxExecArgv + tests (TDD RED)** - `db77af5` (test)
2. **Task 1: Rewire buildSandboxExecArgv + tests (TDD GREEN)** - `27879ef` (feat)
3. **Task 2: Net-blocked.flag toggle test** - `223ab84` (test)
4. **Task 2: Wire WriteShellArtifacts into main.go** - `b5d7aa9` (feat)

## Files Created/Modified

- `exec_darwin.go` (129 lines, was 122) - buildSandboxExecArgv accepts entrypointPath; D-01/D-02 two-branch dispatch using shellquote for path safety
- `exec_other.go` (21 lines, was 20) - execWithKernelEnforcement stub accepts entrypointPath for cross-platform signature sync
- `exec_test.go` (238 lines, was 189) - Five updated tests: Interactive (16 elements), WithUserCommand (18 elements), PreservesAbsolutePathForFlox (unchanged assert), InteractiveUsesRcfileNotDashC (renamed), HandlesUserArgsWithDashes (two -- and two -c tokens)
- `main.go` (172 lines, was 166) - WriteShellArtifacts call + entrypointPath computation inserted between WriteCache and emitDiagnostics
- `cache_test.go` (289 lines, was 242) - TestCacheWriteNetBlockedFlagToggle: blocked->unrestricted->blocked cycle + idempotency check

## Decisions Made

- **Renamed test:** `TestBuildSandboxExecArgs_NoUserArgsDoesNotEmitDoubleDash` renamed to `TestBuildSandboxExecArgs_InteractiveUsesRcfileNotDashC` because the D-01 interactive argv now includes `--` before `bash`, making the old name misleading. The new name describes what the test actually validates: interactive mode uses `--rcfile` and `-i`, not `-c` and `"$@"`.
- **net-blocked.flag already present:** The plan specified adding a net-blocked.flag writer to cache.go, but this was already implemented in Phase 1. Only the toggle test was added (no production code change to cache.go).
- **entrypointPath in Task 1:** To keep the build green after changing the exec function signatures, the entrypointPath computation was added to main.go in Task 1 (minimal wiring). Task 2 then inserted WriteShellArtifacts above it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] main.go signature update needed in Task 1**
- **Found during:** Task 1 (exec_darwin.go signature change)
- **Issue:** Changing `execWithKernelEnforcement` signature in exec_darwin.go broke the call site in main.go. The plan said "DO NOT modify main.go in this task" but the build could not succeed without updating the call.
- **Fix:** Added minimal `entrypointPath` computation and updated call site in main.go during Task 1 to keep the build green. Task 2 then refined the placement by adding WriteShellArtifacts.
- **Files modified:** main.go
- **Verification:** `go build ./...` succeeds, all tests pass
- **Committed in:** 27879ef (Task 1 GREEN commit)

**2. [Rule 3 - Blocking] net-blocked.flag already existed in cache.go**
- **Found during:** Task 2 (cache.go modification)
- **Issue:** The plan specified adding net-blocked.flag writer to cache.go, but this logic was already present from Phase 1 (lines 35-43 of cache.go). Existing tests TestCacheWriteNetBlockedFlag and TestCacheWriteNoNetBlockedFlag already covered basic behavior.
- **Fix:** Skipped the production code change (no modification needed). Added only the flip/toggle test (TestCacheWriteNetBlockedFlagToggle) to verify the D-04 stale-cache-purge scenario.
- **Files modified:** cache_test.go (test only)
- **Verification:** Toggle test passes; existing tests unaffected
- **Committed in:** 223ab84 (Task 2 test commit)

---

**Total deviations:** 2 auto-fixed (both Rule 3 - Blocking)
**Impact on plan:** Both deviations were necessary for correctness. No scope creep -- the end state matches the plan's requirements exactly.

## Issues Encountered

None -- execution was straightforward once the pre-existing cache.go code was identified.

## Known Stubs

None -- all wiring is complete, no placeholder or TODO content.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 03-03 can now run subprocess integration tests exercising the full pipeline: shell artifacts on disk, flox activate launched via bash --rcfile / bash -c, and entrypoint.sh executing under real sandbox-exec
- The complete execution chain is: ParsePolicy -> ResolveConfig -> WriteCache (config + paths + net-blocked.flag) -> WriteShellArtifacts (entrypoint.sh + fs-filter.sh + usercustomize.py) -> emitDiagnostics -> execWithKernelEnforcement (sandbox-exec with D-01/D-02 argv)

## Argv Shape Confirmation

- **Interactive (D-01):** 16 elements ending in `... activate -- bash --rcfile <entrypoint> -i`
- **Non-interactive (D-02):** 16 + len(userArgs) elements ending in `... activate -- bash -c 'source <ep> && exec "$@"' -- CMD ARGS...`
- **Entrypoint path safety:** shellquote wraps the path in single quotes with `'\''` escape for embedded quotes

## Test Counts

- 5 updated Phase 2 argv tests (all PASS)
- 1 new cache toggle test (PASS)
- 20 Plan 03-01 + Phase 2 generator tests (all PASS, no regression)
- Full suite: `go test -count=1 ./...` -- all PASS

## Self-Check: PASSED

- All 5 modified files verified present on disk
- All 4 task commits (db77af5, 27879ef, 223ab84, b5d7aa9) verified in git log
- 03-02-SUMMARY.md verified present

---
*Phase: 03-shell-enforcement-artifacts*
*Completed: 2026-04-17*
