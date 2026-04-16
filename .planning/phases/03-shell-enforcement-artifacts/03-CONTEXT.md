# Phase 3: Shell Enforcement Artifacts - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous)

<domain>
## Phase Boundary

Port the bash `manifest.toml` hook/profile shell enforcement (lines 49-412 of `manifest.toml.v2-bash`) into the Go binary's generated-artifact pipeline. After this phase, running `sandflox` or `sandflox -- CMD` produces an agent environment where: PATH contains only a requisites-filtered symlink bin, 27+ package manager commands are shadowed by armor functions that exit 126 with `[sandflox] BLOCKED:` messages, write commands (`cp`, `mv`, `mkdir`, `rm`, `rmdir`, `ln`, `chmod`, `tee`) route through fs-filter wrappers enforcing writable/read-only/denied policy, Python code running `open()` or `ensurepip` is blocked, and breadcrumb env vars (`FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`, `FLOX_PATH_PATCHED`) are unset before the agent sees the shell. Shell enforcement composes with kernel enforcement from Phase 2 — kernel blocks mutations, shell blocks reach.

</domain>

<decisions>
## Implementation Decisions

### Shell Enforcement Delivery Mechanism
- **D-01:** Interactive mode (`sandflox`) delivers enforcement via `flox activate -- bash --rcfile <entrypoint> -i`. Flox establishes its activation env first, then our rcfile layers sandflox enforcement on top. No manifest `[hook]`/`[profile]` sections needed — keeps the minimal manifest decision from Phase 1 D-11 intact.
- **D-02:** Non-interactive mode (`sandflox -- CMD`) delivers enforcement via `flox activate -- bash -c 'source <entrypoint> && exec "$@"' -- CMD ARGS...`. The `exec "$@"` replaces bash with the user command so the process tree stays clean (no extra bash parent).
- **D-03:** The generated orchestrator script lives at `.flox/cache/sandflox/entrypoint.sh` — matches the bash reference path and reuses the already-provisioned cache directory.
- **D-04:** Regenerate all shell enforcement artifacts every run. Matches Phase 2 D-03 for SBPL, avoids cache invalidation bugs, and generation is <1ms in Go. `WriteCache` gains new writers for the shell tier alongside the existing config/path-list writers.

### Generated Artifact Structure
- **D-05:** Three generated scripts:
  - `.flox/cache/sandflox/entrypoint.sh` — PATH wipe, armor function definitions, breadcrumb cleanup, source of fs-filter.sh, Python env var exports
  - `.flox/cache/sandflox/fs-filter.sh` — write-command wrappers (cp/mv/mkdir/rm/rmdir/ln/chmod/tee) plus `_sfx_check_write_target` helper
  - `.flox/cache/sandflox-python/usercustomize.py` — `builtins.open` monkey-patch + `ensurepip` module injection
  - This matches the bash layout exactly for maintainability and debuggability.
- **D-06:** Script templates are stored as `.tmpl` files embedded via `//go:embed` and rendered with `text/template`. Keeps generation code readable, separates the "what gets generated" from the "when/how" Go logic, and enables straightforward golden-file tests.
- **D-07:** Symlink bin directory: `.flox/cache/sandflox/bin/`. For each name in the active requisites file, create a symlink pointing to `$FLOX_ENV/bin/<name>`. PATH gets wiped and reset to contain only this directory (plus macOS system paths that SBPL already scoped).
- **D-08:** Missing requisite tool (name listed in requisites but not present in `$FLOX_ENV/bin`) emits `[sandflox] WARNING: <tool> listed in requisites but not in $FLOX_ENV/bin — skipping` to stderr and continues. Matches Phase 2 D-05 graceful-degradation convention. A typo in the requisites list should not abort agent startup.

### Python Enforcement Delivery
- **D-09:** `usercustomize.py` lives at `.flox/cache/sandflox-python/usercustomize.py` — matches bash reference exactly. Cache directory is created alongside `.flox/cache/sandflox/`.
- **D-10:** entrypoint.sh exports `PYTHONPATH="$FLOX_ENV_CACHE/sandflox-python:$PYTHONPATH"` and `PYTHONUSERBASE="$FLOX_ENV_CACHE/sandflox-python"` so Python's site-customization loader picks up the generated `usercustomize.py` automatically. Mirrors bash.
- **D-11:** `usercustomize.py` reads policy state from the same text files the shell tier uses: `$FLOX_ENV_CACHE/sandflox/fs-mode.txt`, `writable-paths.txt`, `denied-paths.txt`. One source of truth; no policy duplication across tiers. Matches bash reference.
- **D-12:** `ensurepip` is blocked via module injection — the generated script inserts a stub `ensurepip` module into `sys.modules` that raises `PermissionError` on import. The `builtins.open` wrapper checks the target path against the policy files and raises `PermissionError("[sandflox] BLOCKED: ...")` on deny.

### Testing Strategy
- **D-13:** Unit tests verify generated script content (string-match on key directives: PATH wipe, armor function definitions, fs-filter helper, Python env vars, breadcrumb unsets). Per-generator function gets unit tests mirroring the Phase 2 `sbpl_test.go` pattern.
- **D-14:** Integration tests use real subprocess spawning (matching Phase 2 `exec_integration_test.go`) to verify behavior end-to-end: armor blocks `pip`, PATH is limited, `cp` to denied path produces `[sandflox] BLOCKED:`, Python `open('/etc/passwd', 'w')` raises PermissionError, breadcrumb vars are unset.
- **D-15:** Where feasible, tests compare against the bash reference output as a sanity check — e.g., generate entrypoint.sh for a known policy, spot-check key lines against `manifest.toml.v2-bash`. Not byte-identical (Go may structure output differently), but semantically equivalent.

### Claude's Discretion
- Go function decomposition within the shell tier package (one file per script vs grouped by concern)
- Template variable naming and rendering style (pipeline-heavy vs flat)
- `fs-filter.sh` path-check algorithm implementation details (string matching order, longest-prefix-wins rules)
- Integration test fixture layout under `testdata/`
- Exact wording of `[sandflox] BLOCKED:` messages beyond the established prefix convention

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing Bash Reference (behavior spec to replicate)
- `manifest.toml.v2-bash` lines 49-232 — `[hook] on-activate` policy parsing, fs-filter.sh generation, usercustomize.py generation, net-filter, breadcrumb scrub
- `manifest.toml.v2-bash` lines 317-412 — `[profile] common` requisites symlink bin, armor functions (27 package managers), breadcrumb unset, fs-filter source
- `sandflox.bash` lines 191-291 — SBPL generation (already ported in Phase 2, reference for style)
- `requisites.txt` (~55 tools), `requisites-minimal.txt` (~28 tools), `requisites-full.txt` (~54+ tools) — the binary whitelists to parse and symlink
- `policy.toml` — declarative security policy already parsed in Phase 1

### Existing Go Implementation (extension targets)
- `policy.go` — Policy struct, ParsePolicy (from Phase 1)
- `config.go` — ResolvedConfig (NetMode, FsMode, Writable, ReadOnly, Denied, Requisites, AllowLocalhost), ResolveConfig (from Phase 1)
- `cache.go` — WriteCache writes config.json, path lists, fs-mode.txt, net-mode.txt (from Phase 1; extend for shell tier artifacts)
- `main.go` — execWithKernelEnforcement pipeline (Phase 2); insert shell enforcement generation before the sandbox-exec dispatch
- `sbpl.go` — SBPL generator as a pattern reference for how Go generates text artifacts from ResolvedConfig

### Test Behavior Documentation
- `test-sandbox.sh` (94 lines) — shell enforcement test suite: PATH filtering, armor blocking, breadcrumb absence
- `test-policy.sh` (302 lines) — v2 policy+kernel test suite: contains shell-relevant cases too

### Project Analysis
- `.planning/codebase/CONVENTIONS.md` — `_sfx_` prefix for internal vars/functions, `[sandflox]` prefix for messages, section separators, error handling patterns
- `.planning/codebase/ARCHITECTURE.md` — Layer model (shell enforcement is Layer 2-5; this phase ports all of them)
- `.planning/REQUIREMENTS.md` — SHELL-01 through SHELL-08 for this phase

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ResolvedConfig` already carries everything shell tier needs: `Requisites` (path to requisites file), `Writable`/`ReadOnly`/`Denied` (absolute paths), `NetMode` (for curl removal), `FsMode` (for fs-filter logic)
- `WriteCache` already writes `fs-mode.txt`, `writable-paths.txt`, `read-only-paths.txt`, `denied-paths.txt` — Python enforcement reads these directly (no new files)
- Bash reference scripts at `manifest.toml.v2-bash` encode the canonical shell enforcement logic — port verbatim where semantics match

### Established Patterns
- `[sandflox]` prefix on all diagnostic/error/blocked messages to stderr
- `_sfx_` prefix for internal shell vars and helper functions in generated scripts
- Generated scripts start with `# sandflox <purpose> -- generated shell enforcement` header comment
- `.flox/cache/sandflox/` is the canonical cache root; new `sandflox-python/` sibling for Python artifacts
- Exit code 126 from armor functions ("command cannot execute"); `PermissionError` from Python enforcement
- `syscall.Exec` for clean process replacement at the final step

### Integration Points
- `main.go::execWithKernelEnforcement` is where shell enforcement generation hooks in — before the sandbox-exec argv construction, add a call to `GenerateShellEnforcement(config, cacheDir)`
- `exec_darwin.go`/`exec_other.go` argv construction changes: flox activate invocation wraps the user payload in `bash --rcfile` (interactive) or `bash -c 'source ... && exec "$@"'` (non-interactive)
- `cache.go::WriteCache` gains sibling writers for entrypoint.sh, fs-filter.sh, usercustomize.py, symlink bin directory

</code_context>

<specifics>
## Specific Ideas

- All generated artifacts use `_sfx_` prefix for internal variables and `[sandflox]` prefix for user-visible messages — strict match with existing bash codebase convention.
- Script templates stored under `.tmpl` files embedded via `//go:embed` so git diffs show enforcement changes clearly (not buried in Go string literals).
- The Go code that orchestrates shell-tier generation should live in a new `shell.go` (with companion `shell_test.go`), mirroring the `sbpl.go` / `sbpl_test.go` pattern from Phase 2.
- Symlink bin generation must be idempotent — rerunning should update (not accumulate). Either clear-and-recreate or diff-based; Claude's discretion.

</specifics>

<deferred>
## Deferred Ideas

- SEC-01/SEC-02/SEC-03 env-var scrubbing — explicitly Phase 4 scope. Phase 3 scrubs breadcrumbs (`FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`, `FLOX_PATH_PATCHED`) but does NOT touch `AWS_*`, `GITHUB_TOKEN`, etc.
- `sandflox validate` / `status` / `elevate` subcommands — Phase 5 scope.

</deferred>

---

*Phase: 03-shell-enforcement-artifacts*
*Context gathered: 2026-04-16 via Smart Discuss (autonomous)*
