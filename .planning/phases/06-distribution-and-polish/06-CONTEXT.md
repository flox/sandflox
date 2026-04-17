# Phase 6: Distribution and Polish - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning
**Mode:** Infrastructure phase (discuss skipped)

<domain>
## Phase Boundary

sandflox is published to FloxHub and installable into any Flox environment with a hermetic, reproducible Nix build. This phase finalizes the Nix build expression (lib.fileset.toSource for hermetic source selection, -trimpath for no build path leaks), runs `flox publish` to upload to FloxHub, and verifies `flox install sandflox` works in a fresh environment.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `.flox/pkgs/sandflox.nix` -- existing Nix build expression from Phase 1 (buildGoModule, lib.fileset.toSource, -trimpath)
- `.flox/env/manifest.toml` -- Flox manifest with package declarations and build configuration
- `go.mod` -- Go module definition (zero external deps)

### Established Patterns
- Phase 1 validated `flox build` produces a working binary
- Phase 1 established `lib.fileset.toSource` for hermetic source selection (only .go + go.mod)
- Phase 1 established `-trimpath` in buildFlags for reproducible builds
- `vendorHash = null` confirmed correct for zero external Go deps

### Integration Points
- `flox publish` reads from `.flox/pkgs/sandflox.nix` and manifest
- `flox install` pulls from FloxHub catalog
- Binary must be self-contained (no runtime deps on Python, manifest hooks, etc.)

</code_context>

<specifics>
## Specific Ideas

No specific requirements -- infrastructure phase. Refer to ROADMAP phase description and success criteria.

</specifics>

<deferred>
## Deferred Ideas

None -- infrastructure phase.

</deferred>
