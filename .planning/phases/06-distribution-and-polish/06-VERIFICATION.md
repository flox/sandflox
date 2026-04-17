---
phase: 06-distribution-and-polish
verified: 2026-04-17T12:53:18Z
status: human_needed
score: 3/4 must-haves verified
re_verification: false
human_verification:
  - test: "Confirm flox install 8BitTacoSupreme/sandflox succeeds in a fresh Flox environment and sandflox --help outputs usage"
    expected: "flox install exits 0, sandflox --help shows -debug, -net, -policy, -profile, -requisites flags and subcommands, which sandflox returns a /nix/store/ path"
    why_human: "flox publish and flox install are external service operations (FloxHub). The user confirmed these steps were performed during execution. Automated verification cannot contact FloxHub or inspect the installed catalog entry. The binary built from the expression runs correctly (spot-checked locally), but the install-from-catalog path requires a terminal session with flox available outside the project directory."
---

# Phase 6: Distribution and Polish Verification Report

**Phase Goal:** sandflox is published to FloxHub and installable into any Flox environment with a hermetic, reproducible Nix build
**Verified:** 2026-04-17T12:53:18Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `flox publish` successfully uploads sandflox to FloxHub | ? HUMAN_NEEDED | User confirmed during execution (Task 2 checkpoint). Cannot verify programmatically — requires FloxHub connectivity. |
| 2 | In a fresh Flox environment, `flox install sandflox` makes the `sandflox` command available | ? HUMAN_NEEDED | User confirmed `flox install 8BitTacoSupreme/sandflox` and `sandflox --help` worked. Cannot re-run without terminal + fresh Flox env. |
| 3 | Nix expression uses `lib.fileset.toSource` and `-trimpath` | ✓ VERIFIED | Both patterns confirmed in `.flox/pkgs/sandflox.nix` lines 7 and 20. |
| 4 | `flox build sandflox` produces a binary | ✓ VERIFIED | `result-sandflox/bin/sandflox` symlink points to `/nix/store/sd1dhc3py6vqrxbidh6chifqysmdvx4g-sandflox-0.1.0`. Binary is 3.26 MB Mach-O 64-bit arm64 executable. |

**Score:** 2/4 truths programmatically verified; 2/4 require human confirmation (already provided during execution)

Note: The human-confirmed truths (#1 and #2) have supporting artifact evidence (the correct Nix expression, the working binary, the corrected env.json) that all indicate the pipeline was successfully executed. The score conservatively reflects what can be verified without live FloxHub access.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `.flox/pkgs/sandflox.nix` | Nix build expression with `buildGoModule` | ✓ VERIFIED | 35 lines, substantive. Contains `buildGoModule`, `lib.fileset.toSource`, `../../templates`, `vendorHash = null`, `CGO_ENABLED = "0"`, `-trimpath`, `-X main.Version=0.1.0`. |
| `.flox/env.json` | Local path environment pointer with `"name": "sandflox"` | ✓ VERIFIED | 4 lines. Contains `"name": "sandflox"` and `"version": 1`. Does NOT contain `"owner"` field (which was the documented blocker). |
| `result-sandflox/bin/sandflox` | Built binary symlink into /nix/store | ✓ VERIFIED | Symlink resolves to `/nix/store/sd1dhc3py6vqrxbidh6chifqysmdvx4g-sandflox-0.1.0/bin/sandflox`. 3,262,656 bytes, Mach-O 64-bit arm64 executable, world-executable. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `.flox/pkgs/sandflox.nix` | `flox build sandflox` | Flox discovers `.flox/pkgs/*.nix` automatically | ✓ VERIFIED | Build artifact exists at `result-sandflox/bin/sandflox` pointing to Nix store path, confirming Flox found and built the expression. Commit `562ae0b` documents the fix that made the build succeed. |
| `flox build output` | `flox publish` | Publishes the derivation from `.flox/pkgs/` to FloxHub | ? HUMAN_NEEDED | Execution confirmed `flox publish sandflox` ran. Cannot re-verify without FloxHub access. |
| `flox publish` | `flox install 8BitTacoSupreme/sandflox` | FloxHub catalog serves published packages | ? HUMAN_NEEDED | User confirmed install + `--help` worked in fresh environment. Cannot re-verify without Flox environment. |
| `shell.go go:embed` | `../../templates` in `sandflox.nix` fileset | Nix build includes template files for embed | ✓ VERIFIED | `shell.go:24` has `//go:embed templates/entrypoint.sh.tmpl templates/fs-filter.sh.tmpl templates/usercustomize.py.tmpl`. `sandflox.nix:12` includes `../../templates` in `fileset.unions`. This was the auto-fixed deviation in commit `562ae0b`. |

### Data-Flow Trace (Level 4)

Not applicable — this phase produces a Nix build expression and FloxHub publish, not a component that renders dynamic data. The key data flow (policy.toml → SBPL → sandbox-exec) was established and verified in prior phases.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Built binary is executable Mach-O | `file result-sandflox/bin/sandflox` | `Mach-O 64-bit executable arm64` | ✓ PASS |
| Built binary is non-trivial size | `wc -c result-sandflox/bin/sandflox` | `3,262,656 bytes` (3.26 MB) | ✓ PASS |
| Binary shows correct CLI flags | `result-sandflox/bin/sandflox --help` | Shows `-debug`, `-net`, `-policy`, `-profile`, `-requisites` flags; emits `[sandflox]` diagnostics | ✓ PASS |
| Nix expression has all hermetic patterns | `grep fileset.toSource && grep trimpath && grep templates` | All 3 patterns present | ✓ PASS |
| env.json is local format (no owner field) | `grep -c '"owner"' .flox/env.json` | `0` | ✓ PASS |
| env.lock absent (FloxHub artifact removed) | `ls .flox/env.lock` | File does not exist | ✓ PASS |
| Build symlink points to Nix store | `readlink result-sandflox` | `/nix/store/sd1dhc3py6vqrxbidh6chifqysmdvx4g-sandflox-0.1.0` | ✓ PASS |
| flox install (human-verified) | `flox install 8BitTacoSupreme/sandflox` in fresh env | User confirmed exit 0 and `--help` functional | ? HUMAN (confirmed) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| DIST-02 | 06-01-PLAN.md | sandflox publishes to FloxHub via `flox publish` | ? HUMAN_NEEDED | User confirmed during Task 2 checkpoint. External operation, cannot re-verify programmatically. |
| DIST-03 | 06-01-PLAN.md | sandflox is installable into any Flox environment via `flox install sandflox` | ? HUMAN_NEEDED | User confirmed `flox install 8BitTacoSupreme/sandflox` and `sandflox --help` in fresh env. External operation. |
| DIST-05 | 06-01-PLAN.md | Nix expression uses `lib.fileset.toSource` and `-trimpath` | ✓ VERIFIED | `.flox/pkgs/sandflox.nix` line 7: `lib.fileset.toSource`, line 20: `buildFlags = [ "-trimpath" ]`. |

**Orphaned Requirements Check:** DIST-04 is mapped to Phase 1 in REQUIREMENTS.md (not Phase 6) and does not appear in the Phase 6 plan's `requirements` field. This is correct — DIST-04 was satisfied in Phase 1. No orphaned requirements.

The Phase 6 plan claims exactly DIST-02, DIST-03, DIST-05. REQUIREMENTS.md maps exactly those three to Phase 6. Coverage is complete and consistent.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | None found | — | — |

Scan of `.flox/pkgs/sandflox.nix` and `.flox/env.json` found no TODO, FIXME, placeholder, empty return, or stub patterns.

### Human Verification Required

#### 1. FloxHub Publish + Install Confirmation

**Test:** In a new terminal outside the sandflox project:
```
cd /tmp && mkdir sandflox-verify && cd sandflox-verify
flox init
flox install 8BitTacoSupreme/sandflox
flox activate -- sandflox --help
flox activate -- which sandflox
cd /tmp && rm -rf sandflox-verify
```
**Expected:**
- `flox install` exits 0 with package install confirmation
- `sandflox --help` shows flags: `-debug`, `-net`, `-policy`, `-profile`, `-requisites`
- `which sandflox` returns a path containing `/nix/store/`

**Why human:** `flox publish` and `flox install` are external service operations against FloxHub. Automated verification cannot contact FloxHub, cannot inspect the catalog entry, and cannot create a fresh Flox environment in a subprocess. The user confirmed these steps were performed during execution (Task 2 human-verify checkpoint), but re-confirmation via the steps above would constitute definitive automated-equivalent verification.

**Note:** The user already confirmed this during phase execution. If that confirmation stands as accepted evidence, this item can be marked closed and the overall status promoted to `passed`.

### Gaps Summary

No blocking gaps exist. All three code artifacts are verified correct and substantive:

- `.flox/pkgs/sandflox.nix` is a complete, working Nix buildGoModule expression with all required hermetic patterns (`lib.fileset.toSource`, `-trimpath`, `../../templates` for go:embed, `CGO_ENABLED=0`, `vendorHash=null`)
- `.flox/env.json` is in the correct local-path format (no `"owner"` field that was the documented blocker)
- The build symlink `result-sandflox/bin/sandflox` resolves to a real Nix store derivation and the binary is a functional 3.26 MB Mach-O arm64 executable that responds correctly to `--help`

The only items in `human_needed` status are the two external FloxHub operations (publish + install) which cannot be re-executed by an automated verifier. The user confirmed both during execution. The code evidence fully supports that the pipeline was correctly set up and executed.

---

_Verified: 2026-04-17T12:53:18Z_
_Verifier: Claude (gsd-verifier)_
