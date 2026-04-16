---
phase: 02-kernel-enforcement-sbpl-sandbox-exec
plan: 01
subsystem: kernel-enforcement
tags: [sbpl, sandbox-exec, generator, go, tdd]

dependency-graph:
  requires:
    - config.go (ResolvedConfig struct, ResolvePath — Phase 1)
  provides:
    - GenerateSBPL(cfg *ResolvedConfig, home string) string
    - WriteSBPL(cacheDir string, content string) (string, error)
  affects:
    - Plan 02-02 (exec_darwin.go consumes GenerateSBPL + WriteSBPL)
    - Plan 02-03 (integration tests load the same generated .sb)

tech-stack:
  added: []   # zero-deps maintained — stdlib only (strings, os, path/filepath, fmt)
  patterns:
    - "strings.Builder for SBPL string assembly (preferred over text/template per RESEARCH.md)"
    - "Function decomposition: writeSBPLHeader / writeSBPLDenied / writeSBPLFilesystem / writeSBPLNetwork — each takes *strings.Builder, mirrors bash section ordering"
    - "Unknown FsMode/NetMode values default to workspace/blocked (matches bash workspace|*) and blocked|*) fallthrough)"

key-files:
  created:
    - sbpl.go (191 lines)
    - sbpl_test.go (321 lines)
  modified:
    - .gitignore (added /sandflox binary exclusion)

decisions:
  - "GenerateSBPL is a pure function (no I/O). WriteSBPL is the only I/O entry point. Makes every rule unit-testable at millisecond speed."
  - "Helper decomposition chosen: writeSBPLHeader / writeSBPLDenied / writeSBPLFilesystem / writeSBPLNetwork — one per logical block, matches bash section headers. Keeps diff-to-bash readable for review."
  - "Flox-required overrides block is gated by len(cfg.Denied) > 0 — matches the bash `if [ -s denied-paths.txt ]` conditional at sandflox.bash:209. Without this, permissive+empty-denied configs emit unexpected allow rules."
  - "Network header comment (;; ── Network (*) ──) is emitted unconditionally for both blocked and unrestricted — matches bash echo '' + case pattern at sandflox.bash:274."

metrics:
  duration: "4 minutes"
  completed: "2026-04-16"
  tasks: 2
  files_created: 2
  files_modified: 1
  test_count_added: 10  # top-level functions; ~16 including subtests
  test_count_total: 58  # all go tests passing (Phase 1 regression + new SBPL tests)
---

# Phase 2 Plan 01: SBPL Generator Summary

**One-liner:** Pure-function SBPL emitter (`GenerateSBPL` + `WriteSBPL`) mirrors bash `_sfx_generate_sbpl()` rule-by-rule and produces byte-identical output to `.flox/cache/sandflox/sandflox.sb` for the current policy.

## Context

Phase 2 delivers kernel-level enforcement via Apple's sandbox-exec. The
bash implementation at `sandflox.bash:191-291` generates SBPL profiles
from the resolved policy; Plan 02-01 ports that generator to Go as a
pure, table-testable function. No exec wiring — that's Plan 02-02.

Per D-01 the bash output is the canonical spec. Per D-08 testing is
rule-shape assertion at the string level (no actual sandbox-exec
invocation — that's Plan 02-03).

## What Changed

### Created

| File | Lines | Purpose |
|------|-------|---------|
| `sbpl.go` | 191 | `GenerateSBPL` pure function + `WriteSBPL` cache writer, decomposed into 4 section helpers |
| `sbpl_test.go` | 321 | 10 table-driven test functions covering every fs_mode × net_mode × denied/localhost combination |

### Modified

| File | Why |
|------|-----|
| `.gitignore` | Exclude the compiled `./sandflox` binary that `go build` drops into the project root |

### Untouched (by design)

- `main.go`, `config.go`, `cache.go`, `policy.go`, `cli.go` — Plan 02-02 wires the SBPL generator into exec; this plan only builds the generator.

## Task Summary

| Task | Status | Commit | Files | Key Change |
|------|--------|--------|-------|-----------|
| 1. Implement GenerateSBPL + WriteSBPL | Done | `0162025` | sbpl.go, .gitignore | Pure generator + file writer; 4-helper decomposition |
| 2. Implement table-driven unit tests | Done | `9a342c7` | sbpl_test.go | 10 test functions, stdlib-only assertions |

Total plan duration: ~4 minutes.

## Helper Function Decomposition

Followed the pattern sketched in `02-RESEARCH.md` Pattern 1, one helper
per logical section of the bash `_sfx_generate_sbpl()` switch:

```
GenerateSBPL(cfg, home) string
├── writeSBPLHeader(sb)                                       // (version 1), (allow default)
├── writeSBPLDenied(sb, cfg.Denied, home)                     // deny pairs + flox overrides (both gated by len(Denied)>0)
├── writeSBPLFilesystem(sb, cfg.FsMode, cfg.ReadOnly, home)   // switch: permissive / strict / workspace (default)
└── writeSBPLNetwork(sb, cfg.NetMode, cfg.AllowLocalhost)     // switch: unrestricted / blocked (default)
```

Rationale: each helper takes `*strings.Builder` as the first arg (Go
convention for write-through) and has a single documentation block at
the top showing which bash lines it mirrors. This keeps code review
against `sandflox.bash:191-291` trivial.

## Byte-Compatibility Verification

During execution, a one-off test (subsequently removed, see Deviations)
fed a `ResolvedConfig` matching the current `policy.toml` + home
`/Users/jhogan` into `GenerateSBPL` and compared against the live
`.flox/cache/sandflox/sandflox.sb`:

```
Go output byte-matches canonical .sb (1724 bytes)
```

The Go generator is a drop-in replacement for bash for this policy. No
divergences found — every rule, blank line, comment, and quote
character matches.

## Validation Results

**Quick sampling (per task commit):**
```
$ go test -run '^TestGenerateSBPL|^TestWriteSBPL' -short ./...
PASS — 10 top-level tests (~16 cases with subtests) in 0.01s
```

**Full suite (per wave merge):**
```
$ go test -count=1 ./...
ok  sandflox  0.300s   — 58 tests pass, 0 fail
```

Phase 1 regression: zero regressions introduced.

**Per-task verification map (from 02-VALIDATION.md):**

| Task ID | Requirement | Command | Status |
|---------|------------|---------|--------|
| 02-01-01 | KERN-01, KERN-02 | `go test -run TestGenerateSBPL_AllowDefaultBaseline` | ✅ green |
| 02-01-02 | KERN-01 | `go test -run TestGenerateSBPL_WorkspaceBlocked` | ✅ green |
| 02-01-03 | KERN-03 | `go test -run TestGenerateSBPL_DeniedPaths` | ✅ green |
| 02-01-04 | KERN-07 | `go test -run TestGenerateSBPL_LocalhostAllowed` | ✅ green |
| 02-01-05 | KERN-08 | `go test -run TestGenerateSBPL_UnixSocketAlwaysAllowed` | ✅ green |
| 02-01-06 | KERN-02 | `go test -run TestGenerateSBPL_ParameterSubstitution` | ✅ green |
| 02-01-07 | KERN-01, KERN-02 | `go test -run TestWriteSBPL` | ✅ green |

All 7 mapped requirements green.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Gitignore the compiled `./sandflox` binary**
- **Found during:** Task 1 (after first `go build`)
- **Issue:** `go build ./...` emits a 3.0 MB `./sandflox` Mach-O binary
  into the project root. It appeared as an untracked file in
  `git status`, risking an accidental commit of a platform-specific
  build artifact.
- **Fix:** Added `/sandflox` to `.gitignore` under a new
  "Compiled Go binary" comment. Verified `git status` no longer lists it.
- **Rationale:** This is clearly generated output, not source. Per
  task_commit_protocol ("For any new untracked files: … add to
  `.gitignore` if generated/runtime output").
- **Files modified:** `.gitignore`
- **Commit:** `0162025` (bundled with Task 1)

### Intentional Structural Differences from Bash

None. The bash structure is reproduced rule-for-rule, blank-line for
blank-line, comment-for-comment. Byte diff against the live
`.flox/cache/sandflox/sandflox.sb` confirmed exact equivalence.

### Temporary Files Cleaned Up

**1. `sbpl_diff_check_test.go`** — a one-off test that compared
`GenerateSBPL` output to the live canonical `.sb`. Created to verify
byte-compatibility during Task 1 implementation, then removed once the
assertion passed (1724 bytes, byte-identical). Not committed. Lesson:
this verification was valuable but belongs in Plan 02-03 integration
tests (the plan already has D-09 integration tests scheduled there), not
in the unit-test layer which is machine-portable.

## Known Stubs

None. Both `GenerateSBPL` and `WriteSBPL` are fully implemented and
wired into the test assertions. The Plan 02-02 consumer (exec_darwin.go)
is the only caller yet to exist, but that is expected scope — that's
Plan 02-02's job, not a stub here.

## Deferred Issues

None.

## Authentication Gates

None.

## Next Plan Dependencies

Plan 02-02 can now:
- `import "sandflox"` — already `package main`, direct call
- Call `GenerateSBPL(cfg, home)` to produce the profile string
- Call `WriteSBPL(cacheDir, content)` to write it to disk
- Feed the returned path to `sandbox-exec -f <path>` via `syscall.Exec`

The parameter references `(param "PROJECT")` and `(param "FLOX_CACHE")`
in the generated SBPL require matching `-D` flags at exec time —
Plan 02-02 must pass `-D PROJECT=<projectDir> -D FLOX_CACHE=<cache>`
when invoking `sandbox-exec` (see Pitfall 5 in 02-RESEARCH.md).

## Self-Check: PASSED

Files verified:
- FOUND: `/Users/jhogan/sandflox/sbpl.go`
- FOUND: `/Users/jhogan/sandflox/sbpl_test.go`
- FOUND: `/Users/jhogan/sandflox/.planning/phases/02-kernel-enforcement-sbpl-sandbox-exec/02-01-SUMMARY.md`

Commits verified:
- FOUND: `0162025` — feat(02-01): add SBPL generator (GenerateSBPL + WriteSBPL)
- FOUND: `9a342c7` — test(02-01): add table-driven unit tests for SBPL generator

Tests verified: `go test -count=1 ./...` → 58 pass, 0 fail.
Build verified: `go build ./...` → zero errors, zero external deps.
