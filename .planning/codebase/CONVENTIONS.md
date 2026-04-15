# Coding Conventions

**Analysis Date:** 2026-04-15

## Naming Conventions

**Files:**
- Executable scripts use lowercase-hyphenated names: `sandflox`, `test-policy.sh`, `test-sandbox.sh`, `verify-sandbox.sh`
- Configuration files use lowercase-dot notation: `policy.toml`, `manifest.toml`
- Requisites lists use lowercase-hyphenated with `.txt` extension: `requisites.txt`, `requisites-minimal.txt`, `requisites-full.txt`
- Generated/cached files use lowercase-hyphenated with appropriate extension: `fs-filter.sh`, `net-mode.txt`, `sandflox.sb`

**Shell Variables:**
- All sandflox-internal variables use `_sfx_` prefix to avoid namespace collisions: `_sfx_dir`, `_sfx_policy`, `_sfx_cache`, `_sfx_net_mode`, `_sfx_fs_mode`, `_sfx_config`
- Exported environment variables use `SCREAMING_SNAKE_CASE`: `SANDFLOX_ENABLED`, `SANDFLOX_PROFILE`, `SANDFLOX_MODE`
- Flox-provided variables follow Flox convention: `FLOX_ENV`, `FLOX_ENV_CACHE`, `FLOX_ENV_PROJECT`

**Shell Functions:**
- Blocking armor functions are named after the command they shadow: `flox()`, `nix()`, `pip()`, `docker()`
- Internal helper functions use `_sfx_` prefix: `_sfx_generate_sbpl()`, `_sfx_generate_bwrap()`, `_sfx_check_write_target()`
- The central blocker function uses `_sandflox_blocked()` (full project name, not abbreviation)

**Python Variables:**
- Python code embedded in shell uses underscore_case: `profile_name`, `net_mode`, `fs_mode`
- Private/internal Python variables use `_` prefix: `_blocked`, `_original_open`, `_sandflox_open`, `_sfx_cache`

**TOML Configuration:**
- Section headers use dot-separated namespaces: `[profiles.minimal]`, `[profiles.default]`
- Keys use lowercase-hyphenated: `allow-localhost`, `read-only`, `net-mode`
- Values use standard TOML types: strings, booleans, arrays

## Code Style

**Shell (Bash):**
- Shebang: `#!/usr/bin/env bash` for all shell scripts
- Strict mode: `set -euo pipefail` in the main `sandflox` script; test scripts use `set -uo pipefail` (no `-e` to allow test failure continuation)
- Section separators: Use ASCII box-drawing style comments: `# ── Section Name ────────────────────────────`
- Inline comments explain *why*, not *what*
- `exec` used for final command invocation to replace the shell process rather than spawning a child
- `local` keyword used for all function-scoped variables
- Arrays use bash array syntax: `_sfx_flox_args=()`, `bwrap_args=()`
- String tests use `[ ]` single-bracket syntax, not `[[ ]]` (except where pattern matching is needed)
- Case statements used for multi-branch conditionals on mode values

**Python (Embedded):**
- Python code is embedded inline via bash heredocs or `-c` flag, never in separate `.py` source files (except generated artifacts)
- Uses `try/except` cascading import pattern for `tomllib`/`tomli`/fallback parser
- JSON used as the data interchange format between Python and Bash
- The project includes a minimal inline TOML parser as a fallback for Python < 3.11 without `tomli` installed

**Generated Code:**
- `fs-filter.sh` and `usercustomize.py` are generated at activation time, not committed
- Generated files live in `.flox/cache/sandflox/`
- Generated code includes a header comment identifying it as generated: `# sandflox fs-filter -- generated shell enforcement`

## File Organization

**Root-level scripts:**
- `sandflox` (no extension): The main executable wrapper script
- `test-*.sh`: Test scripts
- `verify-sandbox.sh`: Legacy verification script (v1)
- `policy.toml`: Declarative security policy
- `requisites*.txt`: Binary whitelists per profile

**Flox environment structure:**
- `.flox/env/manifest.toml`: Package declarations, hooks, and profile scripts
- `.flox/cache/sandflox/`: Runtime-generated enforcement artifacts (not committed)
- `.flox/cache/sandflox-python/`: Generated Python customization (not committed)

**Configuration hierarchy:**
- `policy.toml` is the single source of truth for security policy
- `.flox/env/manifest.toml` is the single source of truth for installed packages and activation hooks
- `requisites*.txt` files define per-profile binary whitelists

## Error Handling

**Shell Error Pattern:**
- Preflight checks: Test for required tools (`command -v python3`, `command -v flox`) and exit with clear error messages before proceeding
- Graceful degradation: Missing kernel enforcement tools (`sandbox-exec`, `bwrap`) produce `WARNING` and fall back to shell-only enforcement rather than failing
- Missing policy: No `policy.toml` falls back to `flox activate` directly with a notice
- All error/status messages go to stderr (`>&2`), not stdout
- Error messages use `[sandflox]` prefix for identification: `[sandflox] ERROR:`, `[sandflox] WARNING:`, `[sandflox] BLOCKED:`

**Blocking Pattern:**
- Shell function armor returns exit code `126` (command cannot execute) — not `1` or `127`
- Python blocking raises `PermissionError` with `[sandflox]` prefix for agent-readable messages
- `ensurepip` blocking raises `SystemExit` with `[sandflox]` prefix

**Error Message Format:**
```
[sandflox] BLOCKED: <what> is <reason>
[sandflox] ERROR: <requirement> required to <purpose>
[sandflox] WARNING: <tool> not found -- falling back to <alternative>
```

## Documentation Style

**Script Headers:**
- Every script starts with a comment block identifying purpose and usage:
  ```bash
  #!/usr/bin/env bash
  # sandflox — kernel-enforced sandbox wrapper for flox activate
  # ──────────────────────────────────────────────────────────────
  # Reads policy.toml, generates platform-specific kernel enforcement
  ```
- Usage examples included in the header for the main script

**Inline Comments:**
- Section separators use `# ── Section Name ────────────────` format throughout
- Comments explain design decisions and "why" (e.g., "deny-default + explicit whitelisting is too fragile for flox")
- Layer numbering in comments (Layer 1, Layer 2, etc.) maps to the architecture documentation

**TOML Comments:**
- Values have inline comments explaining options: `mode = "blocked"  # "unrestricted" | "blocked"`
- Section headers have descriptive comments above them

**Manifest Comments:**
- `manifest.toml` uses a prominent ASCII box-drawing header describing all enforcement layers
- Each layer is documented inline where it's implemented

## Git Conventions

**Commit Messages:**
- Use conventional commit prefixes: `feat:`, `docs:`
- Include the component/scope in the message: `feat: sandflox v2 -- declarative policy with kernel enforcement`
- Use em-dash (`--`) as separator in commit descriptions
- Messages are concise, single-line where possible

**Branch Strategy:**
- Single `main` branch (no feature branches observed)
- Linear history

**What Gets Committed:**
- Source scripts, configuration, test scripts, documentation
- `.flox/env/manifest.toml` and `.flox/env/manifest.lock` (committed)
- `.flox/cache/` contents are gitignored (generated at runtime)
- `.flox/log/` and `.flox/run/` are gitignored

## Import / Dependency Pattern

**Python Dependencies:**
- Zero external Python dependencies required at runtime
- Cascading fallback pattern for TOML parsing: `tomllib` (3.11+) -> `tomli` (pip) -> inline minimal parser
- Only standard library modules used: `os`, `sys`, `json`, `re`, `types`, `importlib`, `builtins`, `shutil`

**Shell Dependencies:**
- External: `flox` (required), `python3` (required), `sandbox-exec` (macOS, optional), `bwrap` (Linux, optional)
- All coreutils assumed available during wrapper execution (before sandbox activation)

## Platform Handling

**Pattern:** Platform detection via `uname -s`, then `case` statement branching:
```bash
case "$_sfx_platform" in
  Darwin) ... ;;
  Linux)  ... ;;
  *)      ... ;;
esac
```

**Platform-specific code is isolated into generator functions:**
- `_sfx_generate_sbpl()` for macOS sandbox-exec profiles
- `_sfx_generate_bwrap()` for Linux bubblewrap flags

---

*Convention analysis: 2026-04-15*
