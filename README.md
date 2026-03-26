# sandflox

Immutable agent sandbox powered by [Flox](https://flox.dev).

The agent gets the plastic shovel and pail. The bulldozer stays in the shed.

## Problem

AI coding agents (Claude Code, Cursor, Copilot, etc.) can run shell commands. Even when prompted not to, they can `flox install`, `pip install`, `npm install`, or `brew install` — mutating the environment out from under you. Prompting isn't security. You need technical enforcement.

## How it works

sandflox uses Flox's activation hooks and profile scripts to build a layered defense that makes the environment genuinely immutable from the agent's perspective.

```
┌─────────────────────────────────────────────────────┐
│  Layer 1 (hook)     PATH = $FLOX_ENV/bin only       │  ← blocks ALL system binaries
│  Layer 2 (profile)  requisites.txt binary filter    │  ← narrows to specific tools
│  Layer 3 (profile)  shell function armor (27 cmds)  │  ← defense in depth
│  Layer 4 (hook+profile)  env var breadcrumb scrub   │  ← removes escape routes
│  Layer 5 (optional) flox containerize               │  ← nuclear option
└─────────────────────────────────────────────────────┘
```

**Two levels of granularity:**

- **`[install]` in manifest.toml** — package-level whitelist (what Flox packages exist)
- **`requisites.txt`** — binary-level whitelist (which executables the agent can actually run)

## Quick start

```bash
# Clone and activate
git clone https://github.com/8BitTacoSupreme/sandflox.git
cd sandflox
flox activate

# In interactive mode, you'll see:
# [sandflox] Sandbox active: 54 tools from requisites.txt

# Verify it works
flox install something    # → BLOCKED
pip install requests      # → BLOCKED
python3 -c "print('hi')" # → works
curl https://example.com  # → works
git status                # → works
```

For non-interactive use (CI, scripted agents):

```bash
flox activate -- bash my-agent-script.sh
# Layer 1 active: PATH locked to $FLOX_ENV/bin only
```

## What gets blocked

Every package manager and installer the agent might reach for:

| Category | Blocked commands |
|----------|-----------------|
| System package managers | `flox`, `nix`, `nix-env`, `nix-store`, `nix-shell`, `nix-build`, `apt`, `apt-get`, `yum`, `dnf`, `brew`, `snap`, `flatpak` |
| Language package managers | `pip`, `pip3`, `npm`, `npx`, `yarn`, `pnpm`, `cargo`, `go`, `gem`, `composer`, `uv` |
| Container tools | `docker`, `podman` |
| Python escape vectors | `python3 -m pip`, `python3 -m ensurepip`, `python3 -m venv` |

## What the agent gets

Edit `requisites.txt` to control exactly which binaries are available. The default set:

- **Shell utilities** — bash, cat, ls, cp, mv, mkdir, rm, chmod, etc.
- **Text processing** — grep, sed, awk, cut, diff, etc.
- **Scripting** — python3, jq
- **Network** — curl (read-only fetching)
- **Version control** — git
- **File inspection** — stat, du, sha256sum, file

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

### Package-level only (no requisites.txt)

Delete `requisites.txt` entirely. The sandbox will fall back to exposing all binaries from `[install]` packages (~145 tools), but still block all system binaries (flox, nix, brew, etc.).

## How the layers work

### Layer 1: PATH wipe (hook)

The activation hook replaces the system PATH with only `$FLOX_ENV/bin` — the directory containing binaries from Flox-installed packages. This runs in both interactive and `-- CMD` modes. The system's `flox`, `nix`, `brew`, `pip`, `npm`, `docker`, and everything else simply don't exist in the agent's PATH.

### Layer 2: requisites.txt filter (profile)

In interactive mode, the profile reads `requisites.txt` and builds a restricted bin directory containing symlinks to only the listed binaries. PATH is then replaced with this directory, narrowing from ~145 package binaries to your specific whitelist.

### Layer 3: Shell function armor (profile)

27 package manager commands are shadowed with shell functions that return exit code 126 (permission denied). These are `export -f`'d so they propagate to child processes. This catches agents that try to call blocked commands by name even if they somehow modify PATH.

### Layer 4: Breadcrumb cleanup (hook + profile)

Environment variables that could help an agent discover escape routes are scrubbed:

- `FLOX_ENV_PROJECT` — path to manifest.toml (could be edited)
- `FLOX_ENV_DIRS` — internal Flox paths
- `FLOX_PATH_PATCHED` — internal Flox state

### Bonus: Python ensurepip neutralization

The hook injects a `usercustomize.py` that replaces Python's `ensurepip` module with a stub, preventing agents from bootstrapping pip through Python's stdlib.

## Verification

Run the test suite inside the sandbox:

```bash
flox activate -- bash test-sandbox.sh
```

Expected output: **35 passed, 0 failed.**

## Known limitations

sandflox is shell-level enforcement, not kernel-level isolation. A sufficiently determined agent could:

- **Find binaries by absolute path** — e.g., `/nix/store/<hash>/bin/flox`. Mitigated by the agent not knowing the hash and having no `find` or `locate` in PATH.
- **Write a script that modifies PATH** — the script runs in a child process that inherits the restricted PATH, but could hardcode a path.
- **Use Python to call subprocess with an absolute path** — `subprocess.call(["/nix/store/.../bin/flox", "install", "foo"])`. Requires knowing the store path.
- **Download and execute arbitrary code** — `curl | python3`. Mitigated by removing `curl` from `requisites.txt` if this is a concern.

For kernel-level isolation on Linux, see [flox-bwrap](https://github.com/devusb/flox-bwrap) which uses bubblewrap namespaces to make unmounted paths literally invisible to the agent.

## sandflox vs flox-bwrap

| | sandflox | flox-bwrap |
|---|---|---|
| **Isolation** | Shell-level | Kernel-level (Linux namespaces) |
| **Platform** | macOS + Linux | Linux only |
| **Blocks `flox install`** | Yes (PATH + function armor) | Yes (read-only nix store bind mount) |
| **Blocks network exfil** | No | Yes (`--unshare-all`, opt-in `--net`) |
| **Blocks absolute path escape** | Partially (agent must guess hash) | Fully (unmounted paths don't exist) |
| **Setup** | `flox activate` | Requires Go build + bwrap |
| **Best for** | Local dev, macOS, quick sandboxing | CI/CD, production, high-security |

They're complementary. sandflox is the portable policy layer. flox-bwrap is the Linux enforcement layer.

## Requirements

- [Flox](https://flox.dev) 1.10+
