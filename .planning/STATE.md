---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: verifying
stopped_at: Completed 06-01-PLAN.md
last_updated: "2026-04-17T13:12:00.418Z"
last_activity: 2026-04-17
progress:
  total_phases: 6
  completed_phases: 6
  total_plans: 14
  completed_plans: 14
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-15)

**Core value:** AI agents cannot escape the sandbox -- not through PATH manipulation, absolute paths, shell redirects, or kernel syscalls -- without requiring a Linux VM or devcontainer.
**Current focus:** Phase 06 — distribution-and-polish

## Current Position

Phase: 06
Plan: Not started
Status: Phase complete — ready for verification
Last activity: 2026-04-17

Progress: [##########] 100%

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: -
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

**Recent Trend:**

- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
| Phase 01 P01 | 4min | 2 tasks | 6 files |
| Phase 01 P02 | 4min | 2 tasks | 8 files |
| Phase 01 P03 | 4min | 2 tasks | 3 files |
| Phase 02 P01 | 4min | 2 tasks tasks | 3 files files |
| Phase 02 P02 | 3min | 3 tasks | 5 files |
| Phase 02 P03 | 5min | 1 tasks | 1 files |
| Phase 03 P01 | 5min | 2 tasks | 5 files |
| Phase 03 P02 | 5min | 2 tasks | 5 files |
| Phase 03 P03 | 10min | 1 tasks | 3 files |
| Phase 04 P01 | 4min | 1 tasks | 7 files |
| Phase 04 P02 | 3min | 2 tasks | 4 files |
| Phase 05 P01 | 5min | 2 tasks | 5 files |
| Phase 05 P02 | 4min | 2 tasks | 5 files |
| Phase 06 P01 | 12min | 2 tasks | 2 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: Nix build validation (DIST-01, DIST-04) pulled into Phase 1 to catch build failures early
- Roadmap: Security hardening (SEC-01/02/03) separated from shell enforcement as distinct env-var concern
- Roadmap: Phase 3 depends on Phase 1 (not Phase 2) since shell enforcement needs policy but not SBPL
- [Phase 01]: Module name 'sandflox' (standalone binary, no import path needed)
- [Phase 01]: Custom TOML subset parser (~250 lines) satisfies zero-dep constraint
- [Phase 01]: Package-level var stderr io.Writer for testable diagnostics without subprocess spawning
- [Phase 01]: CLI --profile is strongest override, beating env var and policy file
- [Phase 01]: ResolvedConfig uses json:snake_case tags for byte-compatible config.json
- [Phase 01]: vendorHash = null confirmed correct for zero external Go deps
- [Phase 01]: lib.fileset.toSource includes only .go files and go.mod; runtime config excluded from Nix build
- [Phase 01]: -trimpath in buildFlags (not ldflags) for correct Go compiler flag placement
- [Phase 02]: [Phase 02]: SBPL generator mirrors bash rule-by-rule; byte-identical output confirmed via canonical diff
- [Phase 02]: [Phase 02]: Helper decomposition: writeSBPLHeader/Denied/Filesystem/Network — one per bash section header
- [Phase 02]: [Phase 02]: Flox-required overrides block gated by len(Denied)>0 (matches bash conditional at sandflox.bash:209)
- [Phase 02]: [Phase 02]: Platform split via build tags -- //go:build darwin + //go:build !darwin keeps binary buildable on Linux CI runners while targeting macOS only
- [Phase 02]: [Phase 02]: buildSandboxExecArgv as pure helper -- enables microsecond unit tests on argv shape without spawning sandbox-exec
- [Phase 02]: [Phase 02]: D-07 SBPL diagnostic lives in emitDiagnostics (not exec_darwin) so --debug output is useful for dry-run inspection on any platform
- [Phase 02]: Subprocess pattern (exec.CommandContext not syscall.Exec) is mandatory for integration tests -- test process must stay unsandboxed (Pitfall 7)
- [Phase 02]: TestSandboxBlocksDeniedPath uses /private/tmp as project root to avoid macOS firmlink canonicalization gap (/var/folders -> /private/var/folders at VFS layer)
- [Phase 02]: Manual verification (Task 2) skipped per user decision; plan accepted as verified by 4 automated subprocess tests. KERN-04 process-tree and KERN-05 interactive TTY rely on Plan 02-02 smoke test for confidence.
- [Phase 03]: Improved fs-filter prefix matching: dual case alternatives over bash bug-for-bug compat
- [Phase 03]: text/template (not html/template) for shell generation; shellquote FuncMap for safe quoting
- [Phase 03]: usercustomize.py is static template -- Python reads cached state files at runtime, no Go substitutions
- [Phase 03]: Renamed TestBuildSandboxExecArgs_NoUserArgsDoesNotEmitDoubleDash to InteractiveUsesRcfileNotDashC -- D-01 interactive now includes '--' before bash
- [Phase 03]: net-blocked.flag writer already present in cache.go from Phase 1; only toggle test added in Plan 03-02
- [Phase 03]: Fixed export -f _sfx_check_write_target in fs-filter.sh.tmpl -- required for D-02 non-interactive mode where wrapper functions survive bash -c child process boundary
- [Phase 03]: Replaced cat|tr with bash read builtin in entrypoint.sh.tmpl -- external commands unavailable after PATH wipe to sandflox/bin
- [Phase 03]: Integration test probes use bash builtins (echo, printf, command -v, parameter expansion) instead of external commands for portability across minimal/full flox environments
- [Phase 04]: Allowlist-first env filtering: unknown vars blocked by default, safer than blocklist-only
- [Phase 04]: Passthrough bypasses block check: user explicitly passes a var even if it matches blocked prefix
- [Phase 04]: Forced vars in Go + shell (defense-in-depth): PYTHONDONTWRITEBYTECODE=1 and PYTHON_NOPIP=1 set before exec
- [Phase 04]: Sorted env output for deterministic --debug and test assertions
- [Phase 04]: cfg nil guard in execFlox: no-policy fallback passes os.Environ() unchanged for graceful degradation
- [Phase 05]: extractSubcommand scans all arg positions for known subcommands, stops at -- delimiter for backward compat
- [Phase 05]: WithExitCode testable handler pattern returns int exit code instead of os.Exit
- [Phase 05]: discoverCacheDir prefers FLOX_ENV_CACHE over project-relative path for cache location
- [Phase 05]: buildElevateArgv produces 13 elements (not 12); elevateExec has no shell-only fallback (elevate IS kernel enforcement)
- [Phase 05]: checkElevatePrereqs returns (msg, code) tuple for testable os.Exit-free prereq checking
- [Phase 06]: Fixed env.json by git-restoring to local path format (removed FloxHub owner field that blocked flox build)
- [Phase 06]: Added ../../templates to Nix fileset.unions for go:embed template files (Phase 3 added templates after Phase 1 wrote the Nix expression)

### Pending Todos

None yet.

### Blockers/Concerns

- Research gap: Go version in Flox catalog (1.24 vs 1.25) needs verification during Phase 1
- Research gap: `sandbox-exec -D` flag behavior under `syscall.Exec` should be tested early in Phase 2

## Session Continuity

Last session: 2026-04-17T12:51:00.054Z
Stopped at: Completed 06-01-PLAN.md
Resume file: None
