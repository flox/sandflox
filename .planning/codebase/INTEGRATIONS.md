# External Integrations

**Analysis Date:** 2026-04-15

## APIs & External Services

**None.** sandflox is a local-only sandbox tool with no external API integrations. The entire purpose of the tool is to restrict and control outbound access, not consume external services.

## Platform Sandbox APIs

**macOS Seatbelt (sandbox-exec):**
- Purpose: Kernel-level filesystem, network, and process enforcement on macOS
- Integration: `sandflox` generates SBPL (Seatbelt Profile Language) profiles at runtime
- Generated artifact: `.flox/cache/sandflox/sandflox.sb`
- Invoked via: `sandbox-exec -f <profile> -D PROJECT=... -D HOME=... -D FLOX_CACHE=... flox activate ...`
- Parameters passed: `PROJECT` (workspace root), `HOME` (user home), `FLOX_CACHE` (Flox cache dir)
- Implementation: `_sfx_generate_sbpl()` function in `sandflox` (lines 191-291)
- Documentation: Apple's sandbox-exec is undocumented but stable; SBPL syntax is Scheme-like

**Linux bubblewrap (bwrap):**
- Purpose: Kernel-level filesystem, network, and process isolation on Linux via user namespaces
- Integration: `sandflox` generates bwrap CLI flag arrays at runtime
- Invoked via: `bwrap <flags> -- flox activate ...`
- Key flags used: `--ro-bind`, `--bind`, `--tmpfs`, `--unshare-net`, `--unshare-pid`, `--die-with-parent`, `--proc`, `--dev`
- Implementation: `_sfx_generate_bwrap()` function in `sandflox` (lines 295-351)

## Flox Environment System

**Flox Activate Lifecycle:**
- Purpose: Provides reproducible Nix-based shell environments with hook/profile lifecycle
- Integration: sandflox wraps `flox activate` and injects enforcement via manifest hooks/profile scripts
- Manifest: `.flox/env/manifest.toml`
- Lockfile: `.flox/env/manifest.lock`
- Environment metadata: `.flox/env.json` (name: "sandflox", version: 1)

**Flox Cache Directory (`$FLOX_ENV_CACHE`):**
- Purpose: Runtime artifact staging area for all enforcement scripts
- Staged artifacts:
  - `sandflox/requisites.txt` — Resolved binary whitelist for active profile
  - `sandflox/bin/` — Symlink directory with only whitelisted binaries
  - `sandflox/fs-filter.sh` — Generated shell wrappers for filesystem write enforcement
  - `sandflox/net-blocked.flag` — Marker file indicating network is blocked
  - `sandflox/net-mode.txt` — Resolved network mode string
  - `sandflox/fs-mode.txt` — Resolved filesystem mode string
  - `sandflox/writable-paths.txt` — Resolved absolute writable paths
  - `sandflox/read-only-paths.txt` — Resolved absolute read-only paths
  - `sandflox/denied-paths.txt` — Resolved absolute denied paths
  - `sandflox/active-profile.txt` — Name of active profile
  - `sandflox/config.json` — Full resolved config (written by `sandflox` wrapper)
  - `sandflox/policy.toml` — Staged copy of policy
  - `sandflox/sandflox.sb` — Generated SBPL profile (macOS only)
  - `sandflox/entrypoint.sh` — Non-interactive mode entrypoint
  - `sandflox-python/usercustomize.py` — Python runtime enforcement module

**Flox Environment Variables Used:**
- `FLOX_ENV` — Path to the active Flox environment (provides `$FLOX_ENV/bin`)
- `FLOX_ENV_CACHE` — Path to the Flox environment cache directory
- `FLOX_ENV_PROJECT` — Path to the project root (scrubbed during enforcement to prevent escape)
- `FLOX_ENV_DIRS` — Additional environment directories (scrubbed)
- `FLOX_PATH_PATCHED` — Internal Flox flag (scrubbed)

## Databases & Storage

**None.** sandflox operates entirely on the local filesystem. No databases, object stores, or persistent storage systems are used.

**Local Filesystem Usage:**
- Project directory: Working directory for the sandboxed environment
- `/tmp`: Temporary file storage (always writable in workspace mode)
- `~/.cache/flox`: Flox cache directory (passed to sandbox-exec as `FLOX_CACHE` parameter)
- `~/.config/flox`: Flox configuration (allowed through kernel sandbox for Flox operation)
- `~/.local/share/flox`: Flox data directory (allowed through kernel sandbox)

## Message Queues & Events

**None.** sandflox is a synchronous, single-process tool.

## Third-Party SDKs

**None.** The project has zero external dependencies beyond system tools and Flox-provided packages. The Python code uses only the standard library. The `tomli` package is referenced as an optional fallback but is not required — an inline TOML parser handles Python < 3.11.

## Authentication & Identity

**None.** sandflox does not implement authentication. It is a local enforcement tool.

**Credential Protection:**
- sandflox actively blocks access to credential directories via the `[filesystem] denied` policy:
  - `~/.ssh/`
  - `~/.gnupg/`
  - `~/.aws/`
  - `~/.config/gcloud/`
  - `~/.config/gh/`
- Enforcement: Kernel-level (SBPL deny / bwrap tmpfs mount) + shell-level (fs-filter.sh)

## Monitoring & Observability

**Error Tracking:** None (standalone CLI tool)

**Logs:**
- All output goes to stderr via `echo "[sandflox] ..." >&2`
- Message prefix: `[sandflox]` for all operational messages
- Blocked operation messages: `[sandflox] BLOCKED: <description>` — designed to be parseable by AI agents
- Flox executive logs: `.flox/log/executive.*.log.*` (auto-generated by Flox, not by sandflox)

## CI/CD & Deployment

**Hosting:** GitHub — `https://github.com/8BitTacoSupreme/sandflox.git`

**CI Pipeline:** Not detected. No `.github/workflows/`, `Makefile`, `Dockerfile`, or CI configuration files present.

**Distribution:** `git clone` + `chmod +x sandflox`. No package registry, no container image, no binary releases.

## Webhooks & Callbacks

**Incoming:** None
**Outgoing:** None

## Environment Configuration

**Required environment variables (set automatically by Flox/sandflox):**
- `FLOX_ENV` — Set by `flox activate`; path to Nix environment
- `FLOX_ENV_CACHE` — Set by `flox activate`; path to cache directory
- `SANDFLOX_ENABLED` — Set by `manifest.toml` vars; value `"1"`
- `SANDFLOX_MODE` — Set by `manifest.toml` vars; value `"enforced"`

**Optional environment variables (user-configurable):**
- `SANDFLOX_PROFILE` — Override active profile; values: `minimal`, `default`, `full`

**Environment variables set during enforcement:**
- `PATH` — Wiped to `$FLOX_ENV/bin`, then narrowed to `$FLOX_ENV_CACHE/sandflox/bin`
- `PYTHON_NOPIP` — Set to `"1"`
- `PYTHONDONTWRITEBYTECODE` — Set to `"1"`
- `PYTHONPATH` — Prepended with sandflox-python cache path
- `PYTHONUSERBASE` — Set to sandflox-python cache path
- `ENABLE_USER_SITE` — Set to `"1"`

**Environment variables scrubbed (security):**
- `FLOX_ENV_PROJECT` — Removed to prevent agents from discovering project root
- `FLOX_ENV_DIRS` — Removed to prevent agents from discovering environment paths
- `FLOX_PATH_PATCHED` — Removed (internal Flox state)

**Secrets location:** No secrets. `.env` files listed in `policy.toml` `read-only` array are protected from writes but sandflox itself does not use environment secrets.

---

*Integration audit: 2026-04-15*
