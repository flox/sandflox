# Feature Landscape

**Domain:** macOS-native sandbox for AI coding agents
**Researched:** 2026-04-15

## Competitive Landscape Summary

The AI agent sandboxing space has consolidated around a few approaches by mid-2026:

| Tool | Approach | Platform | Open Source | Strengths |
|------|----------|----------|-------------|-----------|
| **Claude Code built-in** | Seatbelt + bubblewrap + network proxy | macOS/Linux | Yes (sandbox-runtime) | Integrated, domain-level network filtering via proxy |
| **Codex CLI built-in** | Seatbelt + bubblewrap | macOS/Linux/Windows | Yes | Tight workspace-write mode, config.toml driven |
| **Cursor built-in** | Seatbelt + bubblewrap | macOS/Linux/Windows | No | Enterprise admin dashboard, domain allowlists |
| **Ash** | Endpoint Security + Network Extension | macOS | No (closed-source) | Process-level control, env var filtering, policy hub |
| **SandVault** | User account isolation + sandbox-exec | macOS | Yes | Defense-in-depth via OS user separation |
| **Devcontainers** | Docker/VM | Any (via Linux VM) | Yes | Full isolation, reproducible, familiar |
| **flox-bwrap** | bubblewrap namespaces | Linux | Yes | Flox-native, zero-dep Go binary, immutable store |
| **sandflox** | Seatbelt + shell enforcement | macOS | Yes | Declarative policy, two-tier enforcement, Flox-native |

## Table Stakes

Features users expect. Missing any of these and the tool feels incomplete for agent sandboxing.

| Feature | Why Expected | Complexity | Status in sandflox | Notes |
|---------|--------------|------------|-------------------|-------|
| Filesystem write restriction | Every competitor has this. An agent that can write anywhere is not sandboxed. | Low | Existing | SBPL deny file-write + workspace/strict/permissive modes |
| Filesystem read denial for secrets | SSH keys, AWS creds, GPG keys must be unreadable. Claude Code, Codex, Ash all block these. | Low | Existing | `denied` paths in policy.toml |
| Network isolation (on/off) | Codex, Claude Code, Cursor all block network by default. Prevents exfiltration. | Low | Existing | `network.mode = "blocked"` with localhost allowance |
| Binary/tool whitelisting | Controlling what the agent can execute is fundamental. Claude Code uses Seatbelt, sandflox uses requisites. | Low | Existing | Requisites files + symlink bin directory |
| Package manager blocking | Every sandbox blocks `pip install`, `npm install`, etc. Agents default to installing things. | Low | Existing | Function armor for 27+ package managers |
| Declarative configuration | Codex has config.toml, Claude Code has settings.json, Ash has policy.yml. Nobody wants flag-only. | Low | Existing | policy.toml with profiles |
| Profile/preset system | Users need canned postures (minimal/default/full). Claude Code has modes, Codex has sandbox modes. | Low | Existing | Three profiles in policy.toml |
| Agent-friendly error messages | When blocked, agents need to understand why. Generic EPERM is useless. Claude Code and sandflox both do this. | Low | Existing | `[sandflox] BLOCKED: <reason>` messages |
| Graceful degradation | Tool must still work if kernel enforcement is unavailable. Claude Code falls back, sandflox falls back. | Low | Existing | Shell-only mode when sandbox-exec missing |
| CLI wrapping of arbitrary commands | `sandflox -- CMD` pattern. Codex, Claude Code, SandVault all wrap arbitrary processes. | Low | Existing | `./sandflox -- bash agent-script.sh` |
| Read-only path overrides | Protect .git, .env, policy files from writes even within writable workspace. Codex and Claude Code both do this. | Low | Existing | `read-only` list in policy.toml |
| Child process inheritance | Sandbox must apply to ALL subprocesses, not just the top-level shell. OS-level enforcement handles this. | Low | Existing | sandbox-exec inherits to subprocess tree |

## Differentiators

Features that set sandflox apart from competitors. Not expected, but create competitive advantage.

### Already Differentiating (Existing)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Two-tier enforcement (kernel + shell) | No other tool has both. Claude Code has OS-level only. Ash has kernel only. sandflox blocks at shell (agent-understandable errors) AND kernel (unforgeable). | N/A | Unique architecture: shell = reach, kernel = mutate |
| Declarative policy.toml with profiles | Codex has config.toml but no profiles. Claude Code has settings.json but scoped differently. flox-bwrap has flags only. sandflox's policy is version-controllable, team-sharable via FloxHub. | N/A | Strongest policy model in the space |
| Flox-native distribution | `flox install sandflox` into any project. No other sandbox tool integrates with Flox ecosystem. Reproducible environments + sandbox in one workflow. | N/A | Only tool that IS the package manager ecosystem |
| Python write enforcement | usercustomize.py blocks builtins.open writes, ensurepip, venv creation. No other tool patches Python internals. | N/A | Blocks the python -c escape vector |
| Breadcrumb cleanup | Scrubs FLOX_ENV_PROJECT, FLOX_ENV_DIRS. Prevents agents from discovering escape vectors via env inspection. | N/A | Only sandflox does this systematically |

### New Differentiators (Build in Go rewrite)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| `sandflox elevate` (re-exec from inside flox) | Users already in `flox activate` can elevate to sandboxed mode without restarting. No competitor offers mid-session elevation. | Medium | One-time bounce: re-exec shell under sandbox-exec |
| Zero external dependencies | Go binary with no external deps. Claude Code requires Node.js + npm. Ash is closed-source. SandVault requires Homebrew. sandflox is one binary. | Low | Matches flox-bwrap pattern: stdlib-only Go |
| `sandflox validate` (policy checker) | Validate policy.toml syntax and test enforcement before running agents. No competitor has a dry-run/validate mode. | Low | Parse policy, generate SBPL, report what would be enforced |
| `sandflox status` (runtime introspection) | Show current enforcement state: active profile, blocked paths, allowed tools, network mode. Agents and humans can query this. | Low | Useful for debugging and for agent self-awareness |

### Potential New Differentiators (Evaluate for roadmap)

| Feature | Value Proposition | Complexity | Why Differentiated |
|---------|-------------------|------------|-------------------|
| Environment variable scrubbing | Control which env vars are passed into the sandbox. Ash does this. No open-source tool on macOS does. Blocks secret exfiltration even when network is allowed. | Medium | "Environment variable leakage is the biggest security blind spot" (NVIDIA guidance). Ash is the only macOS tool with this, and it's closed-source. |
| Audit log / activity log | Log all blocked actions (filesystem denials, network blocks, tool access attempts) to a structured file. Enables post-session security review. | Medium | No local CLI sandbox tool does this. Claude Code shows notifications but no persistent log. Enterprise-grade feature at CLI-tool cost. |
| Domain-level network filtering | Filter by domain (allow github.com, block everything else) rather than binary on/off. Claude Code does this via proxy. Codex does not. | High | Requires running a proxy (HTTP + SOCKS5). Anthropic's sandbox-runtime shows how. Significantly more complex than socket-level blocking. Out of current scope but the most-requested missing feature. |
| Process execution control | Control which binaries can be spawned by absolute path, not just via PATH. Ash does this via Endpoint Security API. | High | sandbox-exec can restrict process-exec, but SBPL process rules are less documented. Would close the "agent finds /usr/bin/curl" vector that shell-tier blocks but kernel-tier currently doesn't for non-denied paths. |
| Policy inheritance / composition | Compose policies from multiple files (base + project + user overrides). Ash has a "policy hub" with composable policies. | Medium | Enables team base policies + per-project overrides. Claude Code does this with settings precedence (managed > user > project). |
| Init/setup script support | Run a setup script when sandbox activates (like Codex's setup script for pre-installing dependencies). | Low | Codex cloud runs a setup script. For local sandflox, this could prep the workspace (git clone, dependency install) before locking down. |

## Anti-Features

Features to deliberately NOT build. These are tempting but wrong for sandflox.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Container/VM-based isolation | The entire point of sandflox is avoiding the Linux VM layer on macOS. Devcontainers already exist for this. Adding container support dilutes the value proposition. | Use sandbox-exec (Seatbelt) for kernel enforcement. If users want containers, they should use devcontainers. |
| GUI / TUI interface | sandflox is a CLI tool in the Flox ergonomic family. No Flox tool has a TUI. Adding one fragments the UX and adds dependency weight. | CLI-only with `--debug` for visibility, `sandflox status` for introspection. |
| Agent-specific integration | Do not build Claude Code plugins, Codex integrations, or Cursor extensions. sandflox is agent-agnostic. The agent runs INSIDE sandflox, not alongside it. | `sandflox -- claude` wraps any agent. Agent-specific config belongs in the agent's settings, not sandflox. |
| Domain-level network filtering (v1) | Requires a proxy (HTTP + SOCKS5), certificate management, and fundamentally different architecture. Claude Code's sandbox-runtime shows it takes ~2000 lines of proxy code. Wrong scope for Go rewrite milestone. | Keep binary network on/off. Shell-tier curl wrappers provide best-effort domain filtering as defense-in-depth. Evaluate proxy approach as a future milestone. |
| Linux support | flox-bwrap handles Linux with bubblewrap namespaces. sandflox targeting Linux too would be redundant and dilute focus. The existing bwrap codepath in the Bash script should be removed in the Go rewrite. | macOS-only. Document "use flox-bwrap for Linux" explicitly. |
| Windows/WSL support | Windows sandbox is a different technology (restricted tokens, ACLs). WSL uses bubblewrap. Neither is macOS-native. | macOS-only. Out of scope permanently. |
| Mutable mode (package installation inside sandbox) | flox-bwrap has `--mutable` for tmpfs overlay. This contradicts sandflox's core value: immutable environments. Agents should NOT be able to install packages. | Reject this entirely. Immutability is the point. If the agent needs a tool, the human adds it to the Flox manifest and requisites.txt. |
| Interactive policy editor | A wizard/editor for policy.toml adds complexity without value. The file is 40 lines of TOML. | TOML is human-readable. `sandflox validate` confirms it's correct. Documentation covers the schema. |
| Real-time process monitoring daemon | Running a persistent daemon to monitor agent activity is architecturally different from sandbox-exec wrapping. Ash uses Endpoint Security (which requires a daemon). | Append-only audit log of blocked actions is simpler and sufficient. No daemon. |
| Nested sandbox support | Claude Code has `enableWeakerNestedSandbox` for Docker-in-sandbox. This weakens security. | If the user needs Docker, they run it outside the sandbox. `docker` is in the function armor block list. |

## Feature Dependencies

```
policy.toml parsing ──> SBPL generation ──> sandbox-exec wrapping
                    \──> Shell enforcement (requisites, function armor, fs-filter)
                    \──> Config caching (.flox/cache/sandflox/)

CLI flag parsing ──> Override policy.toml values
                 \──> --debug, --profile, --net, --policy, --requisites

sandflox [interactive] ──> sandbox-exec ... flox activate
sandflox -- CMD        ──> sandbox-exec ... flox activate -- CMD
sandflox elevate       ──> Re-exec current shell under sandbox-exec (requires detecting active flox env)

sandflox validate ──> Parse policy.toml + generate SBPL (dry-run, no execution)
sandflox status   ──> Read cached enforcement state + report
```

### Build Order Dependencies

```
1. Go TOML parser (policy.toml)
   |
2. SBPL generation (from parsed policy)     CLI flag parsing (flag package)
   |                                         |
3. sandbox-exec wrapping (syscall.Exec)  <--+
   |
4. Shell enforcement generation (requisites symlink bin, function armor, fs-filter)
   |
5. Python enforcement (usercustomize.py generation)
   |
6. Config caching (.flox/cache/sandflox/)
   |
7. Interactive + non-interactive modes
   |
8. Elevate mode (sandflox elevate)
   |
9. Validate + status subcommands
```

## MVP Recommendation

### Must-Have for Go Rewrite (parity with existing Bash prototype)

1. **Policy parsing** -- TOML parsing in Go, resolve profiles, apply defaults
2. **SBPL generation** -- Generate Seatbelt profiles from policy (filesystem modes, network modes, denied paths)
3. **sandbox-exec wrapping** -- Interactive and non-interactive modes via syscall.Exec
4. **Shell enforcement generation** -- Requisites symlink bin, function armor script, fs-filter script, breadcrumb cleanup
5. **Python enforcement** -- Generate usercustomize.py
6. **CLI flags** -- `--net`, `--profile`, `--policy`, `--debug`, `--requisites` (override policy.toml)
7. **Config caching** -- Write resolved state to `.flox/cache/sandflox/`
8. **Diagnostic output** -- `[sandflox]` prefixed stderr messages

### Should-Have for Go Rewrite (low-hanging new value)

9. **`sandflox validate`** -- Dry-run policy parsing and SBPL generation
10. **`sandflox status`** -- Report current enforcement state
11. **Environment variable scrubbing** -- Control which env vars pass through (HIGH value, MEDIUM complexity)

### Defer to Future Milestones

12. **`sandflox elevate`** -- Mid-session elevation (needs careful design for flox env detection)
13. **Audit log** -- Structured logging of blocked actions
14. **Policy composition/inheritance** -- Base + project + user overlay policies
15. **Domain-level network filtering** -- Requires proxy architecture (out of scope for Go rewrite)

## Competitive Position Matrix

How sandflox compares on key features after the Go rewrite:

| Feature | sandflox | Claude Code | Codex CLI | Cursor | Ash | Devcontainers |
|---------|----------|-------------|-----------|--------|-----|---------------|
| macOS-native (no VM) | YES | YES | YES | YES | YES | No (Docker VM) |
| Open source | YES | YES | YES | No | No | YES |
| Filesystem isolation | YES | YES | YES | YES | YES | YES |
| Network isolation | On/Off | Domain-level | On/Off | Domain-level | Domain-level | On/Off |
| Binary whitelisting | YES (requisites) | No | No | No | YES (process rules) | No |
| Package manager blocking | YES (function armor) | No (relies on fs/net) | No | No | Via process rules | No |
| Declarative policy file | YES (policy.toml) | settings.json | config.toml | Admin dashboard | policy.yml | devcontainer.json |
| Profile presets | YES (3 profiles) | 2 modes | 2 modes | No | Policy hub | No |
| Agent-friendly errors | YES | Notifications | No | No | No | No |
| Two-tier enforcement | YES (shell + kernel) | Kernel only | Kernel only | Kernel only | Kernel only | Kernel only |
| Env var scrubbing | Planned | No | No | No | YES | Via Docker |
| Zero dependencies | YES (Go binary) | Node.js | Node.js | Electron | Closed binary | Docker |
| Flox ecosystem | YES | No | No | No | No | No |

## Sources

- [Claude Code Sandboxing Docs](https://code.claude.com/docs/en/sandboxing) -- HIGH confidence, official documentation
- [Anthropic Engineering: Claude Code Sandboxing](https://www.anthropic.com/engineering/claude-code-sandboxing) -- HIGH confidence
- [Anthropic sandbox-runtime (GitHub)](https://github.com/anthropic-experimental/sandbox-runtime) -- HIGH confidence
- [Codex Sandbox Concepts](https://developers.openai.com/codex/concepts/sandboxing) -- HIGH confidence, official docs
- [Codex Configuration Reference](https://developers.openai.com/codex/config-reference) -- HIGH confidence
- [Codex Sandboxing Implementation (DeepWiki)](https://deepwiki.com/openai/codex/5.6-sandboxing-implementation) -- MEDIUM confidence
- [Cursor Agent Sandboxing Blog](https://cursor.com/blog/agent-sandboxing) -- HIGH confidence, official blog
- [Ash Agent Sandbox](https://ashell.dev/) -- MEDIUM confidence (closed-source, marketing page)
- [Ash Policy Hub](https://hub.ashell.dev/policies/ash/base-macos/0.0.3) -- MEDIUM confidence
- [SandVault (GitHub)](https://github.com/webcoyote/sandvault) -- HIGH confidence, open source
- [NVIDIA: Practical Security Guidance for Sandboxing Agentic Workflows](https://developer.nvidia.com/blog/practical-security-guidance-for-sandboxing-agentic-workflows-and-managing-execution-risk/) -- HIGH confidence
- [Northflank: How to Sandbox AI Agents in 2026](https://northflank.com/blog/how-to-sandbox-ai-agents) -- MEDIUM confidence
- [Devcontainers for AI Agent Security (multiple sources)](https://codewithandrea.com/articles/run-ai-agents-inside-devcontainer/) -- MEDIUM confidence
