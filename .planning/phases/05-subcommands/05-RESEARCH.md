# Phase 5: Subcommands - Research

**Researched:** 2026-04-17
**Domain:** Go CLI subcommand routing, cache state inspection, process re-exec
**Confidence:** HIGH

## Summary

Phase 5 adds three subcommands (`validate`, `status`, `elevate`) to the sandflox Go binary. The existing codebase already contains all the building blocks: policy parsing, config resolution, cache writing, diagnostics emission, SBPL generation, and process exec. Each subcommand is a different slice through this existing pipeline -- `validate` runs steps 1-7 without step 8, `status` reads cached artifacts written by step 5, and `elevate` runs the full pipeline but targets an already-active flox session instead of invoking `flox activate`.

The Go stdlib `flag` package supports subcommand routing natively via positional argument inspection before `FlagSet.Parse()`. The existing `ParseFlags` function uses `flag.NewFlagSet` with `ContinueOnError`, which already handles the case where the first argument is not a recognized flag. Subcommand routing inserts before `ParseFlags` in `main()`, checks `os.Args[1]` against a known set, and delegates to per-subcommand handler functions that reuse the existing pipeline stages.

**Primary recommendation:** Add a `subcommand.go` file containing routing logic + three handler functions (`runValidate`, `runStatus`, `runElevate`). Modify `main.go` minimally (insert router call at top of `main()`). Add `ReadCache` to `cache.go` as the inverse of `WriteCache`. Add `buildElevateArgv` to `exec_darwin.go` as a variant of `buildSandboxExecArgv` that omits `flox activate`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Positional first-arg routing: check `os.Args[1]` against known subcommands (`validate`, `status`, `elevate`) before flag parsing. Matches Go CLI conventions (`go build`, `flox activate`)
- Both flag positions work: `sandflox --debug validate` and `sandflox validate --debug` are equivalent. Parse global flags, strip subcommand, re-parse remainder
- Unknown first args are NOT treated as subcommands: they route to the default exec pipeline (backward-compatible with `sandflox -- CMD` behavior)
- No subcommand-specific flags in v1: global flags (`--debug`, `--profile`, `--policy`, `--net`, `--requisites`) apply to all subcommands
- `validate` and `status` use plain text with `[sandflox]` prefix, matching existing diagnostic output style (Phase 1 CORE-07)
- `validate` summary: profile, network mode, filesystem mode, tool count from requisites, denied path count. `--debug` adds full path lists and SBPL rule count (reuses `emitDiagnostics` logic)
- `status` reads from cache files (`config.json`, `net-mode.txt`, `active-profile.txt`) -- shows "live" cached state
- `status` outside a sandbox: error "Not in a sandflox session -- no cached state found. Run `sandflox` first." Exit 1
- Detect existing sandbox via `SANDFLOX_ENABLED=1` env var (already set by entrypoint.sh from Phase 3). If present -> print "already sandboxed" and exit 0
- Detect flox session via `FLOX_ENV` env var. If missing -> error "Not in a flox session. Run `flox activate` first." Exit 1
- `elevate` accepts the same flags as default mode: `--profile`, `--policy`, `--net`, `--debug` all work. Runs full pipeline (parse policy, resolve config, write cache, generate artifacts) but skips `flox activate` in argv
- Re-exec: `sandbox-exec -f <sbpl> -D ... bash --rcfile <entrypoint> -i` -- wraps current shell under sandbox-exec. Flox activate skipped since already inside it. Uses `syscall.Exec` for clean PID replacement

### Claude's Discretion
- Internal code organization (new files vs extending main.go) at Claude's discretion
- Error message phrasing within `[sandflox]` convention
- Test strategy for subcommands (unit vs integration split)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CMD-01 | `sandflox validate` parses policy.toml, generates SBPL (dry-run), and reports what would be enforced without executing | Reuse existing pipeline steps 1-7 (ParsePolicy, ResolveConfig, WriteCache, WriteShellArtifacts, emitDiagnostics). Add tool count from ParseRequisites. Skip step 8 (exec). |
| CMD-02 | `sandflox status` reads cached enforcement state and reports active profile, blocked paths, allowed tools, network mode | Add ReadCache function to cache.go that reads config.json back into ResolvedConfig. Check FLOX_ENV_CACHE or cwd-based cache dir. Read requisites.txt for tool count. |
| CMD-03 | `sandflox elevate` from within a `flox activate` session re-execs the current shell under sandbox-exec with generated SBPL (one-time bounce with re-entry detection) | Add buildElevateArgv to exec_darwin.go that emits sandbox-exec argv without flox activate. Detect SANDFLOX_ENABLED for re-entry. Detect FLOX_ENV for session check. |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `flag` | 1.22 | CLI flag parsing with FlagSet | Already used in cli.go; supports subcommand pattern natively |
| Go stdlib `encoding/json` | 1.22 | Read config.json for status subcommand | Already used in cache.go for writing; Unmarshal is the inverse |
| Go stdlib `os` | 1.22 | Env var checks, file reads | Already used throughout |
| Go stdlib `syscall` | 1.22 | Process replacement for elevate | Already used in exec_darwin.go |

### Supporting
No new libraries needed. All functionality uses Go stdlib already present in the codebase.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Manual os.Args routing | cobra / urfave/cli | External dependency -- violates CORE-01 zero-dep constraint |
| Custom JSON read | TOML config read | JSON is already the cache format; TOML would be a new read path |

**Installation:**
No new packages. `go.mod` unchanged.

## Architecture Patterns

### Recommended Project Structure
```
.
├── main.go              # Modified: insert subcommand router at top of main()
├── subcommand.go        # NEW: routing logic + runValidate, runStatus, runElevate handlers
├── cli.go               # Modified: extract subcommand from args before flag parsing
├── cache.go             # Modified: add ReadCache (inverse of WriteCache)
├── exec_darwin.go       # Modified: add buildElevateArgv
├── exec_other.go        # Modified: add elevate stub (warn: macOS-only)
├── subcommand_test.go   # NEW: unit tests for routing, validate, status
├── exec_test.go         # Modified: add TestBuildElevateArgv
└── ...
```

### Pattern 1: Positional Subcommand Routing (Go stdlib)

**What:** Extract subcommand from `os.Args` before invoking `flag.FlagSet.Parse`. The Go standard library `flag` package does not support subcommands directly, but the canonical Go CLI pattern (used by `go`, `kubectl`, `flox`) is to inspect `os.Args[1]`, check against a known set, then create per-subcommand flag sets or reuse global ones.

**When to use:** Always -- this is the decided routing mechanism.

**Example:**
```go
// subcommand.go

// knownSubcommands is the set of recognized subcommand names.
var knownSubcommands = map[string]bool{
    "validate": true,
    "status":   true,
    "elevate":  true,
}

// extractSubcommand inspects args for a subcommand name. It handles both
// positions: `sandflox validate --debug` and `sandflox --debug validate`.
// Returns the subcommand name (empty string if none found) and the
// remaining args with the subcommand stripped out.
func extractSubcommand(args []string) (string, []string) {
    var remaining []string
    subcmd := ""
    for _, arg := range args {
        if subcmd == "" && knownSubcommands[arg] {
            subcmd = arg
        } else {
            remaining = append(remaining, arg)
        }
    }
    return subcmd, remaining
}
```

**Confidence:** HIGH -- this is the standard Go idiom and matches the locked decision.

### Pattern 2: ReadCache (Inverse of WriteCache)

**What:** Read `config.json` from the cache directory and deserialize into `ResolvedConfig`. The `status` subcommand needs this to report the live enforcement state.

**When to use:** `sandflox status` -- reads cached state without parsing policy.toml.

**Example:**
```go
// cache.go

// ReadCache reads the cached config.json from cacheDir and returns the
// deserialized ResolvedConfig. Returns an error if the cache directory
// does not exist or config.json is missing/corrupt.
func ReadCache(cacheDir string) (*ResolvedConfig, error) {
    configPath := filepath.Join(cacheDir, "config.json")
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, fmt.Errorf("[sandflox] ERROR: cannot read cached config: %w", err)
    }
    var config ResolvedConfig
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("[sandflox] ERROR: corrupt cached config: %w", err)
    }
    return &config, nil
}
```

**Confidence:** HIGH -- `ResolvedConfig` already has `json:` struct tags from Phase 1, so `json.Unmarshal` round-trips perfectly with `json.MarshalIndent` in `WriteCache`.

### Pattern 3: buildElevateArgv (Variant of buildSandboxExecArgv)

**What:** Pure function that constructs sandbox-exec argv without `flox activate` in the command chain. For `elevate`, the user is already inside a flox session, so the argv wraps the current interactive shell directly.

**When to use:** `sandflox elevate` on darwin.

**Example:**
```go
// exec_darwin.go

// buildElevateArgv constructs the argv slice for elevating an existing flox
// session into a sandbox. Unlike buildSandboxExecArgv, this omits
// `flox activate` from the command chain since the user is already inside
// a flox session.
//
// Argv shape (interactive -- 12 elements):
//   sandbox-exec -f <sbpl> -D PROJECT=... -D HOME=... -D FLOX_CACHE=... bash --rcfile <entrypoint> -i
func buildElevateArgv(sbplPath, projectDir, home, floxCachePath, entrypointPath string) []string {
    return []string{
        "sandbox-exec",
        "-f", sbplPath,
        "-D", "PROJECT=" + projectDir,
        "-D", "HOME=" + home,
        "-D", "FLOX_CACHE=" + floxCachePath,
        "bash", "--rcfile", entrypointPath, "-i",
    }
}
```

**Confidence:** HIGH -- mirrors existing `buildSandboxExecArgv` pattern. The key difference is the omission of `floxAbsPath, "activate", "--"` from the middle of the argv. The entrypoint.sh still applies all shell enforcement (PATH wipe, requisites, armor, fs-filter).

### Pattern 4: Validate Output Format

**What:** `validate` produces human-readable output showing what the sandbox would enforce, without actually launching it. Reuses `emitDiagnostics` for `--debug` mode.

**When to use:** `sandflox validate` -- dry-run policy inspection.

**Example output (normal mode):**
```
[sandflox] Policy: policy.toml (valid)
[sandflox] Profile: default | Network: blocked | Filesystem: workspace
[sandflox] Tools: 55 (from requisites.txt)
[sandflox] Denied paths: 5
```

**Example output (--debug mode):**
Same as above, plus full `emitDiagnostics` debug output (path lists, SBPL rule count, env diagnostic).

**Confidence:** HIGH -- matches locked decision on output format.

### Anti-Patterns to Avoid
- **Subcommand-aware flag parsing**: Do not create per-subcommand `flag.FlagSet` instances. The locked decision says all subcommands share the global flags.
- **Treating unknown args as subcommands**: `sandflox foo` must NOT error with "unknown subcommand"; it should route to the default exec pipeline for backward compatibility.
- **Parsing policy in `status`**: The `status` subcommand reads cached state only. It must NOT require a `policy.toml` in cwd -- the whole point is inspecting the live state of an already-running sandbox.
- **Nested exec for elevate**: `elevate` must use `syscall.Exec` (PID replacement), not `os/exec.Command` (child process). This matches the established pattern in `execWithKernelEnforcement`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON deserialization | Custom config.json parser | `encoding/json.Unmarshal` into `ResolvedConfig` | Struct tags already defined; zero new code needed |
| Subcommand framework | Custom dispatch table with help text | Simple `switch` on extracted subcommand | Three subcommands; a framework is overkill |
| Requisites counting | Re-parse requisites file in validate handler | `ParseRequisites` from config.go | Already handles comments, blank lines, whitespace tokens |

## Common Pitfalls

### Pitfall 1: Flag Position Ambiguity
**What goes wrong:** `sandflox --debug validate` puts `--debug` before the subcommand. If you parse flags first, `validate` ends up in `fs.Args()` as a non-flag argument, then the subcommand extraction never finds it because it already consumed by ParseFlags.
**Why it happens:** `flag.FlagSet.Parse` stops at the first non-flag argument.
**How to avoid:** Extract the subcommand BEFORE flag parsing. Scan all args for a known subcommand name, remove it, then pass the remaining args to `ParseFlags`. This handles both `sandflox validate --debug` and `sandflox --debug validate`.
**Warning signs:** `sandflox --debug validate` works differently than `sandflox validate --debug`.

### Pitfall 2: Elevate Without Flox Session
**What goes wrong:** User runs `sandflox elevate` from a regular terminal (not inside `flox activate`). The sandbox starts, but shell enforcement artifacts don't exist because no Flox hook ran.
**Why it happens:** `FLOX_ENV` and `FLOX_ENV_CACHE` are only set inside a flox session.
**How to avoid:** Check `FLOX_ENV` env var at the start of `runElevate`. If empty, error with a clear message and exit 1.
**Warning signs:** Entrypoint.sh references `$FLOX_ENV_CACHE` which is empty, causing silent failures or wrong PATH.

### Pitfall 3: Elevate Double-Nesting
**What goes wrong:** User runs `sandflox elevate` inside an already-sandboxed session. A second sandbox-exec wraps the first, creating confusing double enforcement.
**Why it happens:** `sandbox-exec` inside `sandbox-exec` is legal on macOS -- Apple's sandbox stacks (most restrictive policy wins).
**How to avoid:** Check `SANDFLOX_ENABLED=1` env var at the start of `runElevate`. If set, print "already sandboxed" and exit 0 (not error -- the desired state is already achieved).
**Warning signs:** `sandbox-exec` shows up twice in the process tree.

### Pitfall 4: Status Cache Discovery
**What goes wrong:** `sandflox status` cannot find the cache directory because it looks in cwd but the sandbox was launched from a different directory.
**Why it happens:** The cache lives at `{projectDir}/.flox/cache/sandflox/`. Inside the sandbox, `FLOX_ENV_CACHE` points to the right place, but that env var may be scrubbed.
**How to avoid:** For `status`, discover the cache via `$FLOX_ENV_CACHE/sandflox/` first (set inside active flox sessions). Fall back to `cwd/.flox/cache/sandflox/` if not set.
**Warning signs:** "No cached state found" when the sandbox is clearly active.

### Pitfall 5: Elevate Argv Must Not Include flox activate
**What goes wrong:** Copy-pasting `buildSandboxExecArgv` for elevate includes `floxAbsPath, "activate", "--"` in the argv, causing a nested `flox activate` inside the already-active session.
**Why it happens:** `buildSandboxExecArgv` was designed for the default launch path.
**How to avoid:** Create a separate `buildElevateArgv` function (pure, testable) that constructs the sandbox-exec argv with `bash --rcfile entrypoint -i` directly, without the flox activate wrapper.
**Warning signs:** Double activation messages, shell hanging, or "already activated" warnings from flox.

### Pitfall 6: Elevate FLOX_ENV_CACHE Discovery
**What goes wrong:** `elevate` needs `FLOX_ENV_CACHE` to locate the cache dir where artifacts are written, but this env var might be scrubbed by the elevate's own env sanitization.
**Why it happens:** `BuildSanitizedEnv` passes `FLOX_` prefixed vars through, but the elevate handler needs to read it BEFORE calling `BuildSanitizedEnv`.
**How to avoid:** Read `FLOX_ENV_CACHE` from `os.Getenv` at the start of `runElevate`, before any env manipulation. Use it for cache dir path. It will pass through env sanitization via the `FLOX_` prefix allowlist.
**Warning signs:** Empty or wrong cache dir in the elevated sandbox.

## Code Examples

### Subcommand Routing in main()
```go
// main.go -- modified main() entry point

func main() {
    // 0. Extract subcommand (if any) before flag parsing
    subcmd, remaining := extractSubcommand(os.Args[1:])

    // 1. Parse CLI flags from remaining args
    flags, userArgs := ParseFlags(remaining)

    // 2. Route to subcommand handler or continue default pipeline
    switch subcmd {
    case "validate":
        runValidate(flags)
    case "status":
        runStatus(flags)
    case "elevate":
        runElevate(flags)
    default:
        // Original pipeline (steps 2-8)
        runDefault(flags, userArgs)
    }
}
```

### Status Subcommand (Reading Cache State)
```go
// subcommand.go

func runStatus(flags *CLIFlags) {
    // 1. Discover cache directory
    cacheDir := discoverCacheDir(flags)
    if cacheDir == "" {
        fmt.Fprintf(stderr, "[sandflox] Not in a sandflox session -- no cached state found. Run `sandflox` first.\n")
        os.Exit(1)
    }

    // 2. Read cached config
    config, err := ReadCache(cacheDir)
    if err != nil {
        fmt.Fprintf(stderr, "%v\n", err)
        os.Exit(1)
    }

    // 3. Count tools from cached requisites
    reqPath := filepath.Join(cacheDir, "requisites.txt")
    tools, _ := ParseRequisites(reqPath)

    // 4. Emit status
    fmt.Fprintf(stderr, "[sandflox] Profile: %s | Network: %s | Filesystem: %s\n",
        config.Profile, config.NetMode, config.FsMode)
    fmt.Fprintf(stderr, "[sandflox] Tools: %d | Denied paths: %d\n",
        len(tools), len(config.Denied))

    if flags.Debug {
        // Reuse emitDiagnostics pattern for verbose output
        fmt.Fprintf(stderr, "[sandflox] Requisites: %s\n", config.Requisites)
        fmt.Fprintf(stderr, "[sandflox] Allow localhost: %v\n", config.AllowLocalhost)
        fmt.Fprintf(stderr, "[sandflox] Writable paths: %v\n", config.Writable)
        fmt.Fprintf(stderr, "[sandflox] Read-only paths: %v\n", config.ReadOnly)
        fmt.Fprintf(stderr, "[sandflox] Denied paths: %v\n", config.Denied)
    }
}

func discoverCacheDir(flags *CLIFlags) string {
    // Prefer FLOX_ENV_CACHE (set inside active flox sessions)
    if envCache := os.Getenv("FLOX_ENV_CACHE"); envCache != "" {
        dir := filepath.Join(envCache, "sandflox")
        if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
            return dir
        }
    }

    // Fall back to cwd-relative path
    projectDir := resolveProjectDir(flags)
    dir := filepath.Join(projectDir, ".flox", "cache", "sandflox")
    if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
        return dir
    }

    return ""
}
```

### Elevate Entry Detection
```go
// subcommand.go

func runElevate(flags *CLIFlags) {
    // 1. Re-entry detection: already sandboxed?
    if os.Getenv("SANDFLOX_ENABLED") == "1" {
        fmt.Fprintf(stderr, "[sandflox] Already sandboxed -- nothing to do.\n")
        os.Exit(0)
    }

    // 2. Flox session detection
    if os.Getenv("FLOX_ENV") == "" {
        fmt.Fprintf(stderr, "[sandflox] Not in a flox session. Run `flox activate` first.\n")
        os.Exit(1)
    }

    // 3. Run full pipeline: parse policy, resolve config, write cache,
    //    generate artifacts, then exec into sandbox-exec WITHOUT flox activate
    // ... (reuse steps 2-7 from main pipeline, then call elevateExec)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Single-command binary | Subcommand routing | This phase | Users can inspect policy without launching sandbox |
| Must launch sandbox to see config | `sandflox status` reads cache | This phase | Faster debugging, no restart needed |
| Must exit and re-enter with sandflox wrapper | `sandflox elevate` in-place | This phase | Smoother workflow for users already in flox |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) 1.22 |
| Config file | go.mod (module sandflox) |
| Quick run command | `go test ./...` |
| Full suite command | `go test -tags integration -count=1 ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CMD-01 | validate outputs policy summary without executing | unit | `go test -run TestValidate -v ./...` | Wave 0 |
| CMD-01 | validate --debug shows full details + SBPL count | unit | `go test -run TestValidateDebug -v ./...` | Wave 0 |
| CMD-02 | status reads cache and reports enforcement state | unit | `go test -run TestStatus -v ./...` | Wave 0 |
| CMD-02 | status outside sandbox prints error, exits 1 | unit | `go test -run TestStatusNoCache -v ./...` | Wave 0 |
| CMD-03 | elevate detects already-sandboxed, exits 0 | unit | `go test -run TestElevateAlreadySandboxed -v ./...` | Wave 0 |
| CMD-03 | elevate detects no flox session, exits 1 | unit | `go test -run TestElevateNoFlox -v ./...` | Wave 0 |
| CMD-03 | elevate argv shape omits flox activate | unit (darwin) | `go test -run TestBuildElevateArgv -v ./...` | Wave 0 |
| CMD-03 | elevate on non-darwin warns and exits | unit (!darwin) | `go test -run TestElevateNonDarwin -v ./...` | Wave 0 |
| ALL | subcommand routing: known subcmds, unknown passthrough | unit | `go test -run TestExtractSubcommand -v ./...` | Wave 0 |
| ALL | flag position equivalence (before/after subcmd) | unit | `go test -run TestSubcommandFlagPosition -v ./...` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./...`
- **Per wave merge:** `go test -tags integration -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `subcommand_test.go` -- covers CMD-01, CMD-02, CMD-03 routing and output assertions
- [ ] `exec_test.go` additions -- covers `buildElevateArgv` argv shape (darwin build tag)
- [ ] `cache_test.go` additions -- covers `ReadCache` round-trip (WriteCache then ReadCache)

## Open Questions

1. **FLOX_ENV_CACHE availability in elevate**
   - What we know: `FLOX_ENV_CACHE` is set by flox during `flox activate`. It is in the `FLOX_` prefix allowlist so it passes through `BuildSanitizedEnv`.
   - What's unclear: Whether `FLOX_ENV_CACHE` resolves to the same path as `{projectDir}/.flox/cache` (it may be a Nix store symlink or a user-level cache). The cache dir used by WriteCache is `filepath.Join(projectDir, ".flox", "cache", "sandflox")`.
   - Recommendation: In `elevate`, use `FLOX_ENV_CACHE` if set (append `/sandflox`), otherwise fall back to cwd-relative. Both should work since the Flox manifest sets `FLOX_ENV_CACHE` to point at `.flox/cache`. Verify this in integration tests.

2. **Elevate entrypoint.sh regeneration**
   - What we know: `elevate` runs the full pipeline including `WriteShellArtifacts`, which regenerates `entrypoint.sh`. This is correct because the entrypoint must match the current policy.
   - What's unclear: Whether the regenerated entrypoint.sh in the cache dir will be visible inside the elevated sandbox (sandbox-exec may have already locked down writes to the cache dir).
   - Recommendation: The cache dir is under `FLOX_CACHE` which is always writable in SBPL (line: `(allow file-write* (subpath (param "FLOX_CACHE")))`). Since we write BEFORE exec-ing into sandbox-exec, this is safe. The write happens in the unsandboxed process; the read happens in the sandboxed bash.

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `main.go`, `cli.go`, `config.go`, `cache.go`, `exec_darwin.go`, `exec_other.go`, `env.go`, `sbpl.go`, `shell.go`
- Codebase analysis: All `*_test.go` files for testing patterns
- Go stdlib `flag` package documentation (verified against training data; `flag.FlagSet` behavior is stable since Go 1.0)

### Secondary (MEDIUM confidence)
- Go CLI subcommand patterns: `go`, `kubectl`, `flox` all use positional first-arg routing (verified against training data and codebase conventions)

## Project Constraints (from CLAUDE.md)

- **Language**: Go -- stdlib only, zero external dependencies
- **Platform**: macOS only (Darwin) for sandbox-exec; stubs for other platforms
- **Backward compatibility**: existing `policy.toml` v2 schema and CLI behavior unchanged
- **Error format**: `[sandflox]` prefix on all stderr messages (ERROR, WARNING, BLOCKED)
- **Process model**: `syscall.Exec` for PID replacement, never child processes
- **Build tags**: `//go:build darwin` / `//go:build !darwin` for platform-specific code
- **Test patterns**: table-driven subtests, package-level `stderr io.Writer` for testable diagnostics
- **Naming**: `_sfx_` prefix for internal variables, `SCREAMING_SNAKE` for exported env vars

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all Go stdlib
- Architecture: HIGH -- all building blocks exist in codebase; subcommands are slices through the existing pipeline
- Pitfalls: HIGH -- identified from codebase analysis (env var availability, argv shape, cache discovery)

**Research date:** 2026-04-17
**Valid until:** 2026-05-17 (stable -- Go stdlib, no external deps)
