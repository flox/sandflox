---
phase: 06
slug: distribution-and-polish
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-17
---

# Phase 06 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test tooling |
| **Quick run command** | `go test ./... -count=1 -timeout 60s` |
| **Full suite command** | `go test ./... -count=1 -timeout 120s -v` |
| **Estimated runtime** | ~1 second |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -count=1 -timeout 60s`
- **After every plan wave:** Run `go test ./... -count=1 -timeout 120s -v`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 2 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | DIST-05 | unit | `go test ./... -run TestNix -count=1` | ✅ | ⬜ pending |
| 06-01-02 | 01 | 1 | DIST-02 | manual | `flox build && flox publish` | N/A | ⬜ pending |
| 06-01-03 | 01 | 1 | DIST-03 | manual | `flox install sandflox` | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. Go test framework already in place from Phase 1.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `flox publish` uploads to FloxHub | DIST-02 | Requires FloxHub authentication and network | Run `flox publish`, verify success output |
| `flox install sandflox` works | DIST-03 | Requires fresh Flox environment | Create fresh env, run `flox install sandflox`, verify `sandflox --help` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 2s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
