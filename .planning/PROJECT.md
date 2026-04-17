# sandflox

## What This Is

A macOS-native sandbox for AI coding agents, distributed as a Flox package. sandflox is a Go binary that wraps `flox activate` under Apple `sandbox-exec`, enforcing what tools an agent can reach (shell enforcement: PATH wipe, requisites filter, function armor) and what it can mutate (kernel enforcement: SBPL filesystem/network policy). Driven by a declarative `policy.toml`. Users `flox install sandflox` into their environment, then run `sandflox` to enter a sandboxed shell — or `sandflox -- claude-code` to wrap any command. Like flox-bwrap for Linux, but macOS-native.

## Core Value

AI agents cannot escape the sandbox — not through PATH manipulation, absolute paths, shell redirects, or kernel syscalls — without requiring a Linux VM or devcontainer.

## Requirements

### Validated

- ✓ Declarative policy via `policy.toml` (profiles, filesystem modes, network modes, denied paths) — existing
- ✓ Shell enforcement: PATH wipe to `$FLOX_ENV/bin` only — existing
- ✓ Shell enforcement: binary whitelist via requisites files (symlink bin directory) — existing
- ✓ Shell enforcement: function armor shadowing 27+ package managers — existing
- ✓ Shell enforcement: filesystem write wrappers (`fs-filter.sh`) with `[sandflox] BLOCKED:` messages — existing
- ✓ Shell enforcement: Python `builtins.open` monkey-patch and `ensurepip` blocking — existing
- ✓ Shell enforcement: breadcrumb cleanup (scrub `FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`) — existing
- ✓ Kernel enforcement: macOS sandbox-exec SBPL profile generation from policy — existing
- ✓ Kernel enforcement: filesystem write blocking, network blocking, denied path blocking — existing
- ✓ Profile system: minimal / default / full security postures — existing
- ✓ Graceful degradation: shell enforcement works standalone when kernel tier unavailable — existing
- ✓ Agent-friendly error messages: `[sandflox] BLOCKED: <reason>` on shell tier — existing
- ✓ Test suites: 29-test policy suite + 35-test shell enforcement suite — existing

### Active

- ✓ Go binary replacing Bash+Python implementation — single compiled artifact, zero external Go deps (Phase 1)
- ✓ Flox build: `.flox/pkgs/sandflox.nix` using `buildGoModule`, minimal manifest (just `go` in `[install]`) (Phase 1)
- ✓ Flox publish/install: `flox publish` to FloxHub, `flox install 8BitTacoSupreme/sandflox` into any project environment (Phase 6)
- ✓ Interactive mode: `sandflox` wraps `sandbox-exec ... flox activate` — sandboxed interactive shell with kernel enforcement (Phase 2)
- ✓ Non-interactive mode: `sandflox -- CMD` wraps arbitrary commands under sandbox-exec (Phase 2 — shell enforcement still pending Phase 3)
- ✓ Re-exec elevation: `sandflox elevate` from within a `flox activate` session re-execs the shell under sandbox-exec (one-time bounce) (Phase 5)
- ✓ Policy parsing in Go: custom TOML subset parser with strict validation and line-numbered errors (Phase 1)
- ✓ SBPL profile generation in Go: generate Apple Seatbelt profiles from policy — filesystem modes, network modes, denied paths (Phase 2)
- ✓ Shell enforcement in Go: binary generates and applies PATH wipe, requisites symlink bin, function armor, fs-filter wrappers, breadcrumb cleanup as part of the activation it controls (Phase 3)
- ✓ CLI flags: `--net`, `--profile <name>`, `--policy <path>`, `--debug` — flags override policy.toml (Phase 1)
- ✓ macOS sandbox-exec enforcement: filesystem modes (workspace/strict/permissive), network modes (blocked/unrestricted), denied paths, localhost allowance (Phase 2)
- ✓ Requisites management: parse requisites files, generate symlink bin directories from `$FLOX_ENV/bin` (Phase 3)
- ✓ Python enforcement: generate `usercustomize.py` for Python write enforcement and ensurepip blocking (Phase 3)
- ✓ Config caching: write resolved config, path lists, and generated artifacts to `.flox/cache/sandflox/` (Phase 1)
- ✓ Diagnostic output: `[sandflox]` prefixed messages to stderr (profile, network mode, filesystem mode, SBPL path/rule count) (Phase 1 + Phase 2 D-07)
- ✓ `syscall.Exec` for clean process replacement — no child process overhead (matching flox-bwrap pattern) (Phase 1 scaffold, Phase 2 wired through sandbox-exec)
- ✓ Environment variable sanitization: allowlist-based filtering blocks credential-carrying vars (AWS_*, SSH_*, GITHUB_*, etc.), policy-configurable passthrough via `[security] env-passthrough`, Python safety flags force-set (Phase 4)

### Out of Scope

- Linux support — flox-bwrap handles Linux; sandflox is macOS-native
- Domain-level network filtering — requires a proxy, fundamentally different architecture
- GUI or TUI — CLI-only tool, Flox ergonomics
- Container/VM-based isolation — the entire point is to avoid this on macOS
- Direct Claude Code integration — sandflox is agent-agnostic; Claude Code runs inside it like any other process

## Context

sandflox v2 exists as a working prototype: a 500-line Bash script with inline Python for policy parsing and SBPL generation, plus ~400 lines of manifest hooks/profile for shell enforcement. It works but isn't distributable as a Flox package — the enforcement logic is embedded in manifest scripts, not a standalone binary.

The reference implementation is [flox-bwrap](https://github.com/devusb/flox-bwrap) (forked at 8BitTacoSupreme/flox-bwrap) — a Go binary that wraps bwrap for Linux namespace isolation. sandflox should match its Flox-native ergonomics (Go binary, `buildGoModule` Nix expression, `flox build`/`flox publish`/`flox install`) while targeting macOS sandbox-exec instead of bwrap.

The competitive alternative is devcontainers, which work but add a Linux VM layer and aren't macOS-native. sandflox's pitch: kernel-level agent sandboxing without leaving macOS.

### Existing Codebase

- `sandflox` (501 lines) — Bash+Python wrapper: policy parsing, SBPL generation, platform dispatch
- `.flox/env/manifest.toml` (418 lines) — Flox manifest with embedded hook/profile enforcement scripts
- `policy.toml` (43 lines) — declarative security policy (v2 schema)
- `requisites.txt` / `requisites-minimal.txt` / `requisites-full.txt` — binary whitelists
- `test-policy.sh` (302 lines) — v2 policy+kernel test suite
- `test-sandbox.sh` (94 lines) — shell enforcement test suite
- `verify-sandbox.sh` (111 lines) — quick verification script

### Key Patterns from flox-bwrap

- Go binary with `buildGoModule` Nix expression in `.flox/pkgs/`
- CLI flag parsing via `flag` package (no external deps)
- `nix-store --query --requisites` for store path enumeration
- `syscall.Exec` for clean process replacement (no child process)
- Build-time path injection via `-ldflags "-X main.var=value"`
- Zero external Go dependencies (`go.mod` has no requires)

## Constraints

- **Language**: Go — matches flox-bwrap pattern, zero external dependencies, single static binary, Nix-friendly build
- **Platform**: macOS only (Darwin) — sandbox-exec is macOS-specific
- **No Python dependency**: current sandflox requires Python 3.6+ for TOML parsing; Go binary eliminates this
- **No external Go deps**: match flox-bwrap's zero-dependency approach (use Go stdlib only)
- **Flox 1.10+**: minimum Flox version for manifest schema, build, and publish support
- **Backward compatibility**: existing `policy.toml` v2 schema must work unchanged
- **Existing tests**: shell enforcement behavior verified by test suites must be preserved

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go over Rust/Bash | Matches flox-bwrap precedent, zero-dep `buildGoModule`, fast compilation, good syscall support | Validated (Phase 1) |
| macOS only | sandbox-exec is the differentiator; Linux has flox-bwrap | Validated (Phase 2 — `exec_darwin.go` / `exec_other.go` split) |
| Binary IS the entrypoint | sandflox wraps sandbox-exec around flox activate — like flox-bwrap wraps bwrap. No hooks, no profile scripts, clean minimal manifest | Validated (Phase 1) |
| Keep policy.toml + flags | Declarative policy is a differentiator over flox-bwrap's flag-only approach. CLI flags override for ad-hoc use. Best of both worlds | Validated (Phase 1) |
| Shell tier = reach, Kernel tier = mutate | Shell enforcement blocks agents from reaching tools (PATH, functions). Kernel enforcement blocks mutations (writes, network). Two concerns, two layers | Both tiers validated (Phase 2 kernel, Phase 3 shell) |
| SBPL byte-identical to bash reference | Go `GenerateSBPL` mirrors bash `_sfx_generate_sbpl()` structure exactly — enables byte-comparison of cached profile to bash output for regression safety | Validated (Phase 2) |
| `syscall.Exec` to `sandbox-exec` (not child process) | Clean PID replacement; no sandflox process in the tree; matches flox-bwrap pattern | Validated (Phase 2) |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-17 after Phase 6 completion — published to FloxHub as 8BitTacoSupreme/sandflox, hermetic Nix build with fileset.toSource and -trimpath, installable via flox install. All v1.0 milestone phases complete.*
