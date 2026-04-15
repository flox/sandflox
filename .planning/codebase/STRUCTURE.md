# Codebase Structure

**Analysis Date:** 2026-04-15

## Directory Layout

```
sandflox/
├── sandflox                    # Main executable — kernel sandbox wrapper (500 lines bash)
├── policy.toml                 # Declarative security policy (profiles, modes, paths)
├── requisites.txt              # Default binary whitelist (~55 tools)
├── requisites-minimal.txt      # Minimal binary whitelist (~28 read-only tools)
├── requisites-full.txt         # Full binary whitelist (~54+ tools)
├── test-policy.sh              # v2 policy + kernel enforcement test suite (301 lines)
├── test-sandbox.sh             # v1-compatible shell enforcement test suite (94 lines)
├── verify-sandbox.sh           # Simplified verification script (110 lines)
├── README.md                   # Project documentation
├── .gitignore                  # Ignores .claude/, .flox/cache/, .flox/log/, .flox/run/
├── .claude/
│   └── settings.local.json     # Claude Code permission allowlist
├── .flox/
│   ├── env.json                # Flox environment metadata (name: "sandflox", version: 1)
│   ├── env/
│   │   ├── manifest.toml       # Flox manifest — packages, hooks, profile scripts (418 lines)
│   │   └── manifest.lock       # Flox lockfile (generated)
│   ├── cache/                  # Runtime-generated artifacts (gitignored)
│   │   ├── sandflox/           # Core sandbox cache
│   │   │   ├── config.json           # Resolved policy config (JSON)
│   │   │   ├── policy.toml           # Staged copy of policy.toml
│   │   │   ├── sandflox.sb           # Generated macOS SBPL profile
│   │   │   ├── entrypoint.sh         # Generated non-interactive entrypoint
│   │   │   ├── fs-filter.sh          # Generated shell write enforcement wrappers
│   │   │   ├── requisites.txt        # Staged copy of active requisites file
│   │   │   ├── active-profile.txt    # Current profile name
│   │   │   ├── net-mode.txt          # Resolved network mode
│   │   │   ├── fs-mode.txt           # Resolved filesystem mode
│   │   │   ├── net-blocked.flag      # Exists when network=blocked
│   │   │   ├── writable-paths.txt    # Resolved absolute writable paths
│   │   │   ├── read-only-paths.txt   # Resolved absolute read-only paths
│   │   │   ├── denied-paths.txt      # Resolved absolute denied paths
│   │   │   └── bin/                  # Symlink directory (filtered PATH)
│   │   ├── sandflox-bin/             # (empty/unused)
│   │   └── sandflox-python/
│   │       └── usercustomize.py      # Python ensurepip block + open() write enforcement
│   ├── log/                    # Executive logs (gitignored)
│   └── run/                    # Runtime state (gitignored)
└── .planning/
    └── codebase/               # GSD analysis documents
```

## Directory Purposes

**Root (`sandflox/`):**
- Purpose: All source files live at root level — flat structure, no `src/` directory
- Contains: Main executable, policy config, requisites lists, test scripts, docs
- Key files: `sandflox`, `policy.toml`, `requisites.txt`

**`.flox/env/`:**
- Purpose: Flox environment definition — the "inner sandbox" configuration
- Contains: `manifest.toml` (packages, hooks, profile scripts), `manifest.lock`
- Key files: `manifest.toml` is the second most important file after `sandflox`

**`.flox/cache/sandflox/`:**
- Purpose: Runtime-generated enforcement artifacts — all regenerated on each activation
- Contains: Resolved configs, generated SBPL profiles, shell wrappers, symlink bin directory
- Generated: Yes — by `sandflox` script and manifest hook
- Committed: No (gitignored)

**`.flox/cache/sandflox-python/`:**
- Purpose: Python runtime enforcement — usercustomize.py for ensurepip blocking and open() wrapping
- Contains: `usercustomize.py`
- Generated: Yes — by manifest hook
- Committed: No (gitignored)

**`.flox/cache/sandflox/bin/`:**
- Purpose: Filtered binary directory — contains only symlinks to whitelisted tools
- Contains: Symlinks from `$FLOX_ENV/bin/<tool>` for each tool in the active requisites file
- Generated: Yes — by profile.common or entrypoint.sh on each activation
- Committed: No (gitignored)

## Key File Locations

**Entry Points:**
- `sandflox`: Main executable. Kernel sandbox wrapper. Parses policy, generates platform enforcement, execs into sandboxed flox.
- `.flox/env/manifest.toml`: Flox manifest. Contains `[hook] on-activate` (Layer 1/5) and `[profile] common` (Layer 2/3/4).

**Configuration:**
- `policy.toml`: Declarative security policy. Network mode, filesystem mode, writable/read-only/denied paths, profile presets.
- `.flox/env/manifest.toml`: Package whitelist (`[install]`), environment variables (`[vars]`), hook and profile scripts.
- `requisites.txt`: Default binary whitelist (~55 tools).
- `requisites-minimal.txt`: Minimal binary whitelist (~28 read-only tools).
- `requisites-full.txt`: Full binary whitelist (~54+ tools).

**Core Logic:**
- `sandflox` (lines 43-131): Policy parsing via embedded Python.
- `sandflox` (lines 191-291): macOS SBPL profile generation (`_sfx_generate_sbpl`).
- `sandflox` (lines 295-351): Linux bwrap flag generation (`_sfx_generate_bwrap`).
- `sandflox` (lines 359-438): Non-interactive entrypoint generation.
- `sandflox` (lines 465-500): Platform dispatch and exec.
- `.flox/env/manifest.toml` (lines 59-227): Hook policy parser and enforcement script generator (Python).
- `.flox/env/manifest.toml` (lines 319-411): Profile — requisites filtering, function armor, fs-filter sourcing.

**Testing:**
- `test-policy.sh`: v2 policy enforcement tests — kernel (macOS sandbox-exec), shell (fs-filter, curl removal, python blocking), profiles, backward compat.
- `test-sandbox.sh`: v1-compatible tests — PATH restriction (blocked/allowed tools), function armor, breadcrumbs, escape vectors.
- `verify-sandbox.sh`: Simplified verification — same categories as test-sandbox.sh with slightly different structure.

**Generated Enforcement (not committed):**
- `.flox/cache/sandflox/sandflox.sb`: macOS SBPL profile (Seatbelt rules).
- `.flox/cache/sandflox/fs-filter.sh`: Shell wrappers for cp/mv/mkdir/rm/rmdir/ln/chmod/tee.
- `.flox/cache/sandflox/entrypoint.sh`: Non-interactive mode entrypoint.
- `.flox/cache/sandflox-python/usercustomize.py`: Python ensurepip block + open() enforcement.

## Key Files

| File | Purpose | Lines |
|------|---------|-------|
| `sandflox` | Main kernel sandbox wrapper — policy parsing, SBPL/bwrap generation, platform dispatch | 500 |
| `.flox/env/manifest.toml` | Flox manifest — packages, hook (Layer 1/5), profile (Layer 2/3/4) | 418 |
| `policy.toml` | Declarative security policy — modes, paths, profiles | 42 |
| `test-policy.sh` | v2 policy + kernel enforcement test suite | 301 |
| `verify-sandbox.sh` | Simplified sandbox verification | 110 |
| `test-sandbox.sh` | v1-compatible shell enforcement test suite | 94 |
| `requisites-full.txt` | Full binary whitelist (~54+ tools) | 76 |
| `requisites.txt` | Default binary whitelist (~55 tools) | 74 |
| `requisites-minimal.txt` | Minimal binary whitelist (~28 tools) | 38 |
| `README.md` | Project documentation | 236 |

## Naming Conventions

**Files:**
- Executables: lowercase, no extension (`sandflox`)
- Configuration: lowercase with extension (`policy.toml`, `requisites.txt`)
- Test scripts: `test-*.sh` or `verify-*.sh` pattern
- Requisites variants: `requisites-<profile>.txt` pattern

**Internal Variables:**
- All sandflox variables prefixed with `_sfx_` (e.g., `_sfx_cache`, `_sfx_net_mode`, `_sfx_fs_mode`)
- Generated shell functions prefixed with `_sandflox_` (e.g., `_sandflox_blocked`)
- Real command references prefixed with `_sfx_real_` (e.g., `_sfx_real_cp`)

**Cache Files:**
- Mode files: `<domain>-mode.txt` (e.g., `net-mode.txt`, `fs-mode.txt`)
- Path lists: `<type>-paths.txt` (e.g., `writable-paths.txt`, `denied-paths.txt`)
- Flags: `<condition>.flag` (e.g., `net-blocked.flag`)

## Where to Add New Code

**New Security Policy Feature:**
- Policy definition: Add section to `policy.toml`
- Kernel enforcement (macOS): Add to `_sfx_generate_sbpl()` in `sandflox` (line 191)
- Kernel enforcement (Linux): Add to `_sfx_generate_bwrap()` in `sandflox` (line 295)
- Shell enforcement: Add to the Python code block in `.flox/env/manifest.toml` `[hook] on-activate` (line 59)
- Non-interactive support: Update generated `entrypoint.sh` in `sandflox` (line 359)

**New Tool to the Sandbox:**
- Add binary name to appropriate `requisites*.txt` file(s)
- If the binary comes from a new package, add to `.flox/env/manifest.toml` `[install]`

**New Blocked Command (function armor):**
- Add function definition to THREE locations:
  1. `.flox/env/manifest.toml` `[profile] common` (line 369-400)
  2. `.flox/env/manifest.toml` `[profile] common` `export -f` list (line 396-400)
  3. `sandflox` entrypoint generation (line 396-426)

**New Test:**
- Kernel + policy tests: Add to `test-policy.sh`
- Shell enforcement tests: Add to `test-sandbox.sh`
- Follow existing `ok()`/`bad()`/`skp()` helper pattern

**New Profile Preset:**
- Add `[profiles.<name>]` section to `policy.toml` with `requisites`, `network`, `filesystem` keys
- Create corresponding `requisites-<name>.txt` file at project root

## Special Directories

**`.flox/cache/sandflox/`:**
- Purpose: All runtime-generated enforcement artifacts
- Generated: Yes — regenerated on every `flox activate` or `./sandflox` invocation
- Committed: No (in `.gitignore`)
- Safe to delete: Yes — will be regenerated on next activation

**`.flox/cache/sandflox/bin/`:**
- Purpose: Symlink directory that becomes the sole PATH entry inside the sandbox
- Generated: Yes — by profile.common or entrypoint.sh
- Committed: No
- Content: One symlink per whitelisted tool, pointing to `$FLOX_ENV/bin/<tool>`

**`.flox/env/`:**
- Purpose: Flox environment definition (committed)
- Generated: `manifest.lock` is generated; `manifest.toml` is hand-authored
- Committed: Yes
- Modifiable: Edit `manifest.toml` to change packages or enforcement scripts

**`.planning/codebase/`:**
- Purpose: GSD analysis documents for AI-assisted development
- Generated: By mapping agents
- Committed: Per project convention

---

*Structure analysis: 2026-04-15*
