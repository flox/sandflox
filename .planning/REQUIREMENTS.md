# Requirements: sandflox

**Defined:** 2026-04-15
**Core Value:** AI agents cannot escape the sandbox -- not through PATH manipulation, absolute paths, shell redirects, or kernel syscalls -- without requiring a Linux VM or devcontainer.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Core Binary

- [x] **CORE-01**: sandflox is a single Go binary with zero external dependencies (stdlib only)
- [x] **CORE-02**: sandflox parses policy.toml v2 schema using a custom Go TOML subset parser (sections, dotted sections, strings, booleans, string arrays)
- [x] **CORE-03**: sandflox resolves profiles via precedence: `$SANDFLOX_PROFILE` env var > `policy.toml [meta] profile` > `"default"`
- [x] **CORE-04**: sandflox merges profile overrides with top-level `[network]` and `[filesystem]` settings
- [x] **CORE-05**: sandflox supports CLI flags `--net`, `--profile <name>`, `--policy <path>`, `--debug`, `--requisites <file>` that override policy.toml values
- [x] **CORE-06**: sandflox writes resolved config, path lists (writable, read-only, denied), and generated artifacts to `.flox/cache/sandflox/`
- [x] **CORE-07**: sandflox emits `[sandflox]` prefixed diagnostic messages to stderr (active profile, network mode, filesystem mode, tool count)

### Kernel Enforcement

- [x] **KERN-01**: sandflox generates Apple SBPL profiles from resolved policy -- filesystem modes (workspace/strict/permissive), network modes (blocked/unrestricted), denied path blocking
- [x] **KERN-02**: sandflox uses `(allow default)` SBPL baseline with selective denials (not deny-default) to support Nix store, macOS frameworks, and system libraries
- [x] **KERN-03**: sandflox resolves `~` and `.` to absolute paths in SBPL rules, using `/private/tmp` instead of `/tmp` for macOS symlink correctness
- [x] **KERN-04**: sandflox wraps `flox activate` under `sandbox-exec -f <profile>` using `syscall.Exec` for clean process replacement (no child process)
- [x] **KERN-05**: `sandflox` (no args) launches an interactive sandboxed shell via `sandbox-exec ... flox activate`
- [x] **KERN-06**: `sandflox -- CMD` wraps arbitrary commands via `sandbox-exec ... flox activate -- CMD`
- [x] **KERN-07**: sandflox allows localhost connections when `network.allow-localhost = true` even when network is blocked
- [x] **KERN-08**: sandflox allows Unix socket communication (Nix daemon) when network is blocked

### Shell Enforcement

- [x] **SHELL-01**: sandflox wipes PATH to contain only the requisites-filtered symlink bin directory
- [x] **SHELL-02**: sandflox parses the active requisites file and creates a symlink bin directory in `.flox/cache/sandflox/bin/` containing only listed tools from `$FLOX_ENV/bin`
- [x] **SHELL-03**: sandflox generates function armor shell code shadowing 27+ package managers (flox, nix, brew, pip, npm, cargo, docker, etc.) with exit 126 and `[sandflox] BLOCKED:` messages
- [x] **SHELL-04**: sandflox generates fs-filter.sh wrapping write commands (cp, mv, mkdir, rm, rmdir, ln, chmod, tee) with path-checking functions that enforce writable/read-only/denied policy
- [x] **SHELL-05**: sandflox generates usercustomize.py that blocks `ensurepip` at import level and wraps `builtins.open` to enforce filesystem policy for Python code
- [x] **SHELL-06**: sandflox scrubs environment breadcrumbs (`FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`, `FLOX_PATH_PATCHED`) to prevent agent escape vector discovery
- [x] **SHELL-07**: sandflox conditionally removes `curl` from the symlink bin when `network.mode = "blocked"`
- [x] **SHELL-08**: shell enforcement generates agent-friendly `[sandflox] BLOCKED: <reason>` error messages that explain why an action was denied

### Distribution

- [x] **DIST-01**: sandflox builds via `flox build` using a `.flox/pkgs/sandflox.nix` expression with `buildGoModule` and `vendorHash = null`
- [ ] **DIST-02**: sandflox publishes to FloxHub via `flox publish`
- [ ] **DIST-03**: sandflox is installable into any Flox environment via `flox install sandflox`
- [x] **DIST-04**: The sandflox build manifest is minimal -- only `go` in `[install]`, no hooks or profile scripts
- [ ] **DIST-05**: sandflox Nix expression uses `lib.fileset.toSource` for hermetic source selection and `-trimpath` in build flags

### Subcommands

- [x] **CMD-01**: `sandflox validate` parses policy.toml, generates SBPL (dry-run), and reports what would be enforced without executing
- [x] **CMD-02**: `sandflox status` reads cached enforcement state and reports active profile, blocked paths, allowed tools, network mode
- [ ] **CMD-03**: `sandflox elevate` from within a `flox activate` session re-execs the current shell under sandbox-exec with generated SBPL (one-time bounce with re-entry detection)

### Security

- [x] **SEC-01**: sandflox scrubs environment variables before passing them into the sandbox -- only allowlisted vars pass through (HOME, USER, TERM, SHELL, LANG, PATH, plus Flox-required vars)
- [x] **SEC-02**: sandflox blocks sensitive env vars by default (AWS_*, SSH_*, GPG_*, GITHUB_TOKEN, etc.) with a configurable allowlist in policy.toml
- [x] **SEC-03**: sandflox sets `PYTHONDONTWRITEBYTECODE=1` and `PYTHON_NOPIP=1` inside the sandbox

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Advanced Security

- **ADV-01**: Audit log -- structured logging of all blocked actions to a file for post-session review
- **ADV-02**: Domain-level network filtering via proxy (HTTP + SOCKS5) for allowlist-based network access
- **ADV-03**: Policy composition/inheritance -- base + project + user override policies
- **ADV-04**: Process execution control via SBPL -- restrict which binaries can be spawned by absolute path

### Operational

- **OPS-01**: Init/setup script support -- run a pre-sandbox setup script for workspace preparation
- **OPS-02**: Configuration precedence model (managed > user > project) like Claude Code settings

## Out of Scope

| Feature | Reason |
|---------|--------|
| Linux support | flox-bwrap handles Linux; sandflox is macOS-native with sandbox-exec |
| Windows/WSL support | Different sandbox technology entirely; out of scope permanently |
| Container/VM isolation | The entire point is avoiding the Linux VM layer on macOS |
| GUI/TUI interface | CLI-only, Flox ergonomics family |
| Agent-specific integrations | sandflox is agent-agnostic; agents run inside it |
| Mutable mode (package install inside sandbox) | Contradicts core value of immutable environments |
| Interactive policy editor | policy.toml is 40 lines of human-readable TOML; `sandflox validate` confirms correctness |
| Real-time process monitoring daemon | Architecturally different from sandbox-exec wrapping; out of scope |
| Nested sandbox support | Weakens security; if users need Docker, they run it outside the sandbox |
| Domain-level network filtering (v1) | Requires proxy architecture (~2000 lines); deferred to v2 |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CORE-01 | Phase 1 | Complete |
| CORE-02 | Phase 1 | Complete |
| CORE-03 | Phase 1 | Complete |
| CORE-04 | Phase 1 | Complete |
| CORE-05 | Phase 1 | Complete |
| CORE-06 | Phase 1 | Complete |
| CORE-07 | Phase 1 | Complete |
| KERN-01 | Phase 2 | Complete |
| KERN-02 | Phase 2 | Complete |
| KERN-03 | Phase 2 | Complete |
| KERN-04 | Phase 2 | Complete |
| KERN-05 | Phase 2 | Complete |
| KERN-06 | Phase 2 | Complete |
| KERN-07 | Phase 2 | Complete |
| KERN-08 | Phase 2 | Complete |
| SHELL-01 | Phase 3 | Complete |
| SHELL-02 | Phase 3 | Complete |
| SHELL-03 | Phase 3 | Complete |
| SHELL-04 | Phase 3 | Complete |
| SHELL-05 | Phase 3 | Complete |
| SHELL-06 | Phase 3 | Complete |
| SHELL-07 | Phase 3 | Complete |
| SHELL-08 | Phase 3 | Complete |
| SEC-01 | Phase 4 | Complete |
| SEC-02 | Phase 4 | Complete |
| SEC-03 | Phase 4 | Complete |
| CMD-01 | Phase 5 | Complete |
| CMD-02 | Phase 5 | Complete |
| CMD-03 | Phase 5 | Pending |
| DIST-01 | Phase 1 | Complete |
| DIST-02 | Phase 6 | Pending |
| DIST-03 | Phase 6 | Pending |
| DIST-04 | Phase 1 | Complete |
| DIST-05 | Phase 6 | Pending |

**Coverage:**
- v1 requirements: 34 total
- Mapped to phases: 34
- Unmapped: 0

---
*Requirements defined: 2026-04-15*
*Last updated: 2026-04-15 after roadmap creation*
