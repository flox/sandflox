# Phase 1: Go Scaffold, Policy Engine, and Build Validation - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

Stand up the Go binary with TOML parsing, CLI flags, config resolution, and prove it builds via Nix. The binary parses policy.toml, resolves profiles with CLI flag overrides, caches resolved config and path lists, emits diagnostics to stderr, and falls through to exec flox activate. Both interactive (`sandflox`) and non-interactive (`sandflox -- CMD`) modes work, but without sandbox-exec wrapping or shell enforcement (those are Phase 2 and 3).

</domain>

<decisions>
## Implementation Decisions

### TOML Parser Strategy
- **D-01:** Custom Go subset parser supporting only what policy.toml v2 uses: sections, dotted sections (`[profiles.minimal]`), string values, booleans, string arrays, and comments. Reject unsupported TOML features with clear errors. Target ~200 lines of Go.
- **D-02:** Strict validation of all parsed values against known enums: network mode (`blocked`/`unrestricted`), filesystem mode (`permissive`/`workspace`/`strict`), policy version (`"2"`). Reject unknown values with `[sandflox] ERROR:` identifying the bad value and its location. Fixes the silent-misconfig gap identified in CONCERNS.md.
- **D-03:** Hard error if `meta.version` is not `"2"`. No warn-and-continue. Future policy format changes get a clear "unsupported policy version" error rather than silent misparse.

### Go Project Layout
- **D-04:** Flat main package — all `.go` files in project root under `package main`. Matches flox-bwrap pattern. Files like `main.go`, `policy.go`, `sbpl.go`, `shell.go`, `cli.go`. No `cmd/` or `internal/` overhead.
- **D-05:** Go source files at project root alongside existing bash scripts, policy.toml, requisites files, and test scripts. `go.mod` at root. Nix build expression picks `.go` files from root.

### Phase 1 Binary Behavior
- **D-06:** Running `sandflox` in Phase 1 does: parse policy.toml -> resolve profile (env var > policy > default) -> merge CLI flag overrides -> write cache artifacts to `.flox/cache/sandflox/` -> emit `[sandflox]` diagnostics to stderr -> exec `flox activate`. Phase 2 inserts `sandbox-exec` before the final exec.
- **D-07:** Both interactive (`sandflox` -> `flox activate`) and non-interactive (`sandflox -- CMD` -> `flox activate -- CMD`) modes implemented in Phase 1. The mode split lives in arg parsing, which is Phase 1 scope.
- **D-08:** Project-dir assumption for Flox environment discovery. The binary assumes it's running from (or is pointed to via `--policy`) a directory with `policy.toml` and `.flox/`. Reads `$FLOX_ENV` from environment or resolves from `.flox/`. No explicit `flox env resolve` calls.
- **D-09:** The overall flow mirrors flox-bwrap's architecture: resolve environment -> read policy/metadata -> build sandbox spec -> exec sandbox wrapping flox activate. Phase 1 covers the first half (resolve + read + cache). Phase 2 adds the sandbox wrapping.

### Coexistence Strategy
- **D-10:** Rename existing bash script to `sandflox.bash` (preserved as reference artifact). Go binary builds as `sandflox`. Clean break from Phase 1 onward — the Go binary IS the product.
- **D-11:** Replace the 418-line `manifest.toml` with a minimal build manifest: just `go` in `[install]`, no hooks or profile scripts (per DIST-04). Old manifest preserved as `manifest.toml.v2-bash` for reference. The Go binary generates enforcement artifacts itself.
- **D-12:** Preserve existing bash test scripts as reference (for behavior documentation). Write Go test files (`policy_test.go`, etc.) that verify the same behaviors. The bash tests document expected behavior; Go tests replicate it.

### Claude's Discretion
- CLI flag parsing implementation details (Go `flag` package usage, flag naming)
- Cache file format and layout (JSON, text files, directory structure in `.flox/cache/sandflox/`)
- Diagnostic message formatting details beyond the `[sandflox]` prefix convention
- Go file organization within the flat package (which types/functions go in which `.go` file)
- Nix expression details for `buildGoModule` (vendorHash, ldflags, fileset selection)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing Implementation (reference for behavior replication)
- `sandflox` -- Bash+Python wrapper (501 lines): policy parsing (lines 43-131), SBPL generation (lines 191-291), platform dispatch (lines 465-500)
- `policy.toml` -- Declarative security policy v2 schema (43 lines): the spec the Go parser must handle
- `.flox/env/manifest.toml` -- Current Flox manifest with hook/profile enforcement (418 lines): reference for what the minimal manifest replaces
- `requisites.txt`, `requisites-minimal.txt`, `requisites-full.txt` -- Binary whitelists the Go binary must parse

### Reference Implementation
- flox-bwrap (https://github.com/devusb/flox-bwrap) -- Go binary pattern: `buildGoModule`, `flag` package, `syscall.Exec`, zero external deps, build-time ldflags injection

### Test Behavior Documentation
- `test-policy.sh` (302 lines) -- v2 policy+kernel test suite: documents expected parsing and enforcement behavior
- `test-sandbox.sh` (94 lines) -- Shell enforcement test suite: documents expected PATH, armor, and breadcrumb behavior

### Project Analysis
- `.planning/codebase/CONCERNS.md` -- Known issues the Go rewrite should fix (TOML parser edge cases, fs-filter multi-arg, silent misconfig)
- `.planning/codebase/STRUCTURE.md` -- Full directory layout and file purposes
- `.planning/codebase/ARCHITECTURE.md` -- Layer model and data flow
- `.planning/REQUIREMENTS.md` -- CORE-01 through CORE-07, DIST-01, DIST-04 requirements for this phase

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `policy.toml` (43 lines): The Go parser targets this exact schema. Use as the primary test fixture.
- `requisites*.txt` files: These are read verbatim by the Go binary (one name per line, comments with #).
- Bash test scripts: Document the expected behaviors that Go tests should replicate.
- `.flox/cache/sandflox/` directory structure: The Go binary writes to the same cache layout.

### Established Patterns
- `[sandflox]` prefixed messages to stderr for all diagnostics and errors
- Profile resolution precedence: `$SANDFLOX_PROFILE` env var > `policy.toml [meta] profile` > `"default"`
- Cache artifacts: `config.json`, `net-mode.txt`, `fs-mode.txt`, `active-profile.txt`, `writable-paths.txt`, `read-only-paths.txt`, `denied-paths.txt`, `requisites.txt` (staged copy)
- `~` expansion to `$HOME` and `.` expansion to `$PWD` for path resolution
- `/private/tmp` instead of `/tmp` for macOS symlink correctness

### Integration Points
- `flox activate` is the final exec target (Phase 1 passes through; Phase 2 wraps with sandbox-exec)
- `.flox/cache/sandflox/` is the write target for all generated artifacts
- `go.mod` at project root, `.flox/pkgs/sandflox.nix` for the Nix build expression
- `flox build` is the build command; `flox publish` in Phase 6

</code_context>

<specifics>
## Specific Ideas

- The overall architecture should mirror flox-bwrap's flow: resolve environment -> read policy/metadata -> build sandbox spec -> pass spec to sandbox and start it up. flox-bwrap calls out to flox to pull the environment, then uses requisites.txt to figure out what to mount, builds the sandbox spec, and runs flox activate inside the sandbox.
- The Go binary should use `syscall.Exec` for clean process replacement (no child process overhead), matching flox-bwrap's pattern.

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 01-go-scaffold-policy-engine-and-build-validation*
*Context gathered: 2026-04-15*
