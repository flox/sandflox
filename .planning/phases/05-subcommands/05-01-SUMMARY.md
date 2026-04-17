---
phase: 05-subcommands
plan: 01
subsystem: cli
tags: [subcommands, routing, validate, status, cache-reader]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "CLI flags, policy parsing, config resolution, cache writer, diagnostics"
  - phase: 03-shell-enforcement
    provides: "WriteShellArtifacts for validate artifact generation"
provides:
  - "extractSubcommand routing for validate/status/elevate subcommands"
  - "runValidate: policy summary without sandbox launch"
  - "runStatus: cached enforcement state reader"
  - "ReadCache: inverse of WriteCache (config.json deserializer)"
  - "discoverCacheDir: FLOX_ENV_CACHE-aware cache discovery"
  - "runDefault: extracted original main() pipeline"
affects: [05-subcommands]

# Tech tracking
tech-stack:
  added: []
  patterns: ["extractSubcommand + ParseFlags two-phase arg parsing", "WithExitCode testable handler pattern"]

key-files:
  created: [subcommand.go, subcommand_test.go]
  modified: [main.go, cache.go, cache_test.go]

key-decisions:
  - "extractSubcommand scans all arg positions (not just first) for known subcommands, enabling --debug validate and validate --debug equivalence"
  - "Stop scanning at -- delimiter to preserve backward compat with sandflox -- CMD exec pipeline"
  - "runValidateWithExitCode / runStatusWithExitCode pattern avoids os.Exit in tests without subprocess spawning"
  - "discoverCacheDir checks FLOX_ENV_CACHE first (inside flox activate), then falls back to project-relative path"
  - "validate writes cache+shell artifacts before printing summary so tool/rule counts are accurate"

patterns-established:
  - "WithExitCode pattern: handler returns int exit code for testability, wrapper calls os.Exit"
  - "discoverCacheDir: env var > project-relative fallback for cache location"

requirements-completed: [CMD-01, CMD-02]

# Metrics
duration: 5min
completed: 2026-04-17
---

# Phase 5 Plan 1: Subcommand Routing + Validate/Status Summary

**Subcommand routing with extractSubcommand + validate (policy dry-run) and status (cached state reader) handlers using WithExitCode testable pattern**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-17T11:50:04Z
- **Completed:** 2026-04-17T11:54:39Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Subcommand routing: extractSubcommand scans all arg positions, routes validate/status/elevate, preserves unknown args for default exec pipeline
- Validate handler: parses policy.toml, prints summary (profile, modes, tool count, denied count) without launching sandbox. --debug emits full diagnostics
- Status handler: reads cached config.json via ReadCache, reports enforcement profile from within a running sandbox session
- Both flag positions work identically: `sandflox --debug validate` == `sandflox validate --debug`
- ReadCache inverse of WriteCache for config.json deserialization
- 20 new test cases across routing, validate, status, cache, and cache discovery

## Task Commits

Each task was committed atomically (TDD: RED then GREEN):

1. **Task 1: Subcommand routing + validate handler**
   - `f69a9e3` (test: failing tests for routing and validate)
   - `27f96a0` (feat: implement routing and validate handler)

2. **Task 2: Status handler + ReadCache**
   - `bee8aad` (test: failing tests for ReadCache, status, discoverCacheDir)
   - `f870ff4` (feat: implement ReadCache, discoverCacheDir, status handler)

## Files Created/Modified
- `subcommand.go` - extractSubcommand routing, runValidate, runStatus, discoverCacheDir, runElevate stub
- `subcommand_test.go` - 20 tests covering routing, flag position, validate output, status output, cache discovery
- `main.go` - Refactored main() with subcommand routing, extracted runDefault for original pipeline
- `cache.go` - Added ReadCache (inverse of WriteCache)
- `cache_test.go` - Added TestReadCacheRoundTrip, TestReadCacheMissing, TestReadCacheCorrupt

## Decisions Made
- extractSubcommand scans all positions, not just index 0, so `--debug validate` works by finding "validate" at index 1
- Stop scanning at "--" delimiter to ensure `sandflox -- echo hello` routes to default exec pipeline (backward compat)
- WithExitCode pattern for testable handlers avoids subprocess spawning, matching the project's existing `var stderr io.Writer` pattern
- validate writes cache + shell artifacts so ParseRequisites and rule counting work on fresh generated files
- discoverCacheDir prefers FLOX_ENV_CACHE (inside `flox activate`) over project-relative path

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Subcommand routing infrastructure ready for elevate implementation (Plan 05-02)
- ReadCache and discoverCacheDir provide the foundation for status reporting inside active sandboxes
- All existing tests pass with no regressions

---
*Phase: 05-subcommands*
*Completed: 2026-04-17*
