---
phase: 01-go-scaffold-policy-engine-and-build-validation
plan: 02
subsystem: config-engine
tags: [go, profile-resolution, cli-flags, cache-writer, path-resolution, diagnostics, syscall-exec]

# Dependency graph
requires:
  - "01-01: Go module scaffold, TOML parser, Policy types"
provides:
  - "ResolveConfig with 3-level profile precedence (env > policy > default) + CLI override layer"
  - "ResolvePath expanding ~, relative paths, /tmp canonicalization on macOS"
  - "ParseRequisites for reading tool whitelist files"
  - "ParseFlags with ContinueOnError and -- separator for CLI"
  - "WriteCache producing all 10 cache artifacts matching bash implementation"
  - "emitDiagnostics with [sandflox] prefix format, debug mode, testable via io.Writer"
  - "main.go full pipeline: parse -> resolve -> cache -> emit -> exec"
  - "35 new passing tests (config, CLI, cache, diagnostics)"
affects: [01-03, phase-02, phase-03]

# Tech tracking
tech-stack:
  added: [go-flag, encoding-json, syscall-exec, os-exec-lookpath]
  patterns: [three-level-profile-precedence, cli-override-layer, package-level-stderr-writer, cache-artifact-layout]

key-files:
  created:
    - config.go
    - config_test.go
    - cli.go
    - cli_test.go
    - cache.go
    - cache_test.go
    - main.go
    - main_test.go
  modified: []

key-decisions:
  - "Package-level var stderr io.Writer = os.Stderr for testable diagnostics without subprocess spawning"
  - "CLI --profile overrides both SANDFLOX_PROFILE env var and policy.Meta.Profile (strongest override)"
  - "ResolvedConfig uses json struct tags with snake_case matching existing config.json format"
  - "WriteCache appends trailing newline to JSON output for shell-friendly file reading"
  - "unwrapPathError helper for os.IsNotExist checks through [sandflox] ERROR: wrapper"

patterns-established:
  - "Three-level profile precedence: SANDFLOX_PROFILE env > policy.Meta.Profile > 'default'"
  - "CLI flags override after profile resolution (--net forces unrestricted, --profile re-resolves)"
  - "Cache artifacts: 10 files in .flox/cache/sandflox/ matching bash implementation layout"
  - "Diagnostics via fmt.Fprintf(stderr, ...) with [sandflox] prefix, not log package"
  - "syscall.Exec for process replacement into flox activate (no child process)"
  - "No-policy fallback: exec bare flox activate with warning"

requirements-completed: [CORE-03, CORE-04, CORE-05, CORE-06, CORE-07]

# Metrics
duration: 4min
completed: 2026-04-16
---

# Phase 01 Plan 02: Config Resolution, CLI, Cache, Main Entry Point Summary

**Full pipeline from CLI flags through profile resolution, cache artifact writing, and syscall.Exec into flox activate with 35 new tests**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-16T02:13:47Z
- **Completed:** 2026-04-16T02:17:59Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Config resolution implements three-level precedence (env var > policy file > default) with CLI override layer on top
- Path resolution expands ~, resolves relative paths, canonicalizes /tmp to /private/tmp on macOS, preserves trailing slash
- Cache writer produces all 10 expected files matching the existing bash+python implementation format
- CLI flag parser handles --profile, --net, --debug, --policy, --requisites with native -- separator
- Main entry point orchestrates full flow: parse -> resolve -> cache -> emit -> exec
- Diagnostics use [sandflox] prefix format, testable via package-level stderr writer
- syscall.Exec for clean process replacement into flox activate (interactive and non-interactive modes)
- 35 new tests across config_test.go (13), cli_test.go (9), cache_test.go (9), main_test.go (3) -- all passing
- Binary builds and runs correctly against real policy.toml, producing correct diagnostics and cache artifacts

## Task Commits

Each task was committed atomically:

1. **Task 1: Config resolution, CLI flags, cache writer with tests** - `0d7be4b` (feat)
2. **Task 2: Main entry point with diagnostics, exec, and unit tests** - `ef92b06` (feat)

## Files Created/Modified
- `config.go` - ResolveConfig, ResolvePath, ParseRequisites (134 lines)
- `config_test.go` - 13 tests for profile resolution, path resolution, requisites parsing (204 lines)
- `cli.go` - CLIFlags struct and ParseFlags function (31 lines)
- `cli_test.go` - 9 tests for flag parsing, -- separator, CLI overrides (97 lines)
- `cache.go` - WriteCache producing all 10 cache artifacts (88 lines)
- `cache_test.go` - 9 tests for cache directory creation, file content, JSON validity (217 lines)
- `main.go` - Full entry point: CLI, policy, config, cache, diagnostics, exec (137 lines)
- `main_test.go` - 3 tests for emitDiagnostics output format (86 lines)

## Decisions Made
- Used package-level `var stderr io.Writer = os.Stderr` to make diagnostics testable without subprocess spawning -- cleaner than accepting io.Writer parameter since main() and execFlox() also write to stderr
- CLI --profile is the strongest override, beating both SANDFLOX_PROFILE env var and policy.Meta.Profile -- matches the intent that CLI flags are ad-hoc overrides for testing
- ResolvedConfig uses `json:"snake_case"` struct tags to produce byte-compatible config.json with the existing bash implementation
- Trailing newline appended to config.json output for shell-friendly reading (`cat config.json` works cleanly)
- unwrapPathError navigates through fmt.Errorf %w wrapping to find the underlying os.PathError for no-policy fallback

## Deviations from Plan

None -- plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None -- all functions are fully implemented. The binary successfully parses real policy.toml, resolves profiles, writes cache, emits diagnostics, and execs into flox activate.

## Next Phase Readiness
- All Go source files export the functions Plan 03 needs: ParsePolicy, ResolveConfig, ParseFlags, WriteCache
- Binary builds with `go build` and is ready for Nix `buildGoModule` integration in Plan 03
- Cache artifacts produced by the Go binary match the bash implementation format
- 48 total tests across all files (14 parser + 35 new) provide regression coverage

## Self-Check: PASSED

- All 8 created files exist on disk
- Commit 0d7be4b found in git log
- Commit ef92b06 found in git log
- All 48 tests pass with `go test -v -count=1 ./...`

---
*Phase: 01-go-scaffold-policy-engine-and-build-validation*
*Completed: 2026-04-16*
