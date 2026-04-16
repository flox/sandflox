---
phase: 2
slug: kernel-enforcement-sbpl-sandbox-exec
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (Go 1.22+) |
| **Config file** | `go.mod` (no additional config required) |
| **Quick run command** | `go test -run 'TestGenerateSBPL\|TestExec' -short ./...` |
| **Full suite command** | `go test ./... -v` |
| **Integration command (darwin)** | `go test -tags integration ./... -run TestSandboxExec -v` |
| **Estimated runtime** | ~5 seconds (unit), ~20 seconds (integration) |

---

## Sampling Rate

- **After every task commit:** Run `go test -run '^TestGenerateSBPL|^TestExec' -short ./...` — SBPL unit tests, fast
- **After every plan wave:** Run `go test ./... -v` — all units + Phase 1 regression
- **Before `/gsd:verify-work`:** Full suite + integration tests must be green: `go test ./... -v && go test -tags integration ./... -v`
- **Max feedback latency:** 5 seconds (unit), 30 seconds (full with integration)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | KERN-01, KERN-02 | unit | `go test -run TestGenerateSBPL_AllowDefaultBaseline ./...` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | KERN-01 | unit | `go test -run TestGenerateSBPL_WorkspaceBlocked ./...` | ❌ W0 | ⬜ pending |
| 02-01-03 | 01 | 1 | KERN-03 | unit | `go test -run TestGenerateSBPL_DeniedPaths ./...` | ❌ W0 | ⬜ pending |
| 02-01-04 | 01 | 1 | KERN-07 | unit | `go test -run TestGenerateSBPL_LocalhostAllowed ./...` | ❌ W0 | ⬜ pending |
| 02-01-05 | 01 | 1 | KERN-08 | unit | `go test -run TestGenerateSBPL_UnixSocketAlwaysAllowed ./...` | ❌ W0 | ⬜ pending |
| 02-01-06 | 01 | 1 | KERN-02 | unit | `go test -run TestGenerateSBPL_ParameterSubstitution ./...` | ❌ W0 | ⬜ pending |
| 02-01-07 | 01 | 1 | KERN-01, KERN-02 | unit | `go test -run TestWriteSBPL ./...` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 2 | KERN-04 | unit | `go test -run TestBuildSandboxExecArgs ./...` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 2 | KERN-04, KERN-05 | integration | `go test -tags integration -run TestSandboxExecLaunch ./...` | ❌ W0 | ⬜ pending |
| 02-02-03 | 02 | 2 | KERN-06 | integration | `go test -tags integration -run TestSandboxExecWrapCommand ./...` | ❌ W0 | ⬜ pending |
| 02-03-01 | 03 | 3 | KERN-01-08 | integration | `go test -tags integration -run TestSandboxBlocksWrite ./...` | ❌ W0 | ⬜ pending |
| 02-03-02 | 03 | 3 | KERN-07 | integration | `go test -tags integration -run TestSandboxAllowsLocalhost ./...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `sbpl.go` — new file implementing `GenerateSBPL(*ResolvedConfig, home string) string` and `WriteSBPL(cacheDir, content string) (string, error)`
- [ ] `sbpl_test.go` — table-driven tests per filesystem × network mode combination
- [ ] `exec_darwin.go` — `execWithKernelEnforcement(*ResolvedConfig, projectDir string, userArgs []string)` with `//go:build darwin`
- [ ] `exec_other.go` — stub with `//go:build !darwin` that falls through to plain flox activate
- [ ] `exec_integration_test.go` — `//go:build darwin && integration` real sandbox-exec subprocess tests
- [ ] Existing Phase 1 test infrastructure (`config_test.go`, `policy_test.go`) covers regression

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Interactive shell drops into sandboxed prompt | KERN-05 | Requires TTY; Go tests cannot easily assert interactive shell semantics | Run `./sandflox` — verify prompt appears and `touch /etc/sf-test` fails with permission error |
| Process tree shows clean exec replacement | KERN-04 | Inspect PID relationship at the OS level | Run `./sandflox -- sleep 30` in one terminal; in another run `pgrep -lf sandflox` — should see NO sandflox parent, only sandbox-exec |
| --debug prints SBPL path and rule count | D-07 | Diagnostic output shape is UX-verified | Run `./sandflox --debug -- echo hi` and confirm stderr shows `[sandflox] sbpl: /path/to/sandflox.sb (N rules)` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (`sbpl.go`, `sbpl_test.go`, `exec_darwin.go`, `exec_other.go`, `exec_integration_test.go`)
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
