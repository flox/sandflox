# Architecture Patterns

**Domain:** macOS-native Go sandbox binary wrapping sandbox-exec + flox activate
**Researched:** 2026-04-15
**Confidence:** HIGH (reference implementation exists, current codebase is working prototype)

## Recommended Architecture

### Overview

A single Go binary (`sandflox`) that composes two enforcement layers: kernel-level (sandbox-exec SBPL) and shell-level (PATH/function/fs wrappers). The binary owns the entire pipeline from policy parsing through process exec -- no Python dependency, no manifest-embedded enforcement logic, no external Go dependencies.

The fundamental shift from the current architecture: **the Go binary becomes the sole owner of all enforcement**. The Flox manifest becomes minimal (just `[install]` packages). All shell enforcement scripts are generated and written by the Go binary before it execs into `sandbox-exec ... flox activate`.

```
policy.toml ----+
CLI flags ------+---> Go binary ---> SBPL profile ------> sandbox-exec
                |                |-> shell scripts ------> flox activate ---> enforced shell
                |                |-> symlink bin dir
                |                |-> usercustomize.py
                |                +-> entrypoint.sh
                |
requisites.txt -+
```

### Key Architectural Difference from flox-bwrap

| Aspect | flox-bwrap (Linux) | sandflox (macOS) |
|--------|-------------------|------------------|
| Kernel mechanism | bwrap (Linux namespaces) | sandbox-exec (SBPL/Seatbelt) |
| Isolation model | Filesystem mount-based (bind mounts, tmpfs) | Policy-based (allow/deny rules on syscalls) |
| Environment | Constructs clean namespace (clearenv, mount /dev, /proc) | Runs in host environment with SBPL restrictions |
| Nix store | Bind-mounts specific store paths into namespace | Host nix store visible; SBPL denies writes |
| Shell enforcement | None (bwrap handles everything at namespace level) | Full shell tier: PATH wipe, requisites, function armor, fs-filter |
| Config source | CLI flags only | policy.toml + CLI flag overrides |
| Process complexity | Single exec into bwrap | Write artifacts to cache, then exec into sandbox-exec |

This is the critical design difference: **bwrap creates a new filesystem namespace where tools simply don't exist**. sandbox-exec applies syscall-level rules to the existing namespace. That means sandflox needs the shell enforcement layer that flox-bwrap doesn't -- agents can see `/usr/bin/pip` on disk even though sandbox-exec blocks execution. Shell enforcement provides clear error messages and removes the tools from PATH.

## Component Boundaries

### Component 1: CLI + Config (`config.go`)

**Responsibility:** Parse CLI flags, load and parse `policy.toml`, merge flag overrides, resolve profile, validate, produce a unified `Config` struct.

**Inputs:**
- `os.Args` (CLI flags: `--net`, `--profile`, `--policy`, `--debug`, `--requisites`)
- `policy.toml` file (TOML)
- `SANDFLOX_PROFILE` environment variable

**Outputs:**
- `Config` struct with resolved: profile name, network mode, filesystem mode, requisites file path, writable/read-only/denied path lists (resolved to absolute), allow-localhost flag

**Communicates with:** main (provides Config), all other components consume Config

**Key design decisions:**
- Inline TOML parser (not `BurntSushi/toml` or `pelletier/go-toml`) to maintain zero external dependencies. The policy.toml schema is a strict subset of TOML -- no nested inline tables, no multi-line strings, no datetime types. A ~150-line parser handles it.
- CLI flags override policy.toml values. `--net` overrides `network.mode`, `--profile` overrides `meta.profile`, `--requisites` overrides profile's requisites file.
- Path resolution happens here: relative paths resolved against project root, `~/` expanded to `$HOME`.

**Confidence:** HIGH -- flox-bwrap's `config.go` is the direct pattern. TOML parser is the one novel piece; the current Python inline parser is ~50 lines and the Go equivalent is straightforward for the restricted subset.

### Component 2: SBPL Generator (`sbpl.go`)

**Responsibility:** Generate a macOS Seatbelt Profile Language file from the resolved Config.

**Inputs:**
- `Config` struct (network mode, filesystem mode, writable/read-only/denied paths, allow-localhost)
- Project directory path
- Home directory path

**Outputs:**
- SBPL profile written to `.flox/cache/sandflox/sandflox.sb`

**Communicates with:** main (called during run phase), Executor (provides path to generated .sb file)

**Key design decisions:**
- **Allow-default strategy:** `(allow default)` baseline with selective denies. Deny-default is too fragile for Flox (nix daemon needs unpredictable store paths, macOS frameworks need system library access). This is a validated architectural decision from the current prototype.
- **SBPL parameters:** Use `sandbox-exec -D KEY=VALUE` to inject runtime paths (PROJECT, HOME, FLOX_CACHE) rather than hardcoding them in the profile. This matches the current pattern at sandflox lines 476-479.
- **Write mode generation:** Three code paths mapping to `permissive`/`workspace`/`strict` filesystem modes. Workspace is the most complex (deny all writes, re-allow project dir and /tmp, then re-deny read-only overrides within project).
- **Network mode generation:** Blocked mode denies `network*`, re-allows unix sockets (nix daemon) and optionally localhost.

**Template approach:** Use `strings.Builder` with helper methods like `writeDenySubpath()`, `writeAllowSubpath()`. Not `text/template` -- the SBPL syntax is irregular enough that explicit builder code is clearer and easier to audit than template logic.

**Confidence:** HIGH -- the current Bash `_sfx_generate_sbpl()` function (lines 191-291 of `sandflox`) is a direct specification for the Go port. SBPL syntax is documented and stable.

### Component 3: Shell Enforcement Generator (`shell.go`)

**Responsibility:** Generate all shell-level enforcement scripts that run inside the sandbox after `flox activate`.

**Inputs:**
- `Config` struct
- Requisites file content
- `$FLOX_ENV/bin` path (resolved at runtime via `FLOX_ENV` env var or computed)

**Outputs (all written to `.flox/cache/sandflox/`):**
- `fs-filter.sh` -- shell function wrappers for write commands (cp, mv, mkdir, rm, etc.)
- `entrypoint.sh` -- non-interactive mode enforcement script
- `function-armor.sh` -- shell function definitions for 27+ package manager commands
- `requisites.txt` -- staged copy of the active requisites file
- `net-mode.txt`, `fs-mode.txt`, `active-profile.txt` -- mode flags
- `writable-paths.txt`, `read-only-paths.txt`, `denied-paths.txt` -- resolved path lists
- `net-blocked.flag` -- marker file for curl removal

**Also generates to `.flox/cache/sandflox-python/`:**
- `usercustomize.py` -- Python builtins.open monkey-patch and ensurepip blocker

**Communicates with:** main (called during run phase), writes to filesystem (consumed by flox activate hooks/profile)

**Key design decisions:**
- **`go:embed` for script templates:** Embed the shell script templates and the Python usercustomize.py as string constants in the Go binary. This eliminates runtime file dependencies. Use `//go:embed` directive with `embed.FS` or raw string constants.
- **The manifest becomes minimal:** With the Go binary generating all enforcement scripts to the cache directory, the `manifest.toml` `[hook]` and `[profile]` sections only need to source them. The binary writes, the manifest sources. This eliminates the duplicated Python TOML parser from the manifest entirely.
- **Requisites symlink bin:** The Go binary can pre-build the symlink bin directory (reading requisites file, creating symlinks from `$FLOX_ENV/bin/<tool>` to `$FLOX_ENV_CACHE/sandflox/bin/<tool>`). However, `$FLOX_ENV` is only known after `flox activate` runs. Two options:
  - **Option A (recommended):** Binary writes the enforcement scripts; `flox activate` profile sources them. The profile script reads the pre-staged requisites.txt and builds symlinks at activation time (same as current).
  - **Option B:** Binary detects `FLOX_ENV` from the current Flox session (if running inside one) or resolves it from `.flox/run/` symlinks. More complex, fragile with Flox internals.
  - Recommendation: **Option A**. The binary generates the scripts; the shell executes them in the `flox activate` context where `$FLOX_ENV` is guaranteed.

**Confidence:** HIGH for script generation (it is a port of existing working code). MEDIUM for the manifest integration pattern -- need to verify that a minimal `[profile] common` that just sources the cached scripts works identically to the current embedded approach.

### Component 4: Executor (`exec.go`)

**Responsibility:** Assemble the final command line and `syscall.Exec` into `sandbox-exec`.

**Inputs:**
- Path to generated SBPL profile
- SBPL parameter key-value pairs (PROJECT, HOME, FLOX_CACHE)
- User command (if `-- CMD` mode) or interactive flag
- Config (for debug mode)

**Outputs:**
- `syscall.Exec` call (replaces the Go process with sandbox-exec)

**Communicates with:** main (final step), uses SBPL profile from generator

**The exec chain:**
```
sandflox (Go binary)
  -> syscall.Exec: sandbox-exec -f sandflox.sb -D PROJECT=... -D HOME=... -D FLOX_CACHE=...
    -> flox activate [-- bash entrypoint.sh CMD]
      -> [hook] on-activate: sources cached enforcement scripts
      -> [profile] common: sources cached enforcement scripts
        -> enforced shell / command
```

**Key design decisions:**
- **`syscall.Exec` not `os/exec.Cmd`:** Process replacement, not child process. Matching flox-bwrap pattern. The Go binary ceases to exist after exec; sandbox-exec is PID 1 of the session.
- **Debug mode:** Print the command that would be exec'd and exit. Essential for troubleshooting SBPL issues.
- **Interactive vs. non-interactive dispatch:**
  - Interactive (`sandflox`): `sandbox-exec ... flox activate` -- profile.common runs, applies enforcement
  - Non-interactive (`sandflox -- CMD`): `sandbox-exec ... flox activate -- bash /path/to/entrypoint.sh CMD` -- entrypoint.sh applies enforcement, then execs CMD
- **Graceful degradation:** If `sandbox-exec` is not found (shouldn't happen on macOS, but defensive), fall back to `flox activate` with shell-only enforcement.

**Confidence:** HIGH -- direct port of current sandflox lines 465-500 plus flox-bwrap's exec pattern.

### Component 5: Re-exec Elevator (`elevate.go`)

**Responsibility:** From within a running `flox activate` session, re-exec the current shell under sandflox + sandbox-exec enforcement.

**Inputs:**
- Detection: `FLOX_ENV` is set (we're inside flox activate), `SANDFLOX_ENABLED` is set but `SANDFLOX_KERNEL` is not (shell enforcement active, kernel enforcement not)
- Current shell, environment, working directory

**Outputs:**
- `syscall.Exec` call: re-execs the sandflox binary which then wraps the current session under sandbox-exec

**Communicates with:** main (subcommand `sandflox elevate`), Config (reads policy), Executor (delegates exec)

**Key design decisions:**
- **One-time bounce:** `sandflox elevate` can only be called once. After elevation, `SANDFLOX_KERNEL=1` is set, and subsequent `sandflox elevate` calls are no-ops with a warning.
- **Environment preservation:** The re-exec must preserve the current Flox environment (FLOX_ENV, FLOX_ENV_CACHE, etc.) while adding kernel enforcement on top.
- **Implementation:**
  1. Detect current Flox session from environment variables
  2. Generate SBPL profile (same as normal path)
  3. Write enforcement scripts to cache (same as normal path)
  4. `syscall.Exec` into `sandbox-exec -f sandflox.sb ... $SHELL` with the current environment plus `SANDFLOX_KERNEL=1`

**Confidence:** MEDIUM -- this is novel functionality not present in the current prototype or flox-bwrap. The concept is sound (re-exec with sandbox-exec wrapping the shell) but needs prototyping to verify that Flox's activation state survives the sandbox-exec re-exec. Specifically: does flox activate's shell state (functions, aliases, PATH) persist through a `sandbox-exec ... bash --login` re-exec? It should if we exec the current `$SHELL` with the current environment rather than re-running `flox activate`.

### Component 6: Main Orchestrator (`main.go`)

**Responsibility:** Wire components together, dispatch subcommands, handle top-level errors.

**Inputs:** `os.Args`

**Outputs:** Delegates to components, calls `syscall.Exec` via Executor

**Flow:**
```
main()
  1. ParseConfig() -> Config, remaining args
  2. Config.Validate()
  3. If subcommand "elevate": delegate to Elevator
  4. WriteShellEnforcement(Config) -> writes scripts to cache
  5. GenerateSBPL(Config) -> writes .sb to cache
  6. Exec(Config, sbplPath, args) -> syscall.Exec (never returns)
```

## Data Flow

### Full Pipeline: `sandflox` or `sandflox -- CMD`

```
[User invokes sandflox]
        |
        v
+--------------------+
| 1. Parse CLI flags |  os.Args
| 2. Load policy.toml|  File read
| 3. Merge overrides |  Flags > TOML
| 4. Resolve profile |  ENV > meta > default
| 5. Resolve paths   |  . -> /abs, ~/ -> /home
+--------------------+
        |
        v  Config struct
+--------------------+
| 6. Write shell     |  .flox/cache/sandflox/fs-filter.sh
|    enforcement      |  .flox/cache/sandflox/entrypoint.sh
|    scripts          |  .flox/cache/sandflox/function-armor.sh
|                     |  .flox/cache/sandflox/requisites.txt
|                     |  .flox/cache/sandflox/{net,fs}-mode.txt
|                     |  .flox/cache/sandflox-python/usercustomize.py
+--------------------+
        |
        v
+--------------------+
| 7. Generate SBPL   |  .flox/cache/sandflox/sandflox.sb
+--------------------+
        |
        v
+--------------------+
| 8. syscall.Exec    |  sandbox-exec -f sandflox.sb \
|                     |    -D PROJECT=/path \
|                     |    -D HOME=/Users/x \
|                     |    -D FLOX_CACHE=/Users/x/.cache/flox \
|                     |    flox activate [-- bash entrypoint.sh CMD]
+--------------------+
        |
        v  (Go process replaced by sandbox-exec)
+--------------------+
| 9. flox activate   |  Triggers [hook] on-activate
|    hook runs        |  Sources cached enforcement scripts
+--------------------+
        |
        v
+--------------------+
| 10. profile.common |  Sources cached enforcement scripts
|     runs           |  Builds symlink bin, applies armor
+--------------------+
        |
        v
+--------------------+
| 11. Enforced shell |  PATH = symlink bin only
|     or command      |  Functions shadow pkg managers
|                     |  fs-filter wraps write commands
|                     |  SBPL blocks at kernel level
+--------------------+
```

### Manifest Integration (The Thin Manifest)

The current manifest has ~400 lines of embedded enforcement logic. The Go binary architecture replaces this with a thin manifest that sources pre-generated scripts:

```toml
[hook]
on-activate = '''
  # sandflox: source pre-generated enforcement if available
  _sfx_stage="${FLOX_ENV_CACHE}/sandflox"
  if [ -f "$_sfx_stage/hook-setup.sh" ]; then
    . "$_sfx_stage/hook-setup.sh"
  fi
'''

[profile]
common = '''
  _sfx_stage="${FLOX_ENV_CACHE}/sandflox"
  if [ -f "$_sfx_stage/profile-setup.sh" ]; then
    . "$_sfx_stage/profile-setup.sh"
  fi
'''
```

The Go binary generates `hook-setup.sh` and `profile-setup.sh` which contain all the enforcement logic currently embedded in the manifest. This is the key architectural simplification: **one source of truth (Go binary) instead of three (sandflox script + manifest hook + manifest profile)**.

### State Files

All runtime state in `.flox/cache/sandflox/` (gitignored):

| File | Written By | Read By | Content |
|------|-----------|---------|---------|
| `sandflox.sb` | SBPL Generator | sandbox-exec | SBPL profile |
| `hook-setup.sh` | Shell Generator | flox on-activate hook | PATH wipe, ensurepip block, breadcrumbs |
| `profile-setup.sh` | Shell Generator | flox profile.common | Requisites filter, function armor, fs-filter |
| `entrypoint.sh` | Shell Generator | Non-interactive mode | Combined hook+profile for `-- CMD` mode |
| `fs-filter.sh` | Shell Generator | profile-setup.sh / entrypoint.sh | Write command wrappers |
| `function-armor.sh` | Shell Generator | profile-setup.sh / entrypoint.sh | Package manager function shadows |
| `requisites.txt` | Config (copies from project) | profile-setup.sh | Active binary whitelist |
| `config.json` | Config | Diagnostic/debug | Resolved config snapshot |
| `net-mode.txt` | Config | Shell scripts | "blocked" or "unrestricted" |
| `fs-mode.txt` | Config | Shell scripts, usercustomize.py | "workspace", "strict", or "permissive" |
| `active-profile.txt` | Config | Shell scripts | Profile name for display |
| `net-blocked.flag` | Config | Shell scripts | Marker: remove curl from PATH |
| `writable-paths.txt` | Config | fs-filter.sh, usercustomize.py | Resolved writable paths |
| `read-only-paths.txt` | Config | fs-filter.sh | Resolved read-only paths |
| `denied-paths.txt` | Config | fs-filter.sh, SBPL, usercustomize.py | Resolved denied paths |
| `usercustomize.py` | Shell Generator | Python runtime (PYTHONPATH) | builtins.open wrapper, ensurepip block |

## Patterns to Follow

### Pattern 1: Zero-Dependency Go Binary

**What:** No external Go modules. Use only Go stdlib (`flag`, `os`, `strings`, `bufio`, `path/filepath`, `syscall`, `embed`, `fmt`).
**When:** Always. This is a hard constraint matching flox-bwrap.
**Why:** `buildGoModule` with `vendorHash = null` means Nix builds with zero network fetches. Single static binary. No supply chain risk.
**Implication:** TOML parser must be hand-written for the policy.toml subset. This is ~150 lines of Go for the restricted schema (string values, boolean values, string arrays, dotted table headers).

### Pattern 2: Build-Time Path Injection

**What:** Use `-ldflags "-X main.sandboxExecPath=..."` to inject the Nix store path of `sandbox-exec` at build time.
**When:** Nix builds via `buildGoModule`.
**Why:** Ensures the binary uses the exact `sandbox-exec` from the Nix closure, not whatever is on PATH.
**Example from flox-bwrap:**
```nix
ldflags = [ "-X main.bwrapPath=${bubblewrap}/bin/bwrap" ];
```
**For sandflox:** sandbox-exec is a macOS system binary (`/usr/bin/sandbox-exec`), not a Nix package. Build-time injection is less critical here since it's always at a known system path. But the pattern should still be used for the `flox` binary path to ensure we use the Nix-provided flox.

### Pattern 3: Process Replacement via syscall.Exec

**What:** Replace the Go process entirely with `sandbox-exec`. No child process, no signal forwarding, no PID management.
**When:** Final step of every invocation.
**Why:** Clean process tree. The sandbox-exec process IS the session. `Ctrl-C` goes to the right place. Exit codes propagate naturally.
**Example:**
```go
syscall.Exec(sandboxExecPath,
    []string{"sandbox-exec", "-f", sbplPath,
        "-D", "PROJECT=" + projectDir,
        "-D", "HOME=" + homeDir,
        "-D", "FLOX_CACHE=" + floxCache,
        "flox", "activate", "--"},
    os.Environ())
```

### Pattern 4: Embedded Script Templates via go:embed

**What:** Embed shell script templates and the Python usercustomize.py as compile-time assets using `//go:embed`.
**When:** For all generated enforcement scripts.
**Why:** The binary is self-contained. No external file dependencies at runtime. Templates are compiled into the binary and written to the cache directory with config-specific values substituted.
**Example:**
```go
import "embed"

//go:embed templates/entrypoint.sh
var entrypointTemplate string

//go:embed templates/usercustomize.py
var usercustomizePy string
```

### Pattern 5: Cache-Based Artifact Generation

**What:** Generate all enforcement artifacts to `.flox/cache/sandflox/` before exec'ing. The cache is ephemeral (gitignored), regenerated on every invocation.
**When:** Every sandflox invocation.
**Why:** Clean separation between the Go binary (generator) and the shell environment (consumer). The manifest only needs `source` commands, not enforcement logic. Regenerating every time ensures consistency with the current policy.toml.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Manifest-Embedded Logic

**What:** Putting enforcement logic directly in `manifest.toml` `[hook]` and `[profile]` sections.
**Why bad:** This is the current architecture and it creates three sources of truth (sandflox script, hook, profile). Changes require updating multiple places. The Python TOML parser is duplicated between the script and the hook. The entrypoint.sh duplicates profile.common logic.
**Instead:** Go binary generates ALL enforcement scripts. Manifest just sources them.

### Anti-Pattern 2: External Go Dependencies for TOML

**What:** Using `BurntSushi/toml` or `pelletier/go-toml` for policy parsing.
**Why bad:** Breaks the zero-dependency constraint. Makes `vendorHash` non-null in Nix builds, requiring network fetches and hash management.
**Instead:** Hand-written parser for the policy.toml subset. The schema is constrained: string keys, string/boolean values, string arrays, dotted section headers. No inline tables, no datetime, no multi-line strings. ~150 lines of Go.

### Anti-Pattern 3: Child Process Instead of syscall.Exec

**What:** Using `os/exec.Cmd.Run()` or `Start()` to run sandbox-exec as a child process.
**Why bad:** The Go binary stays alive as a parent process. Signals need forwarding. Exit codes need propagation. Process tree is messy. Ctrl-C behavior is wrong.
**Instead:** `syscall.Exec` replaces the process entirely.

### Anti-Pattern 4: Deny-Default SBPL Strategy

**What:** Starting the SBPL profile with `(deny default)` and whitelisting everything needed.
**Why bad:** Flox depends on the nix daemon (unix sockets to `/nix/var/nix/daemon-socket`), macOS system frameworks (`/System/Library/`), XPC services, Security.framework, and dozens of other system paths that vary across macOS versions. A deny-default profile would break constantly.
**Instead:** `(allow default)` with targeted denies for filesystem writes, network, and sensitive paths. Defense in depth comes from the shell tier, not from making the SBPL profile exhaustively restrictive.

### Anti-Pattern 5: Runtime Detection of FLOX_ENV in the Go Binary

**What:** Having the Go binary resolve `$FLOX_ENV` (the Nix store path of the active environment) to pre-build the symlink bin directory.
**Why bad:** `$FLOX_ENV` is only set after `flox activate` runs. The Go binary runs BEFORE `flox activate`. Trying to resolve it from `.flox/run/` symlinks couples the binary to Flox's internal directory structure which may change.
**Instead:** Let the shell scripts (sourced during `flox activate`) handle anything that depends on `$FLOX_ENV`. The Go binary handles everything that can be resolved before activation.

## Suggested Build Order

Components can be built and tested incrementally:

### Phase 1: Config + Minimal CLI (can be built independently)

Files: `main.go`, `config.go`
- CLI flag parsing (stdlib `flag`)
- Inline TOML parser for policy.toml
- Config struct with validation
- Profile resolution (env var > meta.profile > default)
- Path resolution (relative to absolute, tilde expansion)
- **Testable independently:** Parse various policy.toml files, verify Config output

### Phase 2: SBPL Generator (depends on Phase 1)

Files: `sbpl.go`
- Generate SBPL profile string from Config
- Handle all three filesystem modes
- Handle network modes
- Denied path rules, read-only overrides
- SBPL parameter placeholders (PROJECT, HOME, FLOX_CACHE)
- **Testable independently:** Generate SBPL from Config, diff against known-good profiles from current implementation

### Phase 3: Shell Enforcement Generator (depends on Phase 1)

Files: `shell.go`, `templates/` (embedded)
- Generate fs-filter.sh from Config
- Generate entrypoint.sh
- Generate function-armor.sh
- Generate hook-setup.sh and profile-setup.sh
- Stage requisites.txt, mode files, path list files
- Generate usercustomize.py
- **Testable independently:** Generate scripts, diff against current implementation output. Note: Phase 3 can be built in parallel with Phase 2.

### Phase 4: Executor + Integration (depends on Phases 1-3)

Files: `exec.go`
- Assemble sandbox-exec command line
- Handle interactive vs. non-interactive dispatch
- Debug mode (print command, exit)
- Graceful degradation (no sandbox-exec)
- `syscall.Exec` call
- **Integration test:** Full pipeline from policy.toml to running sandbox

### Phase 5: Re-exec Elevator (depends on Phase 4)

Files: `elevate.go`
- Detect running Flox session
- One-time elevation guard
- Environment preservation
- Re-exec into sandflox with kernel enforcement
- **Integration test:** Requires active Flox session to test

### Phase 6: Nix Build + Flox Package (depends on Phases 1-4)

Files: `.flox/pkgs/sandflox.nix`, updated `manifest.toml`
- `buildGoModule` expression
- Build-time ldflags injection
- Thin manifest (source-only hook/profile)
- `flox build` / `flox publish` verification

## File Layout

```
sandflox/
  main.go              # Orchestrator, subcommand dispatch
  config.go            # CLI parsing, TOML parsing, Config struct
  toml.go              # Inline TOML parser (policy.toml subset)
  sbpl.go              # SBPL profile generator
  shell.go             # Shell enforcement script generator
  exec.go              # sandbox-exec command assembly, syscall.Exec
  elevate.go           # Re-exec elevation subcommand
  templates/
    entrypoint.sh      # Embedded: non-interactive enforcement
    hook-setup.sh      # Embedded: on-activate enforcement
    profile-setup.sh   # Embedded: profile.common enforcement
    function-armor.sh  # Embedded: package manager function shadows
    usercustomize.py   # Embedded: Python write enforcement
  go.mod               # Zero dependencies
  policy.toml          # Declarative policy (unchanged)
  requisites*.txt      # Binary whitelists (unchanged)
  .flox/
    pkgs/sandflox.nix  # buildGoModule Nix expression
    env/manifest.toml  # Thin manifest (source-only)
```

## Platform and Deprecation Considerations

### sandbox-exec Status

`sandbox-exec` has been marked deprecated for several macOS releases, but continues to work through macOS Sequoia (15.x) and is used by major projects including OpenAI Codex CLI and Google Gemini CLI for agent sandboxing. Apple uses the Seatbelt framework internally for system process sandboxing, making complete removal unlikely in the near term. However:

- **Risk:** Future macOS versions could remove or restrict `sandbox-exec` access
- **Mitigation:** The two-tier architecture means shell enforcement works independently. If sandbox-exec breaks, sandflox degrades to shell-only enforcement with a clear warning
- **Detection:** Check `sandbox-exec` availability at runtime; the current graceful degradation pattern handles this

### macOS-Specific SBPL Notes

- SBPL uses Scheme-like syntax (LISP dialect): `(version 1)`, `(allow default)`, `(deny file-write* (subpath "/path"))`
- Parameters: `(param "KEY")` references values passed via `sandbox-exec -D KEY=VALUE`
- Path matching: `subpath` (recursive), `literal` (exact), `regex` (pattern)
- Network matching: `(remote unix-socket)`, `(remote ip "localhost:*")`
- Violation monitoring: `log stream --style compact --predicate 'sender=="Sandbox"'`

## Sources

- flox-bwrap reference implementation: `/tmp/flox-bwrap/` (all Go source files)
- Current sandflox prototype: `/Users/jhogan/sandflox/sandflox` (500-line Bash+Python)
- Current architecture analysis: `/Users/jhogan/sandflox/.planning/codebase/ARCHITECTURE.md`
- [sandbox-exec overview](https://igorstechnoclub.com/sandbox-exec/) -- SBPL syntax, -D parameters, deprecation status
- [Chromium Mac Sandbox Design](https://chromium.googlesource.com/chromium/src/+/HEAD/sandbox/mac/seatbelt_sandbox_design.md) -- SBPL patterns at scale
- [Go embed package](https://pkg.go.dev/embed) -- go:embed directive for script templates
- [Go by Example: Exec'ing Processes](https://gobyexample.com/execing-processes) -- syscall.Exec pattern
- [sandbox-exec deprecation discussion](https://github.com/openai/codex/issues/215) -- still functional, used by AI agent tools
- [HN: sandbox-exec frustration](https://news.ycombinator.com/item?id=44283454) -- community perspective on deprecation
- [Alcoholless sandbox](https://medium.com/nttlabs/alcoholless-a-lightweight-security-sandbox-for-macos-programs-homebrew-ai-agents-etc-ccf0d1927301) -- alternative macOS sandbox approach

---

*Architecture research: 2026-04-15*
