# sandflox

Immutable agent sandbox powered by [Flox](https://flox.dev).

The agent gets the plastic shovel and pail. The bulldozer stays in the shed.

## Problem

AI coding agents (Claude Code, Cursor, Copilot, etc.) can run shell commands. Even when prompted not to, they can `flox install`, `pip install`, `npm install`, or `brew install` — mutating the environment out from under you. Prompting isn't security. You need technical enforcement.

## Architecture

sandflox v2 provides two enforcement tiers that compose:

```
policy.toml (declarative, version-controlled, team-sharable via FloxHub)
    │
    ▼
./sandflox (wrapper script)
    ├── macOS: policy.toml → .sb profile → sandbox-exec
    └── Linux: policy.toml → bwrap flags → bwrap
    │
    ▼
flox activate (inside the sandbox)
    → hook:    PATH wipe, breadcrumb cleanup, python ensurepip block
    → profile: binary whitelist, function armor, fs-filter

┌──────────────────────────────────────────────────────────────────┐
│  Tier 1 — Kernel (sandbox-exec / bwrap)                         │
│  Blocks: redirects, absolute-path binaries, raw socket I/O      │
│  Enforcement: filesystem writes, network, denied paths           │
├──────────────────────────────────────────────────────────────────┤
│  Tier 2 — Shell (flox activate hooks + profile)                  │
│  Blocks: package managers, escape vectors, breadcrumb discovery  │
│  Enforcement: PATH wipe, requisites filter, function armor       │
└──────────────────────────────────────────────────────────────────┘
```

| Tier | macOS | Linux | Intercepts redirects? | Intercepts abs paths? |
|------|-------|-------|----------------------|----------------------|
| Shell (`flox activate`) | PATH + functions | PATH + functions | No | No |
| Kernel (`./sandflox`) | sandbox-exec SBPL | bwrap namespaces | Yes | Yes |

## Quick start

### Hardened activation (recommended for agent sessions)

```bash
git clone https://github.com/8BitTacoSupreme/sandflox.git
cd sandflox
chmod +x sandflox

# Interactive shell with kernel enforcement
./sandflox

# Run a command
./sandflox -- bash agent-script.sh

# Override profile
SANDFLOX_PROFILE=minimal ./sandflox -- python3 agent.py
```

### Shell-only activation (no kernel enforcement)

```bash
flox activate
# [sandflox] Sandbox active: 55 tools (profile: default)
```

### Verification

```bash
# Full test suite (kernel + shell)
./sandflox -- bash test-policy.sh    # 29 passed, 0 failed

# Legacy test suite
./sandflox -- bash test-sandbox.sh   # 35 passed, 0 failed
```

## `policy.toml` — Declarative Policy

The policy file controls network access, filesystem restrictions, and profile selection.

```toml
[meta]
version = "2"
profile = "default"              # selects from [profiles.*]

[network]
mode = "blocked"                 # "unrestricted" | "blocked"
allow-localhost = true           # allow loopback even when blocked

[filesystem]
mode = "workspace"               # "permissive" | "workspace" | "strict"
writable = [".", "/tmp"]
read-only = [".flox/env/", ".git/", ".env", "policy.toml", "requisites.txt"]
denied = ["~/.ssh/", "~/.gnupg/", "~/.aws/", "~/.config/gcloud/", "~/.config/gh/"]

[profiles.minimal]
requisites = "requisites-minimal.txt"
network = "blocked"
filesystem = "strict"

[profiles.default]
requisites = "requisites.txt"
network = "blocked"
filesystem = "workspace"

[profiles.full]
requisites = "requisites-full.txt"
network = "unrestricted"
filesystem = "permissive"
```

### Profile resolution

1. `$SANDFLOX_PROFILE` env var (highest priority)
2. `policy.toml [meta] profile`
3. `"default"`

### Network modes

| Mode | Behavior |
|------|----------|
| `blocked` | All TCP/UDP denied at kernel level. Unix sockets allowed (nix daemon). Localhost allowed if `allow-localhost = true`. curl removed from PATH. |
| `unrestricted` | No network restrictions. |

Domain-level filtering requires a proxy and is out of scope. Shell-level curl wrappers remain as defense-in-depth.

### Filesystem modes

| Mode | Behavior |
|------|----------|
| `workspace` | Writes allowed to project dir + /tmp. Read-only overrides for .git, .flox/env, etc. Denied paths blocked at kernel level. |
| `strict` | No user-writable paths. Only temp dirs for shell operation. |
| `permissive` | No write restrictions. |

### Requisites profiles

| Profile | Tools | Use case |
|---------|-------|----------|
| `requisites-minimal.txt` | ~28 | Read-only inspection. No cp/mv/rm/curl/git. Untrusted agents. |
| `requisites.txt` | ~55 | Default. Shell utils, text processing, python3, jq, curl, git. |
| `requisites-full.txt` | ~54+ | All non-manager binaries. Trusted agents. |

## What gets blocked

| Category | Blocked commands |
|----------|-----------------|
| System package managers | `flox`, `nix`, `nix-env`, `nix-store`, `nix-shell`, `nix-build`, `apt`, `apt-get`, `yum`, `dnf`, `brew`, `snap`, `flatpak` |
| Language package managers | `pip`, `pip3`, `npm`, `npx`, `yarn`, `pnpm`, `cargo`, `go`, `gem`, `composer`, `uv` |
| Container tools | `docker`, `podman` |
| Python escape vectors | `python3 -m pip`, `python3 -m ensurepip`, `python3 -m venv` |
| Filesystem (kernel) | Writes outside workspace, reads to `~/.ssh`, `~/.gnupg`, `~/.aws` |
| Network (kernel) | All outbound TCP/UDP when `network = "blocked"` |

## Customization

### Add a tool

Add the binary name to `requisites.txt`:
```
# --- My additions ---
wget
tree
```

If the binary comes from a package not yet installed, add it to `[install]` in the manifest:
```bash
# Outside the sandbox (before activating):
flox edit
# Add: wget.pkg-path = "wget"
```

### Remove a tool

Delete or comment out the line in `requisites.txt`:
```
# curl    ← commented out, agent can no longer fetch URLs
```

### Tune the policy

Edit `policy.toml` to change network mode, filesystem mode, or denied paths. Changes take effect on next `./sandflox` invocation.

## How the enforcement layers work

### Kernel tier (./sandflox wrapper)

- **macOS**: Generates an SBPL profile from `policy.toml` and invokes `sandbox-exec`. Enforces filesystem writes, network, and denied path blocking at the kernel level.
- **Linux**: Generates bwrap flags from `policy.toml`. Uses namespaces for filesystem isolation (`--ro-bind`), network isolation (`--unshare-net`), and process isolation (`--unshare-pid`).

### Shell tier (flox activate)

- **Layer 1 (hook)**: PATH wipe — only `$FLOX_ENV/bin` survives
- **Layer 2 (profile/entrypoint)**: `requisites.txt` filter — symlink bin directory with only listed tools
- **Layer 3 (profile/entrypoint)**: Function armor — 27 package managers shadowed with exit 126
- **Layer 4 (hook)**: Breadcrumb cleanup — scrub `FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`
- **Layer 5 (hook)**: Policy staging — parse `policy.toml`, generate `fs-filter.sh`, stage config
- **fs-filter.sh**: Shell wrappers for `cp`, `mv`, `mkdir`, `rm`, etc. that check write targets against policy. Provides clear `[sandflox] BLOCKED: write to '~/.ssh' outside workspace policy` error messages.
- **usercustomize.py**: Blocks `ensurepip`, wraps `builtins.open` for write-mode path checking.

### Why both tiers?

Kernel enforcement returns generic "Operation not permitted". Shell enforcement returns `[sandflox] BLOCKED: ...` — agents can understand and adapt. Users who run `flox activate` directly (without `./sandflox`) still get shell-level protection.

## Backward compatibility

No `policy.toml` → all v2 code is skipped. The hook checks `[ -f "${FLOX_ENV_PROJECT}/policy.toml" ]` before any policy work. Existing v1 behavior (PATH wipe + requisites + function armor) is unchanged.

## sandflox vs flox-bwrap

sandflox v2 achieves kernel-level parity with [flox-bwrap](https://github.com/8BitTacoSupreme/flox-bwrap) on macOS via `sandbox-exec`, and uses bwrap directly on Linux.

| | sandflox v2 | flox-bwrap |
|---|---|---|
| **Isolation** | Kernel + shell (two tiers) | Kernel (Linux namespaces) |
| **Platform** | macOS + Linux | Linux only |
| **Blocks `flox install`** | Yes (PATH + function armor + kernel) | Yes (read-only nix store bind) |
| **Blocks network** | Yes (`sandbox-exec` / `bwrap --unshare-net`) | Yes (`--unshare-all`, opt-in `--net`) |
| **Blocks filesystem writes** | Yes (SBPL deny / `--ro-bind`) | Yes (`--ro-bind`) |
| **Blocks absolute path escape** | Yes (kernel denies reads to sensitive paths) | Yes (unmounted paths don't exist) |
| **Blocks `>` redirects** | Yes (kernel write deny) | Yes (read-only bind mount) |
| **Declarative policy** | Yes (`policy.toml` — profiles, modes, denied paths) | No (flags only) |
| **Agent-friendly errors** | Yes (`[sandflox] BLOCKED: ...` via shell tier) | No (generic EPERM) |
| **Setup** | `chmod +x sandflox && ./sandflox` | Requires Go build + bwrap |
| **Best for** | macOS + Linux, agent sandboxing, team policy | Linux-only, minimal attack surface |

They're complementary. sandflox is the declarative policy layer with cross-platform kernel enforcement. flox-bwrap is the Linux-native namespace approach.

## Requirements

- [Flox](https://flox.dev) 1.10+
- Python 3.6+ (system Python for `./sandflox` wrapper; tomllib used on 3.11+, inline parser fallback for older)
- macOS: `sandbox-exec` (included with macOS)
- Linux: `bwrap` (bubblewrap) for kernel enforcement
