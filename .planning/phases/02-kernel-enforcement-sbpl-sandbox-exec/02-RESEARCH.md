# Phase 2: Kernel Enforcement (SBPL + sandbox-exec) - Research

**Researched:** 2026-04-16
**Domain:** macOS Seatbelt (Apple Sandbox Profile Language) + Go syscall process replacement
**Confidence:** HIGH

## Summary

Phase 2 ports the existing Bash `_sfx_generate_sbpl()` function (sandflox.bash:191-291) to Go and wires `sandbox-exec` into the main binary's exec path. The bash implementation is production-proven on this machine: the generated `.flox/cache/sandflox/sandflox.sb` in the current working tree is a 46-line SBPL profile using `(allow default)` baseline, parameter substitution via `-D PROJECT=... -D HOME=... -D FLOX_CACHE=...`, and `(subpath (param "PROJECT"))` / `(literal ...)` rules — exactly the pattern documented as idiomatic by Apple system profiles and Chromium's Seatbelt sandbox.

The only fresh engineering is the Go side: generate the SBPL string from `ResolvedConfig`, write it to cache, then `syscall.Exec("/usr/bin/sandbox-exec", [...], os.Environ())` replacing the Go process with `sandbox-exec -f <sb> -D PROJECT=... flox activate [-- CMD]`. `syscall.Exec` preserves PID (satisfying success criterion #5 — no intermediate sandflox parent process) and never returns on success, matching the existing `execFlox()` pattern in main.go.

**Primary recommendation:** Mirror the bash implementation structurally. Use `strings.Builder` (not `text/template`) for SBPL generation because the output is driven by flat conditionals on `FsMode` / `NetMode`, not data-parameterized templating. Put generation in a new `sbpl.go` file with `GenerateSBPL(cfg *ResolvedConfig, home string) string` and `WriteSBPL(cacheDir string, content string) (string, error)`. Modify `main.go execFlox()` to route through `sandbox-exec` on Darwin, falling back to direct `flox activate` when `sandbox-exec` is missing. Write table-driven unit tests on the SBPL string + an integration test file gated by `runtime.GOOS == "darwin"` and `sandbox-exec` availability.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**SBPL Generation Strategy**
- **D-01:** Mirror the existing bash SBPL structure exactly: `(allow default)` baseline with selective denials. Same rule ordering. The bash implementation is proven in production; diverging risks breaking flox/Nix compatibility.
- **D-02:** Use SBPL parameters via `-D` flags (`-D "PROJECT=$dir" -D "HOME=$home" -D "FLOX_CACHE=$cache"`). Reference parameters in the profile with `(param "PROJECT")` etc. Keeps the `.sb` file portable — matches bash pattern.
- **D-03:** Regenerate the SBPL profile every run (no caching). Generation is fast (~1ms), avoids cache invalidation bugs, matches bash behavior.
- **D-04:** Read-only path overrides in workspace mode use `(deny file-write* (subpath ...))` for directory paths (trailing `/`) and `(deny file-write* (literal ...))` for file paths — same logic as bash.

**Fallback and Error Behavior**
- **D-05:** When sandbox-exec is not available: print `[sandflox] WARNING: sandbox-exec not found — falling back to shell-only` and exec `flox activate` directly. Matches ARCHITECTURE.md "graceful degradation" convention and existing bash behavior.
- **D-06:** When sandbox-exec fails (bad SBPL, permission denied): exit with `[sandflox] ERROR:` including sandbox-exec's stderr. Do NOT fall back silently — enforcement failures are security-relevant. This is different from "not available" (which is a platform limitation, not a failure).
- **D-07:** `--debug` flag prints the generated SBPL file path and key rules to stderr, in addition to the existing profile/mode/paths diagnostics from Phase 1.

**Testing Strategy**
- **D-08:** Unit test the generated SBPL string content — verify correct rules for each filesystem mode (permissive/workspace/strict), each network mode (blocked/unrestricted), denied paths, localhost allowance, and Flox-required overrides. Parse the SBPL output and check rule presence/absence.
- **D-09:** Integration tests that actually run `sandbox-exec` with the generated profile — test write blocking, network blocking, denied path access. These tests require macOS and real sandbox-exec; skip gracefully on other platforms.
- **D-10:** Preserve existing bash test scripts (`test-policy.sh`, `test-sandbox.sh`, `verify-sandbox.sh`) as behavioral documentation. Write Go test equivalents for the same scenarios.

### Claude's Discretion
- Go function decomposition for SBPL generation (single function vs per-section helpers)
- SBPL comment style and formatting within the generated profile
- Integration test helper patterns (subprocess management, timeout handling)
- Debug output formatting beyond the required SBPL path and key rules

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| KERN-01 | Generate Apple SBPL profiles from resolved policy — filesystem modes (workspace/strict/permissive), network modes (blocked/unrestricted), denied path blocking | Bash `_sfx_generate_sbpl()` is the canonical spec (sandflox.bash:191-291). SBPL rule forms documented in "Standard Stack" and "Code Examples" below. |
| KERN-02 | Use `(allow default)` SBPL baseline with selective denials (not deny-default) | Bash implementation uses exactly this pattern. Rationale: flox/Nix/macOS frameworks need unpredictable read paths; deny-default is "too fragile" (sandflox.bash:197). |
| KERN-03 | Resolve `~` and `.` to absolute paths; use `/private/tmp` instead of `/tmp` | Already implemented by `config.go ResolvePath()` from Phase 1 — it canonicalizes `/tmp` → `/private/tmp` on darwin and expands `~`. `ResolvedConfig.Writable/ReadOnly/Denied` are already resolved. |
| KERN-04 | Wrap `flox activate` under `sandbox-exec -f <profile>` using `syscall.Exec` | `syscall.Exec` does Unix `execve()` — PID preserved, no child process (verified via pkg.go.dev/syscall). Existing `execFlox()` in main.go already uses this pattern. Phase 2 changes the argv to route through sandbox-exec. |
| KERN-05 | `sandflox` (no args) launches interactive sandboxed shell | Existing `execFlox(userArgs)` with `len(userArgs) == 0` → `flox activate` (no `--`). Sandbox-exec wrapping adds one layer of argv prefix. |
| KERN-06 | `sandflox -- CMD` wraps arbitrary commands | Existing `execFlox(userArgs)` with `len(userArgs) > 0` → `flox activate -- CMD`. Sandbox-exec wrapping preserves this. |
| KERN-07 | Allow localhost connections when `network.allow-localhost = true` even when network is blocked | SBPL rule: `(allow network* (remote ip "localhost:*"))` emitted after `(deny network*)`. Later rules have higher precedence per SBPL semantics. |
| KERN-08 | Allow Unix socket communication (Nix daemon) when network is blocked | SBPL rule: `(allow network* (remote unix-socket))` always emitted when net=blocked. Matches bash behavior at sandflox.bash:283. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

**Global (jhogan):**
- Prefer direct, opinionated answers with tradeoffs; no hedging preamble
- Code blocks: always specify language, realistic variable names
- Confluent Canon rules apply when relevant (not relevant this phase)

**Project (sandflox/CLAUDE.md):**
- GSD workflow enforcement — start work through a GSD command, no direct edits
- All file operations prefer specialized tools over bash `cat`/`grep`

**Project (CONVENTIONS.md + ARCHITECTURE.md):**
- **Naming**: lowercase-hyphenated scripts, `_sfx_` prefix for internal vars, `SANDFLOX_*` for exported env, section headers use `# ── Name ────` box-drawing
- **Error format**: all errors/warnings use `[sandflox]` prefix and go to stderr; `ERROR:` / `WARNING:` / `BLOCKED:` keywords
- **Exit codes**: shell function armor returns 126 (not 1/127); hard errors exit 1
- **Graceful degradation**: missing kernel tools produce WARNING and fall back to shell-only (not hard-fail)
- **Error philosophy**: security-relevant failures (bad SBPL) do NOT fall back silently — they exit with ERROR and sandbox-exec stderr
- **Go style (Phase 1 precedent)**: package `main`, zero external deps, stdlib only, package-level `stderr io.Writer` variable for testability, `ResolvedConfig` is the single source of truth for generation

## Standard Stack

### Core (Go stdlib only — project constraint)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `strings` | stdlib (Go 1.22) | `strings.Builder` for SBPL generation | Idiomatic Go string building; Go's `text/template` itself uses Builder internally per Go source. Correct choice when output is driven by conditionals, not data-parameterized templates. |
| `syscall` | stdlib (Go 1.22) | `syscall.Exec(argv0, argv, envv)` for process replacement | PID-preserving Unix `execve()`. Only way to satisfy success criterion #5 (clean exec replacement). Already used by Phase 1 `execFlox()`. |
| `os/exec` | stdlib (Go 1.22) | `exec.LookPath("sandbox-exec")` for preflight existence check | Standard way to probe for a binary without spawning it. |
| `os` | stdlib (Go 1.22) | File writes, env access, `os.UserHomeDir()` | Already in use across Phase 1 code. |
| `path/filepath` | stdlib (Go 1.22) | `filepath.Join()` for cache paths | Already in use. |
| `runtime` | stdlib (Go 1.22) | `runtime.GOOS == "darwin"` platform check | Already used by `config.go` for /tmp canonicalization. |
| `testing` | stdlib (Go 1.22) | Unit tests for SBPL string content | Phase 1 test pattern. |

### Supporting
| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| `sandbox-exec` | macOS system binary (code-signed `com.apple.sandbox-exec`, at `/usr/bin/sandbox-exec`) | Kernel enforcement | Always on darwin when policy.toml exists. Verified present on target (x86_64 + arm64e universal binary). |

**Verified version on target:** `/usr/bin/sandbox-exec` — Mach-O universal (x86_64, arm64e), shipped with macOS since 10.5. Marked DEPRECATED in man page since 2017 but **still functional and still used by Apple itself** (see `/System/Library/Sandbox/Profiles/*.sb`, over 200 active profiles used by system daemons on this very machine).

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `strings.Builder` | `text/template` | Templates shine when data is structured and iteration is heavy. Here, output shape is driven by flat switch on `FsMode`/`NetMode`. Template adds ceremony with no payoff. **Use strings.Builder.** |
| `syscall.Exec` | `exec.Command(...).Run()` | exec.Command spawns a child, preserving sandflox as parent. Violates success criterion #5 and burns one PID. **Use syscall.Exec.** |
| `syscall.Exec` | `os.StartProcess` | StartProcess also spawns a child (fork+exec). Same problem as exec.Command. **Use syscall.Exec.** |
| `(deny default)` with explicit allow list | `(allow default)` with explicit deny list | deny-default is the "secure" default but breaks flox: Nix daemon, macOS CoreFoundation frameworks, system dylibs, locale files, timezone data all need unpredictable reads. Bash implementation already proved `(allow default)` is the only viable baseline for this domain. **Locked by D-02 and validated by existing .sb in cache.** |
| Inline `-p "PROFILE_STRING"` | `-f profile.sb` | Inline strings hit shell quoting hell, especially with SBPL parens and the `(param "KEY")` form. File-based is cleaner, enables `--debug` path printing, and matches bash behavior. |

**Installation:** None — all stdlib and a macOS system binary. `go.mod` has zero external dependencies (verified: Phase 1 `go.mod` contains only `module sandflox` and `go 1.22`).

**Version verification:**
```bash
# Verified during research
/usr/bin/sandbox-exec                    # present, universal binary
go version                                # Go 1.22+ per go.mod
```

## Architecture Patterns

### Recommended File Organization
```
sandflox/
├── main.go            # unchanged structure; execFlox() gains sandbox-exec wrapping
├── cli.go             # unchanged
├── policy.go          # unchanged
├── config.go          # unchanged (ResolvePath already handles /tmp → /private/tmp)
├── cache.go           # unchanged (WriteCache is fine as-is)
├── sbpl.go            # NEW: SBPL generation + write-to-cache
├── sbpl_test.go       # NEW: table-driven string-content tests
├── exec_darwin.go     # NEW: Darwin-specific exec with sandbox-exec wrapping
├── exec_other.go      # NEW: Non-darwin stub that falls through to plain flox activate
├── exec_integration_test.go  # NEW: gated by darwin + sandbox-exec availability
└── ...
```

Rationale:
- `sbpl.go` keeps SBPL concerns in one file — easy to match against `sandflox.bash:191-291` during code review
- Darwin split via Go build tags (`//go:build darwin` / `//go:build !darwin`) keeps the binary buildable on Linux CI runners even though it only enforces on macOS
- Integration test in a separate file with its own build tag (`//go:build darwin && integration`) so `go test` runs fast by default

### Pattern 1: SBPL Generation Pipeline
**What:** Take `*ResolvedConfig` + home dir, return SBPL string. Pure function — no I/O.
**When to use:** Every sandflox invocation on darwin.
**Example:**
```go
// sbpl.go — one function per logical section, all writing to the same Builder
func GenerateSBPL(cfg *ResolvedConfig, home string) string {
    var sb strings.Builder
    writeSBPLHeader(&sb)
    writeSBPLDeniedPaths(&sb, cfg.Denied, home)
    writeSBPLFilesystem(&sb, cfg.FsMode, cfg.ReadOnly, home)
    writeSBPLNetwork(&sb, cfg.NetMode, cfg.AllowLocalhost)
    return sb.String()
}

func writeSBPLHeader(sb *strings.Builder) {
    sb.WriteString("(version 1)\n\n")
    sb.WriteString(";; sandflox — generated SBPL profile\n")
    sb.WriteString(";; Baseline: allow everything, then restrict per policy.toml\n")
    sb.WriteString("(allow default)\n")
}
```

### Pattern 2: Darwin Exec Wrapping via Build Tags
**What:** Separate Darwin-specific exec path from generic stub.
**When to use:** Any platform-specific syscall pattern.
**Example:**
```go
// exec_darwin.go
//go:build darwin

package main

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "syscall"
)

func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, userArgs []string) {
    sbxPath, err := exec.LookPath("sandbox-exec")
    if err != nil {
        fmt.Fprintf(stderr, "[sandflox] WARNING: sandbox-exec not found — falling back to shell-only\n")
        execFlox(userArgs)
        return
    }

    home, _ := os.UserHomeDir()
    sbplContent := GenerateSBPL(cfg, home)
    sbplPath := filepath.Join(projectDir, ".flox", "cache", "sandflox", "sandflox.sb")
    if err := os.WriteFile(sbplPath, []byte(sbplContent), 0644); err != nil {
        fmt.Fprintf(stderr, "[sandflox] ERROR: cannot write SBPL profile: %v\n", err)
        os.Exit(1)
    }

    fmt.Fprintf(stderr, "[sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)\n")

    floxPath, err := exec.LookPath("flox")
    if err != nil {
        fmt.Fprintf(stderr, "[sandflox] ERROR: flox not found in PATH\n")
        os.Exit(1)
    }

    argv := []string{
        "sandbox-exec",
        "-f", sbplPath,
        "-D", "PROJECT=" + projectDir,
        "-D", "HOME=" + home,
        "-D", "FLOX_CACHE=" + filepath.Join(home, ".cache", "flox"),
        floxPath, "activate",
    }
    if len(userArgs) > 0 {
        argv = append(argv, "--")
        argv = append(argv, userArgs...)
    }

    // syscall.Exec: PID preserved, no child process
    if err := syscall.Exec(sbxPath, argv, os.Environ()); err != nil {
        fmt.Fprintf(stderr, "[sandflox] ERROR: sandbox-exec failed: %v\n", err)
        os.Exit(1)
    }
}
```

```go
// exec_other.go
//go:build !darwin

package main

// On non-darwin, fall through to plain flox activate (shell-only enforcement).
func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, userArgs []string) {
    fmt.Fprintf(stderr, "[sandflox] WARNING: kernel enforcement only available on darwin — falling back to shell-only\n")
    execFlox(userArgs)
}
```

### Pattern 3: Table-Driven SBPL Unit Tests (D-08)
**What:** Generate SBPL from constructed `ResolvedConfig`, assert presence/absence of specific rules.
**When to use:** For every filesystem mode × network mode × denied-paths combination.
**Example:**
```go
// sbpl_test.go
func TestGenerateSBPL_WorkspaceBlocked(t *testing.T) {
    cfg := &ResolvedConfig{
        Profile: "default",
        FsMode:  "workspace",
        NetMode: "blocked",
        AllowLocalhost: true,
        Writable:  []string{"/Users/x/proj", "/private/tmp"},
        ReadOnly:  []string{"/Users/x/proj/.git/", "/Users/x/proj/policy.toml"},
        Denied:    []string{"/Users/x/.ssh/", "/Users/x/.aws/"},
    }
    out := GenerateSBPL(cfg, "/Users/x")

    assertContains(t, out, "(version 1)")
    assertContains(t, out, "(allow default)")
    assertContains(t, out, `(deny file-read* (subpath "/Users/x/.ssh"))`)
    assertContains(t, out, `(deny file-write* (subpath "/Users/x/.ssh"))`)
    assertContains(t, out, "(deny file-write*)")
    assertContains(t, out, `(subpath (param "PROJECT"))`)
    assertContains(t, out, `(subpath "/private/tmp")`)
    assertContains(t, out, `(deny file-write* (subpath "/Users/x/proj/.git"))`)       // trailing-slash → subpath
    assertContains(t, out, `(deny file-write* (literal "/Users/x/proj/policy.toml"))`) // no slash → literal
    assertContains(t, out, "(deny network*)")
    assertContains(t, out, "(allow network* (remote unix-socket))")
    assertContains(t, out, `(allow network* (remote ip "localhost:*"))`)
}

func TestGenerateSBPL_StrictNoLocalhost(t *testing.T) { /* ... */ }
func TestGenerateSBPL_PermissiveUnrestricted(t *testing.T) { /* no deny rules expected */ }
```

### Anti-Patterns to Avoid
- **Using `text/template` for SBPL generation**: Adds indirection with no gain. Output is not data-driven; it's mode-driven. Write direct `strings.Builder` calls.
- **Using `exec.Command().Run()` instead of `syscall.Exec`**: Leaves sandflox as parent process. Breaks success criterion #5 and wastes a PID on a process that does nothing after exec.
- **Caching the SBPL file across runs**: Policy might change between invocations (env var profile, CLI `--profile`). Regenerate every run per D-03 — it's <1ms.
- **Passing the SBPL via `-p "INLINE STRING"`**: Shell-quoting hell with parens, quotes, and embedded `(param "...")`. Use `-f file.sb`.
- **Using `sandbox-exec -n no-internet` pre-defined profile**: Locks you into Apple's profile list; gives up per-project control. Use `-f` with your generated profile.
- **Trying to use `(deny default)` + whitelist**: Will break flox/Nix/macOS frameworks. Multiple prior attempts documented in community sources (bazel, opam) confirm this. D-02 locks `(allow default)`.
- **Forgetting `/tmp` → `/private/tmp`**: macOS `/tmp` is a symlink. SBPL path matching is pre-symlink-resolution for `literal`/`subpath`. Already handled by Phase 1 `ResolvePath()`, but SBPL hardcoded rules for `/private/tmp` and `/private/var/folders` must use the resolved form (bash does this correctly at lines 238, 253).
- **Using `(path "X")` where `(literal "X")` is needed**: In SBPL, `(path "...")` and `(literal "...")` are sometimes treated as synonyms in system profiles, but the Apple Sandbox Guide and Chromium seatbelt design doc both use `(literal ...)` for exact file matches. Bash uses `(literal ...)` — mirror it.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Process replacement (preserve PID) | `fork` + `exec` manually via `syscall` | `syscall.Exec(argv0, argv, envv)` | Unix `execve()` directly. Stdlib primitive. |
| Path canonicalization (`/tmp` → `/private/tmp`, `~` expansion) | Custom string munging | Phase 1's `ResolvePath()` (already in `config.go`) | Already tested, handles darwin symlink quirks correctly. |
| TOML parsing | Reimplement for sandbox concerns | Phase 1's `ParsePolicy` (already in `policy.go`) | Already tested against v2 schema. |
| SBPL rule dialect / S-expression formatting | Write a full Lisp emitter | String concatenation with literal SBPL snippets | The profile is ~50 lines of boilerplate + a handful of parameterized rules. Overengineering it is pointless. |
| Cross-platform sandboxing abstraction | Build a "platform-agnostic sandbox interface" | Split `exec_darwin.go` / `exec_other.go` with build tags | Project is explicitly macOS-only (PROJECT.md constraint). bwrap is intentionally out of scope for this phase. |
| Detecting "am I inside sandbox-exec?" | Parse process ancestry | Check for presence of the generated `.sb` file in cache (already done by `test-policy.sh:40`) | Simple, robust, no syscall voodoo. |

**Key insight:** The hard part of this phase is already solved — by the bash implementation sitting in the repo. Everything except `syscall.Exec` + `strings.Builder` is plumbing that Phase 1 delivered. Resist the urge to invent new abstractions.

## Runtime State Inventory

**Scope check:** This is not a rename/refactor phase — it's a port. But there is runtime state to inventory because the bash implementation is still live and we need to confirm the Go port integrates cleanly.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — no databases or persistent records | None |
| Live service config | None — sandbox-exec is stateless (profile is read at invocation, discarded at exit) | None |
| OS-registered state | None — no launchd plists, no persistent daemons | None |
| Secrets/env vars | `SANDFLOX_PROFILE` (already read by `ResolveConfig`) — no new env vars introduced | None; passed through by `os.Environ()` |
| Build artifacts | `.flox/cache/sandflox/sandflox.sb` — currently generated by bash, will be overwritten by Go. `result-sandflox` / `result-sandflox-log` (Nix build outputs) — unchanged | Go overwrites `.sb` on every run (D-03). Nix rebuild after Phase 2 to get new binary with SBPL generation. |

**The canonical question:** *After every file in the repo is updated, what runtime systems still have the old string cached, stored, or registered?* — **Answer: none.** The existing `.flox/cache/sandflox/sandflox.sb` in the repo will be regenerated on first run of the new Go binary. Phase 2 has no data-migration burden.

## Common Pitfalls

### Pitfall 1: `syscall.Exec` called through a wrapper function doesn't take effect if the wrapper returns on error
**What goes wrong:** If `syscall.Exec` fails (bad argv, missing binary), it returns an error. If the caller ignores it or falls through, the Go process continues running and may do weird things.
**Why it happens:** `syscall.Exec` only returns on failure; success never returns.
**How to avoid:** After `syscall.Exec`, always `fmt.Fprintf(stderr, ...)` and `os.Exit(1)`. Already the pattern in Phase 1 `execFlox()`.
**Warning signs:** Any code after `syscall.Exec(...)` that doesn't immediately exit is a bug.

### Pitfall 2: `sandbox-exec` swallows the child's stderr on certain profile errors
**What goes wrong:** Malformed SBPL produces `sandbox-exec: failed to initialize sandbox` on stderr but no line-number info. Some errors (unbalanced parens, unknown operation) fail silently.
**Why it happens:** `sandbox-exec` is an Apple Private Interface; error reporting was never hardened.
**How to avoid:** Unit-test the generated SBPL string structure (D-08) so you catch rule-shape bugs at Go test time, not at sandbox-exec invocation time. Keep rules minimal — don't add novel SBPL constructs without verifying against an existing system profile in `/System/Library/Sandbox/Profiles/`.
**Warning signs:** sandbox-exec exits non-zero immediately with cryptic message → check your generated `.sb` by hand (or via `sandbox-exec -f bad.sb /bin/true` in isolation).

### Pitfall 3: SBPL rule ordering matters — later rules override earlier ones
**What goes wrong:** If you write `(allow file-write* (subpath ...))` before `(deny file-write*)`, the deny wins (later), and your writable paths are blocked.
**Why it happens:** "SBPL rule precedence: later rules have higher precedence" — HackTricks, Igor's Techno Club.
**How to avoid:** Bash implementation already has correct ordering: deny broad → allow specific. Mirror it rule-by-rule. Unit tests should assert rule ORDER, not just presence, where order is semantically critical.
**Warning signs:** Writes work outside project dir but not inside it → rule order is wrong.

### Pitfall 4: `/tmp` must be `/private/tmp` in SBPL on macOS
**What goes wrong:** `(subpath "/tmp")` doesn't match writes because macOS resolves `/tmp` → `/private/tmp` at the VFS layer, and SBPL's path predicates operate on the canonical path.
**Why it happens:** Apple's FS uses firmlinks/symlinks for legacy Unix paths; SBPL sees the post-firmlink path.
**How to avoid:** Phase 1's `ResolvePath()` already converts `/tmp` → `/private/tmp` on darwin for user-supplied policy paths. Hardcoded rules in SBPL generator (writable tmp fallback, var/folders) must also use `/private/` prefix — bash does this at lines 238-239, 252-253.
**Warning signs:** `/tmp/foo` writes blocked even when policy says `/tmp` is writable → path is not `/private/tmp`-canonicalized.

### Pitfall 5: `(param "KEY")` requires matching `-D KEY=value` on command line
**What goes wrong:** If the SBPL references `(param "PROJECT")` but `-D PROJECT=...` is not passed, sandbox-exec errors with parameter-not-defined, profile fails to compile.
**Why it happens:** Parameters are late-bound — referenced in profile text, resolved at command invocation.
**How to avoid:** The Go code MUST pass the same `-D` flags that the SBPL template references. Bash does this at lines 476-479 (PROJECT, HOME, FLOX_CACHE). Any new `(param "X")` reference requires corresponding `-D X=` — make this a code-review check.
**Warning signs:** `sandbox-exec: policy syntax error` with no other detail → check param references vs `-D` flags.

### Pitfall 6: flox itself is a Nix symlink — absolute path matters for `syscall.Exec`
**What goes wrong:** `syscall.Exec("/usr/bin/sandbox-exec", ["sandbox-exec", "-f", ..., "flox", "activate"], env)` may fail because argv's `flox` is a relative name, not the resolved binary.
**Why it happens:** sandbox-exec inherits its PATH from the caller's env. If PATH resolution inside sandbox-exec finds a different `flox`, behavior diverges.
**How to avoid:** Resolve `flox` path with `exec.LookPath("flox")` BEFORE building argv, and pass the absolute path in argv. Bash escapes this by letting shell do the PATH lookup, but we lose that in direct `syscall.Exec`. See Pattern 2 example above.
**Warning signs:** `sandbox-exec` spawns something but it's not the flox you expected → argv flox was not absolute.

### Pitfall 7: Integration tests for kernel enforcement are inherently hostile to `go test`
**What goes wrong:** A test that actually runs sandbox-exec, then tries to write outside the sandbox, will EITHER succeed (bug: sandbox didn't activate) OR fail with EPERM (test framework may crash, permission errors may confuse test harness).
**Why it happens:** Kernel enforcement is global to the process; a subprocess under sandbox-exec cannot see "outside."
**How to avoid:** Run integration tests via `exec.Command` spawning a fresh subprocess with sandbox-exec. Collect exit code + stderr. Don't try to have the test process itself be under sandbox-exec. See Validation Architecture below.
**Warning signs:** Flaky test with "permission denied" on totally unrelated operations → test process got sandboxed unintentionally.

## Code Examples

Verified patterns from official sources and the existing bash implementation:

### SBPL Header + Baseline (matches bash sandflox.bash:200-206)
```scheme
(version 1)

;; sandflox — generated SBPL profile
;; Baseline: allow everything, then restrict per policy.toml
(allow default)
```
Source: `.flox/cache/sandflox/sandflox.sb` (live generated output in repo)

### SBPL Denied Paths (matches bash sandflox.bash:212-217)
```scheme
;; ── Denied paths (sensitive data) ──
(deny file-read* (subpath "/Users/jhogan/.ssh"))
(deny file-write* (subpath "/Users/jhogan/.ssh"))
(deny file-read* (subpath "/Users/jhogan/.aws"))
(deny file-write* (subpath "/Users/jhogan/.aws"))
```
Source: `.flox/cache/sandflox/sandflox.sb` lines 8-17. Note: trailing slash on the input (`~/.ssh/`) is stripped before the rule (`subpath` semantics don't need the slash).

### SBPL Filesystem Workspace Mode with Parameters (matches bash sandflox.bash:248-257)
```scheme
;; ── Filesystem writes (workspace) ──
(deny file-write*)
(allow file-write*
  (subpath (param "PROJECT"))
  (subpath "/private/tmp")
  (subpath "/private/var/folders")
  (subpath "/dev")
  (subpath (param "FLOX_CACHE"))
  (subpath "/Users/jhogan/.config/flox")
  (subpath "/Users/jhogan/.local/share/flox"))
```
Source: `.flox/cache/sandflox/sandflox.sb` lines 25-33. Rule ordering: broad deny → specific allow. `(param "PROJECT")` resolved from `-D PROJECT=...` at invocation.

### SBPL Read-Only Overrides (matches bash sandflox.bash:262-269)
```scheme
;; Read-only overrides within project
(deny file-write* (subpath "/Users/jhogan/sandflox/.flox/env"))   ;; input ended with /
(deny file-write* (subpath "/Users/jhogan/sandflox/.git"))         ;; input ended with /
(deny file-write* (literal "/Users/jhogan/sandflox/.env"))         ;; no trailing slash
(deny file-write* (literal "/Users/jhogan/sandflox/policy.toml"))  ;; no trailing slash
```
Source: `.flox/cache/sandflox/sandflox.sb` lines 36-40. Rule: trailing-slash input → `subpath`, no trailing slash → `literal`. D-04 locks this behavior.

### SBPL Network Rules (matches bash sandflox.bash:275-288)
```scheme
;; ── Network (blocked) ──
(deny network*)
(allow network* (remote unix-socket))         ;; always — nix daemon IPC
(allow network* (remote ip "localhost:*"))    ;; only if allow-localhost=true
```
Source: `.flox/cache/sandflox/sandflox.sb` lines 43-45. For `unrestricted` mode, just emit a comment and no deny rule.

### Go syscall.Exec Pattern (Phase 1 precedent from main.go:127-130)
```go
// Source: main.go execFlox()
floxPath, err := exec.LookPath("flox")
if err != nil {
    fmt.Fprintf(stderr, "[sandflox] ERROR: flox not found in PATH\n")
    os.Exit(1)
}

argv := []string{"flox", "activate"}
if len(userArgs) > 0 {
    argv = append(argv, "--")
    argv = append(argv, userArgs...)
}

// Replace this process with flox -- does not return on success
execErr := syscall.Exec(floxPath, argv, os.Environ())
// If we get here, exec failed
fmt.Fprintf(stderr, "[sandflox] ERROR: exec failed: %v\n", execErr)
os.Exit(1)
```
Source: `/Users/jhogan/sandflox/main.go` lines 112-131.

### sandbox-exec Invocation (matches bash sandflox.bash:476-480)
```bash
sandbox-exec -f /Users/jhogan/sandflox/.flox/cache/sandflox/sandflox.sb \
  -D "PROJECT=/Users/jhogan/sandflox" \
  -D "HOME=/Users/jhogan" \
  -D "FLOX_CACHE=/Users/jhogan/.cache/flox" \
  flox activate
```
Source: `sandflox.bash:476-480`. This is the canonical command shape. In Go, split into argv + absolute `flox` path.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `sandbox_init()` C API (private) | `sandbox-exec` CLI wrapping | Always (CLI is the supported public surface) | CLI is stable even though the language is "private"; Apple itself keeps sandbox-exec working because hundreds of system profiles depend on it |
| Bash+Python inline parsing | Compiled Go binary (Phase 1) + Go SBPL generator (Phase 2) | 2026-04 (this rewrite) | No Python dependency at runtime; ~1ms generation cost vs ~50ms bash+python startup; single binary distributable via `flox install` |
| `deny default` restrictive profile | `allow default` with targeted denies | Bash implementation pinned this ~2 years ago after flox/Nix incompatibilities | Locked by D-02; accepted tradeoff — denied paths + writable mode still provide real security |
| `/tmp` in SBPL paths | `/private/tmp` in SBPL paths | macOS 10.15 firmlinks era | SBPL path matching is post-firmlink; `/tmp/*` rules silently do not match |

**Deprecated/outdated:**
- **`sandbox-exec` is marked DEPRECATED** in its own man page (since March 2017: "Developers who wish to sandbox an app should instead adopt the App Sandbox feature"). However, App Sandbox requires code signing, entitlements, and an Apple Developer account — it's the wrong tool for wrapping `flox activate`. sandbox-exec remains the only public API for ad-hoc sandboxing on macOS, and Apple still ships/uses it extensively. **Risk: Apple could remove it in a future macOS major version.** Mitigate by: (a) this is v1 scope, (b) graceful fallback to shell-only enforcement if sandbox-exec disappears, (c) watch for macOS major-version announcements.
- **Pre-defined profile names (`-n no-internet`, `-n pure-computation`)** — these work but don't support parameter substitution or custom rules. Out of scope for our needs.

## Open Questions

1. **Does `syscall.Exec` preserve `os.Args[0]` correctly under sandbox-exec?**
   - What we know: The test `test-policy.sh:40` checks for the `.sb` file to detect kernel enforcement. Argv0 under sandbox-exec should be `flox` (what we pass) since sandbox-exec's own execve replaces argv with its child's argv.
   - What's unclear: Whether any tooling inside the sandbox reads argv0 and fails if it's not an expected value.
   - Recommendation: Verify during Phase 2 execution. If the bash wrapper works (it does, per current working implementation), Go will too. D-07 `--debug` flag should print the exact argv passed to `syscall.Exec` for diagnosis.

2. **Does `sandbox-exec` honor `$TMPDIR` or only hardcoded `/private/var/folders`?**
   - What we know: Bash hardcodes `/private/var/folders` and `/private/tmp` in SBPL (lines 238-239, 252-253). This works on the current machine.
   - What's unclear: If a user has a non-default `TMPDIR`, will the sandbox still allow writes there?
   - Recommendation: Ignore for Phase 2 — matches bash behavior. If it becomes an issue, add a discovered-path that resolves `$TMPDIR` at runtime. Not blocking.

3. **How should the Go binary detect that we're already running inside a sandbox-exec invocation (re-entry)?**
   - What we know: bash doesn't handle this; double-wrapping just doubles the sandbox (usually harmless). `CMD-03 sandflox elevate` in Phase 5 will need this detection.
   - What's unclear: Whether nested sandbox-exec is even permitted by macOS (some SBPL rules prevent spawning sandbox-exec from within a sandbox).
   - Recommendation: Defer to Phase 5 (CMD-03 elevate). Phase 2 doesn't need re-entry detection.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | `go test`, `go build` | ✓ | Go 1.22+ (per go.mod, verified building via flox in Phase 1) | — |
| `/usr/bin/sandbox-exec` | KERN-01 through KERN-08 runtime | ✓ | macOS universal binary (x86_64 + arm64e, Platform identifier 26) | D-05: warning + fall back to shell-only enforcement |
| `/System/Library/Sandbox/Profiles/` | Research reference only | ✓ | Ships with macOS | N/A (reference material) |
| `flox` CLI | Exec target | ✓ (assumed from Phase 1) | Flox 1.10+ | Hard error — cannot continue without flox |
| `bash` | Integration test helpers (optional) | ✓ (system `/bin/bash`) | 3.2+ | Not required for Go unit tests |

**Missing dependencies with no fallback:** None on darwin. On Linux, `sandbox-exec` will be absent — per PROJECT.md constraint, sandflox is macOS-only for this phase; `exec_other.go` stub prints warning and falls through to plain `flox activate`.

**Missing dependencies with fallback:** `sandbox-exec` absence (theoretical — would indicate macOS regression or non-standard install) falls back to shell-only per D-05.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.22+) |
| Config file | `go.mod` (no test config file required) |
| Quick run command | `go test -run 'TestGenerateSBPL|TestExec' -short` |
| Full suite command | `go test ./... -v` |
| Integration tag | `go test -tags integration ./... -run TestSandboxExec` (darwin only) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| KERN-01 | SBPL profile generated from ResolvedConfig | unit | `go test -run TestGenerateSBPL_WorkspaceBlocked` | ❌ Wave 0 — new sbpl_test.go |
| KERN-02 | `(allow default)` baseline with selective denials | unit | `go test -run TestGenerateSBPL_AllowDefaultBaseline` | ❌ Wave 0 |
| KERN-03 | `/tmp` → `/private/tmp`, `~` expansion | unit (already tested in config_test.go from Phase 1) | `go test -run TestResolvePath` | ✅ Phase 1 test exists; add SBPL-output assertion |
| KERN-04 | `syscall.Exec` wraps flox activate under sandbox-exec | integration (darwin) | `go test -tags integration -run TestSandboxExecLaunch` | ❌ Wave 0 — new exec_integration_test.go |
| KERN-05 | `sandflox` (no args) interactive mode | integration (darwin) | `go test -tags integration -run TestSandboxExecInteractive` | ❌ Wave 0 |
| KERN-06 | `sandflox -- CMD` non-interactive mode | integration (darwin) | `go test -tags integration -run TestSandboxExecWrapCommand` | ❌ Wave 0 |
| KERN-07 | Localhost allowance when allow-localhost=true | unit (SBPL string) + integration (real socket) | `go test -run TestGenerateSBPL_LocalhostAllowed` | ❌ Wave 0 |
| KERN-08 | Unix socket always allowed when net=blocked | unit (SBPL string) | `go test -run TestGenerateSBPL_UnixSocketAlwaysAllowed` | ❌ Wave 0 |

### Integration Test Strategy (D-09)

Because kernel enforcement is process-global, integration tests spawn a subprocess:

```go
//go:build darwin && integration

func TestSandboxExecBlocksWriteOutsideWorkspace(t *testing.T) {
    if _, err := exec.LookPath("sandbox-exec"); err != nil {
        t.Skip("sandbox-exec not available")
    }

    tmpDir := t.TempDir()
    sbplPath := filepath.Join(tmpDir, "test.sb")
    os.WriteFile(sbplPath, []byte(`
(version 1)
(allow default)
(deny file-write*)
(allow file-write* (subpath "`+tmpDir+`"))
`), 0644)

    // Try to write OUTSIDE the sandbox-allowed dir; should fail
    cmd := exec.Command("sandbox-exec", "-f", sbplPath,
        "/bin/sh", "-c", "echo hi > /etc/sandflox-test-"+fmt.Sprint(os.Getpid()))
    err := cmd.Run()
    if err == nil {
        t.Fatal("expected sandbox to block write to /etc, but it succeeded")
    }

    // Write INSIDE the allowed dir; should succeed
    cmd = exec.Command("sandbox-exec", "-f", sbplPath,
        "/bin/sh", "-c", "echo hi > "+tmpDir+"/test.txt")
    if err := cmd.Run(); err != nil {
        t.Fatalf("expected sandbox to allow write to %s, got error: %v", tmpDir, err)
    }
}
```

Key patterns:
- **Subprocess isolation**: The test process is NOT sandboxed — only the spawned child is. Prevents test-harness confusion.
- **Timeout**: Use `context.WithTimeout` + `exec.CommandContext` to avoid hung tests on network operations.
- **Build tags**: `//go:build darwin && integration` keeps CI fast; run integration explicitly via `-tags integration`.
- **Skip on missing sandbox-exec**: `t.Skip()` instead of failing when the dep is absent (e.g., Linux CI, even though we don't target it).

### Sampling Rate
- **Per task commit:** `go test -run '^TestGenerateSBPL|^TestExec' -short` (SBPL unit tests, fast)
- **Per wave merge:** `go test ./... -v` (all units + Phase 1 regression)
- **Phase gate:** `go test ./... -v && go test -tags integration ./... -v` (full suite + macOS integration before `/gsd:verify-work`)

### Wave 0 Gaps
- [ ] `sbpl.go` — new file implementing `GenerateSBPL(*ResolvedConfig, home string) string` and `WriteSBPL(cacheDir, content string) (string, error)`
- [ ] `sbpl_test.go` — table-driven tests per filesystem × network mode combination (D-08)
- [ ] `exec_darwin.go` — new file with `//go:build darwin` and `execWithKernelEnforcement(*ResolvedConfig, projectDir string, userArgs []string)`
- [ ] `exec_other.go` — new file with `//go:build !darwin` stub that falls through to plain flox activate
- [ ] `exec_integration_test.go` — new file with `//go:build darwin && integration` for real sandbox-exec subprocess tests (D-09)
- [ ] `main.go` modification — route through `execWithKernelEnforcement()` when policy exists on darwin, keep existing `execFlox()` as fallback
- [ ] `main.go` diagnostic: on `--debug`, print generated SBPL path and key rules count (D-07)

## Sources

### Primary (HIGH confidence)
- `/Users/jhogan/sandflox/sandflox.bash` lines 191-291, 440-500 — canonical SBPL generation and sandbox-exec invocation logic (proven in production on this machine)
- `/Users/jhogan/sandflox/.flox/cache/sandflox/sandflox.sb` — live generated SBPL output showing exact expected shape
- `/Users/jhogan/sandflox/main.go` lines 107-131 — Phase 1 `execFlox()` pattern for `syscall.Exec` + exec.LookPath
- `/Users/jhogan/sandflox/config.go` lines 90-121 — Phase 1 `ResolvePath()` with darwin `/tmp` → `/private/tmp` canonicalization
- `/System/Library/Sandbox/Profiles/bsd.sb`, `airlock.sb` — Apple's own use of SBPL: confirms `(version 1)`, `(import ...)`, `(param ...)`, `(when ...)`, `(allow file-read-metadata)`, regex/subpath/literal rule forms
- `man sandbox-exec` on target machine — confirms `-f profile-file`, `-n profile-name`, `-p profile-string`, `-D key=value` flags
- [Go syscall.Exec documentation](https://pkg.go.dev/syscall#Exec) — confirms PID-preserving execve semantics, "never returns on success"
- [Chromium Seatbelt sandbox design doc](https://chromium.googlesource.com/chromium/src/+/HEAD/sandbox/mac/seatbelt_sandbox_design.md) — authoritative source on SBPL parameter substitution and rule semantics
- [Chromium common.sb](https://chromium.googlesource.com/chromium/src/+/HEAD/sandbox/policy/mac/common.sb) — real-world production SBPL with `(param ...)`, `(subpath ...)`, regex, and network rules

### Secondary (MEDIUM confidence)
- [tekumara/seatbelt codex.sb](https://github.com/tekumara/seatbelt/blob/main/codex.sb) — OpenAI-derived SBPL profile showing `(deny default)` + whitelist pattern and `(debug deny)` for Console.app logging
- [OpenAI Codex sandboxing discussion #6807](https://github.com/openai/codex/issues/6807) — confirms `(allow network* (remote ip "localhost:*"))` + `(allow network* (remote unix-socket))` as the canonical localhost-only pattern
- [Julio Merino — "A quick glance at macOS' sandbox-exec"](https://jmmv.dev/2019/11/macos-sandbox-exec.html) — general SBPL overview, confirms Scheme-like syntax and rule precedence (later rules win)
- [Igor's Techno Club — "sandbox-exec: macOS's Little-Known Command-Line Sandboxing Tool"](https://igorstechnoclub.com/sandbox-exec/) — confirms literal/subpath/regex predicates and practical example shapes
- [HackTricks macOS Sandbox](https://hacktricks.wiki/en/macos-hardening/macos-security-and-privilege-escalation/macos-security-protections/macos-sandbox/index.html) — confirms SBPL rule list and parameter substitution patterns
- [Go pkg/os/exec](https://pkg.go.dev/os/exec) — `exec.LookPath` for binary discovery, contrast with `syscall.Exec`

### Tertiary (LOW confidence, flagged for validation)
- [Apple Sandbox Guide v1.0 (2011)](https://reverse.put.as/wp-content/uploads/2011/09/Apple-Sandbox-Guide-v1.0.pdf) — dated (2011) reverse-engineering PDF; used only for general shape reference, not as authoritative. Cannot extract full text from binary PDF via WebFetch.
- Community sources asserting "sandbox-exec could be removed in future macOS" — speculative but prudent to note; no Apple announcement exists. Mitigated by graceful fallback (D-05).

## Metadata

**Confidence breakdown:**
- Standard stack: **HIGH** — all stdlib, pattern already proven in Phase 1 `execFlox()`, no external Go deps needed
- SBPL rule shapes: **HIGH** — have live generated `.sb` file in repo + matching bash source as canonical spec + Apple's own system profiles confirming syntax
- sandbox-exec CLI behavior: **HIGH** — verified on target (man page + working bash implementation + codesigned universal binary present)
- `syscall.Exec` behavior: **HIGH** — Go stdlib docs explicit, Phase 1 already uses it successfully
- Integration test strategy: **MEDIUM** — subprocess spawning pattern is well-understood, but exact SBPL corner cases (e.g., how `-D` handles spaces in values) may need iteration during implementation
- Deprecation risk of sandbox-exec: **LOW** — no official removal announcement, but marked DEPRECATED in man page since 2017. Graceful fallback mitigates.

**Research date:** 2026-04-16
**Valid until:** 2026-07-16 (90 days) — SBPL is a stable/frozen language (Apple hasn't changed it in years), Go syscall semantics are stable, and the bash implementation provides regression coverage. Only meaningful risk: a macOS major version could deprecate `sandbox-exec` outright; watch WWDC announcements.
