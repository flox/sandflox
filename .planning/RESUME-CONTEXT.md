# Resume Context — v3 Milestone

## Status: COMPLETE

All v3 phases implemented, bugs fixed, published to FloxHub, and verified end-to-end.

## What's Done

### v3 Phases (commit a9b5019)
1. **FIX-01/FIX-02:** Elevate re-entry detection uses `SANDFLOX_SANDBOX` (not `SANDFLOX_ENABLED`), cache dir passed consistently
2. **INIT-01/INIT-02:** `sandflox init` writes embedded policy.toml, `sandflox prepare` falls back to embedded default
3. **SBX-01/SBX-02/SBX-03:** flox-sbx manifest has three-tier policy discovery, entrypoint exports `SANDFLOX_ENABLED=1`
4. **Phase 4:** 7 new tests, all 56 pass
5. **DOC-01:** README updated with two-artifact model, init, flox-sbx quickstart

### Publishing fixes (commits 164fb66, 2b8dc4b, 47194d0)
6. **Nix fileset fix:** Added policy.toml and requisites*.txt to .flox/pkgs/sandflox.nix fileset (go:embed)
7. **Elevate embedded fallback:** `sandflox elevate` now falls back to embedded default policy (matching `prepare`)
8. **Absolute sandbox-exec:** Uses `/usr/bin/sandbox-exec` instead of LookPath (works inside restricted PATH)
9. **Entrypoint absolute paths:** Generated entrypoint.sh uses `${FLOX_ENV}/bin/rm`, `mkdir`, `ln` for setup phase
10. **Published:** sandflox 0.1.0 on FloxHub (aarch64-darwin), flox-sbx pushed to FloxHub

### Documentation (pending commit)
11. **TESTPLAN.txt:** Updated with all test fixes (T1-T7, W1-W4), resolved bugs (B1-B6), embedded default flow
12. **README.md:** Updated tool count (~57), elevate docs (no -policy needed), platform note (aarch64-darwin)

## What's Left

### Open TODOs
- [ ] claude-code packaging for flox-sbx
- [ ] Test on macOS Ventura/Sonoma/Sequoia
- [ ] Test SANDFLOX_POLICY_PATH override
- [ ] Publish sandflox for x86_64-darwin (needs Intel Mac or CI)

### Cross-compile note
x86_64-darwin build works via `nix-build --system x86_64-darwin` (Rosetta + extra-platforms in nix.conf).
`flox publish` lacks a `--system` flag so publishing requires an x86_64 host or CI runner.
`/etc/nix/nix.conf` now has `extra-platforms = x86_64-darwin`.

## Key Files
| File | Role |
|------|------|
| `environments/flox-sbx/.flox/env/manifest.toml` | flox-sbx environment definition |
| `.flox/pkgs/sandflox.nix` | Nix build expression (includes all go:embed files) |
| `subcommand.go` | init, prepare, elevate, validate, status handlers |
| `exec_darwin.go` | sandbox-exec wrapping, absolute paths |
| `templates/entrypoint.sh.tmpl` | Shell enforcement with absolute setup paths |
| `policy.go` | Embedded policy, TOML parser |
| `TESTPLAN.txt` | Manual E2E test plan (tests 1-28) |
| `README.md` | User-facing documentation |
