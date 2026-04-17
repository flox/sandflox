# sandflox

macOS-native sandbox for AI coding agents, distributed as a [Flox](https://flox.dev) package.

The agent gets the plastic shovel and pail. The bulldozer stays in the shed.

## Problem

AI coding agents (Claude Code, Cursor, Copilot, etc.) can run shell commands. Even when prompted not to, they can `flox install`, `pip install`, `npm install`, or `brew install` — mutating your environment out from under you. Prompting isn't security. You need technical enforcement.

sandflox wraps `flox activate` under Apple's `sandbox-exec`, enforcing what tools an agent can reach (shell enforcement) and what it can mutate (kernel enforcement). Driven by a declarative `policy.toml`. Zero external dependencies. Single static Go binary.

## Install

```bash
flox install 8BitTacoSupreme/sandflox
```

That's it. The `sandflox` binary is now available inside any `flox activate` session.

## Quick Start

### 1. Create a policy

Drop a `policy.toml` in your project root (or copy the example from this repo):

```toml
[meta]
version = "2"
profile = "default"

[network]
mode = "blocked"
allow-localhost = true

[filesystem]
mode = "workspace"
writable = [".", "/tmp"]
read-only = [".flox/env/", ".git/", ".env", "policy.toml"]
denied = ["~/.ssh/", "~/.gnupg/", "~/.aws/", "~/.config/gcloud/", "~/.config/gh/"]

[security]
env-passthrough = []

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

### 2. Validate your policy (dry run)

```bash
sandflox validate
```

Prints what would be enforced — profile, filesystem mode, network mode, denied paths, tool count — without launching a sandbox.

```bash
sandflox validate -debug
```

Adds SBPL rule count, cache artifact details, and full diagnostic output.

### 3. Launch a sandboxed shell

```bash
# Interactive shell with kernel enforcement
sandflox

# Run a single command under sandbox
sandflox -- echo "hello from the sandbox"

# Override profile
sandflox -profile minimal

# Wrap an agent session
sandflox -- claude-code
```

### 4. Verify enforcement

Inside the sandbox:

```bash
# PATH is restricted to allowed tools only
echo $PATH                    # only the sandflox symlink bin directory

# Package managers are blocked
which pip                     # not found
pip install requests          # [sandflox] BLOCKED: pip is not available in this sandbox

# Filesystem writes outside workspace are blocked
cp /etc/hosts /tmp/stolen     # [sandflox] BLOCKED: write to '/etc/hosts' outside workspace policy

# Network is blocked at the kernel level
curl https://example.com      # Operation not permitted

# Credentials are scrubbed
echo $AWS_SECRET_ACCESS_KEY   # empty (even if set in parent shell)
echo $HOME                    # preserved (allowlisted)
```

### 5. Check status from inside the sandbox

```bash
sandflox status
```

Reports the active profile, blocked paths, allowed tools, and network mode from cached state.

### 6. Elevate an existing flox session

Already inside `flox activate` but want sandbox enforcement?

```bash
sandflox elevate
```

Re-execs the shell under `sandbox-exec`. Running it again prints "already sandboxed" instead of nesting.

## Architecture

```
policy.toml (declarative, version-controlled)
    │
    ▼
sandflox (Go binary)
    ├── Parse policy.toml → ResolvedConfig
    ├── Generate shell artifacts (entrypoint.sh, fs-filter.sh, usercustomize.py)
    ├── Generate SBPL profile (macOS kernel sandbox rules)
    ├── Sanitize environment (scrub credentials)
    └── exec sandbox-exec ... flox activate -- bash

┌──────────────────────────────────────────────────────────────────┐
│  Tier 1 — Kernel (sandbox-exec SBPL)                             │
│  Blocks: redirects, absolute-path binaries, raw socket I/O       │
│  Enforcement: filesystem writes, network, denied paths            │
├──────────────────────────────────────────────────────────────────┤
│  Tier 2 — Shell (generated bash artifacts)                        │
│  Blocks: package managers, escape vectors, breadcrumb discovery   │
│  Enforcement: PATH wipe, requisites filter, function armor        │
├──────────────────────────────────────────────────────────────────┤
│  Tier 3 — Environment                                             │
│  Blocks: credential leakage, Python pip bootstrap                 │
│  Enforcement: allowlist-based env scrubbing, forced safety flags   │
└──────────────────────────────────────────────────────────────────┘
```

### Why both tiers?

Kernel enforcement returns generic "Operation not permitted". Shell enforcement returns `[sandflox] BLOCKED: ...` — agents can understand and adapt. Environment scrubbing prevents credential leakage even if the agent never hits a kernel deny.

## CLI Reference

```
sandflox [flags] [-- command args...]
sandflox <subcommand> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-policy <path>` | Path to policy.toml (default: `./policy.toml`) |
| `-profile <name>` | Override active profile (beats env var and policy file) |
| `-net` | Override network to unrestricted |
| `-debug` | Emit verbose diagnostics to stderr |

### Subcommands

| Command | Description |
|---------|-------------|
| `sandflox validate` | Dry-run — print what would be enforced without launching a sandbox |
| `sandflox status` | Report active enforcement state from inside a sandboxed session |
| `sandflox elevate` | Re-exec current flox session under sandbox-exec |

### Profile Resolution

1. `-profile` CLI flag (highest priority)
2. `$SANDFLOX_PROFILE` env var
3. `policy.toml [meta] profile`
4. `"default"`

## What Gets Blocked

| Category | Blocked |
|----------|---------|
| System package managers | `flox`, `nix`, `nix-env`, `nix-store`, `nix-shell`, `nix-build`, `apt`, `apt-get`, `yum`, `dnf`, `brew`, `snap`, `flatpak` |
| Language package managers | `pip`, `pip3`, `npm`, `npx`, `yarn`, `pnpm`, `cargo`, `go`, `gem`, `composer`, `uv` |
| Container tools | `docker`, `podman` |
| Python escape vectors | `python3 -m pip`, `python3 -m ensurepip`, `python3 -m venv` |
| Filesystem (kernel) | Writes outside workspace, reads to `~/.ssh`, `~/.gnupg`, `~/.aws` |
| Network (kernel) | All outbound TCP/UDP when `network = "blocked"` |
| Environment | `AWS_*`, `GITHUB_*`, `SSH_*`, `GCP_*`, and 20+ credential-carrying prefixes |

## Network Modes

| Mode | Behavior |
|------|----------|
| `blocked` | All TCP/UDP denied at kernel level. Unix sockets allowed (nix daemon). Localhost allowed if `allow-localhost = true`. curl removed from PATH. |
| `unrestricted` | No network restrictions. |

## Filesystem Modes

| Mode | Behavior |
|------|----------|
| `workspace` | Writes allowed to project dir + /tmp. Read-only overrides for .git, .flox/env, etc. Denied paths blocked at kernel level. |
| `strict` | No user-writable paths. Only temp dirs for shell operation. |
| `permissive` | No write restrictions. |

## Requisites Profiles

| Profile | Tools | Use case |
|---------|-------|----------|
| `requisites-minimal.txt` | ~28 | Read-only inspection. No cp/mv/rm/curl/git. Untrusted agents. |
| `requisites.txt` | ~55 | Default. Shell utils, text processing, python3, jq, curl, git. |
| `requisites-full.txt` | ~54+ | All non-manager binaries. Trusted agents needing find, touch, etc. |

## Customization

### Add a tool

Add the binary name to `requisites.txt`:

```
# My additions
wget
tree
```

If the tool isn't in your Flox environment yet:

```bash
flox install wget
```

### Allow a credential through

Add it to `policy.toml`:

```toml
[security]
env-passthrough = ["ANTHROPIC_API_KEY"]
```

### Tune filesystem policy

Edit `policy.toml` to change modes, writable paths, or denied paths. Changes take effect on next `sandflox` invocation.

## Requirements

- macOS (Darwin) — sandbox-exec is macOS-specific
- [Flox](https://flox.dev) 1.10+

## sandflox vs alternatives

| | sandflox | flox-bwrap | devcontainer |
|---|---|---|---|
| **Platform** | macOS | Linux | Any (VM) |
| **Isolation** | Kernel + shell + env | Kernel (namespaces) | Full VM |
| **Setup** | `flox install` | Go build + bwrap | Docker + config |
| **Declarative policy** | Yes (`policy.toml`) | No (flags only) | Partial (devcontainer.json) |
| **Agent-friendly errors** | Yes (`[sandflox] BLOCKED:`) | No (generic EPERM) | No |
| **Performance** | Native (no VM) | Native | VM overhead |
| **Credential scrubbing** | Yes (allowlist-based) | No | Manual |

## License

MIT
