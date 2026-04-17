---
phase: 04-security-hardening
plan: 01
subsystem: security
tags: [env-sanitization, allowlist, credential-blocking, go-stdlib]

requires:
  - phase: 01-foundation
    provides: "Policy struct, ResolvedConfig, ParsePolicy, ResolveConfig"
  - phase: 03-shell-enforcement
    provides: "entrypoint.sh.tmpl with PYTHONDONTWRITEBYTECODE/PYTHON_NOPIP"
provides:
  - "BuildSanitizedEnv() function -- filtered env slice for syscall.Exec"
  - "SecuritySection in Policy struct with EnvPassthrough"
  - "EnvPassthrough field in ResolvedConfig"
  - "[security] section in policy.toml schema"
  - "Allowlist/blocklist/forced var constants"
affects: [04-02-integration-wiring, exec_darwin, main]

tech-stack:
  added: []
  patterns: [allowlist-based-env-filtering, 4-phase-filter-pipeline, defense-in-depth-forced-vars]

key-files:
  created: [env.go, env_test.go]
  modified: [policy.go, policy_test.go, config.go, config_test.go, policy.toml]

key-decisions:
  - "Allowlist-first design: unknown vars blocked by default (safer than blocklist-only)"
  - "Passthrough bypasses block check: user explicitly chose to pass a var, even if it matches a blocked prefix"
  - "Forced vars override parent env: PYTHONDONTWRITEBYTECODE=1 and PYTHON_NOPIP=1 always set (defense-in-depth with entrypoint.sh)"
  - "Output sorted with sort.Strings for deterministic --debug output and test assertions"

patterns-established:
  - "4-phase env filter: exact allowlist -> prefix allowlist -> user passthrough -> forced overrides"
  - "envToMap with strings.SplitN(_, '=', 2) for correct handling of values containing '='"
  - "seen map for dedup across filter phases"

requirements-completed: [SEC-01, SEC-02, SEC-03]

duration: 4min
completed: 2026-04-17
---

# Phase 4 Plan 1: Env Sanitization Engine Summary

**Allowlist-based env filtering with 20+ blocked credential prefixes, policy-driven passthrough, and forced Python safety vars**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-17T01:41:23Z
- **Completed:** 2026-04-17T01:45:31Z
- **Tasks:** 1 (TDD: RED + GREEN)
- **Files modified:** 7

## Accomplishments
- BuildSanitizedEnv() filters environment through 4-phase pipeline: exact allowlist, prefix matching, user passthrough, forced vars
- Blocked 20+ credential-carrying prefixes (AWS_, SSH_, GPG_, GITHUB_, DOCKER_, OPENAI_, etc.) plus 12 exact names
- Policy schema extended with [security] section supporting env-passthrough user overrides
- 13 new unit tests covering all SEC-01/02/03 requirements with zero regressions on existing 69 tests

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): Failing tests for env sanitization** - `ff01c4f` (test)
2. **Task 1 (GREEN): Implement env sanitization engine** - `6153ed0` (feat)

_No REFACTOR commit -- implementation was clean from GREEN phase._

## Files Created/Modified
- `env.go` - BuildSanitizedEnv function with allowlist/blocklist/forced constants, envToMap and isBlocked helpers
- `env_test.go` - 9 unit tests: allowlist essentials, prefix allowlist, unknown blocking, sensitive prefix blocking, passthrough, passthrough-overrides-block, forced vars, forced override, sorted output
- `policy.go` - SecuritySection struct added to Policy, mapToPolicy parses [security] section
- `policy_test.go` - 3 tests: SecuritySection parsing, empty passthrough, missing section
- `config.go` - EnvPassthrough []string field added to ResolvedConfig, wired in ResolveConfig
- `config_test.go` - 1 test: EnvPassthrough resolution through config pipeline
- `policy.toml` - New [security] section with env-passthrough = []

## Decisions Made
- Allowlist-first design over blocklist-only: unknown vars are excluded by default, safer against future credential-carrying env vars
- Passthrough bypasses block check intentionally: if a user puts AWS_SECRET_ACCESS_KEY in env-passthrough, they know what they're doing
- Forced vars set in Go env builder AND entrypoint.sh (belt-and-suspenders): Python safety flags survive even if shell enforcement is bypassed
- sort.Strings on output for deterministic test assertions and reproducible --debug logs

## Deviations from Plan

None -- plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None -- no external service configuration required.

## Known Stubs
None -- all functionality is fully wired. BuildSanitizedEnv is a complete, tested function ready to be called from exec paths (wiring happens in Plan 04-02).

## Next Phase Readiness
- BuildSanitizedEnv is ready to replace os.Environ() at both exec call sites (exec_darwin.go and main.go)
- Plan 04-02 will wire BuildSanitizedEnv into the exec paths and add integration tests proving env scrubbing works end-to-end

## Self-Check: PASSED

- All 7 source/test files exist
- Both commits verified (ff01c4f RED, 6153ed0 GREEN)
- SUMMARY.md created at expected path

---
*Phase: 04-security-hardening*
*Completed: 2026-04-17*
