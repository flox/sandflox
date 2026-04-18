# Resume Context — v3 Milestone

## What's Done
All 5 phases of the v3 plan are implemented, tested (56 tests pass), committed, and pushed to GitHub.

**Commit:** `a9b5019` on `main`
**Commit message:** `feat: v3 flox-native sandflox -- init, elevate fix, embedded policy, flox-sbx env`

### Completed Changes
1. **FIX-01/FIX-02:** Elevate re-entry detection uses `SANDFLOX_SANDBOX` (not `SANDFLOX_ENABLED`), cache dir passed consistently
2. **INIT-01/INIT-02:** `sandflox init` writes embedded policy.toml, `sandflox prepare` falls back to embedded default
3. **SBX-01/SBX-02/SBX-03:** flox-sbx manifest has three-tier policy discovery, entrypoint exports `SANDFLOX_ENABLED=1`
4. **Phase 4:** 7 new tests, all 56 pass
5. **DOC-01:** README updated with two-artifact model, init, flox-sbx quickstart

## What's Left — Publishing

### 1. Build sandflox for x86_64-darwin
The sandflox binary is currently only published for `aarch64-darwin`. It needs to be built and published for `x86_64-darwin` too (or the flox-sbx env needs to be darwin-only with sandflox restricted to aarch64-darwin).

**Action:** From the repo root (`~/sandflox`):
```bash
# Build for x86_64-darwin (cross-compile or on an Intel Mac)
flox build   # builds for current arch
flox publish # publishes to FloxHub
```

### 2. Push flox-sbx environment to FloxHub
The flox-sbx environment at `environments/flox-sbx/` was missing `env.json`. The user ran `flox init` and copied the manifest back. The manifest now has:
- `[options] systems = ["aarch64-darwin", "x86_64-darwin"]` — restricts to macOS only
- `sandflox.systems = ["aarch64-darwin"]` — sandflox binary only available for ARM Mac currently

**Action:** From `~/sandflox/environments/flox-sbx`:
```bash
flox push
```

### 3. Commit the manifest systems fix
The `environments/flox-sbx/.flox/env/manifest.toml` was updated to add `[options] systems` and `sandflox.systems`. This change needs to be committed and pushed.

**Action:**
```bash
cd ~/sandflox
git add environments/flox-sbx/.flox/env/manifest.toml
git commit -m "fix: restrict flox-sbx to darwin systems, sandflox to aarch64-darwin"
git push
```

### 4. Verify the full consumer workflow
```bash
# On a clean machine or new terminal:
flox pull 8BitTacoSupreme/flox-sbx
flox activate          # should show shell enforcement
sandflox elevate       # should add kernel enforcement
sandflox status        # should show enforcement state
```

## Key Files
| File | Role |
|------|------|
| `environments/flox-sbx/.flox/env/manifest.toml` | flox-sbx environment definition (just updated with systems restriction) |
| `subcommand.go` | init handler, prepare fallback, elevate fix |
| `policy.go` | embedded policy, ParsePolicyBytes, DefaultPolicy |
| `exec_darwin.go` | SANDFLOX_SANDBOX injection, cacheDir param |
| `README.md` | two-artifact docs |
