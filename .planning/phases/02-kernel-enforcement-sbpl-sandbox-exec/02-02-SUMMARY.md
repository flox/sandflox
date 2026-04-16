---
phase: 02-kernel-enforcement-sbpl-sandbox-exec
plan: 02
subsystem: kernel-enforcement
tags: [sandbox-exec, syscall-exec, darwin, build-tags, go, tdd]

dependency-graph:
  requires:
    - sbpl.go (GenerateSBPL, WriteSBPL -- Plan 02-01)
    - config.go (ResolvedConfig -- Phase 1)
    - main.go (execFlox, stderr package var -- Phase 1)
  provides:
    - execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, userArgs []string)  # darwin + stub
    - buildSandboxExecArgv(sbplPath, projectDir, home, floxCachePath, floxAbsPath string, userArgs []string) []string  # pure, testable
  affects:
    - Plan 02-03 (integration tests exercise the full wired exec path end-to-end)

tech-stack:
  added: []  # stdlib only -- fmt, os, os/exec, path/filepath, syscall, strings, reflect, testing
  patterns:
    - "Platform split via build tags: //go:build darwin vs //go:build !darwin"
    - "Pure argv-builder helper (buildSandboxExecArgv) decoupled from I/O for unit testing"
    - "syscall.Exec for PID-preserving process replacement (KERN-04); never returns on success"
    - "Absolute flox path resolved via exec.LookPath before argv build (Pitfall 6)"
    - "Deviation Rule 3 applied: Phase 1 main_test.go call sites updated to match new emitDiagnostics signature"

key-files:
  created:
    - exec_darwin.go (122 lines)
    - exec_other.go (20 lines)
    - exec_test.go (188 lines)
  modified:
    - main.go (wired execWithKernelEnforcement; added strings import; emitDiagnostics signature + SBPL diagnostic)
    - main_test.go (updated 3 emitDiagnostics call sites; added D-07 sbpl diagnostic assertion)

decisions:
  - "argv[0] is the literal string \"sandbox-exec\" (not the absolute path). Unix execve() convention -- argv[0] is conventionally basename even when the exec path arg is absolute. Matches bash `exec sandbox-exec ...`."
  - "buildSandboxExecArgv is pure (no I/O, no syscalls). This pays off in exec_test.go: 5 argv-shape tests run in microseconds without spawning sandbox-exec."
  - "exec_test.go has //go:build darwin because buildSandboxExecArgv only exists in exec_darwin.go. On Linux, `go test` runs only the platform-shared tests (still passes thanks to exec_other.go stub)."
  - "D-07 SBPL diagnostic lives in emitDiagnostics (not execWithKernelEnforcement). Reason: diagnostics run before exec on all platforms; the actual SBPL write happens in exec_darwin.go. This keeps the --debug line visible even on non-darwin platforms (where the file won't actually be written but the diagnostic is still useful for dry-run inspection)."
  - "emitDiagnostics signature changed from (cfg, debug) to (cfg, projectDir, debug) to compute the sbpl path for the D-07 diagnostic. Phase 1 main_test.go was updated to match (Rule 3 -- blocking issue caused by this task's changes)."

metrics:
  duration: "3 minutes"
  completed: "2026-04-16"
  tasks: 3
  files_created: 3
  files_modified: 2
  test_count_added: 5  # buildSandboxExecArgv tests (all darwin-gated)
  test_count_total: 63  # all go tests passing (Phase 1 + Plan 02-01 + Plan 02-02)
---

# Phase 2 Plan 02: Sandbox-Exec Wrapper Summary

**One-liner:** Wire the Plan 02-01 SBPL generator into the main exec path via a
build-tagged `execWithKernelEnforcement` that calls `syscall.Exec` on
`sandbox-exec -f <sb> -D PROJECT=... -D HOME=... -D FLOX_CACHE=... flox activate`;
adds `--debug` SBPL diagnostic per D-07.

## Context

Plan 02-01 delivered the pure SBPL generator. This plan connects it to the
main exec path so `./sandflox` and `./sandflox -- CMD` actually run under
`sandbox-exec`. The core invariants -- PID-preserving `syscall.Exec`,
absolute flox path in argv, exact mirror of `sandflox.bash:476-480` argv
shape -- are enforced by unit tests on the pure `buildSandboxExecArgv`
helper; end-to-end subprocess verification is Plan 02-03's scope.

Key pitfalls avoided (from 02-RESEARCH.md):
- Pitfall 1: code after `syscall.Exec` always prints error + `os.Exit(1)`
- Pitfall 5: every `(param "KEY")` SBPL reference has a matching `-D KEY=` flag
- Pitfall 6: flox path is resolved absolute via `exec.LookPath` before argv build

## What Changed

### Created

| File | Lines | Purpose |
|------|-------|---------|
| `exec_darwin.go` | 122 | `//go:build darwin` -- `execWithKernelEnforcement` + pure `buildSandboxExecArgv` helper |
| `exec_other.go` | 20 | `//go:build !darwin` -- stub that warns + falls through to `execFlox` |
| `exec_test.go` | 188 | `//go:build darwin` -- 5 unit tests on `buildSandboxExecArgv` argv shape |

### Modified

| File | Why |
|------|-----|
| `main.go` | (1) added `"strings"` import; (2) wired `execWithKernelEnforcement(config, projectDir, userArgs)` replacing the terminal `execFlox(userArgs)` call; (3) `emitDiagnostics` signature grew `projectDir string` and now emits `[sandflox] sbpl: {path} ({N} rules)` under `--debug` (D-07) |
| `main_test.go` | Phase 1 tests call `emitDiagnostics` directly; their 3 call sites were updated to pass the new `projectDir` parameter, plus a new assertion verifies the D-07 SBPL line appears in `--debug` output and is absent in non-debug output |

### Untouched (by design)

- `sbpl.go`, `sbpl_test.go` -- Plan 02-01's deliverables; consumed, not modified
- `config.go`, `cache.go`, `policy.go`, `cli.go` -- unrelated to the exec wiring
- `sandflox.bash` -- `git diff --stat sandflox.bash` reports zero changes (verified post-commit)
- `.flox/cache/sandflox/sandflox.sb` -- regenerated by the Go binary on invocation, content byte-identical to bash for current policy

## Task Summary

| Task | Status | Commit | Files | Key Change |
|------|--------|--------|-------|------------|
| 1. Create build-tagged exec wrappers | Done | `73b12ed` | exec_darwin.go, exec_other.go | Darwin impl + non-darwin stub; pure buildSandboxExecArgv helper |
| 2. Unit tests for argv shape | Done | `c2f790c` | exec_test.go | 5 tests covering interactive, -- CMD mode, absolute flox path, no stray `--`, `-c` inside userArgs lands after `--` |
| 3. Wire into main + --debug SBPL diagnostic | Done | `d75c3e0` | main.go, main_test.go | Route through execWithKernelEnforcement; D-07 diagnostic; update Phase 1 tests for new signature |

Total plan duration: ~3 minutes (execution) + smoke verification.

## Argv Shape Decisions

### Interactive mode (`./sandflox`)

userArgs is empty -- argv has exactly 11 elements:

```
[ 0] "sandbox-exec"                          -- basename, not path (Unix convention)
[ 1] "-f"
[ 2] "/<project>/.flox/cache/sandflox/sandflox.sb"
[ 3] "-D"
[ 4] "PROJECT=/<project>"
[ 5] "-D"
[ 6] "HOME=/<home>"
[ 7] "-D"
[ 8] "FLOX_CACHE=/<home>/.cache/flox"
[ 9] "/<absolute>/flox"                      -- ABSOLUTE, not "flox" (Pitfall 6)
[10] "activate"
```

### Wrap-command mode (`./sandflox -- CMD args...`)

userArgs is non-empty -- argv grows to 11 + 1 (`--`) + len(userArgs) elements.
Example for `./sandflox -- echo hello`:

```
[ 0..10 ] same as interactive mode
[11] "--"
[12] "echo"
[13] "hello"
```

The `--` boundary ensures that dash-prefixed userArgs (e.g. `bash -c "..."`)
are positional after the boundary and NOT parsed as sandbox-exec's own flags.
Test `TestBuildSandboxExecArgs_HandlesUserArgsWithDashes` explicitly asserts
that any `-c` in userArgs has a higher index than the `--`.

## Build Matrix Confirmation

| Command | Result |
|---------|--------|
| `go build ./...` (darwin, native) | zero errors |
| `GOOS=linux go build ./...` | zero errors -- exec_other.go takes effect |
| `go test -count=1 ./...` (darwin) | 63 tests pass, 0 fail |
| `go test -run TestBuildSandboxExecArgs -v` | 5 PASS lines |
| `git diff --stat sandflox.bash` | empty (bash reference untouched) |

The `GOOS=linux` cross-build check is the key regression guard: if either
build tag were wrong (missing, misspelled, or missing blank line before
`package main`), one of the two GOOS targets would fail.

## Manual Smoke Test

With the repo's own `policy.toml`, `sandbox-exec` in PATH, and `flox` in PATH:

```
$ go build -o /tmp/sandflox-phase2 .
$ /tmp/sandflox-phase2 --debug -- /usr/bin/true 2>&1 | grep -E "\[sandflox\] (Profile|Kernel enforcement|sbpl):"
[sandflox] Profile: default | Network: blocked | Filesystem: workspace
[sandflox] sbpl: /Users/jhogan/sandflox/.flox/cache/sandflox/sandflox.sb (24 rules)
[sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)
```

Three diagnostic lines observed in the expected order. Rule count `24`
matches the hand-count of `(deny ` + `(allow ` prefixes in the generated
`.sb` for the current policy. The binary successfully handed off to
`sandbox-exec` and ran `/usr/bin/true` under the generated profile.

## Validation Results

**Per-task verification map (from 02-VALIDATION.md):**

| Task ID | Requirement | Command | Status |
|---------|-------------|---------|--------|
| 02-02-01 | KERN-04 | `go test -run TestBuildSandboxExecArgs ./...` | green (5/5 PASS) |

The KERN-04/KERN-05/KERN-06 integration entries (02-02-02, 02-02-03)
are explicitly Plan 02-03's scope per the plan's "DO NOT write integration
tests that actually invoke sandbox-exec" constraint.

**Full suite (after wave merge):**

```
$ go test -count=1 ./...
ok  sandflox  0.194s  -- 63 tests pass, 0 fail
```

Phase 1 regression: zero regressions introduced. The only Phase 1 tests
that required updating were the three direct callers of `emitDiagnostics`
in `main_test.go` -- tracked below as a Rule 3 deviation.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 -- Blocking] main_test.go call sites broken by emitDiagnostics signature change**

- **Found during:** Task 3 verification (after editing main.go)
- **Issue:** Plan 02-02 Task 3 changes `emitDiagnostics` signature from
  `(cfg, debug)` to `(cfg, projectDir, debug)` to enable the D-07 SBPL
  diagnostic. Phase 1 left three direct callers in `main_test.go`:
  `TestDiagnosticsBasicFormat`, `TestDiagnosticsDebugOutput`,
  `TestDiagnosticsMinimalProfile`. Build failed:
  `not enough arguments in call to emitDiagnostics`.
- **Fix:** Added `"/test/project"` as the new `projectDir` argument at all
  three call sites. Additionally strengthened `TestDiagnosticsDebugOutput`
  to positively assert the D-07 sbpl diagnostic line appears, and
  `TestDiagnosticsBasicFormat` to negatively assert it does NOT appear
  when debug is false.
- **Rationale:** This is a blocking issue directly caused by the task's
  own signature change -- the scope boundary rule applies (fixing the
  task's own fallout, not pre-existing unrelated failures).
- **Files modified:** `main_test.go` (3 call sites + 2 new assertions)
- **Commit:** `d75c3e0` (bundled with Task 3)

### Intentional Structural Differences

None. The implementation follows the plan's Action blocks step-by-step.
The 11 verbatim steps in Task 1's behavior block for
`execWithKernelEnforcement` map 1:1 to the code.

### Temporary Files Cleaned Up

`/tmp/sandflox-phase2` binary from the smoke test -- removed post-verification.

## Known Stubs

None. Every new function is fully implemented and exercised by tests or
by the manual smoke test. The only deliberate "stub-like" element is
`exec_other.go`, but it is a real implementation of its contract
(warn + fall through) -- not a placeholder.

## Deferred Issues

None. Everything in scope was implemented or explicitly routed to Plan 02-03
(integration tests that invoke real sandbox-exec).

## Authentication Gates

None.

## Open Questions for Plan 02-03

Per the plan's output specification:

1. **Does `syscall.Exec(sandbox-exec)` actually replace the PID?** The bash
   wrapper is proven to do so (`exec sandbox-exec ...`). The Go version uses
   `syscall.Exec`, which is Unix `execve()` -- identical kernel-level
   semantics. Plan 02-03 should confirm via `pgrep -lf sandflox` after
   launching `./sandflox -- sleep 30`: expect zero sandflox ancestor, only
   sandbox-exec in the process tree.

2. **Does the SBPL profile actually take effect when invoked via Go vs bash?**
   The argv shape is byte-identical to the bash invocation (verified by
   inspection and `TestBuildSandboxExecArgs_Interactive`). The `.sb` file is
   byte-identical for the same inputs (verified in Plan 02-01). So kernel
   behavior must be identical -- but Plan 02-03 should prove this
   empirically with a write-outside-workspace test and a network-blocked
   test using the real `sandbox-exec` subprocess pattern (D-09).

3. **Does `--debug` show the SBPL rule count consistently across profiles?**
   Smoke test confirmed 24 rules for the default profile in this repo.
   Plan 02-03 should spot-check `./sandflox --profile minimal --debug --`
   and `./sandflox --profile full --debug --` rule counts differ
   appropriately.

## Next Plan Dependencies

Plan 02-03 can now:

- Spawn `sandbox-exec -f <generated-sb>` via `exec.CommandContext` and verify
  kernel-level enforcement (write-blocking, network-blocking, denied-path-blocking)
- Use `buildSandboxExecArgv` as a reference for constructing test invocations
  (the argv shape is now a stable contract tested at the unit level)
- Add `//go:build darwin && integration` tests that launch sandflox as a
  subprocess and inspect its stderr for the three expected diagnostic lines
- Rely on `exec_other.go` to keep `go test ./...` green on non-darwin CI
  runners even though integration tests need darwin

## Self-Check: PASSED

Files verified:
- FOUND: `/Users/jhogan/sandflox/exec_darwin.go`
- FOUND: `/Users/jhogan/sandflox/exec_other.go`
- FOUND: `/Users/jhogan/sandflox/exec_test.go`
- FOUND: `/Users/jhogan/sandflox/main.go` (modified)
- FOUND: `/Users/jhogan/sandflox/main_test.go` (modified)
- FOUND: `/Users/jhogan/sandflox/.planning/phases/02-kernel-enforcement-sbpl-sandbox-exec/02-02-SUMMARY.md`

Commits verified:
- FOUND: `73b12ed` -- feat(02-02): add sandbox-exec wrapper with build-tagged execWithKernelEnforcement
- FOUND: `c2f790c` -- test(02-02): add buildSandboxExecArgv unit tests
- FOUND: `d75c3e0` -- feat(02-02): wire execWithKernelEnforcement into main and add --debug SBPL diagnostic

Tests verified: `go test -count=1 ./...` -> 63 pass, 0 fail.
Builds verified: `go build ./...` and `GOOS=linux go build ./...` both zero errors.
Smoke verified: `./sandflox --debug -- /usr/bin/true` emits all three expected diagnostic lines.
