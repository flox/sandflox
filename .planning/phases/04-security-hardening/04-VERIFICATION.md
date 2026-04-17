---
phase: 04-security-hardening
verified: 2026-04-17T02:15:00Z
status: passed
score: 9/9 must-haves verified
gaps: []
human_verification:
  - test: "Inside a real sandbox, verify AWS_SECRET_ACCESS_KEY and GITHUB_TOKEN injected from parent shell are empty"
    expected: "Both vars print as empty strings inside the sandbox"
    why_human: "Integration tests require flox + sandbox-exec + a warm flox cache to run; cannot verify without real subprocess in CI-equivalent environment"
  - test: "Inside a real sandbox, verify HOME and TERM are non-empty"
    expected: "HOME starts with / and TERM is non-empty inside the sandbox"
    why_human: "Same integration test constraint -- TestEnvScrubbing_AllowlistPassesEssentials requires running subprocess"
  - test: "Inside a real sandbox, verify PYTHONDONTWRITEBYTECODE=1 and PYTHON_NOPIP=1"
    expected: "Both vars equal '1' inside sandbox"
    why_human: "TestEnvScrubbing_PythonSafetyFlags is tagged darwin && integration -- requires real flox environment"
---

# Phase 4: Security Hardening Verification Report

**Phase Goal:** sandflox scrubs the environment before sandbox entry so sensitive credentials and configuration do not leak into the agent's execution context
**Verified:** 2026-04-17T02:15:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | BuildSanitizedEnv returns only allowlisted env vars plus Flox/Nix/macOS system vars | VERIFIED | `env.go` lines 110-166: 4-phase filter (exact allowlist + prefix allowlist + passthrough + forced). `TestBuildSanitizedEnv_AllowlistPassesEssentials`, `TestBuildSanitizedEnv_UnknownVarsBlocked` both PASS |
| 2 | Sensitive prefix patterns (AWS_*, SSH_*, GITHUB_*) are blocked even if parent shell has them | VERIFIED | `env.go` lines 40-60: `blockedPrefixes` and `blockedExact` vars. `TestBuildSanitizedEnv_BlocksSensitivePrefixes` PASSES |
| 3 | User-configured env-passthrough vars from policy.toml pass through the filter | VERIFIED | `env.go` lines 136-144: Phase 3 passthrough bypasses block check. `TestBuildSanitizedEnv_Passthrough` and `TestBuildSanitizedEnv_PassthroughDoesNotOverrideBlock` PASS |
| 4 | PYTHONDONTWRITEBYTECODE=1 and PYTHON_NOPIP=1 are force-set in the sanitized env | VERIFIED | `env.go` lines 65-68: `forcedVars` map. `TestBuildSanitizedEnv_ForcedVars` and `TestBuildSanitizedEnv_ForcedVarsOverride` PASS |
| 5 | Output env slice is sorted for deterministic test assertions and --debug output | VERIFIED | `env.go` line 164: `sort.Strings(result)`. `TestBuildSanitizedEnv_Sorted` PASSES |
| 6 | syscall.Exec in exec_darwin.go receives the sanitized env from BuildSanitizedEnv, not os.Environ() | VERIFIED | `exec_darwin.go` line 126: `env := BuildSanitizedEnv(cfg)` fed to `syscall.Exec(sbxPath, argv, env)`. No `os.Environ()` at exec call site |
| 7 | execFlox fallback path uses sanitized env, covering both darwin and non-darwin platforms | VERIFIED | `main.go` lines 158-161: `env := os.Environ(); if cfg != nil { env = BuildSanitizedEnv(cfg) }`. `exec_other.go` line 19: `execFlox(cfg, userArgs)` passes cfg through |
| 8 | --debug mode logs env filtering counts (N passed, M blocked, K forced) | VERIFIED | `main.go` lines 123-130: `[sandflox] Env: %d vars passed, %d blocked, %d forced` in `emitDiagnostics` debug block |
| 9 | Inside a real sandbox subprocess, env scrubbing tests exist and are structured to verify all three success criteria | VERIFIED | `shell_integration_test.go` lines 599-719: 4 `TestEnvScrubbing_*` tests; integration tag `darwin && integration`; `runSandfloxProbeWithEnv` and `runSandfloxWithFlags` helpers present |

**Score:** 9/9 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `env.go` | BuildSanitizedEnv + allowlist/blocklist/forced constants | VERIFIED | 167 lines; exports `BuildSanitizedEnv`; contains `defaultAllowlist`, `allowedPrefixes`, `blockedPrefixes`, `blockedExact`, `forcedVars`, `envToMap`, `isBlocked` |
| `env_test.go` | Unit tests for env filtering (SEC-01, SEC-02, SEC-03) | VERIFIED | 189 lines; contains 9 subtests covering all plan-specified behaviors including `TestBuildSanitizedEnv_AllowlistPassesEssentials`, `TestBuildSanitizedEnv_BlocksSensitivePrefixes`, `TestBuildSanitizedEnv_Passthrough`, `TestBuildSanitizedEnv_ForcedVars`, `TestBuildSanitizedEnv_Sorted` |
| `policy.go` | SecuritySection struct + [security] parsing in mapToPolicy | VERIFIED | Lines 22-24: `type SecuritySection struct { EnvPassthrough []string }`. Lines 17-18: `Security SecuritySection` in Policy. Lines 267-272: `sections["security"]` parsing block |
| `policy_test.go` | Tests for [security] section parsing | VERIFIED | Lines 413-459: `TestParsePolicy_SecuritySection`, `TestParsePolicy_SecuritySectionEmpty`, `TestParsePolicy_NoSecuritySection` -- all PASS |
| `config.go` | EnvPassthrough field in ResolvedConfig | VERIFIED | Line 24: `EnvPassthrough []string \`json:"env_passthrough"\``. Lines 79-80: `envPassthrough := policy.Security.EnvPassthrough`. Line 92: `EnvPassthrough: envPassthrough` in returned struct |
| `config_test.go` | TestResolveConfig_EnvPassthrough | VERIFIED | Lines 170-186: `TestResolveConfig_EnvPassthrough` creates Policy with SecuritySection, calls ResolveConfig, asserts EnvPassthrough -- PASSES |
| `policy.toml` | New [security] section with env-passthrough | VERIFIED | Lines 25-29: `[security]` section with `env-passthrough = []` and explanatory comment |
| `exec_darwin.go` | BuildSanitizedEnv wired into execWithKernelEnforcement | VERIFIED | Lines 124-126: `env := BuildSanitizedEnv(cfg)` replaces `os.Environ()`; line 88: `execFlox(cfg, userArgs)` fallback wired |
| `main.go` | execFlox accepts *ResolvedConfig, uses sanitized env | VERIFIED | Line 141: `func execFlox(cfg *ResolvedConfig, userArgs []string)`. Lines 158-161: nil guard + `BuildSanitizedEnv(cfg)`. Line 42: `execFlox(nil, userArgs)` for no-policy fallback |
| `exec_other.go` | Non-darwin stub passes cfg to execFlox | VERIFIED | Line 19: `execFlox(cfg, userArgs)`. No `_ = cfg` suppression. Clean 20-line file |
| `shell_integration_test.go` | 4 TestEnvScrubbing_* integration tests | VERIFIED | Lines 599-719: `TestEnvScrubbing_AllowlistPassesEssentials`, `TestEnvScrubbing_SensitiveVarsBlocked`, `TestEnvScrubbing_PythonSafetyFlags`, `TestEnvScrubbing_DebugDiagnostic` plus helpers `runSandfloxProbeWithEnv` and `runSandfloxWithFlags` |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `env.go` | `config.go` | `BuildSanitizedEnv` takes `*ResolvedConfig` | WIRED | `func BuildSanitizedEnv(cfg *ResolvedConfig) []string` -- signature confirmed at line 110 |
| `config.go` | `policy.go` | `ResolveConfig` reads `policy.Security.EnvPassthrough` | WIRED | `config.go` line 80: `envPassthrough := policy.Security.EnvPassthrough` |
| `policy.go` | `policy.toml` | `mapToPolicy` parses `[security]` section | WIRED | `policy.go` lines 267-272: `if sec, ok := sections["security"]; ok { ... }` |
| `exec_darwin.go` | `env.go` | `BuildSanitizedEnv(cfg)` replacing `os.Environ()` | WIRED | `exec_darwin.go` line 126: `env := BuildSanitizedEnv(cfg)` -- confirmed, no `os.Environ()` at exec call site |
| `main.go` | `env.go` | `execFlox` calls `BuildSanitizedEnv(cfg)` | WIRED | `main.go` line 160: `env = BuildSanitizedEnv(cfg)` inside `if cfg != nil` guard |
| `shell_integration_test.go` | `exec_darwin.go` | Subprocess probe verifies env filtering end-to-end | WIRED | `TestEnvScrubbing_SensitiveVarsBlocked` (line 633) injects `AWS_SECRET_ACCESS_KEY` via `runSandfloxProbeWithEnv` and asserts empty in subprocess |

### Data-Flow Trace (Level 4)

Not applicable to this phase. Phase 4 produces a security filter function (`BuildSanitizedEnv`) and wires it into exec paths -- it is not a component that renders dynamic data to a UI. Data flows are verified via unit tests (BuildSanitizedEnv's output is the exec env slice) and integration tests (subprocess env assertions).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All unit tests pass | `go test ./... -count=1` | `ok sandflox 0.215s` | PASS |
| Phase 4 specific tests pass | `go test -run "TestBuildSanitizedEnv\|TestParsePolicy_Security\|TestResolveConfig_EnvPassthrough\|TestParsePolicyToml" -count=1 -v` | 13 tests, all PASS | PASS |
| Binary builds clean | `go build ./...` | No output, exit 0 | PASS |
| Commits verified (04-01 RED) | `git log --oneline` | `ff01c4f test(04-01): add failing tests` | PASS |
| Commits verified (04-01 GREEN) | `git log --oneline` | `6153ed0 feat(04-01): implement env sanitization engine` | PASS |
| Commits verified (04-02 Task 1) | `git log --oneline` | `9204502 feat(04-02): wire BuildSanitizedEnv into exec paths` | PASS |
| Commits verified (04-02 Task 2) | `git log --oneline` | `d406ac6 test(04-02): integration tests for env scrubbing` | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SEC-01 | 04-01-PLAN.md, 04-02-PLAN.md | sandflox scrubs environment variables before passing them into the sandbox -- only allowlisted vars pass through (HOME, USER, TERM, SHELL, LANG, PATH, plus Flox-required vars) | SATISFIED | `defaultAllowlist` in `env.go`; `TestBuildSanitizedEnv_AllowlistPassesEssentials` PASS; `TestEnvScrubbing_AllowlistPassesEssentials` integration test exists |
| SEC-02 | 04-01-PLAN.md, 04-02-PLAN.md | sandflox blocks sensitive env vars by default (AWS_*, SSH_*, GPG_*, GITHUB_TOKEN, etc.) with a configurable allowlist in policy.toml | SATISFIED | `blockedPrefixes` + `blockedExact` in `env.go`; `[security] env-passthrough` in `policy.toml`; `TestBuildSanitizedEnv_BlocksSensitivePrefixes` PASS; `TestEnvScrubbing_SensitiveVarsBlocked` integration test injects and verifies scrubbing |
| SEC-03 | 04-01-PLAN.md, 04-02-PLAN.md | sandflox sets PYTHONDONTWRITEBYTECODE=1 and PYTHON_NOPIP=1 inside the sandbox | SATISFIED | `forcedVars` map in `env.go`; `TestBuildSanitizedEnv_ForcedVars` + `TestBuildSanitizedEnv_ForcedVarsOverride` PASS; `TestEnvScrubbing_PythonSafetyFlags` integration test exists |

No orphaned requirements found. REQUIREMENTS.md Traceability table maps SEC-01, SEC-02, SEC-03 exclusively to Phase 4. All three declared in both plan frontmatter fields; all three verified.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | - | - | - | - |

No TODOs, FIXMEs, placeholders, empty returns, or stub patterns found in any Phase 4 modified files (`env.go`, `env_test.go`, `policy.go`, `policy_test.go`, `config.go`, `config_test.go`, `exec_darwin.go`, `main.go`, `exec_other.go`, `shell_integration_test.go`, `policy.toml`).

The only `os.Environ()` remaining in `main.go` is the correct nil-guard fallback at line 158 (`env := os.Environ()`) which is immediately overwritten by `BuildSanitizedEnv(cfg)` when a config exists. This is intentional graceful degradation documented in the code comment.

### Human Verification Required

These items require running the actual sandboxed subprocess with `flox` available:

**1. Sensitive Vars Blocked End-to-End**

**Test:** Set `export AWS_SECRET_ACCESS_KEY=testvalue GITHUB_TOKEN=ghtoken` in parent shell, then run `./sandflox -- /bin/bash -c 'echo "AWS=$AWS_SECRET_ACCESS_KEY GH=$GITHUB_TOKEN"'`
**Expected:** Output shows `AWS= GH=` (both empty)
**Why human:** Integration test `TestEnvScrubbing_SensitiveVarsBlocked` exists and is structurally correct but requires `darwin && integration` build tag, `flox` in PATH, and a warm `.flox` cache

**2. Allowlisted Vars Pass Through**

**Test:** Run `./sandflox -- /bin/bash -c 'echo "HOME=$HOME USER=$USER TERM=$TERM"'`
**Expected:** All three variables are non-empty, reflecting parent shell values
**Why human:** Same integration constraint as above

**3. Python Safety Flags Forced**

**Test:** Run `./sandflox -- /bin/bash -c 'echo "PYDWB=$PYTHONDONTWRITEBYTECODE NOPIP=$PYTHON_NOPIP"'`
**Expected:** Output shows `PYDWB=1 NOPIP=1` regardless of what those vars are set to in the parent
**Why human:** `TestEnvScrubbing_PythonSafetyFlags` covers this but requires real sandbox execution

### Gaps Summary

No gaps. All automated checks pass:
- 9/9 must-have truths verified against actual codebase
- All 11 artifacts exist, are substantive, and are wired
- All 6 key links confirmed in source code
- 3 commits from Plan 01 verified (ff01c4f, 6153ed0, plus docs commit)
- 4 commits from Plan 02 verified (9204502, d406ac6, plus docs commits)
- Full unit test suite passes: `go test ./... -count=1` exits 0
- Binary builds clean: `go build ./...` exits 0
- No `os.Environ()` at either `syscall.Exec` call site (exec_darwin.go line 130, main.go line 164 -- both use sanitized env)
- SEC-01, SEC-02, SEC-03 fully satisfied by implementation + unit tests

Phase goal is achieved. The 3 human verification items are confirmations of integration-test behavior that cannot run without a live flox environment -- they do not represent missing implementation.

---

_Verified: 2026-04-17T02:15:00Z_
_Verifier: Claude (gsd-verifier)_
