# sandflox

macOS-native sandbox for AI coding agents, distributed as a [Flox](https://flox.dev) package.

The agent gets the plastic shovel and pail. The bulldozer stays in the shed.

## Problem

AI coding agents (Claude Code, Cursor, Copilot, etc.) run shell commands. Even when prompted not to, they can `pip install`, `npm install`, `brew install`, or `flox install` — mutating your environment out from under you. Shell redirections (`> /etc/passwd`) bypass any command wrapper. Prompting isn't security. You need technical enforcement.

sandflox provides two tiers of enforcement, each effective independently, strongest combined:

- **Shell tier** — PATH wipe, requisites filtering, function armor, fs-filter wrappers, Python enforcement. Provides clear `[sandflox] BLOCKED:` messages agents can understand.
- **Kernel tier** — Apple `sandbox-exec` (SBPL) denies filesystem writes, network sockets, and denied-path reads at the syscall level. Catches shell redirections and absolute-path binaries that bypass shell enforcement.

Driven by a declarative `policy.toml`. Zero external dependencies. Single static Go binary.

## Two Artifacts

sandflox ships as two complementary Flox artifacts:

| Artifact | What it is | Install |
|----------|-----------|---------|
| `flox/sandflox` | Go binary (FloxHub package) | `flox install flox/sandflox` |
| `flox/flox-sbx` | Hardened Flox environment (FloxHub environment) | `flox pull flox/flox-sbx` |

**The binary** provides CLI commands (`sandflox`, `sandflox validate`, `sandflox elevate`, etc.) and generates enforcement artifacts. **The environment** wires everything together — `flox activate` gives you shell enforcement out of the box, and `sandflox elevate` adds kernel enforcement on top.

**Platform:** aarch64-darwin (ARM Mac). The sandflox binary is currently published for ARM Macs only.

## Quick Start (Zero Config)

```bash
# Pull the pre-configured sandbox environment
flox pull flox/flox-sbx

# Activate — shell enforcement is immediate
flox activate

# Add kernel enforcement (sandbox-exec)
sandflox elevate
```

That's it. No `policy.toml` needed — the embedded default policy applies automatically (workspace filesystem mode, blocked network, ~57 tools).

## Quick Start (Custom Policy)

### 1. Create a policy

```bash
sandflox init
```

This writes a default `policy.toml` to the current directory:

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
# [sandflox] Policy: policy.toml (valid)
# [sandflox] Profile: default | Network: blocked | Filesystem: workspace
# [sandflox] Tools: 57 (from requisites.txt)
# [sandflox] Denied paths: 5
```

Add `-debug` for SBPL rule count and full diagnostic output.

### 3. Launch a sandboxed shell

```bash
# Full sandbox (kernel + shell enforcement)
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

# Package managers are blocked (function armor, exit code 126)
pip install requests          # [sandflox] BLOCKED: pip is not available. Environment is immutable.

# Filesystem writes outside workspace are blocked
cp /etc/hosts /usr/local/x    # [sandflox] BLOCKED: write to "/usr/local/x" outside workspace policy
echo pwned > /etc/test        # kernel EPERM — caught at syscall level

# Network is blocked at the kernel level
python3 -c "import socket; socket.create_connection(('example.com', 80))"
                              # OSError: [Errno 1] Operation not permitted

# Credentials are scrubbed
echo $AWS_SECRET_ACCESS_KEY   # empty (even if set in parent shell)
echo $HOME                    # preserved (allowlisted)
```

### 5. Check status from inside the sandbox

```bash
sandflox status
# [sandflox] Profile: default | Network: blocked | Filesystem: workspace
# [sandflox] Tools: 57 | Denied paths: 5
```

### 6. Elevate an existing flox session

Already inside `flox activate` but want kernel enforcement?

```bash
sandflox elevate
# [sandflox] Elevating to sandboxed shell (sandbox-exec)
```

Re-execs the shell under `sandbox-exec`. No `-policy` flag needed when using `flox-sbx` — the embedded default policy is used automatically. Running `sandflox elevate` again prints "Already sandboxed -- nothing to do." instead of nesting.

## Enforcement Tiers

### Tier 1: Shell (agent-readable)

Active immediately on `flox activate` (via `flox-sbx` hooks) or when `sandflox` launches.

| Layer | What it does | How |
|-------|-------------|-----|
| PATH wipe | Remove all system dirs from PATH | `export PATH="$FLOX_ENV/bin"` |
| Requisites filter | Restrict PATH to whitelisted binaries only | Symlinks from `$FLOX_ENV/bin/<tool>` into sandflox bin dir |
| Function armor | Block 26 package managers by name | Shell functions returning exit 126 with `[sandflox] BLOCKED:` |
| fs-filter | Check write targets against policy | Wrappers around cp, mv, mkdir, rm, rmdir, ln, chmod, tee |
| Python enforcement | Block ensurepip, wrap `builtins.open()` | `usercustomize.py` with path checking |
| Env scrubbing | Remove credential-carrying env vars | Allowlist-based — only safe vars pass through |
| Breadcrumb cleanup | Hide flox internals from agent | Unset `FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`, `FLOX_PATH_PATCHED` |

Shell enforcement returns `[sandflox] BLOCKED: <reason>` — agents can parse and adapt.

### Tier 2: Kernel (escape-proof)

Active when `sandflox` launches directly or after `sandflox elevate`.

| What it blocks | Mechanism |
|---------------|-----------|
| Filesystem writes outside workspace | SBPL `deny file-write*` rules |
| Reads to denied paths (~/.ssh, ~/.aws, etc.) | SBPL `deny file-read*` rules |
| All outbound TCP/UDP (when network=blocked) | SBPL `deny network*` rules |
| Shell redirections (`>`, `>>`, `\|`) to protected paths | Caught at syscall level — bash can't bypass |
| Absolute-path binaries (`/usr/bin/curl`) | Process not in allowed file-read paths |

Kernel enforcement returns generic "Operation not permitted" — no information leakage.

### Why both tiers?

Shell redirections (`>`, `>>`, `|`) are handled by bash before any command runs and cannot be intercepted at the shell tier. Kernel enforcement blocks these at the syscall level.

Conversely, kernel enforcement returns opaque EPERM errors. Shell enforcement provides actionable `[sandflox] BLOCKED:` messages that help agents understand what they can and can't do.

The two tiers are complementary: shell for UX, kernel for security.

### Elevation workflow

```
flox activate (flox-sbx)       sandflox elevate
        |                            |
        v                            v
  Shell enforcement           Kernel enforcement
  (PATH, armor, fs-filter)    (sandbox-exec SBPL)
        |                            |
        +------- combined -----------+
        |
        v
  Full sandbox (shell + kernel + env scrubbing)
```

`flox activate` applies shell-tier enforcement via the `sandflox prepare` hook. `sandflox elevate` re-execs the shell under `sandbox-exec` to add kernel-tier enforcement. Both tiers are effective independently; combined they provide defense in depth.

## CLI Reference

```
sandflox [flags] [-- command args...]
sandflox <subcommand> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-policy <path>` | Path to policy.toml (default: `./policy.toml`; falls back to embedded default) |
| `-profile <name>` | Override active profile (beats env var and policy file) |
| `-net` | Override network to unrestricted |
| `-debug` | Emit verbose diagnostics to stderr |

### Subcommands

| Command | Description |
|---------|-------------|
| `sandflox init` | Write a default `policy.toml` to the current directory |
| `sandflox validate` | Dry-run — print what would be enforced without launching a sandbox |
| `sandflox prepare` | Generate enforcement artifacts without launching a sandbox (used by flox-sbx hooks) |
| `sandflox status` | Report active enforcement state from inside a sandboxed session |
| `sandflox elevate` | Re-exec current shell under sandbox-exec to add kernel enforcement |

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
| Filesystem (shell) | Write commands (cp, mv, mkdir, rm, ln, chmod, tee) checked against policy |
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
| `workspace` | Writes allowed to project dir + /tmp. Read-only overrides for .git, .flox/env, policy.toml. Denied paths blocked at kernel level. |
| `strict` | No user-writable paths. Only temp dirs for shell operation. |
| `permissive` | No write restrictions. |

## Requisites Profiles

| Profile | Tools | Use case |
|---------|-------|----------|
| `requisites-minimal.txt` | ~28 | Read-only inspection. No cp/mv/rm/curl/git. Untrusted agents. |
| `requisites.txt` | ~57 | Default. Shell utils, text processing, python3, jq, curl, git. |
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

Edit `policy.toml` to change modes, writable paths, or denied paths. Changes take effect on next `sandflox` invocation or `flox activate`.

## Requirements

- macOS (Darwin) — sandbox-exec is macOS-specific
- aarch64-darwin (ARM Mac) for the sandflox binary
- [Flox](https://flox.dev) 1.10+
- Your Flox environment must include the tools sandflox will whitelist. The `sandflox`
  binary enforces policy over tools already present in `$FLOX_ENV/bin` — it does not
  install them. At minimum: `flox install bash coreutils python3 jq curl git`

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
