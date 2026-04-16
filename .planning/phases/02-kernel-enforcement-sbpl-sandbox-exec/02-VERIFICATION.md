---
phase: 02-kernel-enforcement-sbpl-sandbox-exec
verified: 2026-04-16T00:00:00Z
status: passed
score: 5/5 must-haves verified
re_verification: false
human_verification:
  - test: "Interactive sandboxed shell drops agent to a sandboxed prompt (KERN-05)"
    expected: "Running ./sandflox with no args opens an interactive shell; touch /etc/sandflox-test fails with 'Operation not permitted'; touch in project dir succeeds"
    why_human: "TTY is required for interactive shell sessions; go test cannot drive a real interactive shell session"
  - test: "Process tree shows clean exec replacement — no sandflox parent (KERN-04 success criterion #5)"
    expected: "pgrep -lf sandflox returns empty; pgrep -lf sandbox-exec shows the process; pgrep -lf 'sleep 30' shows the child"
    why_human: "Requires a second terminal window and live process-tree inspection while sandflox is running"
  - test: "--debug output format human-readable with 3 diagnostic lines (D-07)"
    expected: "Running ./sandflox --debug -- /usr/bin/true prints [sandflox] Profile: ..., [sandflox] sbpl: .../sandflox.sb (N rules) with N >= 5, [sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)"
    why_human: "Smoke test was confirmed in Plan 02-02 execution, but no automated re-run was captured in Plan 02-03 due to user decision to skip Task 2"
---

# Phase 2: Kernel Enforcement (SBPL + sandbox-exec) Verification Report

**Phase Goal:** sandflox wraps flox activate under sandbox-exec with a generated SBPL profile that enforces filesystem modes, network modes, and denied paths at the kernel level
**Verified:** 2026-04-16
**Status:** passed
**Re-verification:** No — initial verification

## Context Note: Manual Verification Deviation

Per the phase brief and Plan 02-03-SUMMARY.md: Task 2 (manual verification checkpoint) was skipped by explicit user decision. Plan 02-02 smoke test (`/tmp/sandflox-phase2 --debug -- /usr/bin/true`) confirmed the 3 expected diagnostic lines. Four of 5 integration tests passed with real sandbox-exec subprocesses; `TestBuiltBinaryWrapsCommand` passed (not skipped, confirmed in this verification run with flox in PATH). KERN-04, KERN-05, and D-07 are verified by automated tests and code-level inspection; the TTY and process-tree checks listed under human_verification above are the only items not auto-confirmed.

---

## Goal Achievement

### Observable Truths (from ROADMAP.md Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `sandflox` (no args) drops into interactive sandboxed shell where writes outside workspace are kernel-blocked | VERIFIED (auto + human-needed) | `TestSandboxBlocksWrite` PASS; `execWithKernelEnforcement` uses `syscall.Exec`; interactive TTY path not re-run post-02-02 smoke |
| 2 | `sandflox -- echo hello` executes under sandbox-exec and exits cleanly | VERIFIED | `TestBuiltBinaryWrapsCommand` PASS — built real binary, ran `-- /bin/echo hello`, got exit 0 with "hello" in output |
| 3 | With `network.mode = "blocked"`, network requests fail at kernel level; localhost connections succeed when `allow-localhost = true` | VERIFIED | `TestSandboxAllowsLocalhost/with_localhost` PASS; `TestSandboxAllowsLocalhost/without_localhost` PASS |
| 4 | Denied paths from policy.toml are inaccessible (read or write) from within the sandbox | VERIFIED | `TestSandboxBlocksDeniedPath` PASS — `/bin/cat` on a denied-path file returns non-zero with "Operation not permitted" or "Permission denied" |
| 5 | Process tree shows clean exec replacement (no intermediate sandflox parent process) | HUMAN-NEEDED (code-verified) | `execWithKernelEnforcement` uses `syscall.Exec` (line 119 of exec_darwin.go) which is Unix `execve()` — correct PID-replacing semantics; live pgrep inspection not performed |

**Score:** 4/5 truths fully automated-verified + 1 code-verified (KERN-04 process tree); 3 items flagged for human confirmation

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `sbpl.go` | GenerateSBPL pure function + WriteSBPL cache writer; >= 120 lines | VERIFIED | 191 lines; exports `GenerateSBPL(cfg *ResolvedConfig, home string) string` and `WriteSBPL(cacheDir string, content string) (string, error)`; stdlib only |
| `sbpl_test.go` | Table-driven unit tests for all fs/net mode combinations; >= 200 lines | VERIFIED | 321 lines; 10 test functions, all PASS |
| `exec_darwin.go` | Darwin-specific `execWithKernelEnforcement` using `syscall.Exec`; >= 80 lines | VERIFIED | 122 lines; `//go:build darwin`; `execWithKernelEnforcement` + `buildSandboxExecArgv`; 1 `syscall.Exec` call |
| `exec_other.go` | Non-darwin stub that warns and falls through; >= 15 lines | VERIFIED | 20 lines; `//go:build !darwin`; warns + calls `execFlox(userArgs)` |
| `exec_test.go` | Unit tests for buildSandboxExecArgv argv shape; >= 80 lines | VERIFIED | 188 lines; 5 tests, all PASS |
| `exec_integration_test.go` | Integration tests with real sandbox-exec subprocess; >= 200 lines | VERIFIED | 392 lines; `//go:build darwin && integration`; 5 tests, all PASS |
| `main.go` | Wires `execWithKernelEnforcement`; D-07 SBPL diagnostic | VERIFIED | line 64: `execWithKernelEnforcement(config, projectDir, userArgs)`; line 111: `GenerateSBPL(config, home)` for rule count; `[sandflox] sbpl:` format present |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `sbpl.go` | `config.go` | consumes `cfg.FsMode`, `cfg.NetMode`, `cfg.AllowLocalhost`, `cfg.ReadOnly`, `cfg.Denied` | WIRED | `GenerateSBPL(cfg *ResolvedConfig, home string)` — all ResolvedConfig fields consumed |
| `sbpl_test.go` | `sbpl.go` | calls `GenerateSBPL(` and `WriteSBPL(` | WIRED | 10 test functions directly invoke both exported functions |
| `main.go` | `exec_darwin.go` / `exec_other.go` | calls `execWithKernelEnforcement(config, projectDir, userArgs)` after WriteCache | WIRED | Line 64 of main.go; function resolves to darwin or non-darwin via build tags |
| `exec_darwin.go` | `sbpl.go` | calls `GenerateSBPL(` and `WriteSBPL(` | WIRED | Lines 93 and 95 of exec_darwin.go |
| `exec_darwin.go` | `sandbox-exec` binary | `syscall.Exec` replaces process with `/usr/bin/sandbox-exec` | WIRED | Line 119: `execErr := syscall.Exec(sbxPath, argv, os.Environ())`; `sbxPath` resolved via `exec.LookPath("sandbox-exec")` |
| `exec_integration_test.go` | `sbpl.go` | calls `GenerateSBPL(` + `WriteSBPL(` to produce profiles | WIRED | Lines 7 (comment) and 55 (`sbpl := GenerateSBPL(cfg, home)`) |
| `exec_integration_test.go` | `sandbox-exec` binary | `exec.CommandContext(ctx, "sandbox-exec", ...)` spawns subprocess | WIRED | Lines 84 and 383 |

---

### Data-Flow Trace (Level 4)

Phase 2 produces no UI components or pages rendering dynamic data — artifacts are a binary, test files, and a kernel-enforcement exec path. Level 4 data-flow trace is not applicable for these artifact types. The relevant "data flow" is the policy-to-SBPL pipeline, which is verified by unit tests (sbpl_test.go) and integration tests (exec_integration_test.go).

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Zero external Go dependencies | `go list -m all` | `sandflox` (only module) | PASS |
| Darwin build succeeds | `go build ./...` | exit 0 | PASS |
| Linux cross-build succeeds | `GOOS=linux go build ./...` | exit 0 | PASS |
| Unit test suite | `go test -count=1 ./...` | `ok sandflox 0.200s` | PASS |
| Integration tests (real sandbox-exec) | `go test -tags integration -run "TestSandbox\|TestBuiltBinary" ./...` | All 5 PASS (including TestBuiltBinaryWrapsCommand) | PASS |
| SBPL unit tests (10 functions) | `go test -run "TestGenerateSBPL\|TestWriteSBPL" ./...` | 10 PASS | PASS |
| buildSandboxExecArgv tests (5 functions) | `go test -run TestBuildSandboxExecArgs ./...` | 5 PASS | PASS |
| bash reference file untouched | `git diff --stat sandflox.bash` | empty (no changes) | PASS |

---

### Requirements Coverage

All 8 KERN-XX requirements are assigned to Phase 2 in REQUIREMENTS.md. All 3 plans (02-01, 02-02, 02-03) collectively claim them.

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| KERN-01 | 02-01, 02-03 | Generate SBPL profiles from resolved policy (fs modes, network modes, denied paths) | SATISFIED | `GenerateSBPL` produces correct rules for all combinations; `TestGenerateSBPL_WorkspaceBlocked`, `TestGenerateSBPL_StrictMode`, `TestGenerateSBPL_PermissiveMode`, `TestGenerateSBPL_DeniedPaths` all PASS; `TestSandboxBlocksWrite` and `TestSandboxBlocksDeniedPath` empirically confirm kernel enforcement |
| KERN-02 | 02-01, 02-03 | Use `(allow default)` SBPL baseline with selective denials | SATISFIED | `writeSBPLHeader` always emits `(allow default)`; `TestGenerateSBPL_AllowDefaultBaseline` PASS; `TestGenerateSBPL_ParameterSubstitution` confirms `(param "PROJECT")` and `(param "FLOX_CACHE")` present |
| KERN-03 | 02-01, 02-03 | Resolve paths to absolute; use `/private/tmp` instead of `/tmp` | SATISFIED | `sbpl.go` emits `/private/tmp` and `/private/var/folders` literally (lines 125-126, 138-139); path canonicalization done by Phase 1 `ResolvePath` which `go test -run TestResolvePathTmpDarwin` confirms |
| KERN-04 | 02-02, 02-03 | Wrap `flox activate` under `sandbox-exec` using `syscall.Exec` for clean process replacement | SATISFIED (code-verified) | `exec_darwin.go` line 119: `syscall.Exec(sbxPath, argv, os.Environ())`; `TestBuildSandboxExecArgs_Interactive` validates argv shape; `TestBuiltBinaryWrapsCommand` PASS (full pipeline); process-tree pgrep not run (human-needed) |
| KERN-05 | 02-02, 02-03 | `sandflox` (no args) launches interactive sandboxed shell | SATISFIED (code-verified) | `buildSandboxExecArgv` with empty `userArgs` produces 11-element argv ending in `activate` (no `--`); `TestBuildSandboxExecArgs_NoUserArgsDoesNotEmitDoubleDash` PASS; interactive TTY not re-run post-02-02 smoke (human-needed) |
| KERN-06 | 02-02, 02-03 | `sandflox -- CMD` wraps arbitrary commands | SATISFIED | `buildSandboxExecArgv` appends `--` then userArgs when len > 0; `TestBuildSandboxExecArgs_WithUserCommand` PASS; `TestBuiltBinaryWrapsCommand` runs `-- /bin/echo hello` end-to-end and gets "hello" output with exit 0 |
| KERN-07 | 02-01, 02-03 | Allow localhost when `network.allow-localhost = true` | SATISFIED | `writeSBPLNetwork` emits `(allow network* (remote ip "localhost:*"))` only when `allowLocalhost=true`; `TestGenerateSBPL_LocalhostAllowed` PASS; `TestSandboxAllowsLocalhost/with_localhost` PASS; `TestSandboxAllowsLocalhost/without_localhost` PASS |
| KERN-08 | 02-01, 02-03 | Allow Unix socket communication (Nix daemon) when network is blocked | SATISFIED | `writeSBPLNetwork` always emits `(allow network* (remote unix-socket))` in blocked mode; `TestGenerateSBPL_UnixSocketAlwaysAllowed` PASS; `TestSandboxAllowsUnixSocket` PASS — unix socket bind under `blocked+AllowLocalhost=false` succeeds |

**No orphaned requirements:** REQUIREMENTS.md maps exactly KERN-01 through KERN-08 to Phase 2. All 8 are accounted for across the 3 plans.

---

### Anti-Patterns Found

Scanned all phase 2 source files: `sbpl.go`, `sbpl_test.go`, `exec_darwin.go`, `exec_other.go`, `exec_test.go`, `exec_integration_test.go`, and modified `main.go`.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | No TODOs, FIXMEs, placeholders, or empty returns found | — | — |
| `exec_other.go` | 15-19 | `_ = cfg; _ = projectDir` — unused parameter suppression | Info | Intentional stub pattern per plan spec; correct behavior for non-darwin platforms |

**No blockers. No warnings.** The `exec_other.go` stub is a complete implementation of its contract (warn + fall through), not a placeholder.

---

### Human Verification Required

Three items cannot be verified programmatically:

#### 1. Interactive Sandboxed Shell (KERN-05, ROADMAP success criterion #1)

**Test:** From `/Users/jhogan/sandflox`, run `./sandflox` (no args). At the sandbox prompt, run `touch /etc/sandflox-test-$$` and `touch ./test-write-ok`, then `rm ./test-write-ok; exit`.
**Expected:** `touch /etc/...` fails with "Operation not permitted"; `touch ./test-write-ok` succeeds; the shell prompt is functional.
**Why human:** Requires interactive TTY; cannot be driven by `go test`.

#### 2. Process Tree Shows Clean Exec Replacement (KERN-04 criterion #5)

**Test:** In terminal 1, run `./sandflox -- /bin/sleep 30`. In terminal 2, run `pgrep -lf sandflox`, `pgrep -lf sandbox-exec`, `pgrep -lf "sleep 30"`. Then Ctrl+C in terminal 1.
**Expected:** `pgrep -lf sandflox` returns empty (no sandflox process); `pgrep -lf sandbox-exec` shows the process; `pgrep -lf "sleep 30"` shows the sleep child.
**Why human:** Requires live two-terminal session with concurrent process inspection; cannot be tested without actual exec into sandbox.

#### 3. --debug Output Format (D-07 re-confirmation)

**Test:** Run `./sandflox --debug -- /usr/bin/true 2>&1 | head -20`.
**Expected:** Output contains all three lines in any order: `[sandflox] Profile: default | Network: blocked | Filesystem: workspace`, `[sandflox] sbpl: /Users/jhogan/sandflox/.flox/cache/sandflox/sandflox.sb (N rules)` with N >= 5, `[sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)`, plus `(no output on stdout from /usr/bin/true)`.
**Why human:** Plan 02-02 smoke confirmed this; Plan 02-03 Task 2 was skipped. The `TestDiagnosticsDebugOutput` unit test asserts the sbpl line is present in emitDiagnostics output, but end-to-end with the real binary has not been re-run post-02-03.

---

### Gaps Summary

No gaps block goal achievement. The phase goal — "sandflox wraps flox activate under sandbox-exec with a generated SBPL profile that enforces filesystem modes, network modes, and denied paths at the kernel level" — is empirically achieved:

- **SBPL generation:** Pure function verified by 10 unit tests covering all mode combinations
- **Kernel enforcement wiring:** `syscall.Exec` into `sandbox-exec` confirmed by code review and argv-shape unit tests
- **Filesystem enforcement:** `TestSandboxBlocksWrite` PASS — writes outside workspace are kernel-blocked, writes inside are allowed
- **Network enforcement:** `TestSandboxAllowsLocalhost` (both subtests) PASS — blocked/allow-localhost semantics correct at kernel level
- **Unix socket allowance:** `TestSandboxAllowsUnixSocket` PASS — Nix daemon IPC works under blocked network
- **Denied paths:** `TestSandboxBlocksDeniedPath` PASS — denied-path reads return "Operation not permitted"
- **End-to-end pipeline:** `TestBuiltBinaryWrapsCommand` PASS — real binary with real sandbox-exec with real flox wraps `-- echo hello` and exits 0

Three items are flagged for human verification (interactive TTY, process tree, debug format re-run) but none prevent the phase goal from being met. These are observational confirmations, not blocking gaps.

---

*Verified: 2026-04-16*
*Verifier: Claude (gsd-verifier)*
