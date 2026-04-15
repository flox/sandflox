# Architecture

**Analysis Date:** 2026-04-15

## Pattern Overview

**Overall:** Two-tier defense-in-depth sandbox — kernel-level enforcement (OS sandbox primitives) composed with shell-level enforcement (Flox activate hooks/profile scripts).

**Key Characteristics:**
- Declarative policy file (`policy.toml`) drives all enforcement decisions
- Platform-adaptive kernel enforcement: macOS sandbox-exec (SBPL) or Linux bwrap (bubblewrap namespaces)
- Shell-level enforcement operates independently as a fallback when kernel tier is unavailable
- Profile system allows switching security postures (minimal / default / full) without editing policy
- Generated artifacts (SBPL profiles, shell wrappers, symlink bins) are cached in `.flox/cache/sandflox/`

## Layers

**Layer 1 — PATH Wipe (hook: `on-activate`):**
- Purpose: Eliminate all system binaries from PATH, leaving only `$FLOX_ENV/bin`
- Location: `.flox/env/manifest.toml` `[hook] on-activate` (line 40-313)
- Contains: PATH export, policy.toml parsing, Python ensurepip blocking, breadcrumb cleanup
- Depends on: Flox runtime (`$FLOX_ENV`, `$FLOX_ENV_CACHE`, `$FLOX_ENV_PROJECT`)
- Used by: All subsequent layers inherit the wiped PATH

**Layer 2 — Binary Whitelist (profile: `common`):**
- Purpose: Filter `$FLOX_ENV/bin` down to only binaries listed in the active requisites file
- Location: `.flox/env/manifest.toml` `[profile] common` (line 317-412)
- Contains: Symlink bin directory creation from requisites.txt, conditional curl removal
- Depends on: `$FLOX_ENV_CACHE/sandflox/requisites.txt` (staged by Layer 1)
- Used by: Agent processes — PATH is set to the symlink bin directory

**Layer 3 — Function Armor (profile: `common`):**
- Purpose: Shadow 27 package manager commands with shell functions returning exit 126
- Location: `.flox/env/manifest.toml` `[profile] common` (line 364-400)
- Contains: Shell function definitions for flox, nix, brew, pip, npm, cargo, docker, etc.
- Depends on: Nothing — pure shell function definitions
- Used by: Catches agents that call blocked tools by name even via absolute paths or after PATH manipulation

**Layer 4 — Breadcrumb Cleanup (hook + profile):**
- Purpose: Scrub environment variables an agent could use to discover escape paths
- Location: `.flox/env/manifest.toml` hook (line 313) and profile (line 410-411)
- Contains: `unset FLOX_ENV_PROJECT FLOX_ENV_DIRS FLOX_PATH_PATCHED`
- Depends on: Nothing
- Used by: Prevents agents from finding Flox internals

**Layer 5 — Policy Enforcement Scripts (hook-generated):**
- Purpose: Parse `policy.toml` and generate runtime enforcement artifacts
- Location: `.flox/env/manifest.toml` `[hook] on-activate` (line 59-233); generated files in `.flox/cache/sandflox/`
- Contains: Python policy parser, fs-filter.sh generator, net-blocked flag, path resolution
- Depends on: `policy.toml`, Python 3.6+
- Used by: Layer 2 (requisites selection), Layer 6 (kernel config), fs-filter.sh (profile)

**Layer 6 — Kernel Enforcement (`./sandflox` wrapper):**
- Purpose: OS-level filesystem, network, and path blocking via sandbox-exec (macOS) or bwrap (Linux)
- Location: `sandflox` (the main executable, 500 lines)
- Contains: Policy parsing, SBPL generation, bwrap flag generation, entrypoint generation, platform dispatch
- Depends on: `policy.toml`, Python 3.6+, platform-specific tools (sandbox-exec or bwrap), Flox
- Used by: Invoked directly by user/CI to wrap `flox activate`

**Generated: fs-filter.sh (shell write enforcement):**
- Purpose: Wrap write commands (cp, mv, mkdir, rm, rmdir, ln, chmod, tee) with path-checking functions that produce clear `[sandflox] BLOCKED:` error messages
- Location: Generated at `.flox/cache/sandflox/fs-filter.sh`
- Contains: `_sfx_check_write_target()` function with denied/writable/read-only path checks, command wrappers
- Depends on: Policy-derived path lists in `.flox/cache/sandflox/{writable,read-only,denied}-paths.txt`
- Used by: Sourced by profile (Layer 5) and entrypoint script

**Generated: usercustomize.py (Python write enforcement):**
- Purpose: Block `ensurepip` at import level and wrap `builtins.open` to enforce filesystem policy for Python code
- Location: Generated at `.flox/cache/sandflox-python/usercustomize.py`
- Contains: Module injection for ensurepip, `builtins.open` monkey-patch with path checking
- Depends on: `.flox/cache/sandflox/fs-mode.txt`, `writable-paths.txt`, `denied-paths.txt`
- Used by: Python runtime via `PYTHONPATH` / `PYTHONUSERBASE` env vars

**Generated: entrypoint.sh (non-interactive mode):**
- Purpose: Apply requisites filtering and function armor for `./sandflox -- CMD` mode (where `profile.common` does not run)
- Location: Generated at `.flox/cache/sandflox/entrypoint.sh`
- Contains: Duplicated logic from profile.common — requisites filtering, function armor, fs-filter sourcing
- Depends on: Same cached artifacts as the profile
- Used by: `./sandflox -- <command>` invocations

## Data Flow

**Policy Resolution Flow:**

1. User runs `./sandflox` or `./sandflox -- <command>`
2. `sandflox` script reads `policy.toml` via embedded Python (lines 43-131)
3. Profile resolved: `$SANDFLOX_PROFILE` env var > `[meta] profile` > `"default"`
4. Profile overrides merged with top-level `[network]` and `[filesystem]` settings
5. Resolved config written to `.flox/cache/sandflox/config.json`
6. Path lists (writable, read-only, denied) resolved to absolute paths and written to cache
7. Platform detected (`uname -s`)
8. macOS: SBPL profile generated at `.flox/cache/sandflox/sandflox.sb`
   Linux: bwrap argument list generated dynamically
9. `sandbox-exec -f sandflox.sb ... flox activate` or `bwrap ... -- flox activate` exec'd

**Shell Enforcement Flow (inside the sandbox):**

1. `flox activate` triggers `[hook] on-activate`
2. Hook re-parses `policy.toml` (independent of kernel tier) to generate shell enforcement
3. Hook stages requisites file, generates `fs-filter.sh`, writes mode flags
4. Hook wipes PATH to `$FLOX_ENV/bin`, blocks ensurepip, cleans breadcrumbs
5. `[profile] common` runs: filters PATH to requisites bin dir, applies function armor, sources fs-filter.sh

**Interactive vs. Non-Interactive:**
- Interactive (`./sandflox`): `flox activate` runs both hook and profile.common
- Non-interactive (`./sandflox -- CMD`): hook runs, but profile.common does not; `entrypoint.sh` replicates profile logic

**State Management:**
- All runtime state stored in `.flox/cache/sandflox/` (gitignored)
- Key state files: `config.json`, `net-mode.txt`, `fs-mode.txt`, `active-profile.txt`, `net-blocked.flag`
- Path lists: `writable-paths.txt`, `read-only-paths.txt`, `denied-paths.txt`, `requisites.txt`
- Generated enforcement: `sandflox.sb` (macOS SBPL), `fs-filter.sh`, `entrypoint.sh`
- Python enforcement: `.flox/cache/sandflox-python/usercustomize.py`

## Key Abstractions

**Policy Profiles:**
- Purpose: Named security posture presets (network mode, filesystem mode, requisites file)
- Configuration: `policy.toml` `[profiles.*]` sections
- Selection: `SANDFLOX_PROFILE` env var > `[meta] profile` > `"default"`
- Examples: `minimal` (strict/blocked/~28 tools), `default` (workspace/blocked/~55 tools), `full` (permissive/unrestricted/~54+ tools)

**Requisites Files:**
- Purpose: Binary-level whitelist — names of executables the agent may use
- Examples: `requisites.txt` (default, ~55 tools), `requisites-minimal.txt` (~28 read-only tools), `requisites-full.txt` (~54+ tools)
- Pattern: One binary name per line, comments with `#`, blank lines ignored
- Mechanism: Symlinks from `$FLOX_ENV/bin/<tool>` into `$FLOX_ENV_CACHE/sandflox/bin/`

**Filesystem Modes:**
- `permissive`: No write restrictions
- `workspace`: Writes allowed to project dir + /tmp, with read-only overrides for `.git/`, `.flox/env/`, `policy.toml`, etc., and denied paths for sensitive dirs
- `strict`: No user-writable paths; only temp dirs for shell operation

**Network Modes:**
- `blocked`: All TCP/UDP denied at kernel level; unix sockets allowed (nix daemon); localhost optionally allowed; curl removed from PATH
- `unrestricted`: No network restrictions

## Entry Points

**`sandflox` (main wrapper script):**
- Location: `sandflox`
- Triggers: User invokes `./sandflox` or `./sandflox -- <command>`
- Responsibilities: Parse policy, generate kernel enforcement config, exec into sandbox-exec/bwrap wrapping flox activate

**`.flox/env/manifest.toml` `[hook] on-activate`:**
- Location: `.flox/env/manifest.toml` (line 40)
- Triggers: Every `flox activate` invocation (with or without `./sandflox`)
- Responsibilities: PATH wipe, policy parsing, enforcement script generation, ensurepip blocking, breadcrumb cleanup

**`.flox/env/manifest.toml` `[profile] common`:**
- Location: `.flox/env/manifest.toml` (line 317)
- Triggers: Interactive shell sessions inside `flox activate`
- Responsibilities: Requisites filtering, function armor, fs-filter sourcing, final breadcrumb cleanup

**`.flox/cache/sandflox/entrypoint.sh`:**
- Location: Generated at `.flox/cache/sandflox/entrypoint.sh`
- Triggers: Non-interactive `./sandflox -- <command>` mode
- Responsibilities: Same as profile.common — requisites filtering, function armor, fs-filter, then `exec "$@"`

## Error Handling

**Strategy:** Graceful degradation with clear warnings. Each tier falls back independently.

**Patterns:**
- Missing `policy.toml`: Falls back to shell-only enforcement (v1 behavior). `sandflox` script: `exec flox activate "$@"`
- Missing `sandbox-exec` or `bwrap`: Warning printed, falls back to shell-only. `sandflox` script: lines 467-469, 484-486
- Missing Python 3: Hard error (required for policy parsing). Exit 1 with message.
- Missing `flox`: Hard error. Exit 1 with message.
- Unsupported platform: Warning, falls back to shell-only enforcement.
- TOML parsing: Tries `tomllib` (3.11+), then `tomli` (pip package), then inline minimal parser.
- Shell-level blocked operations: Return exit code 126 with `[sandflox] BLOCKED: <reason>` on stderr.
- Kernel-level blocked operations: Return OS-level EPERM (generic "Operation not permitted").
- Python-level blocked operations: Raise `PermissionError` with `[sandflox] BLOCKED: <reason>`.

## Cross-Cutting Concerns

**Logging:** Diagnostic messages to stderr prefixed with `[sandflox]`. Examples: `[sandflox] Profile: default | Network: blocked | Filesystem: workspace`, `[sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)`, `[sandflox] Sandbox active: 55 tools (profile: default)`.

**Validation:** Preflight checks for required tools (python3, flox) in `sandflox` script (lines 29-37). Policy parsing validates structure implicitly through Python dict access.

**Authentication:** Not applicable. No auth system — this is a local sandbox enforcement tool.

**Configuration:** Single declarative `policy.toml` at project root. Profile selection via env var or policy default. Package whitelist via `manifest.toml` `[install]`. Binary whitelist via `requisites*.txt` files.

**Platform Abstraction:** `uname -s` detection in `sandflox` script dispatches to `_sfx_generate_sbpl()` (macOS) or `_sfx_generate_bwrap()` (Linux). Both read the same cached path lists and mode files.

## Architecture Decisions

**Two-tier enforcement (kernel + shell):** Kernel enforcement returns generic EPERM — agents cannot understand or adapt. Shell enforcement returns descriptive `[sandflox] BLOCKED:` messages — agents can understand why an action failed. Users running `flox activate` directly (without `./sandflox`) still get shell-level protection.

**Allow-default SBPL strategy (macOS):** The SBPL profile starts from `(allow default)` and selectively denies, rather than deny-default with explicit whitelisting. Rationale: Flox depends on nix daemon, macOS frameworks, and system libraries that need unpredictable read paths. Deny-default is too fragile for this use case (noted in `sandflox` line 198-199).

**Duplicated logic in entrypoint.sh:** The non-interactive entrypoint duplicates profile.common logic because `flox activate`'s `[profile] common` only runs for interactive shells. This is a deliberate design trade-off for non-interactive `-- CMD` mode support.

**Inline TOML parser:** `sandflox` and the manifest hook both embed a minimal TOML parser in Python for environments without `tomllib` (pre-3.11) or `tomli`. This avoids a pip dependency — which would be ironic for a tool that blocks pip.

**Symlink bin directory pattern:** Rather than modifying PATH entries, requisites filtering creates a new bin directory of symlinks. This is clean, atomic, and avoids race conditions with the original `$FLOX_ENV/bin`.

---

*Architecture analysis: 2026-04-15*
