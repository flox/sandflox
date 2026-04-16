# Phase 3: Shell Enforcement Artifacts - Research

**Researched:** 2026-04-16
**Domain:** Go artifact generation (`text/template` + `//go:embed`), bash shell-tier enforcement porting, Python `usercustomize.py` loader mechanics, `flox activate --` argv composition, `os.Symlink` idempotency, subprocess-based integration testing
**Confidence:** HIGH (bash reference and Go Phase 1/2 code are both in-tree; flox activate `--` contract confirmed by Phase 2's working integration tests; all library mechanics verified against official docs)

## Summary

This phase ports the bash `manifest.toml` hook/profile logic and `sandflox.bash` entrypoint generation into Go. Every enforcement layer already has a canonical bash implementation in `manifest.toml.v2-bash` (lines 49-412) and `sandflox.bash` (lines 353-438). The Go port's job is to: (1) produce three generated artifacts — `entrypoint.sh`, `fs-filter.sh`, `usercustomize.py` — byte-compatible enough that shell behavior matches the bash reference, (2) rewire `execWithKernelEnforcement` so `flox activate` is invoked with `-- bash --rcfile <entrypoint>` (interactive) or `-- bash -c 'source <entrypoint> && exec "$@"' -- CMD...` (non-interactive), and (3) manage the requisites symlink bin under `.flox/cache/sandflox/bin/` idempotently.

Library surface is tiny: Go stdlib `embed` + `text/template` + `os.Symlink`. No external Go deps (CORE-01 constraint holds). Runtime library surface on the shell side is also tiny: bash 5.x, Python 3.6+ with `site` module's usercustomize hook. The novel risk area is **template quoting** — `text/template` does NOT auto-escape, so paths with special characters could break the generated bash; research into this hazard below.

**Primary recommendation:** Create `shell.go` (generator) + `shell_test.go` (unit tests) + `shell_integration_test.go` (subprocess tests), mirroring the Phase 2 `sbpl.go` / `sbpl_test.go` / `exec_integration_test.go` layout. Store the three script bodies as `.tmpl` files in a sibling `templates/` directory embedded via `//go:embed templates/*.tmpl`. Generate all three artifacts plus the symlink bin on every run from inside `WriteCache` (Phase 1 already owns the cache layer). Rewire `buildSandboxExecArgv` to wrap the flox argv in a `bash --rcfile` (interactive) or `bash -c 'source entrypoint && exec "$@"'` (non-interactive) envelope.

## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Interactive mode (`sandflox`) delivers enforcement via `flox activate -- bash --rcfile <entrypoint> -i`. Flox establishes its activation env first, then our rcfile layers sandflox enforcement on top. No manifest `[hook]`/`[profile]` sections needed — keeps the minimal manifest decision from Phase 1 D-11 intact.
- **D-02:** Non-interactive mode (`sandflox -- CMD`) delivers enforcement via `flox activate -- bash -c 'source <entrypoint> && exec "$@"' -- CMD ARGS...`. The `exec "$@"` replaces bash with the user command so the process tree stays clean (no extra bash parent).
- **D-03:** The generated orchestrator script lives at `.flox/cache/sandflox/entrypoint.sh` — matches the bash reference path and reuses the already-provisioned cache directory.
- **D-04:** Regenerate all shell enforcement artifacts every run. Matches Phase 2 D-03 for SBPL, avoids cache invalidation bugs, and generation is <1ms in Go. `WriteCache` gains new writers for the shell tier alongside the existing config/path-list writers.
- **D-05:** Three generated scripts:
  - `.flox/cache/sandflox/entrypoint.sh` — PATH wipe, armor function definitions, breadcrumb cleanup, source of fs-filter.sh, Python env var exports
  - `.flox/cache/sandflox/fs-filter.sh` — write-command wrappers (cp/mv/mkdir/rm/rmdir/ln/chmod/tee) plus `_sfx_check_write_target` helper
  - `.flox/cache/sandflox-python/usercustomize.py` — `builtins.open` monkey-patch + `ensurepip` module injection
- **D-06:** Script templates are stored as `.tmpl` files embedded via `//go:embed` and rendered with `text/template`. Enables golden-file tests.
- **D-07:** Symlink bin directory: `.flox/cache/sandflox/bin/`. For each name in the active requisites file, create a symlink pointing to `$FLOX_ENV/bin/<name>`. PATH gets wiped and reset to contain only this directory.
- **D-08:** Missing requisite tool emits `[sandflox] WARNING: <tool> listed in requisites but not in $FLOX_ENV/bin — skipping` to stderr and continues. Graceful degradation.
- **D-09:** `usercustomize.py` lives at `.flox/cache/sandflox-python/usercustomize.py` — matches bash reference.
- **D-10:** entrypoint.sh exports `PYTHONPATH="$FLOX_ENV_CACHE/sandflox-python:$PYTHONPATH"` and `PYTHONUSERBASE="$FLOX_ENV_CACHE/sandflox-python"` so Python's site-customization loader picks up the generated `usercustomize.py`. Mirrors bash.
- **D-11:** `usercustomize.py` reads policy state from the same text files the shell tier uses: `$FLOX_ENV_CACHE/sandflox/fs-mode.txt`, `writable-paths.txt`, `denied-paths.txt`. One source of truth.
- **D-12:** `ensurepip` blocked via module injection — stub `ensurepip` module in `sys.modules` that raises `PermissionError` on import. `builtins.open` wrapper raises `PermissionError("[sandflox] BLOCKED: ...")` on deny.
- **D-13:** Unit tests verify generated script content (string-match on key directives). Mirror Phase 2 `sbpl_test.go` pattern.
- **D-14:** Integration tests use real subprocess spawning (matching Phase 2 `exec_integration_test.go`).
- **D-15:** Tests compare against the bash reference output as a sanity check. Semantically equivalent, not byte-identical.

### Claude's Discretion

- Go function decomposition within the shell tier package (one file per script vs grouped by concern)
- Template variable naming and rendering style (pipeline-heavy vs flat)
- `fs-filter.sh` path-check algorithm implementation details (string matching order, longest-prefix-wins rules)
- Integration test fixture layout under `testdata/`
- Exact wording of `[sandflox] BLOCKED:` messages beyond the established prefix convention

### Deferred Ideas (OUT OF SCOPE)

- SEC-01/SEC-02/SEC-03 env-var scrubbing — explicitly Phase 4 scope. Phase 3 scrubs breadcrumbs (`FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`, `FLOX_PATH_PATCHED`) but does NOT touch `AWS_*`, `GITHUB_TOKEN`, etc.
- `sandflox validate` / `status` / `elevate` subcommands — Phase 5 scope.

## Project Constraints (from CLAUDE.md)

- GSD workflow enforcement — direct repo edits only via GSD commands (we are inside `/gsd:plan-phase`, so this is satisfied)
- No emojis in output files
- Absolute file paths in responses
- Codebase conventions (`CONVENTIONS.md`): `_sfx_` prefix, `[sandflox]` message prefix, `#!/usr/bin/env bash` shebang, section separators, `[sandflox] BLOCKED: <what> is <reason>` format, exit code 126 for shell armor, `PermissionError` for Python enforcement

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SHELL-01 | Wipe PATH to contain only requisites-filtered symlink bin | entrypoint.sh `export PATH="$FLOX_ENV_CACHE/sandflox/bin"` after bin repopulation (bash reference manifest.toml.v2-bash:351). Go generator emits the export. |
| SHELL-02 | Parse requisites file and create symlink bin with listed tools from `$FLOX_ENV/bin` | Two-part: (a) Go `ParseRequisites()` already exists (`config.go:140`); extend `WriteCache` or call from new `WriteShellArtifacts` to iterate + `os.Symlink` into cache bin dir. (b) entrypoint.sh re-verifies at activation time (handles FLOX_ENV changes between runs) — mirrors bash reference manifest.toml.v2-bash:324-341. |
| SHELL-03 | Generate function armor for 27+ package managers (exit 126, `[sandflox] BLOCKED:` prefix) | Port bash function armor block verbatim — 27 functions listed in manifest.toml.v2-bash:369-394 and sandflox.bash:396-421. Also `export -f` block (manifest.toml.v2-bash:396-400). Template with list of names. |
| SHELL-04 | Generate fs-filter.sh wrapping cp/mv/mkdir/rm/rmdir/ln/chmod/tee with path-check policy | Port bash `_sfx_check_write_target` and 8 wrappers from manifest.toml.v2-bash:165-215. Template uses writable/read-only/denied path lists already written by Phase 1 `WriteCache`. |
| SHELL-05 | Generate usercustomize.py blocking ensurepip + wrapping builtins.open | Port embedded Python from manifest.toml.v2-bash:250-306 verbatim (same blocking semantics, reads same cache files). Needs `ENABLE_USER_SITE=1` + `PYTHONPATH` prepend to load. |
| SHELL-06 | Scrub breadcrumb env vars: FLOX_ENV_PROJECT, FLOX_ENV_DIRS, FLOX_PATH_PATCHED | `unset` line at end of entrypoint.sh (manifest.toml.v2-bash:313, 410). |
| SHELL-07 | Conditionally remove curl from symlink bin when `network.mode = "blocked"` | After symlink creation, check `config.NetMode == "blocked"` — if so, `os.Remove(filepath.Join(binDir, "curl"))` (ignore NotExist). Or defer to entrypoint.sh at run time using `net-blocked.flag` (matches bash). Research recommends doing it in Go at WriteCache time (deterministic + one fewer runtime dependency). |
| SHELL-08 | Generated `[sandflox] BLOCKED: <reason>` messages explain the denial | Message format locked by `CONVENTIONS.md`: `[sandflox] BLOCKED: <what> is <reason>`. fs-filter.sh emits `write to "<target>" outside workspace policy` / `is read-only by policy` / `is denied by policy`. Armor emits `<cmd> is not available. Environment is immutable.` Python emits `write to '<file>' is denied by policy` / `outside workspace policy` / `filesystem is read-only (strict mode)`. |

## Standard Stack

### Core (all stdlib)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `embed` | Go 1.22+ (repo uses 1.22, Go 1.26 locally) | Compile-in `.tmpl` files | Only way to ship a single-binary artifact that bundles scripts; zero-dep requirement from CORE-01 |
| `text/template` | Go 1.22+ | Render templates to script text | Mandatory over `html/template`: shell scripts are text, not HTML; html/template auto-escapes `<`, `>`, `&` and would corrupt shell |
| `os` (Symlink, MkdirAll, Remove, RemoveAll, Stat) | Go 1.22+ | Create/refresh symlink bin | Standard POSIX operations; portable |
| `path/filepath` | Go 1.22+ | Path composition | Already used throughout |
| `os/exec` (LookPath) | Go 1.22+ | Test tools in `$FLOX_ENV/bin` at generator time (only for diagnostics — actual symlink targets come from requisites × FLOX_ENV join at runtime) | stdlib |

### Version verification

Go `embed` stabilized in Go 1.16. `text/template` has been stable since Go 1.0. Both shipped unchanged as of Go 1.26 (`go version` on this machine: `go1.26.0 darwin/arm64`). go.mod declares `go 1.22` — compatible with everything needed.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `text/template` | Raw string concatenation with `fmt.Sprintf` / `strings.Builder` | Simpler for small scripts, but 500+ lines of string-building is harder to diff vs. a .tmpl file. Phase 2 `sbpl.go` successfully used `strings.Builder` for a ~100-line output; shell artifacts are 3× larger and have more repeated structure (the armor function list, 8 wrapper declarations). Templates win on maintainability per D-06. |
| `text/template` | `html/template` | `html/template` auto-escapes `<`, `>`, `&`, `'`, `"` — would mangle shell redirects and quoting. NEVER USE for shell generation. |
| `//go:embed` | Inline raw-string literals (`\`` ... \``) | Inlining works but: (a) loses syntax highlighting in editors, (b) no line numbers for template errors, (c) harder to golden-test. `//go:embed` wins. |
| Runtime shell-tier generation | Everything at `WriteCache` time in Go | Runtime (bash/python in entrypoint) generates the same output but adds a dependency on Python at activation time — exactly what we're trying to eliminate. Keep generation in Go per project thesis (PROJECT.md: "No Python dependency"). |

### Installation

No installation. All stdlib.

## Architecture Patterns

### Recommended File Layout

```
/Users/jhogan/sandflox/
├── shell.go              # GenerateShellArtifacts + WriteShellArtifacts + buildBashWrapperArgv
├── shell_test.go         # Unit tests (D-13) — table-driven string-contains assertions
├── shell_integration_test.go   # Subprocess tests (D-14) with `//go:build darwin && integration`
├── templates/
│   ├── entrypoint.sh.tmpl
│   ├── fs-filter.sh.tmpl
│   └── usercustomize.py.tmpl
└── (existing files: policy.go, config.go, cache.go, main.go, sbpl.go, exec_darwin.go, exec_other.go)
```

### Pattern 1: Embedded Template + Rendering

```go
// shell.go — pattern (not committed yet; illustrative)
package main

import (
    "embed"
    "fmt"
    "io"
    "strings"
    "text/template"
)

//go:embed templates/entrypoint.sh.tmpl templates/fs-filter.sh.tmpl templates/usercustomize.py.tmpl
var shellTemplates embed.FS

// Source: pkg.go.dev/embed + pkg.go.dev/text/template (verified HIGH)
type shellTemplateData struct {
    NetMode       string
    FsMode        string
    Writable      []string
    ReadOnly      []string
    Denied        []string
    ArmoredCmds   []string        // 27 package manager names
    WriteCmds     []string        // cp, mv, mkdir, rm, rmdir, ln, chmod, tee
    FloxEnvCache  string          // $FLOX_ENV_CACHE — emitted as literal shell variable reference in most places
}

func renderTemplate(name string, data shellTemplateData) (string, error) {
    tmpl, err := template.ParseFS(shellTemplates, "templates/"+name)
    if err != nil {
        return "", fmt.Errorf("[sandflox] ERROR: parse template %s: %w", name, err)
    }
    var buf strings.Builder
    if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
        return "", fmt.Errorf("[sandflox] ERROR: render template %s: %w", name, err)
    }
    return buf.String(), nil
}
```

### Pattern 2: Idempotent Symlink Bin

```go
// WriteRequisitesBin creates .flox/cache/sandflox/bin/ as a fresh symlink
// farm matching the requisites list. Removes the whole dir first for
// deterministic state (D-04). Idempotent: safe to call repeatedly.
//
// Source: The remove-then-create pattern is used by the bash reference
// (manifest.toml.v2-bash:328-329: `rm -rf "$_sfx_bin"; mkdir -p ...`).
// This cache path is under the project dir; no attacker-controlled
// parent, so the symlink race class (Trail of Bits 2020) is not a
// live threat in sandflox's trust model.
func WriteRequisitesBin(cacheDir, floxEnvBin string, tools []string, netBlocked bool) (int, error) {
    binDir := filepath.Join(cacheDir, "bin")
    if err := os.RemoveAll(binDir); err != nil {
        return 0, fmt.Errorf("[sandflox] ERROR: reset bin dir: %w", err)
    }
    if err := os.MkdirAll(binDir, 0755); err != nil {
        return 0, fmt.Errorf("[sandflox] ERROR: create bin dir: %w", err)
    }
    count := 0
    for _, tool := range tools {
        src := filepath.Join(floxEnvBin, tool)
        if _, err := os.Stat(src); err != nil {
            // D-08 graceful degradation
            fmt.Fprintf(stderr, "[sandflox] WARNING: %s listed in requisites but not in $FLOX_ENV/bin — skipping\n", tool)
            continue
        }
        if netBlocked && tool == "curl" {
            continue  // SHELL-07
        }
        dst := filepath.Join(binDir, tool)
        if err := os.Symlink(src, dst); err != nil {
            return count, fmt.Errorf("[sandflox] ERROR: symlink %s: %w", tool, err)
        }
        count++
    }
    return count, nil
}
```

**Important caveat on "WriteRequisitesBin" timing:** The bash reference performs symlink creation at activation time in `[profile] common` (manifest.toml.v2-bash:326-341) because `$FLOX_ENV` is a flox-activation-time variable. In the Go port, we do NOT have `$FLOX_ENV` at `WriteCache` time (sandflox runs BEFORE flox activate). Two options:

1. **Defer symlink creation to entrypoint.sh** (matches bash behavior): `entrypoint.sh` reads `$FLOX_ENV` from flox's activation context, iterates requisites.txt, and creates symlinks. Go only needs to render the entrypoint template that embeds the requisites tool list or reads it from the staged file.
2. **Hybrid:** Render a list of tools into entrypoint.sh; entrypoint resolves `$FLOX_ENV/bin/<tool>` and symlinks at activation time.

**Recommendation: Option 1 (defer to entrypoint.sh).** Keeps the logic co-located with the PATH export that consumes it, matches bash reference line-for-line, and handles `$FLOX_ENV` naturally. Go's role is to emit the entrypoint template with the armor list baked in and a reference to the staged `requisites.txt`.

### Pattern 3: Bash Wrapper Argv Composition (D-01/D-02)

```go
// Replaces buildSandboxExecArgv's flox-activate tail.
// Interactive:     flox activate -- bash --rcfile <entrypoint> -i
// Non-interactive: flox activate -- bash -c 'source <entrypoint> && exec "$@"' -- user-cmd args...
func buildFloxActivateTail(entrypointPath string, userArgs []string) []string {
    if len(userArgs) == 0 {
        return []string{"--", "bash", "--rcfile", entrypointPath, "-i"}
    }
    // Shell script: source entrypoint, then exec "$@" so bash replaces
    // itself with the user command (clean process tree).
    script := fmt.Sprintf(`source %q && exec "$@"`, entrypointPath)
    argv := []string{"--", "bash", "-c", script, "--"}
    argv = append(argv, userArgs...)
    return argv
}
```

**Source:** Phase 2 `exec_darwin.go::buildSandboxExecArgv` already appends `-- userArgs...` after `flox activate`. Replacing the userArgs with the bash wrapper preserves the existing argv contract. The `--` between `bash -c script` and the user args is the POSIX convention for `bash -c`: `bash -c script name [arg0 arg1 ...]` — `$0` becomes `name` and `$1..` become args. We use `--` as the placeholder name so `$@` (which does not include `$0`) expands to exactly the user command, and `exec "$@"` then replaces the bash process with the user command.

### Anti-Patterns to Avoid

- **Using `html/template` for shell:** Auto-escapes `>`, `<`, `&` — breaks shell redirects like `> /dev/null`. Use `text/template`.
- **Dynamic path substitution inside templates without shell quoting:** A writable path containing `$` or backtick would be interpreted by bash. Mitigation: quote ALL path substitutions in templates with `%q`-compatible single-quote-and-escape. Bash single-quoted strings have no escape mechanism, but you can use the `'\''` pattern to embed single quotes. See Pitfall 1 below.
- **Hand-rolling `exec "$@"` without the `--` argv0 placeholder:** `bash -c 'source X && exec "$@"' CMD ARGS` — here `CMD` becomes `$0` and only `ARGS` are in `$@`. Result: CMD is lost. Use `bash -c 'source X && exec "$@"' -- CMD ARGS` so `--` takes `$0` and `CMD ARGS` are `$@`. (Non-obvious but confirmed by POSIX spec and tested in Phase 2 integration pattern.)
- **Calling `syscall.Exec` inside a test:** Phase 2 Pitfall 7. Test process would inherit the sandbox and tests would self-destruct. Always use `exec.CommandContext` in tests.
- **Running symlink creation + fs-filter sourcing + PATH export in the wrong order:** PATH must be set AFTER the symlink bin is populated, or else the first `export PATH="...bin"` leaves a process that can't find `cat`, `cp`, etc. before the bin contents exist. Bash reference is careful about this; the Go-emitted template must preserve the ordering.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TOML parsing in shell tier | Reparse policy.toml in entrypoint.sh | Read `fs-mode.txt`, `writable-paths.txt`, etc. from `$FLOX_ENV_CACHE/sandflox/` | Phase 1 `WriteCache` already writes these. Duplicating policy parsing creates drift. D-11 enforces one source of truth. |
| Template rendering | `fmt.Sprintf` for 400+ line shell scripts | `text/template` + `//go:embed` | Templates give golden-file testable output, syntax highlighting, smaller diffs when policy evolves |
| Bash function armor list | Hard-code in template literal | Pass `[]string` of armor names through template data | Test-friendly; one source of truth; matches policy-driven style |
| Shell single-quote escaping | Roll your own quote function | Use `%q` verbs with printf only when values reach shell assignment; for paths inside case-statements and literal strings, emit with `printf '%q'` in bash OR escape with the `'\''` pattern in Go helper | Shell quoting is treacherous; see Common Pitfalls below for the canonical escape helper |
| Python `open()` wrapping | Import-hook magic | Monkey-patch `builtins.open` (bash reference proven pattern) | The bash reference's monkey-patch is 40 lines and already works; porting verbatim is cheaper than re-architecting |
| Subprocess integration tests | Unit-test `syscall.Exec` flow | Build binary + `exec.CommandContext` (Phase 2 `TestBuiltBinaryWrapsCommand`) | Unit tests can't observe kernel/shell-tier behavior; subprocess + skip-on-missing-tool is the established Phase 2 pattern |

**Key insight:** Every shell-tier concern already has a canonical bash implementation. The Go port's value is not in inventing new approaches; it's in moving the generation step into a statically-typed, unit-testable context so the generated artifacts are provably correct for all policy inputs.

## Runtime State Inventory

This phase is a **greenfield addition** (new artifact generators + exec rewire), not a rename or migration. Most categories do not apply, but there are two items worth explicit call-out:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — Phase 3 adds generated files under `.flox/cache/sandflox{,-python}/` which are gitignored and regenerated every run (D-04) | None |
| Live service config | None — no external services involved | None |
| OS-registered state | None — no launchd/systemd/task scheduler involvement | None |
| Secrets/env vars | Phase 3 introduces reads of `$FLOX_ENV`, `$FLOX_ENV_CACHE` inside entrypoint.sh, and exports `PYTHONPATH`, `PYTHONUSERBASE`, `ENABLE_USER_SITE`, `PYTHONDONTWRITEBYTECODE`, `PYTHON_NOPIP` — all new, not renaming existing names | None (these are additions) |
| Build artifacts / installed packages | `.flox/cache/sandflox/bin/` symlink farm is a new generated artifact; also new cache dir `.flox/cache/sandflox-python/` | gitignore should already cover `.flox/cache/*` — verify during plan. No manual migration: `os.RemoveAll(binDir)` at every run ensures a fresh state |

**Runtime state audit conclusion:** Nothing to migrate. The entire shell tier is regenerated at every sandflox invocation. The one state concern is: **if a user had the bash v1 `sandflox` still installed and running in the same repo, the previous `.flox/cache/sandflox/` contents would be overwritten on the next Go-sandflox invocation.** This is intentional and matches D-04 (regenerate every run).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| bash (in `$FLOX_ENV/bin`) | entrypoint.sh, fs-filter.sh execution | ✓ | Flox `[install] bash.pkg-path = "bash"` in existing manifest | — |
| python3 (in `$FLOX_ENV/bin`) | usercustomize.py loading | ✓ | Flox `[install] python3.pkg-path = "python3"`; ≥3.6 assumed | — |
| `go` build toolchain | Compile-time only (embed) | ✓ | 1.26.0 detected; go.mod declares 1.22 | — |
| `//go:embed` | Compile-time | ✓ | Go 1.16+ (have 1.26) | — |
| `sandbox-exec` | Still needed from Phase 2 chain | ✓ (Darwin) / ✗ (other) | macOS built-in | Phase 2's existing `exec_other.go` fallback already handles non-Darwin |
| `flox activate -- bash --rcfile <file>` support | D-01 interactive delivery | Assumed ✓ (flox uses standard `exec`-after-`--` contract; confirmed working in Phase 2 `TestBuiltBinaryWrapsCommand` which passes `-- /bin/echo hello`) | Flox 1.10+ | If `--rcfile` semantics break on a future Flox version, fall back to `bash -c 'source <entry> && exec bash -i'` pattern |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None for the tier-3 case; Phase 2's Darwin/other split already covers the kernel tier.

## Common Pitfalls

### Pitfall 1: Shell Quoting in Template Substitutions
**What goes wrong:** A path like `/home/user/my'projects/` in the writable list ends up emitted as `case "$resolved" in /home/user/my'projects/*) _allowed=1;; esac`. The unmatched single quote breaks the case statement.
**Why it happens:** `text/template` does not escape its inputs; paths come from policy.toml and the user could have arbitrary characters.
**How to avoid:** Add a `FuncMap` with a `shellquote` function that wraps every path substitution. The canonical algorithm: single-quote the string, and replace every embedded single quote with `'\''`.

```go
// shellquote returns a string safe for inclusion in bash single-quoted
// contexts. Empty strings become ''.
func shellquote(s string) string {
    return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

Then in the template: `case "$resolved" in {{.Path | shellquote}}*) ...` becomes `case "$resolved" in '/home/user/my'\''projects/'*) ...`. Bash reads this as a single token `/home/user/my'projects/` followed by `*`.

**Warning signs:** Test with a fixture path containing `'`, `$`, `` ` ``, `"`, space; assert the generated shell still parses.

### Pitfall 2: Python usercustomize.py Import Mechanism
**What goes wrong:** The generated `usercustomize.py` is written to `.flox/cache/sandflox-python/usercustomize.py`, `PYTHONUSERBASE` is set — but Python still doesn't load it.
**Why it happens:** Python's `site.execusercustomize()` runs `import usercustomize`, which relies on `sys.path`. Two things must be true:
  1. `ENABLE_USER_SITE` must be `True` (the `site` module's flag). This is toggled via `PYTHONNOUSERSITE` env var (setting it disables user site) and the `-s` command-line flag.
  2. The directory containing `usercustomize.py` must be on `sys.path`. `PYTHONUSERBASE` on its own is NOT enough: Python computes `USER_SITE = PYTHONUSERBASE/lib/python3.X/site-packages` on non-Windows, and `usercustomize.py` is expected at `$USER_SITE/usercustomize.py` — NOT `$PYTHONUSERBASE/usercustomize.py`.
**How to avoid:** The bash reference uses a PYTHONPATH trick: prepend `$FLOX_ENV_CACHE/sandflox-python` to `PYTHONPATH`, which puts the directory on `sys.path` directly. This works because `site.execusercustomize()` does a plain `import usercustomize` — any directory on `sys.path` with a `usercustomize.py` satisfies the import. The entrypoint must export:
  - `ENABLE_USER_SITE=1` (verified: bash reference manifest.toml.v2-bash:308)
  - `PYTHONUSERBASE="$FLOX_ENV_CACHE/sandflox-python"` (belt-and-suspenders — makes the USER_SITE-based lookup also work)
  - `PYTHONPATH="$FLOX_ENV_CACHE/sandflox-python:$PYTHONPATH"` (the actual loading mechanism)
**Warning signs:** Integration test must run `python3 -c "import sys; print('sandflox-python' in ':'.join(sys.path))"` to verify sys.path is correct, then run `python3 -c "open('/etc/tryme', 'w')"` to verify the monkey-patch fires.

### Pitfall 3: `export -f` Does Not Survive `bash -c "script"` Without `BASH_FUNC_*` Propagation
**What goes wrong:** Armor functions defined in entrypoint.sh are exported via `export -f flox nix ...` but a child subshell spawned with `bash -c "flox"` doesn't see them.
**Why it happens:** `export -f` serializes functions into the env as `BASH_FUNC_name%%=...`. For child bash processes to re-import them, they must be invoked as `bash`, not `sh`, and with env propagation intact. Since CVE-2014-6271 (Shellshock), bash is also stricter about which env-var names look like function serializations.
**How to avoid:** (a) Export from entrypoint.sh (already planned); (b) ensure the entrypoint itself runs under `bash`, not `sh` (guaranteed because D-01/D-02 invoke `flox activate -- bash ...`); (c) for non-interactive mode, the `exec "$@"` step preserves env so child bash procs see the BASH_FUNC_* vars. Potential gap: if the user runs `sandflox -- /bin/sh -c 'flox'`, armor is bypassed (sh isn't bash). This is accepted risk — the requisites bin also doesn't include `flox` absolute path, so Layer 1 (PATH) + Layer 5 (kernel) still enforce.
**Warning signs:** Integration test: `sandflox -- bash -c 'pip --version'` must return 126.

### Pitfall 4: `case "$resolved" in path*)` Prefix Matching Edge Cases
**What goes wrong:** `/tmp-foo/file` matches the `case` pattern `/tmp*)` because shell glob is greedy — `/tmp` + any suffix. User's writable list is `/tmp` (without trailing slash), so `/tmp-foo/file` falsely resolves as writable.
**Why it happens:** Bash glob patterns `*` match any characters, not "path separator or end-of-string". The bash reference uses `case "$resolved" in /tmp*)` expecting `/tmp` to be followed by `/` or end-of-string.
**How to avoid:** The bash reference already has this bug (manifest.toml.v2-bash:190-192). We inherit it on the bash side. Options: (a) preserve bug-for-bug compatibility (D-15 says "semantically equivalent" not byte-identical) — accept known limitation; (b) append `/` to directory paths at resolve time (Phase 1 `ResolvePath` preserves trailing slash indicator) and emit pattern `<path>/*|<path>` to cover exact-match and subpath cases. Option (b) is an improvement; discuss with planner whether to make it.
**Warning signs:** Test case: writable `[".", "/tmp"]`, check `resolved=/tmp-foo` does NOT match writable. If the port matches Phase 3's fs-filter to the bash bug, the test will show the same false positive. Document in Open Questions.

### Pitfall 5: `_last_arg="${!#}"` Destination Detection for Write Commands
**What goes wrong:** `cp -r /src /dst` → last arg is `/dst`, check passes. But `cp /src /dst -v` → last arg is `-v`, check wrongly inspects a flag. Or `mkdir -p a b c` → creates three dirs but only checks `c`.
**Why it happens:** The bash reference uses the simple heuristic "last arg is the destination". GNU cp/mv/mkdir support post-source flags and multiple args.
**How to avoid:** Accept the bash limitation (D-15). Kernel tier (Phase 2) catches the actual write; shell tier is an agent-readable early error. Document that `fs-filter.sh` can miss some flag arrangements but kernel will catch; the UX cost is an EPERM instead of a nice `[sandflox] BLOCKED:` message for pathological argv shapes. Not worth rebuilding a full argv parser in shell.
**Warning signs:** Test `cp -r file dir` vs `cp file dir -v`; if one passes and one doesn't, document as known.

### Pitfall 6: Generator Regenerating `.flox/cache/sandflox/bin/` While It's On $PATH
**What goes wrong:** Rerunning `sandflox` from inside an existing sandbox (e.g., nested invocation during debugging) regenerates the symlink bin while the current shell's PATH still points at it. If `os.RemoveAll(binDir)` runs before re-symlinking, the shell briefly has no `cat`, `ls`, etc. — any command run during the window fails with 127.
**Why it happens:** `sandflox` is not meant to be re-invoked inside a sandbox, but nothing currently prevents it. CMD-03 (`sandflox elevate`) in Phase 5 is the sanctioned way.
**How to avoid:** Phase 3 scope is just generation. Phase 5 `elevate` will add nesting detection. For Phase 3: detect nested invocation via `$SANDFLOX_ENABLED=1` and print a warning before regeneration, OR write the new bin to `bin.tmp/`, then `os.Rename(binDir, binDir+".old")` + `os.Rename(binDir+".tmp", binDir)` + `os.RemoveAll(binDir+".old")` for atomic swap.
**Warning signs:** Integration test: detect `$SANDFLOX_ENABLED` in entrypoint template and emit a `[sandflox] WARNING: nested invocation` if set. Flag for planner.

### Pitfall 7: Template `{{.Variable}}` Ambiguity With Shell `${var}` Syntax
**What goes wrong:** A `.tmpl` file contains both Go template `{{.FloxEnvCache}}` substitutions and literal bash `${FLOX_ENV_CACHE}` variable references. A careless template author writes `{{.Var}}` intending to emit the literal string `${FLOX_ENV_CACHE}` but Go template interprets braces and fails.
**Why it happens:** `{{` and `}}` are Go template delimiters; they do not conflict with shell `${}` syntactically but the mental overlap is a hazard.
**How to avoid:** In templates, emit shell's `${VAR}` as literal text (no interpolation at Go side) — Go template only touches `{{.Name}}` constructs. Shell `$VAR` and `${VAR}` pass through untouched. Review by searching `.tmpl` files for `{{` and confirming each one is intentional.
**Warning signs:** `go test ./...` catches template parse errors loudly. Unit test should render every template with a fully-populated `shellTemplateData`.

### Pitfall 8: Subprocess Test Must Run Outside the Sandbox (Phase 2 Pitfall 7 Redux)
**What goes wrong:** Shell integration test for `cp /etc/hosts /tmp/stolen` blocks by fs-filter — but the test process itself is under a sandbox (because Go's test runner inherited it from a sandflox invocation).
**Why it happens:** Same class as Phase 2 Pitfall 7. If someone runs `go test` from inside a sandflox shell, the test process inherits the sandbox.
**How to avoid:** Never use `syscall.Exec` from test process. Always use `exec.CommandContext` so subprocesses are children of the unsandboxed test process. Add a skip clause: `if os.Getenv("SANDFLOX_ENABLED") == "1" { t.Skip("cannot run integration tests inside a sandflox shell") }` — though this is mostly belt-and-suspenders since the bash integration hook doesn't kick in unless flox is active.
**Warning signs:** Suite fails mysteriously when run in certain terminals. Check `SANDFLOX_ENABLED` env.

## Code Examples

### Example 1: embedded template rendering

```go
// Source: pkg.go.dev/embed + pkg.go.dev/text/template (HIGH confidence)
// templates/fs-filter.sh.tmpl (illustrative excerpt):
//   # sandflox fs-filter -- generated shell enforcement
//   _sfx_check_write_target() {
//     local target="$1"
//     local resolved
//     resolved="$(cd "$(dirname "$target")" 2>/dev/null && pwd)/$(basename "$target")" 2>/dev/null || resolved="$target"
//     # Check denied paths
//     {{range .Denied}}case "$resolved" in {{. | shellquote}}*) echo "[sandflox] BLOCKED: write to {{.}} is denied by policy" >&2; return 126;; esac
//     {{end}}
//     {{if eq .FsMode "strict"}}
//     echo "[sandflox] BLOCKED: filesystem is read-only (strict mode)" >&2
//     return 126
//     {{else}}
//     local _allowed=0
//     {{range .Writable}}case "$resolved" in {{. | shellquote}}*) _allowed=1;; esac
//     {{end}}
//     if [ "$_allowed" -eq 0 ]; then
//       echo "[sandflox] BLOCKED: write to \"$target\" outside workspace policy" >&2
//       return 126
//     fi
//     {{range .ReadOnly}}case "$resolved" in {{. | shellquote}}*) echo "[sandflox] BLOCKED: \"$target\" is read-only by policy" >&2; return 126;; esac
//     {{end}}
//     {{end}}
//     return 0
//   }
//   {{range .WriteCmds}}_sfx_real_{{.}}="$(command -v {{.}} 2>/dev/null)"
//   {{.}}() {
//     local _last_arg="${!#}"
//     _sfx_check_write_target "$_last_arg" || return 126
//     "$_sfx_real_{{.}}" "$@"
//   }
//   export -f {{.}}
//   {{end}}

func GenerateFsFilter(cfg *ResolvedConfig) (string, error) {
    data := shellTemplateData{
        FsMode:    cfg.FsMode,
        Writable:  cfg.Writable,
        ReadOnly:  cfg.ReadOnly,
        Denied:    cfg.Denied,
        WriteCmds: []string{"cp", "mv", "mkdir", "rm", "rmdir", "ln", "chmod", "tee"},
    }
    return renderTemplateWithFuncs("fs-filter.sh.tmpl", data)
}
```

### Example 2: armor function list as template data

```go
// Source: manifest.toml.v2-bash:369-394 — 27 package managers
var ArmoredCommands = []string{
    "flox", "nix", "nix-env", "nix-store", "nix-shell", "nix-build",
    "apt", "apt-get", "yum", "dnf",
    "brew", "snap", "flatpak",
    "pip", "pip3", "npm", "npx", "yarn", "pnpm",
    "cargo", "go", "gem", "composer", "uv",
    "docker", "podman",
}
// Template emits:
//   _sandflox_blocked() {
//     echo "[sandflox] BLOCKED: $1 is not available. Environment is immutable." >&2
//     return 126
//   }
//   {{range .ArmoredCmds}}{{.}}() { _sandflox_blocked {{.}}; }
//   {{end}}
//   export -f _sandflox_blocked {{range .ArmoredCmds}}{{.}} {{end}}
```

### Example 3: buildSandboxExecArgv rewire for D-01/D-02

```go
// Source: D-01/D-02 in CONTEXT.md; Phase 2 exec_darwin.go:48-62 as base
func buildSandboxExecArgv(sbplPath, projectDir, home, floxCachePath, floxAbsPath, entrypointPath string, userArgs []string) []string {
    argv := []string{
        "sandbox-exec",
        "-f", sbplPath,
        "-D", "PROJECT=" + projectDir,
        "-D", "HOME=" + home,
        "-D", "FLOX_CACHE=" + floxCachePath,
        floxAbsPath, "activate",
    }
    if len(userArgs) == 0 {
        // Interactive (D-01)
        argv = append(argv, "--", "bash", "--rcfile", entrypointPath, "-i")
    } else {
        // Non-interactive (D-02)
        script := fmt.Sprintf(`source %q && exec "$@"`, entrypointPath)
        argv = append(argv, "--", "bash", "-c", script, "--")
        argv = append(argv, userArgs...)
    }
    return argv
}
```

Existing Phase 2 `exec_test.go` table assertions need to be updated — Wave 0 gap below.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Shell enforcement in `manifest.toml` `[hook]` + `[profile]` (bash reference) | Shell enforcement generated by Go binary and delivered via `flox activate -- bash --rcfile` | Phase 3 of Go rewrite | No manifest content beyond `[install]` (D-11 Phase 1); distribution simpler; tests now stdlib-only Go |
| Python policy parser embedded in bash hook | Single Go policy parser (Phase 1) + cached state files that shell tier reads | Phase 1 | One source of truth (D-11); no TOML drift between kernel & shell tiers |
| `[profile] common` + `entrypoint.sh` duplication for interactive vs non-interactive | Single `entrypoint.sh` sourced by both modes via `--rcfile` (interactive) and `bash -c 'source'` (non-interactive) | Phase 3 D-01/D-02 | De-duplication; fewer moving parts |

**Deprecated/outdated:**
- The `manifest.toml` `[hook] on-activate` embedded Python block (manifest.toml.v2-bash:61-227) is the old approach. New approach: Go generator + sourced bash script. The manifest.toml stays minimal (Phase 1 D-11).
- Runtime requisites symlink creation done by bash in `[profile] common` (manifest.toml.v2-bash:326-341) moves into entrypoint.sh — same logic, same location pattern, but rendered from a Go template instead of written inline in the manifest.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` stdlib (Go 1.22+) — same as Phase 2 |
| Config file | none — built into `go test` |
| Quick run command | `go test -run 'TestGenerate(Entrypoint\|FsFilter\|Usercustomize\|Armor\|RequisitesBin)' ./...` (unit only, seconds) |
| Full suite command | `go test -tags integration ./...` (unit + subprocess integration, needs flox + sandbox-exec on darwin) |
| Phase gate | `go test -tags integration ./...` fully green from the repo root |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SHELL-01 | PATH wipe — generated entrypoint.sh exports `PATH` to contain only the cache bin dir | unit (content match) | `go test -run TestGenerateEntrypoint_PathExport ./...` | Wave 0 — `shell_test.go` + `templates/entrypoint.sh.tmpl` |
| SHELL-01 | PATH wipe — observable via subprocess: `echo $PATH` inside sandbox contains only cache bin | integration | `go test -tags integration -run TestShellEnforces_PathWipe ./...` | Wave 0 — `shell_integration_test.go` |
| SHELL-02 | Requisites parse — `ParseRequisites()` already exists; unit-test it against fixture files | unit | `go test -run TestParseRequisites ./...` (add coverage if missing) | Partial: `config.go::ParseRequisites` exists; needs `config_test.go` coverage (Wave 0 gap check) |
| SHELL-02 | Requisites symlink — Go generator creates `$cacheDir/bin/<tool>` symlinks pointing to `$FLOX_ENV/bin/<tool>` | unit (file mode + readlink) | `go test -run TestWriteRequisitesBin_Symlinks ./...` | Wave 0 |
| SHELL-02 | Requisites symlink — runtime verified via subprocess `readlink $binDir/bash` matches `$FLOX_ENV/bin/bash` | integration | `go test -tags integration -run TestShellEnforces_SymlinkBin ./...` | Wave 0 |
| SHELL-03 | Armor content — generated entrypoint.sh contains `flox()`, `nix()`, ... `podman()` functions and `export -f` block | unit (strings.Contains across all 27 names) | `go test -run TestGenerateEntrypoint_ArmorFunctions ./...` | Wave 0 |
| SHELL-03 | Armor behavior — subprocess invocation `bash -c 'source entrypoint.sh; pip --version; echo $?'` prints `[sandflox] BLOCKED:` and returns 126 | integration | `go test -tags integration -run TestShellEnforces_ArmorBlocks ./...` | Wave 0 |
| SHELL-04 | fs-filter content — generated fs-filter.sh contains `_sfx_check_write_target` + 8 wrapper functions + `export -f` block | unit | `go test -run TestGenerateFsFilter_Wrappers ./...` | Wave 0 |
| SHELL-04 | fs-filter behavior — `bash -c 'source fs-filter.sh; cp /etc/hosts /tmp/stolen'` emits `[sandflox] BLOCKED:` to stderr and exits 126 | integration | `go test -tags integration -run TestShellEnforces_FsFilterBlocks ./...` | Wave 0 |
| SHELL-05 | Python content — usercustomize.py contains ensurepip stub + `builtins.open` wrapper + reads `fs-mode.txt`/path lists | unit | `go test -run TestGenerateUsercustomize_BlocksEnsurepip ./...` | Wave 0 |
| SHELL-05 | Python behavior — `python3 -c "open('/etc/passwd','w')"` with PYTHONPATH set raises PermissionError with `[sandflox]` prefix | integration | `go test -tags integration -run TestShellEnforces_PythonOpenBlocked ./...` | Wave 0 |
| SHELL-05 | Python behavior — `python3 -m ensurepip` with PYTHONPATH set raises SystemExit with `[sandflox]` prefix | integration | `go test -tags integration -run TestShellEnforces_PythonEnsurepipBlocked ./...` | Wave 0 |
| SHELL-06 | Breadcrumb unset — entrypoint.sh contains `unset FLOX_ENV_PROJECT FLOX_ENV_DIRS FLOX_PATH_PATCHED` | unit | `go test -run TestGenerateEntrypoint_BreadcrumbUnset ./...` | Wave 0 |
| SHELL-06 | Breadcrumb unset — runtime: `bash -c 'source entry; echo ${FLOX_ENV_PROJECT-UNSET}'` prints `UNSET` | integration | `go test -tags integration -run TestShellEnforces_BreadcrumbsCleared ./...` | Wave 0 |
| SHELL-07 | curl removal — when `NetMode == "blocked"`, the generated symlink bin does NOT contain a `curl` entry | unit (directory listing) | `go test -run TestWriteRequisitesBin_SkipsCurlWhenNetBlocked ./...` | Wave 0 |
| SHELL-07 | curl removal — subprocess: `command -v curl` inside sandbox when blocked returns non-zero | integration | `go test -tags integration -run TestShellEnforces_CurlRemovedWhenNetBlocked ./...` | Wave 0 |
| SHELL-08 | Error message format — all BLOCKED messages in generated artifacts match `[sandflox] BLOCKED: <what> is <reason>` regex | unit (regex across all three templates) | `go test -run TestGenerate_BlockedMessagesFormat ./...` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -run 'TestGenerate|TestWriteRequisitesBin' ./...` — unit shell tests only, <1s
- **Per wave merge:** `go test ./...` — full unit suite (includes Phase 1/2 tests too), <5s
- **Phase gate:** `go test -tags integration ./...` — full unit + subprocess integration with sandbox-exec + flox; 60s budget matches Phase 2

### Wave 0 Gaps

- [ ] `/Users/jhogan/sandflox/templates/` directory — new, does not exist yet
- [ ] `/Users/jhogan/sandflox/templates/entrypoint.sh.tmpl` — covers SHELL-01, SHELL-03, SHELL-06, SHELL-10 (D-10 Python env), PYTHONPATH setup
- [ ] `/Users/jhogan/sandflox/templates/fs-filter.sh.tmpl` — covers SHELL-04, SHELL-08
- [ ] `/Users/jhogan/sandflox/templates/usercustomize.py.tmpl` — covers SHELL-05, SHELL-08
- [ ] `/Users/jhogan/sandflox/shell.go` — GenerateShellArtifacts, WriteShellArtifacts, buildBashWrapperArgv (or integrated into buildSandboxExecArgv); consumes `shellquote` FuncMap helper
- [ ] `/Users/jhogan/sandflox/shell_test.go` — unit tests listed above
- [ ] `/Users/jhogan/sandflox/shell_integration_test.go` — `//go:build darwin && integration`; subprocess tests using real bash + python3 + sandbox-exec
- [ ] `/Users/jhogan/sandflox/config_test.go` — add `TestParseRequisites` table test (covers SHELL-02; existing config_test.go tests are for ResolveConfig)
- [ ] Update `/Users/jhogan/sandflox/exec_test.go` — the existing Phase 2 `TestBuildSandboxExecArgs_Interactive` and `_WithUserCommand` tests will FAIL after the argv rewire for D-01/D-02. They must be updated to expect the new `-- bash --rcfile ...` tail shapes. Flag prominently for planner.
- [ ] Update `/Users/jhogan/sandflox/cache.go::WriteCache` — add calls to `WriteShellArtifacts(cacheDir, config)` and `WriteRequisitesBin(...)` — OR factor shell-tier writes into their own function called from `main.go` alongside `WriteCache`. Recommend the latter to keep `WriteCache` focused on config/data and the new `shell.go` focused on shell artifacts.
- [ ] Framework install: none — Go stdlib only; `go test` already works

## Open Questions

1. **Bug-for-bug bash compat on fs-filter prefix matching (Pitfall 4)**
   - What we know: Bash reference uses `case "$resolved" in <path>*)` which over-matches `/tmp-foo` when writable is `/tmp`.
   - What's unclear: Do we preserve the bug for D-15 semantic equivalence, or improve the pattern to `<path>/*|<path>`?
   - Recommendation: Improve. Kernel tier is the backstop; false positives in shell tier are safer than false negatives. Plan should include the improvement as a sub-task and update the bash-comparison test to assert the stricter pattern.

2. **Symlink creation timing — WriteCache vs entrypoint runtime**
   - What we know: `$FLOX_ENV` is not set when sandflox runs (before `flox activate`). Bash reference does symlinking inside profile/entrypoint.
   - What's unclear: D-04 says "regenerate every run" — does that mean Go regenerates the symlink bin, or the entrypoint.sh does?
   - Recommendation: entrypoint.sh does the symlinking, Go renders the template that tells entrypoint how. This matches bash reference and handles `$FLOX_ENV` correctly. SHELL-07 curl removal also lives in entrypoint.sh via the `net-blocked.flag` check that Phase 1 already writes.

3. **`ENABLE_USER_SITE=1` semantics across flox's Python**
   - What we know: `site` module requires `ENABLE_USER_SITE=True` to import usercustomize. Python auto-sets this False if `PYTHONNOUSERSITE` is set or `-s` was passed.
   - What's unclear: Does Flox's Python invocation path pass `-s` or set `PYTHONNOUSERSITE`? If yes, the usercustomize trick breaks.
   - Recommendation: Integration test SHELL-05 validates this end-to-end. If it fails, fall back to `sitecustomize.py` (ignores ENABLE_USER_SITE) with a `PYTHONSTARTUP` trick or direct `-c "exec(open('...').read())"` injection. Flag as a risk for the plan.

4. **Nested invocation detection (Pitfall 6)**
   - What we know: `SANDFLOX_ENABLED=1` is already set inside an active sandbox. Detecting this in sandflox (outer) is easy.
   - What's unclear: Should Phase 3 warn and continue, hard-error, or silently regenerate (and risk the in-flight PATH race)?
   - Recommendation: Defer to Phase 5 `elevate` subcommand for the principled nested case. For Phase 3, emit a WARNING if `SANDFLOX_ENABLED=1` is detected and use the rename-then-swap pattern to avoid the PATH race. Flag for planner.

## Sources

### Primary (HIGH confidence)

- **In-tree canonical bash reference:** `/Users/jhogan/sandflox/manifest.toml.v2-bash` (418 lines) — line-by-line spec for hook + profile.common shell tier
- **In-tree canonical bash reference:** `/Users/jhogan/sandflox/sandflox.bash` (500 lines) — entrypoint.sh generation pattern (lines 353-438) and SBPL generation (already ported in Phase 2)
- **In-tree Go implementation:** `/Users/jhogan/sandflox/policy.go`, `config.go`, `cache.go`, `main.go`, `sbpl.go`, `exec_darwin.go`, `exec_other.go`, `cli.go` — Phase 1/2 extension targets; argv-building pattern and test harness established
- **In-tree test references:** `/Users/jhogan/sandflox/sbpl_test.go`, `exec_test.go`, `exec_integration_test.go`, `cache_test.go`, `main_test.go`, `policy_test.go`, `config_test.go`, `cli_test.go`
- **In-tree requisites files:** `requisites.txt` (55 tools), `requisites-minimal.txt` (28 tools), `requisites-full.txt` (55 tools)
- **In-tree policy + tests:** `policy.toml`, `test-sandbox.sh`, `test-policy.sh` — behavioral spec for shell tier
- **Go embed package docs:** https://pkg.go.dev/embed — verified //go:embed directive syntax, glob patterns, embed.FS usage with text/template via ParseFS
- **Go text/template docs:** https://pkg.go.dev/text/template — verified FuncMap, ParseFS, ExecuteTemplate; verified text/template does NOT auto-escape (mandatory over html/template for shell)
- **Python site module docs:** https://docs.python.org/3/library/site.html — verified usercustomize.py loading mechanism; import goes through sys.path so PYTHONPATH trick works
- **Phase 2 completion artifacts:** `.planning/phases/02-kernel-enforcement-sbpl-sandbox-exec/02-*-PLAN.md` (referenced by Phase 2 state) — established subprocess integration test pattern

### Secondary (MEDIUM confidence)

- **Flox activate CLI contract:** https://flox.dev/docs/reference/command-reference/flox-activate/ — redirects to man page; the `--` argv contract is documented as "exec `CMD` directly after activation except `[profile]` scripts"; `--rcfile` pass-through is untested against Flox by us but matches generic bash `--` semantics; Phase 2 TestBuiltBinaryWrapsCommand proves flox's `--` accepts bash directly
- **Trail of Bits symlink safety:** https://blog.trailofbits.com/2020/11/24/smart-and-simple-ways-to-prevent-symlink-attacks-in-go/ — informs "is the remove-then-create pattern safe?" answer — yes, in this trust model (cache dir is not attacker-controlled)
- **CPython site.py source (verified):** https://github.com/python/cpython/blob/main/Lib/site.py — confirms `import usercustomize` uses sys.path; any directory on sys.path with a usercustomize.py satisfies the import

### Tertiary (LOW confidence — flagged)

- General claims about `export -f` function propagation across `bash -c` invocations — tested empirically in Phase 3 integration tests is the best verification
- Claims about `flox activate`'s ability to pass `--rcfile` through `--` — direct test in an integration subprocess is required to verify; if it breaks, the plan's fallback is to replace the script with a self-sourcing wrapper like `bash -c '[ -z "$SF_SOURCED" ] && SF_SOURCED=1 source <entry>; exec bash -i'` (brittle, avoid unless D-01 pattern fails)

## Metadata

**Confidence breakdown:**
- Standard stack (Go embed, text/template, stdlib): HIGH — all libraries are stdlib, all used successfully in Phase 2, all docs verified against official sources
- Architecture (three artifacts + shell.go + templates/): HIGH — matches Phase 2 sbpl.go pattern, matches bash reference layout, matches D-05/D-06 user decisions
- Python mechanism (usercustomize loading): HIGH — verified against Python docs + CPython source; the PYTHONPATH trick is how the bash reference already works
- `flox activate -- bash --rcfile` contract: MEDIUM — generic bash behavior is clear, Flox-specific pass-through proven for `-- CMD` but not specifically for `--rcfile`; integration test will validate
- Pitfalls (shell quoting, prefix matching, bash -c argv0): HIGH — well-documented shell hazards with known remedies
- Integration test strategy: HIGH — Phase 2 `exec_integration_test.go` pattern is proven (4 passing tests, 1 manually skipped) and directly transplants to Phase 3

**Research date:** 2026-04-16
**Valid until:** 2026-05-16 (30 days — stack is stdlib and stable; the only time-sensitive source is Flox's activate contract which could change in a major version)

## RESEARCH COMPLETE

**Phase:** 03 - Shell Enforcement Artifacts
**Confidence:** HIGH

### Key Findings
1. Every shell-tier enforcement layer has a canonical bash implementation in `manifest.toml.v2-bash` (lines 49-412) and `sandflox.bash` (lines 353-438). The Go port is a faithful translation, not a redesign.
2. `//go:embed` + `text/template` from stdlib covers the generator needs with zero external deps. `html/template` MUST NOT be used — it escapes shell-significant characters.
3. Python `usercustomize.py` loads via `import usercustomize` on sys.path; the bash reference's `PYTHONPATH` prepend + `ENABLE_USER_SITE=1` + `PYTHONUSERBASE=...` triple is the working incantation and must be preserved.
4. The `-- bash --rcfile <entry> -i` (interactive) and `-- bash -c 'source <entry> && exec "$@"' --` (non-interactive) argv patterns from D-01/D-02 require a rewrite of `buildSandboxExecArgv` — existing Phase 2 tests (`exec_test.go`) assert the old argv shape and MUST be updated as part of this phase, not broken.
5. Symlink bin creation is best deferred to entrypoint.sh runtime (not Go WriteCache time) because `$FLOX_ENV` is only available inside `flox activate`. Go's responsibility is to render the entrypoint template with the requisites tool list baked in via `text/template`.

### File Created
`/Users/jhogan/sandflox/.planning/phases/03-shell-enforcement-artifacts/03-RESEARCH.md`

### Confidence Assessment
| Area | Level | Reason |
|------|-------|--------|
| Standard Stack | HIGH | All stdlib; already in use; docs verified |
| Architecture | HIGH | Mirrors Phase 2 sbpl.go proven pattern and locked D-05/D-06 decisions |
| Pitfalls | HIGH | All catalogued from documented shell/Python/Go hazards with remedies |
| Validation | HIGH | Phase 2 subprocess test pattern already proven; requirement-to-test mapping is 1:1 |
| Flox `--rcfile` contract | MEDIUM | Integration test will verify; fallback documented |

### Open Questions
1. fs-filter.sh prefix-match bug — preserve or improve? (Recommend improve.)
2. Symlink timing — Go or entrypoint? (Recommend entrypoint for `$FLOX_ENV` access.)
3. Python `ENABLE_USER_SITE` — does Flox's python3 honor it? (Integration-test will reveal.)
4. Nested sandflox detection — Phase 3 warn or Phase 5 handle? (Recommend Phase 5 ownership; Phase 3 warn-only.)

### Ready for Planning
Research complete. Planner can now create `03-01-PLAN.md`, `03-02-PLAN.md`, `03-03-PLAN.md` with confident task decomposition, a clear requirements→test map, and all known pitfalls documented.

---

Sources:
- [flox activate — Flox Docs](https://flox.dev/docs/reference/command-reference/flox-activate/)
- [Go embed package docs](https://pkg.go.dev/embed)
- [Go text/template docs](https://pkg.go.dev/text/template)
- [Python site module docs](https://docs.python.org/3/library/site.html)
- [CPython Lib/site.py source](https://github.com/python/cpython/blob/main/Lib/site.py)
- [Trail of Bits — Smart and simple ways to prevent symlink attacks in Go](https://blog.trailofbits.com/2020/11/24/smart-and-simple-ways-to-prevent-symlink-attacks-in-go/)
- [Bash Reference Manual — Command Execution Environment](https://www.gnu.org/software/bash/manual/html_node/Command-Execution-Environment.html)
- [PEP 370 — Per user site-packages directory](https://peps.python.org/pep-0370/)
