---
phase: 3
slug: shell-enforcement-artifacts
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` stdlib (Go 1.22+) — same as Phase 2 |
| **Config file** | none — built into `go test` |
| **Quick run command** | `go test -run 'TestGenerate|TestWriteRequisitesBin' ./...` |
| **Full suite command** | `go test -tags integration ./...` |
| **Estimated runtime** | ~60 seconds (unit <5s; integration ~55s) |

---

## Sampling Rate

- **After every task commit:** Run `go test -run 'TestGenerate|TestWriteRequisitesBin' ./...`
- **After every plan wave:** Run `go test ./...` (full unit suite)
- **Before `/gsd:verify-work`:** Full suite (`go test -tags integration ./...`) must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

> Filled in during planning. See 03-RESEARCH.md § "Validation Architecture" for requirement → test mapping (SHELL-01 through SHELL-08 fully covered).

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| (TBD)   | 01   | 1    | SHELL-01    | unit      | `go test -run TestGenerateEntrypoint_PathExport ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-01    | integration | `go test -tags integration -run TestShellEnforces_PathWipe ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-02    | unit      | `go test -run TestWriteRequisitesBin_Symlinks ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-02    | integration | `go test -tags integration -run TestShellEnforces_SymlinkBin ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-03    | unit      | `go test -run TestGenerateEntrypoint_ArmorFunctions ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-03    | integration | `go test -tags integration -run TestShellEnforces_ArmorBlocks ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-04    | unit      | `go test -run TestGenerateFsFilter_Wrappers ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-04    | integration | `go test -tags integration -run TestShellEnforces_FsFilterBlocks ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-05    | unit      | `go test -run TestGenerateUsercustomize_BlocksEnsurepip ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-05    | integration | `go test -tags integration -run TestShellEnforces_PythonOpenBlocked ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-06    | unit      | `go test -run TestGenerateEntrypoint_BreadcrumbUnset ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-06    | integration | `go test -tags integration -run TestShellEnforces_BreadcrumbsCleared ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-07    | unit      | `go test -run TestWriteRequisitesBin_SkipsCurlWhenNetBlocked ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 03   | 3    | SHELL-07    | integration | `go test -tags integration -run TestShellEnforces_CurlRemovedWhenNetBlocked ./...` | ❌ W0 | ⬜ pending |
| (TBD)   | 01   | 1    | SHELL-08    | unit      | `go test -run TestGenerate_BlockedMessagesFormat ./...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `/Users/jhogan/sandflox/templates/` directory — new
- [ ] `/Users/jhogan/sandflox/templates/entrypoint.sh.tmpl` — covers SHELL-01, SHELL-03, SHELL-06
- [ ] `/Users/jhogan/sandflox/templates/fs-filter.sh.tmpl` — covers SHELL-04, SHELL-08
- [ ] `/Users/jhogan/sandflox/templates/usercustomize.py.tmpl` — covers SHELL-05, SHELL-08
- [ ] `/Users/jhogan/sandflox/shell.go` — GenerateShellArtifacts, WriteShellArtifacts, shellquote FuncMap
- [ ] `/Users/jhogan/sandflox/shell_test.go` — unit tests from Per-Task Verification Map
- [ ] `/Users/jhogan/sandflox/shell_integration_test.go` — `//go:build darwin && integration`; subprocess tests
- [ ] `/Users/jhogan/sandflox/config_test.go` — add TestParseRequisites if missing (covers SHELL-02)
- [ ] Update `/Users/jhogan/sandflox/exec_test.go` — Phase 2 argv tests will fail after D-01/D-02 rewire
- [ ] Framework install: none — Go stdlib only

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| None — all SHELL-01 through SHELL-08 behaviors have automated subprocess integration tests per Phase 2 precedent | — | — | — |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
