---
phase: 03-shell-enforcement-artifacts
plan: 01
subsystem: shell-enforcement
tags: [go, text-template, embed, bash, python, shell-enforcement, artifact-generation]

# Dependency graph
requires:
  - phase: 01-go-binary-foundation
    provides: ResolvedConfig, ParseRequisites, WriteCache, policy.go, config.go
  - phase: 02-kernel-enforcement-sbpl-sandbox-exec
    provides: sbpl.go code style pattern, sbpl_test.go assertContains/assertNotContains helpers
provides:
  - GenerateEntrypoint(cfg) renders entrypoint.sh from embedded template
  - GenerateFsFilter(cfg) renders fs-filter.sh with write-command wrappers
  - GenerateUsercustomize(cfg) renders usercustomize.py with ensurepip block + open() wrapper
  - WriteShellArtifacts(cacheDir, cfg) writes all three files to cache dirs
  - ArmoredCommands []string (26 package manager names)
  - WriteCmds []string (8 write command names)
  - shellquote(s) helper for safe bash single-quoting
  - Three embedded .tmpl files under templates/
affects: [03-02, 03-03, phase-04]

# Tech tracking
tech-stack:
  added: [embed, text/template]
  patterns: [embedded-template-rendering, funcmap-shellquote, pure-generator-functions]

key-files:
  created:
    - shell.go
    - shell_test.go
    - templates/entrypoint.sh.tmpl
    - templates/fs-filter.sh.tmpl
    - templates/usercustomize.py.tmpl
  modified: []

key-decisions:
  - "Improved fs-filter prefix matching: dual case alternatives (exact|subpath) instead of bash bug-for-bug prefix glob (Open Question 1 from 03-RESEARCH.md)"
  - "text/template (not html/template) for all shell generation -- html/template auto-escapes shell-significant chars"
  - "shellquote FuncMap helper for safe single-quote escaping of paths in generated bash"
  - "usercustomize.py is a static template with no Go substitutions -- Python reads cached state files at runtime"
  - "Permissive fs-mode renders a no-op fs-filter.sh (return 0) rather than skipping the file entirely"

patterns-established:
  - "Embedded template rendering: //go:embed + text/template.ParseFS + FuncMap for shell-safe quoting"
  - "Pure generator pattern: Generate*(cfg) -> (string, error) with no I/O -- enables microsecond unit tests"
  - "WriteShellArtifacts sibling pattern: writes to cacheDir + pythonCacheDir (sandflox-python/) as sibling"

requirements-completed: [SHELL-01, SHELL-02, SHELL-03, SHELL-04, SHELL-05, SHELL-06, SHELL-07, SHELL-08]

# Metrics
duration: 5min
completed: 2026-04-17
---

# Phase 03 Plan 01: Shell Enforcement Generators Summary

**Go text/template generators for entrypoint.sh (PATH wipe + armor + breadcrumbs), fs-filter.sh (write-command wrappers with improved prefix matching), and usercustomize.py (ensurepip block + builtins.open wrapper) -- all 11 unit tests pass, zero third-party deps**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-17T00:24:53Z
- **Completed:** 2026-04-17T00:30:00Z
- **Tasks:** 2
- **Files created:** 5

## Accomplishments

- Created three Go text/template files under `templates/` that render the complete shell-tier enforcement artifacts (entrypoint.sh, fs-filter.sh, usercustomize.py)
- Implemented `shell.go` with 6 exported symbols: `GenerateEntrypoint`, `GenerateFsFilter`, `GenerateUsercustomize`, `WriteShellArtifacts`, `ArmoredCommands`, `WriteCmds` plus the `shellquote` helper
- All 11 unit tests pass covering SHELL-01 through SHELL-08, with no Phase 1/2 regression
- Improved fs-filter.sh prefix matching from bash's over-matching `<path>*` to dual-alternative `<path>|<path>/*` pattern
- Zero third-party Go dependencies maintained

## Task Commits

Each task was committed atomically:

1. **Task 1: Create embedded templates** - `439cd2f` (feat)
2. **Task 2: Implement shell.go generators + unit tests** - `26b7cb4` (feat)

## Files Created/Modified

- `templates/entrypoint.sh.tmpl` (73 lines) - Bash entrypoint template: PATH wipe, requisites loop, 26 armor functions, curl removal, Python env exports, breadcrumb unset
- `templates/fs-filter.sh.tmpl` (43 lines) - fs-filter template: _sfx_check_write_target with denied/writable/readonly case statements, 8 write-command wrappers
- `templates/usercustomize.py.tmpl` (65 lines) - Python enforcement: ensurepip stub injection + builtins.open monkey-patch
- `shell.go` (176 lines) - Generator functions, embedded templates FS, ArmoredCommands/WriteCmds constants, shellquote helper, WriteShellArtifacts writer
- `shell_test.go` (345 lines) - 11 unit tests: PathExport, ArmorFunctions, BreadcrumbUnset, PythonEnvExports, CurlRemovalGated, FsFilter_Wrappers, BlocksEnsurepip, WrapsOpen, BlockedMessagesFormat, ShellquoteEscapes, WriteAllThreeFiles

## Decisions Made

- **Improved prefix matching (Open Question 1):** Chose to improve over bash bug-for-bug compat. fs-filter.sh now emits `'/path'|'/path'/*` dual alternatives instead of `'/path'*`, preventing `/tmp-foo` from matching writable `/tmp`. Kernel tier remains the backstop for edge cases.
- **Static usercustomize.py template:** No Go-side substitutions needed -- Python reads `fs-mode.txt`, `writable-paths.txt`, `denied-paths.txt` from cache at runtime. Keeps template simple and matches bash reference exactly.
- **Permissive mode renders a no-op file:** Instead of skipping fs-filter.sh generation for permissive mode, the template renders a minimal file with `return 0`. This means the entrypoint's `if [ -f fs-filter.sh ]` guard always finds the file, but it's a no-op. Simpler than conditional generation.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Minor: unused variable in test loop caught by `go build` -- removed the redundant loop and relied on the more thorough check below it. Fixed inline.

## Known Stubs

None -- all generators produce complete artifacts with no placeholder or TODO content.

## Next Phase Readiness

- Plan 03-02 can now call `WriteShellArtifacts(cacheDir, cfg)` from `main.go` and reference `ArmoredCommands`/`WriteCmds` if needed
- Plan 03-02 will rewire `buildSandboxExecArgv` to use `-- bash --rcfile entrypoint.sh` (interactive) and `-- bash -c 'source entrypoint.sh && exec "$@"'` (non-interactive)
- Plan 03-03 will add subprocess integration tests exercising the full shell-tier enforcement chain

## Self-Check: PASSED

- All 5 created files verified present on disk
- Both task commits (439cd2f, 26b7cb4) verified in git log
- 03-01-SUMMARY.md verified present

---
*Phase: 03-shell-enforcement-artifacts*
*Completed: 2026-04-17*
