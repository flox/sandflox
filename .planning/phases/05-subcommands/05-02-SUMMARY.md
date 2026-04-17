---
phase: 05-subcommands
plan: 02
subsystem: cli
tags: [subcommands, elevate, sandbox-exec, re-entry-detection, kernel-enforcement]

# Dependency graph
requires:
  - phase: 05-subcommands
    plan: 01
    provides: "extractSubcommand routing, knownSubcommands map, runElevate stub, WithExitCode pattern"
  - phase: 02-kernel
    provides: "buildSandboxExecArgv pattern, execWithKernelEnforcement, GenerateSBPL, WriteSBPL"
  - phase: 04-env-sanitization
    provides: "BuildSanitizedEnv for environment filtering before syscall.Exec"
provides:
  - "buildElevateArgv: pure 13-element argv builder for sandbox-exec without flox activate"
  - "elevateExec (darwin): SBPL generation + env sanitization + syscall.Exec for re-exec"
  - "elevateExec (non-darwin): hard error stub -- elevate requires macOS sandbox-exec"
  - "checkElevatePrereqs: testable prereq function for re-entry and flox session detection"
  - "runElevateWithExitCode: full elevate pipeline following WithExitCode pattern"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: ["checkElevatePrereqs returns (msg, code) for testable exit-code logic", "buildElevateArgv mirrors buildSandboxExecArgv but omits flox activate"]

key-files:
  created: []
  modified: [exec_darwin.go, exec_test.go, exec_other.go, subcommand.go, subcommand_test.go]

key-decisions:
  - "buildElevateArgv produces 13 elements (not 12 as plan spec miscounted) -- 3 -D params each take 2 argv positions"
  - "elevateExec has no fallback (unlike execWithKernelEnforcement) -- elevate's purpose IS kernel enforcement"
  - "checkElevatePrereqs returns (msg, code) tuple for testability without os.Exit or subprocess spawning"
  - "NoFloxActivate test checks standalone argv elements and /flox suffix (excluding -D KEY=VALUE params)"
  - "FLOX_ENV_CACHE captured early in runElevateWithExitCode before any env manipulation (Pitfall 6)"

patterns-established:
  - "checkElevatePrereqs pattern: prereq checks separated from handler for unit testing"
  - "buildElevateArgv: same pure-function-testable-argv pattern as buildSandboxExecArgv"

requirements-completed: [CMD-03]

# Metrics
duration: 4min
completed: 2026-04-17
---

# Phase 5 Plan 2: Elevate Subcommand Summary

**`sandflox elevate` re-execs existing flox sessions under sandbox-exec with re-entry detection, flox session detection, and 13-element argv without flox activate**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-17T11:57:07Z
- **Completed:** 2026-04-17T12:01:09Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- buildElevateArgv: pure function producing sandbox-exec argv that wraps bash directly (no flox activate) for in-place elevation
- elevateExec (darwin): full pipeline -- SBPL generation, env sanitization, syscall.Exec into sandbox-exec
- elevateExec (non-darwin): hard error with clear platform message (no fallback)
- Re-entry detection: SANDFLOX_ENABLED=1 -> "Already sandboxed" exit 0 (prevents double-nesting)
- Flox session detection: no FLOX_ENV -> "Not in a flox session" exit 1 (clear user guidance)
- 8 new test cases: 4 argv shape tests + 4 detection/prereq tests

## Task Commits

Each task was committed atomically (TDD: RED then GREEN):

1. **Task 1: buildElevateArgv + elevateExec + non-darwin stub**
   - `138e252` (test: failing tests for buildElevateArgv argv shape)
   - `16e6786` (feat: implement buildElevateArgv + elevateExec + non-darwin stub)

2. **Task 2: runElevate handler with detection logic**
   - `784bb7c` (test: failing tests for elevate detection logic)
   - `2708e81` (feat: implement runElevate with detection logic and prereq checks)

## Files Created/Modified
- `exec_darwin.go` - buildElevateArgv pure argv builder + elevateExec with SBPL/env/syscall.Exec
- `exec_test.go` - 4 tests: Interactive shape, NoFloxActivate, HasEntrypoint, SandboxExecParams
- `exec_other.go` - elevateExec non-darwin stub with hard error (no fallback)
- `subcommand.go` - checkElevatePrereqs, runElevate, runElevateWithExitCode (replaced stub)
- `subcommand_test.go` - 4 tests: AlreadySandboxed, NoFlox, HasFloxEnv, NoPolicyExits

## Decisions Made
- Plan specified 12-element argv but actual count is 13 (3 `-D` params = 6 elements, not 3). Tests adjusted to match reality.
- elevateExec has NO fallback to shell-only (unlike execWithKernelEnforcement's graceful degradation). Elevate exists specifically for kernel enforcement -- falling back defeats the purpose.
- checkElevatePrereqs returns `(msg string, code int)` tuple rather than using an exitFunc variable. Simpler than the alternatives and follows the project's established pattern of testable helpers.
- NoFloxActivate test skips `-D KEY=VALUE` params when checking for flox binary paths (FLOX_CACHE contains "flox" but is a parameter, not an invocation).
- FLOX_ENV_CACHE read before any env manipulation follows Pitfall 6 from RESEARCH.md.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected argv element count from 12 to 13**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Plan spec stated "exactly 12 elements" but 3 `-D` parameters produce 6 elements (key+value), making the total 13
- **Fix:** Updated test assertions and comments to expect 13 elements
- **Files modified:** exec_test.go
- **Verification:** TestBuildElevateArgv_Interactive passes with reflect.DeepEqual + len check
- **Committed in:** 16e6786 (Task 1 GREEN commit)

**2. [Rule 1 - Bug] Refined NoFloxActivate test to exclude -D parameter values**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Original test flagged `FLOX_CACHE=/home/x/.cache/flox` as containing "flox" -- false positive on a sandbox parameter
- **Fix:** Test now checks for standalone `flox`/`activate` elements and `/flox` suffix (excluding `=` containing elements)
- **Files modified:** exec_test.go
- **Verification:** TestBuildElevateArgv_NoFloxActivate passes correctly
- **Committed in:** 16e6786 (Task 1 GREEN commit)

---

**Total deviations:** 2 auto-fixed (2 bugs in plan spec)
**Impact on plan:** Both fixes correct inaccuracies in the plan's test specifications. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Elevate subcommand complete -- all three subcommands (validate, status, elevate) are now functional
- Phase 05 (subcommands) fully complete
- All existing tests pass with no regressions (full suite green)

## Self-Check: PASSED

All 5 files verified present. All 4 commit hashes found in git log.

---
*Phase: 05-subcommands*
*Completed: 2026-04-17*
