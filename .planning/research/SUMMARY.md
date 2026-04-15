# Research Summary: sandflox Go Rewrite

**Domain:** macOS-native process sandbox tool, distributed as a Flox package
**Researched:** 2026-04-15
**Overall confidence:** HIGH

## Executive Summary

The sandflox Go rewrite is a well-scoped port of an existing 500-line Bash+Python tool to a zero-dependency Go binary. The technology stack is proven: flox-bwrap provides an exact reference implementation of the Go + buildGoModule + Flox build/publish pattern, already in production. The primary engineering challenge is not technology selection but faithful translation of the existing SBPL generation, TOML parsing, and shell enforcement artifact generation into Go.

The zero-external-dependency constraint is achievable because Go's stdlib provides everything needed: `flag` for CLI, `os`/`syscall` for process control, `strings`/`fmt` for SBPL template generation, and basic string parsing for the limited TOML subset that `policy.toml` uses. The only "novel" code is a ~100-150 line TOML subset parser, which mirrors the existing ~40-line Python parser already embedded in the sandflox Bash script.

Go has no `encoding/toml` in the standard library, and no proposal for one exists. This was verified -- not assumed. The zero-dep constraint means we write a purpose-built parser for the 5 TOML features policy.toml uses (sections, dotted sections, strings, booleans, string arrays). This is the right tradeoff: 100 lines of parser code vs. an external dependency that complicates the Nix build.

The `sandbox-exec` deprecation risk is real but manageable. Apple deprecated the CLI tool years ago but continues to ship it through macOS 15 Sequoia, and major products (OpenAI Codex, Chrome, Claude Code) rely on it. The Seatbelt kernel extension backing it powers App Store sandboxing. Apple's WWDC 2025 Containerization framework (macOS 26) targets Linux containers in VMs, NOT macOS process sandboxing -- sandbox-exec has no replacement. sandflox already has graceful degradation to shell-only enforcement when sandbox-exec is unavailable, which is the correct mitigation.

The Flox build/publish pipeline is the distribution story. `flox build` runs the Nix expression via `buildGoModule` with `vendorHash = null` (zero deps). `flox publish` uploads to FloxHub. `flox install <owner>/sandflox` makes it available in any Flox environment. This is the same pipeline flox-bwrap uses.

## Key Findings

**Stack:** Go 1.24.x, zero external dependencies, `buildGoModule` Nix expression with `vendorHash = null`, `lib.fileset.toSource` for hermetic source selection, `flox build`/`flox publish` for distribution. Direct port of flox-bwrap patterns.

**Architecture:** Single binary that parses policy.toml (custom subset parser), generates SBPL profile + shell enforcement artifacts to `.flox/cache/sandflox/`, then `syscall.Exec`s into `sandbox-exec -f sandflox.sb -D KEY=VALUE flox activate`. Binary replaces itself in the process tree.

**Critical pitfall:** The TOML parser must handle dotted table paths (`[profiles.minimal]`) correctly by splitting on `.` and walking nested maps. Silent misparse leads to wrong profile resolution and permissive security posture. Strict validation (reject unknown TOML constructs, validate enum values) is essential.

## Implications for Roadmap

Based on research, suggested phase structure:

1. **Scaffold + Policy Parsing** - Stand up the Go module, config struct, flag parsing, and TOML subset parser. This is the foundation everything else depends on.
   - Addresses: go.mod, main.go, config.go, policy.go
   - Avoids: Starting with SBPL generation before the config it reads from is solid

2. **SBPL Generation + sandbox-exec** - Generate SBPL profiles from parsed policy and exec into sandbox-exec. This is the kernel enforcement layer.
   - Addresses: sbpl.go, sandbox.go, the `syscall.Exec` invocation
   - Avoids: Trying to replicate all shell enforcement before kernel enforcement works

3. **Shell Enforcement Artifacts** - Generate fs-filter.sh, usercustomize.py, entrypoint.sh, requisites bin directory, and cache files.
   - Addresses: shell.go, all the artifact generation the current Bash+Python does
   - Avoids: Shell enforcement is complex and has 35 existing tests -- save it for after the core works

4. **Nix Build + Flox Publish** - Wire up .flox/pkgs/sandflox.nix, test `flox build`, publish to FloxHub.
   - Addresses: Build expression, manifest updates, distribution
   - Avoids: Building before the binary is functionally complete

**Phase ordering rationale:**
- Policy parsing feeds SBPL generation (dependency)
- SBPL generation feeds sandbox-exec invocation (dependency)
- Shell enforcement artifacts are independent of kernel enforcement but need policy parsing
- Build/publish is the final packaging step after the binary works

**Research flags for phases:**
- Phase 1: Likely needs deeper research on Go TOML parser edge cases (dotted keys, inline comment stripping). Test against the existing 29-test policy suite.
- Phase 2: Standard patterns, SBPL generation is a direct port from existing Bash. Use `/private/tmp` not `/tmp` in SBPL rules (macOS symlink resolution).
- Phase 3: Standard patterns, but many artifact types to generate correctly. `$FLOX_ENV` is not available at generation time -- use runtime variable references.
- Phase 4: Standard patterns, flox-bwrap provides exact Nix expression template. Test `flox build` early, not at the end.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Go + buildGoModule + flox publish is proven by flox-bwrap. All Go stdlib packages verified. |
| Features | HIGH | Exact feature parity with existing Bash+Python -- no invention needed |
| Architecture | HIGH | Direct port of flox-bwrap's exec-based architecture, adapted for sandbox-exec |
| Pitfalls | MEDIUM | sandbox-exec deprecation timeline is Apple's decision; TOML edge cases need testing |
| Build/Publish | HIGH | flox-bwrap Nix expression and workflow verified against Flox docs |

## Gaps to Address

- **Go version in Flox catalog**: Need to verify whether Flox catalog has Go 1.24 or 1.25 at build time. The go.mod version should match what `flox show go` provides. flox-bwrap uses `go 1.25.5` suggesting the catalog may have 1.25.
- **`sandbox-exec -D` flag behavior**: The existing script uses `-D KEY=VALUE` for parameter passing. The `-D` flag is not in the man page but works in practice. The C API (`sandbox_init_with_parameters`) is documented. Existing sandflox proves `-D` works, but the parameter passing mechanism in the context of `syscall.Exec` argv should be tested early.
- **flox publish org namespace**: Whether sandflox publishes under a personal namespace or an org. Not a technical blocker but affects the install command users run.
- **`-trimpath` in Nix expression**: Flox Go docs recommend `-trimpath` to prevent build path leaks. Should be added to `buildFlags` in the Nix expression.
- **`lib.fileset` glob vs. explicit listing**: flox-bwrap lists individual `.go` files. A `fileFilter (f: f.hasExt "go")` glob would be more maintainable but needs verification in the Flox build context.
