---
phase: 5
slug: subcommands
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-17
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) 1.22 |
| **Config file** | go.mod (module sandflox) |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test -tags integration -count=1 ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test -tags integration -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | CMD-01,CMD-02,CMD-03 | unit | `go test -run TestExtractSubcommand -v ./...` | Wave 0 | pending |
| 05-01-02 | 01 | 1 | CMD-01 | unit | `go test -run TestValidate -v ./...` | Wave 0 | pending |
| 05-01-03 | 01 | 1 | CMD-01 | unit | `go test -run TestValidateDebug -v ./...` | Wave 0 | pending |
| 05-01-04 | 01 | 1 | CMD-02 | unit | `go test -run TestStatus -v ./...` | Wave 0 | pending |
| 05-01-05 | 01 | 1 | CMD-02 | unit | `go test -run TestStatusNoCache -v ./...` | Wave 0 | pending |
| 05-01-06 | 01 | 1 | CMD-03 | unit | `go test -run TestElevateAlreadySandboxed -v ./...` | Wave 0 | pending |
| 05-01-07 | 01 | 1 | CMD-03 | unit | `go test -run TestElevateNoFlox -v ./...` | Wave 0 | pending |
| 05-01-08 | 01 | 1 | CMD-03 | unit (darwin) | `go test -run TestBuildElevateArgv -v ./...` | Wave 0 | pending |
| 05-01-09 | 01 | 1 | ALL | unit | `go test -run TestSubcommandFlagPosition -v ./...` | Wave 0 | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

- [ ] `subcommand_test.go` — stubs for routing, validate, status, elevate tests
- [ ] `exec_test.go` additions — covers `buildElevateArgv` argv shape (darwin build tag)
- [ ] `cache_test.go` additions — covers `ReadCache` round-trip

*Existing infrastructure covers test framework — go test already configured.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `sandflox elevate` inside flox activate wraps shell under sandbox-exec | CMD-03 | Requires live flox session and sandbox-exec process | 1. Run `flox activate` 2. Run `sandflox elevate` 3. Verify sandbox-exec in process tree 4. Run `sandflox elevate` again — should print "already sandboxed" |

---

## Validation Sign-Off

- [ ] All tasks have automated verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
