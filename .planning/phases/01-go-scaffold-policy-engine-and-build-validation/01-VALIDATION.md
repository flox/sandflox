---
phase: 1
slug: go-scaffold-policy-engine-and-build-validation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-15
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — Wave 0 creates test files |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test -v -count=1 ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test -v -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | CORE-02 | unit | `go test -run TestParsePolicyToml ./...` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | CORE-02 | unit | `go test -run TestParseErrors ./...` | ❌ W0 | ⬜ pending |
| 01-02-01 | 02 | 1 | CORE-03, CORE-04 | unit | `go test -run TestProfileResolution ./...` | ❌ W0 | ⬜ pending |
| 01-02-02 | 02 | 1 | CORE-05 | unit | `go test -run TestCLIFlags ./...` | ❌ W0 | ⬜ pending |
| 01-03-01 | 03 | 2 | CORE-06 | unit | `go test -run TestCacheWrite ./...` | ❌ W0 | ⬜ pending |
| 01-03-02 | 03 | 2 | CORE-07 | unit | `go test -run TestDiagnostics ./...` | ❌ W0 | ⬜ pending |
| 01-03-03 | 03 | 2 | DIST-01, DIST-04 | integration | `flox build` | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `policy_test.go` — stubs for CORE-02 (TOML parsing, error handling)
- [ ] `config_test.go` — stubs for CORE-03, CORE-04, CORE-05 (profile resolution, CLI flags)
- [ ] `cache_test.go` — stubs for CORE-06 (config caching)
- [ ] `main_test.go` — stubs for CORE-07 (diagnostics output)

*Go test infrastructure is built-in — no framework install needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `flox build` produces binary | DIST-01 | Requires Flox runtime and Nix store | Run `flox build` in project root, verify binary at `result/bin/sandflox` |
| Binary exec into flox activate | CORE-01 | Requires Flox environment | Run `./result/bin/sandflox` and verify it drops into flox activate |

*`flox build` is an integration test that requires the Flox runtime — cannot be fully automated in unit tests.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
