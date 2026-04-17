---
phase: 05-subcommands
verified: 2026-04-17T12:15:00Z
status: passed
score: 9/9 must-haves verified
re_verification: false
human_verification:
  - test: "Run `sandflox validate` in a project with policy.toml"
    expected: "Prints [sandflox] Policy: policy.toml (valid), profile summary, tool count, denied count to stderr. Does NOT launch sandbox-exec or flox activate."
    why_human: "Requires a live flox environment to confirm no exec is invoked."
  - test: "Run `sandflox elevate` inside an active `flox activate` session (with FLOX_ENV set, SANDFLOX_ENABLED unset)"
    expected: "Re-execs current shell under sandbox-exec without invoking flox activate. Process becomes sandbox-exec."
    why_human: "Requires an active flox session; syscall.Exec cannot be unit-tested without a real flox environment."
  - test: "Run `sandflox elevate` when already sandboxed (SANDFLOX_ENABLED=1)"
    expected: "Prints '[sandflox] Already sandboxed -- nothing to do.' and exits 0."
    why_human: "Re-entry detection requires an active sandflox session environment."
---

# Phase 5: Subcommands Verification Report

**Phase Goal:** Users can inspect policy without executing (validate), check enforcement state (status), and elevate an existing flox session into the sandbox (elevate)
**Verified:** 2026-04-17T12:15:00Z
**Status:** PASSED
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Running `sandflox validate` prints policy summary without launching a sandbox | VERIFIED | `runValidateWithExitCode` in subcommand.go:73 returns 0 after printing summary, never calls execFlox or execWithKernelEnforcement. TestValidateOutput passes. |
| 2 | Running `sandflox validate --debug` prints full path lists and SBPL rule count | VERIFIED | subcommand.go:122-124 calls `emitDiagnostics(..., true)` when `flags.Debug`. TestValidateDebugOutput asserts `[sandflox] Requisites:` and `[sandflox] sbpl:` lines. |
| 3 | Running `sandflox status` inside a sandbox reports profile, network mode, filesystem mode, tool count | VERIFIED | `runStatusInternal` in subcommand.go:175 reads config via ReadCache and prints the summary. TestStatusOutput passes with all expected lines. |
| 4 | Running `sandflox status` outside a sandbox prints an error and exits 1 | VERIFIED | subcommand.go:176-179 prints "Not in a sandflox session" and returns 1 when cacheDir is "". TestStatusNoCache passes. |
| 5 | Both `sandflox --debug validate` and `sandflox validate --debug` produce identical behavior | VERIFIED | extractSubcommand scans all positions; both produce remaining=["--debug"] which ParseFlags reads as flags.Debug=true. TestSubcommandFlagPosition passes. |
| 6 | Unknown first args route to the default exec pipeline (backward compat) | VERIFIED | extractSubcommand stops scanning at "--"; ["--", "echo", "hello"] returns ("", ["--","echo","hello"]). TestExtractSubcommand/double-dash_then_echo_hello passes. |
| 7 | `sandflox elevate` from within a flox session re-execs shell under sandbox-exec | VERIFIED | runElevateWithExitCode calls elevateExec (subcommand.go:291) which calls syscall.Exec in exec_darwin.go:211. buildElevateArgv produces 13-element argv without flox activate. |
| 8 | `sandflox elevate` inside an already-sandboxed session prints "Already sandboxed" and exits 0 | VERIFIED | checkElevatePrereqs returns ("..Already sandboxed..", 0) when SANDFLOX_ENABLED=1. TestElevateAlreadySandboxed passes. |
| 9 | `sandflox elevate` outside a flox session prints error and exits 1 | VERIFIED | checkElevatePrereqs returns ("..Not in a flox session..", 1) when FLOX_ENV="". TestElevateNoFlox passes. |

**Score:** 9/9 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `subcommand.go` | Subcommand routing, runValidate, runStatus, discoverCacheDir, runElevate | VERIFIED | 297 lines. All required functions present and substantive. |
| `subcommand_test.go` | Unit tests for routing, validate, status, elevate | VERIFIED | 461 lines. 14 test functions covering all required behaviors. |
| `cache.go` | ReadCache function (inverse of WriteCache) | VERIFIED | ReadCache at line 82. Reads config.json, unmarshals JSON, returns ResolvedConfig. |
| `main.go` | Updated main() with subcommand routing | VERIFIED | extractSubcommand called at line 26. switch on subcmd routes to validate/status/elevate/default. |
| `exec_darwin.go` | buildElevateArgv pure function, elevateExec | VERIFIED | buildElevateArgv at line 149 (13-element pure function). elevateExec at line 174. |
| `exec_other.go` | elevateExec stub for non-darwin | VERIFIED | elevateExec stub at line 28. Prints "elevate requires macOS sandbox-exec" and exits 1. |
| `exec_test.go` | TestBuildElevateArgv argv shape tests | VERIFIED | 4 tests at lines 246-339: Interactive (13-elem), NoFloxActivate, HasEntrypoint, SandboxExecParams. |
| `cache_test.go` | TestReadCacheRoundTrip, TestReadCacheMissing, TestReadCacheCorrupt | VERIFIED | All 3 present at lines 267-337. All pass. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `main.go` | `subcommand.go` | `extractSubcommand(os.Args[1:])` | WIRED | main.go:26 calls extractSubcommand; routes via switch at line 32-42. |
| `subcommand.go (runValidateWithExitCode)` | `main.go (emitDiagnostics)` | `emitDiagnostics(config, projectDir, true)` | WIRED | subcommand.go:123. emitDiagnostics is a package-level function in main.go:123. |
| `subcommand.go (runStatusInternal)` | `cache.go (ReadCache)` | `ReadCache(cacheDir)` | WIRED | subcommand.go:181. ReadCache in cache.go:82. |
| `subcommand.go (runElevate)` | `exec_darwin.go (elevateExec)` | `elevateExec(config, projectDir, entrypointPath)` | WIRED | subcommand.go:291 calls elevateExec. Platform-dispatched via build tags. |
| `exec_darwin.go (elevateExec)` | `exec_darwin.go (buildElevateArgv)` | `buildElevateArgv(...)` call | WIRED | exec_darwin.go:205 calls buildElevateArgv. Pure function, results passed to syscall.Exec. |
| `subcommand.go (runElevateWithExitCode)` | `main.go (emitDiagnostics)` | `emitDiagnostics(config, projectDir, flags.Debug)` | WIRED | subcommand.go:288 calls emitDiagnostics before elevateExec. |

### Data-Flow Trace (Level 4)

These are CLI command handlers, not data-rendering components. Data flows from policy.toml / cached config.json through resolution and is printed to stderr. No DB queries or hollow props apply.

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `subcommand.go (runValidateWithExitCode)` | `config` | `ParsePolicy` + `ResolveConfig` called on real policy.toml | Yes -- reads and parses actual file | FLOWING |
| `subcommand.go (runStatusInternal)` | `config` | `ReadCache(cacheDir)` reads actual config.json from disk | Yes -- deserializes real cached state | FLOWING |
| `subcommand.go (runElevateWithExitCode)` | `config`, `cacheDir` | ParsePolicy + ResolveConfig + FLOX_ENV_CACHE env var | Yes -- real policy + real env var | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full test suite compiles and passes | `go test ./...` | `ok sandflox` | PASS |
| Binary compiles with no errors | `go build -o /dev/null .` | (no output, exit 0) | PASS |
| extractSubcommand routes all 3 subcommands | `go test -run TestExtractSubcommand -v` | 8/8 subtests PASS | PASS |
| Validate prints policy summary | `go test -run TestValidateOutput -v` | PASS | PASS |
| Validate --debug emits SBPL + requisites | `go test -run TestValidateDebugOutput -v` | PASS | PASS |
| Status reads cached config and reports | `go test -run TestStatusOutput -v` | PASS | PASS |
| Status outside sandbox exits 1 | `go test -run TestStatusNoCache -v` | PASS | PASS |
| Elevate re-entry detection (SANDFLOX_ENABLED=1) | `go test -run TestElevateAlreadySandboxed -v` | PASS | PASS |
| Elevate no-flox detection (FLOX_ENV unset) | `go test -run TestElevateNoFlox -v` | PASS | PASS |
| buildElevateArgv argv shape (13 elements, no flox activate) | `go test -run TestBuildElevateArgv -v` | 4/4 PASS | PASS |
| ReadCache round-trips with WriteCache | `go test -run TestReadCacheRoundTrip -v` | PASS | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| CMD-01 | 05-01-PLAN.md | `sandflox validate` parses policy.toml, generates SBPL (dry-run), and reports what would be enforced without executing | SATISFIED | runValidateWithExitCode: ParsePolicy + ResolveConfig + WriteCache + WriteShellArtifacts + print summary, no exec call. TestValidateOutput verifies full output format. |
| CMD-02 | 05-01-PLAN.md | `sandflox status` reads cached enforcement state and reports active profile, blocked paths, allowed tools, network mode | SATISFIED | runStatusInternal: ReadCache + ParseRequisites + prints profile/network/filesystem/tools/denied. TestStatusOutput + TestStatusDebugOutput verify all reported fields. |
| CMD-03 | 05-02-PLAN.md | `sandflox elevate` from within a `flox activate` session re-execs current shell under sandbox-exec with generated SBPL (one-time bounce with re-entry detection) | SATISFIED | checkElevatePrereqs detects SANDFLOX_ENABLED + FLOX_ENV. elevateExec: GenerateSBPL + WriteSBPL + buildElevateArgv (no flox activate) + syscall.Exec. exec_other.go hard-errors on non-darwin. |

No orphaned requirements: REQUIREMENTS.md maps exactly CMD-01, CMD-02, CMD-03 to Phase 5 -- all accounted for.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `exec_darwin.go` | 142 | Comment says "Returns exactly 12 elements" but function returns 13 | INFO | Stale docstring only; test at exec_test.go:265 correctly asserts 13 elements and passes. No behavioral impact. |

No TODOs, FIXMEs, placeholder returns, empty handlers, or stub implementations found in any phase-05 files.

### Human Verification Required

#### 1. Live `sandflox validate` Against Real Policy

**Test:** In a project directory containing `policy.toml`, run `sandflox validate` without entering a flox session.
**Expected:** Prints `[sandflox] Policy: policy.toml (valid)`, profile/network/filesystem summary, tool count, and denied path count to stderr. No `flox activate` or `sandbox-exec` process is spawned.
**Why human:** Confirming that exec is NOT invoked requires observing the process tree (no child process appears). Unit tests verify the code path is correct, but live confirmation eliminates any platform-dispatch edge cases.

#### 2. Live `sandflox elevate` Inside Active Flox Session

**Test:** Run `flox activate` in a project with `policy.toml` and `requisites.txt`. From within that shell, run `sandflox elevate`.
**Expected:** Shell re-execs under `sandbox-exec` without launching a new `flox activate`. Process becomes `sandbox-exec`. The new shell enforces the SBPL policy. Running `sandflox status` from inside shows the active profile.
**Why human:** `syscall.Exec` replaces the process; cannot be verified without a real flox environment. The unit tests verify the argv shape and detection logic but not the actual exec.

#### 3. Live Re-Entry Detection for `sandflox elevate`

**Test:** Run `sandflox` (entering the sandboxed session) then run `sandflox elevate` from within.
**Expected:** Prints `[sandflox] Already sandboxed -- nothing to do.` and exits 0. No double-nesting occurs.
**Why human:** Requires SANDFLOX_ENABLED=1 to be set by the actual sandflox session launch, not a test env var.

### Gaps Summary

No gaps. All 9 observable truths are verified, all 8 required artifacts exist and are substantive, all 6 key links are wired, all 3 requirement IDs are satisfied, the test suite passes (28 new tests for this phase), and the binary compiles cleanly.

The only finding is a stale docstring in `exec_darwin.go:142` claiming 12 elements where the function returns 13. This does not affect behavior; the test correctly asserts 13 and passes. It is informational only.

---

_Verified: 2026-04-17T12:15:00Z_
_Verifier: Claude (gsd-verifier)_
