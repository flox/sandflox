# Phase 2: Kernel Enforcement (SBPL + sandbox-exec) - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement SBPL profile generation in Go and wire sandbox-exec invocation into the main binary's exec path. After this phase, `sandflox` (interactive) and `sandflox -- CMD` (non-interactive) wrap `flox activate` under `sandbox-exec -f <profile>` with kernel-level enforcement of filesystem modes, network modes, and denied paths. Graceful degradation when sandbox-exec is unavailable.

</domain>

<decisions>
## Implementation Decisions

### SBPL Generation Strategy
- **D-01:** Mirror the existing bash SBPL structure exactly: `(allow default)` baseline with selective denials. Same rule ordering. The bash implementation is proven in production; diverging risks breaking flox/Nix compatibility.
- **D-02:** Use SBPL parameters via `-D` flags (`-D "PROJECT=$dir" -D "HOME=$home" -D "FLOX_CACHE=$cache"`). Reference parameters in the profile with `(param "PROJECT")` etc. Keeps the `.sb` file portable — matches bash pattern.
- **D-03:** Regenerate the SBPL profile every run (no caching). Generation is fast (~1ms), avoids cache invalidation bugs, matches bash behavior.
- **D-04:** Read-only path overrides in workspace mode use `(deny file-write* (subpath ...))` for directory paths (trailing `/`) and `(deny file-write* (literal ...))` for file paths — same logic as bash.

### Fallback and Error Behavior
- **D-05:** When sandbox-exec is not available: print `[sandflox] WARNING: sandbox-exec not found — falling back to shell-only` and exec `flox activate` directly. Matches ARCHITECTURE.md "graceful degradation" convention and existing bash behavior.
- **D-06:** When sandbox-exec fails (bad SBPL, permission denied): exit with `[sandflox] ERROR:` including sandbox-exec's stderr. Do NOT fall back silently — enforcement failures are security-relevant. This is different from "not available" (which is a platform limitation, not a failure).
- **D-07:** `--debug` flag prints the generated SBPL file path and key rules to stderr, in addition to the existing profile/mode/paths diagnostics from Phase 1.

### Testing Strategy
- **D-08:** Unit test the generated SBPL string content — verify correct rules for each filesystem mode (permissive/workspace/strict), each network mode (blocked/unrestricted), denied paths, localhost allowance, and Flox-required overrides. Parse the SBPL output and check rule presence/absence.
- **D-09:** Integration tests that actually run `sandbox-exec` with the generated profile — test write blocking, network blocking, denied path access. These tests require macOS and real sandbox-exec; skip gracefully on other platforms.
- **D-10:** Preserve existing bash test scripts (`test-policy.sh`, `test-sandbox.sh`, `verify-sandbox.sh`) as behavioral documentation. Write Go test equivalents for the same scenarios.

### Claude's Discretion
- Go function decomposition for SBPL generation (single function vs per-section helpers)
- SBPL comment style and formatting within the generated profile
- Integration test helper patterns (subprocess management, timeout handling)
- Debug output formatting beyond the required SBPL path and key rules

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `policy.go` — ParsePolicy, Policy struct with NetworkSection, FilesystemSection, ProfileSection (from Phase 1)
- `config.go` — ResolveConfig, ResolvedConfig struct with resolved paths (writable, read-only, denied as absolute paths)
- `cache.go` — WriteCache writes config.json, path lists, and net-mode/fs-mode files to .flox/cache/sandflox/
- `main.go` — Full pipeline: ParseFlags → ParsePolicy → ResolveConfig → WriteCache → emitDiagnostics → exec flox. Phase 2 inserts SBPL generation + sandbox-exec wrapping before the final exec.

### Established Patterns
- `syscall.Exec` for clean process replacement (no child processes)
- `[sandflox]` prefix on all stderr messages (ERROR, WARNING, diagnostics)
- `--debug` flag for verbose output
- All paths resolved to absolute (no `~` or `.`) by config.go

### Integration Points
- main.go `execFlox()` function is where sandbox-exec wrapping goes — currently does `syscall.Exec("flox", ...)`, needs to become `syscall.Exec("sandbox-exec", ["-f", sbplPath, "-D", params..., "flox", "activate", ...])`
- ResolvedConfig has all the data needed for SBPL generation: NetworkMode, FilesystemMode, DeniedPaths, WritablePaths, ReadOnlyPaths, ProjectDir, AllowLocalhost
- The bash reference at `sandflox.bash` lines 191-291 (`_sfx_generate_sbpl`) is the canonical SBPL generation logic to port

</code_context>

<specifics>
## Specific Ideas

No specific requirements — all grey areas resolved to "mirror bash behavior". The bash implementation at `sandflox.bash:191-291` is the spec.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 02-kernel-enforcement-sbpl-sandbox-exec*
*Context gathered: 2026-04-16 via Smart Discuss (autonomous)*
