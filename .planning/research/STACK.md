# Technology Stack

**Project:** sandflox (Go rewrite)
**Researched:** 2026-04-15

## Recommended Stack

### Core Language

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go | 1.24.x | Binary language | Matches flox-bwrap precedent. Zero-dep `buildGoModule` Nix builds. `syscall.Exec` for clean process replacement. Static binary, no runtime deps. Go 1.24.13 is current stable (supported through May 2026). Go 1.25 (Aug 2025) and 1.26 exist but nixpkgs/Flox catalog pins to 1.24.x as of writing. The flox-bwrap reference uses `go 1.25.5` in its go.mod but this project should target 1.24.x for broadest Flox compatibility unless the catalog updates. **Confidence: HIGH** |

### TOML Parsing (Zero External Dependencies)

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Custom minimal parser | n/a (in-tree) | Parse `policy.toml` | Go has **no stdlib TOML package** (`encoding/toml` has never been proposed). The zero-external-dep constraint rules out `BurntSushi/toml` and `pelletier/go-toml`. The existing sandflox already contains a ~40-line Python TOML subset parser that handles the exact `policy.toml` schema. Port that same approach to Go: a ~100-150 line parser handling `[tables]`, `[dotted.tables]`, `key = "string"`, `key = true/false`, `key = ["array", "of", "strings"]`, and `# comments`. This is safe because `policy.toml` is a controlled schema we own -- we are not parsing arbitrary TOML. **Confidence: HIGH** |

**TOML Parser Implementation Notes:**

The `policy.toml` schema uses exactly these TOML features:
- Section headers: `[meta]`, `[network]`, `[filesystem]`, `[profiles.minimal]`
- String values: `mode = "blocked"`
- Boolean values: `allow-localhost = true`
- String arrays: `writable = [".", "/tmp"]`
- Comments: `# ...` and inline `# ...`

It does **not** use: inline tables, multiline strings, integers, floats, dates, nested arrays, or any advanced TOML features. A purpose-built parser for this subset is ~100 lines of Go and eliminates the entire `BurntSushi/toml` dependency tree.

```go
// Approximate parser structure (sketch, not final)
type Policy struct {
    Meta       MetaSection
    Network    NetworkSection
    Filesystem FilesystemSection
    Profiles   map[string]ProfileSection
}

func ParsePolicy(path string) (*Policy, error) {
    // Line-by-line: track current section, parse key=value pairs
    // Regex: ^\\[([^\\]]+)\\]$ for sections
    // Regex: ^([^=]+?)\\s*=\\s*(.+)$ for key-value
    // Value parser: quoted string, boolean, array
}
```

### SBPL Profile Generation

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go `strings.Builder` + `fmt.Fprintf` | stdlib | Generate SBPL (Seatbelt Profile Language) profiles | SBPL is a Scheme-like DSL. The existing sandflox generates it via bash `echo` statements. In Go, use `strings.Builder` or `bytes.Buffer` with `fmt.Fprintf` to build the profile string. Write to `.flox/cache/sandflox/sandflox.sb`, then pass to `sandbox-exec -f <path> -D KEY=VALUE`. No SBPL library exists for Go (or any language, really -- SBPL is undocumented Apple-internal). String templating is the only approach. **Confidence: HIGH** |

**SBPL Generation Pattern:**

```go
func GenerateSBPL(cfg *ResolvedConfig) string {
    var sb strings.Builder
    sb.WriteString("(version 1)\n\n")
    sb.WriteString("(allow default)\n")

    // Denied paths
    for _, path := range cfg.DeniedPaths {
        fmt.Fprintf(&sb, "(deny file-read* (subpath %q))\n", path)
        fmt.Fprintf(&sb, "(deny file-write* (subpath %q))\n", path)
    }

    // Filesystem mode (workspace/strict/permissive)
    switch cfg.FSMode {
    case "workspace":
        sb.WriteString("(deny file-write*)\n")
        sb.WriteString("(allow file-write*\n")
        sb.WriteString("  (subpath (param \"PROJECT\"))\n")
        // ... writable paths, flox dirs, /private/tmp, etc.
        sb.WriteString(")\n")
    case "strict":
        sb.WriteString("(deny file-write*)\n")
        // Only essential writes
    }

    // Network mode
    if cfg.NetMode == "blocked" {
        sb.WriteString("(deny network*)\n")
        sb.WriteString("(allow network* (remote unix-socket))\n")
        if cfg.AllowLocalhost {
            sb.WriteString("(allow network* (remote ip \"localhost:*\"))\n")
        }
    }

    return sb.String()
}
```

### Process Execution

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `syscall.Exec` | stdlib | Replace current process with sandbox-exec | Matches flox-bwrap pattern exactly. `syscall.Exec` does Unix `execve(2)` -- replaces the Go process entirely with `sandbox-exec`, which in turn runs `flox activate`. No child process overhead, no zombie processes, signal forwarding is free. This is the critical pattern for Flox tool wrappers. **Confidence: HIGH** |
| `os/exec.Command` | stdlib | Run subprocesses for setup (nix-store queries, file ops) | For pre-exec setup like running `nix-store --query --requisites` to enumerate store paths (if needed). NOT for the final sandbox-exec invocation -- that MUST use `syscall.Exec`. **Confidence: HIGH** |
| `os/exec.LookPath` | stdlib | Find `sandbox-exec` and `flox` binaries | Locates binaries on PATH. For `sandbox-exec`, which lives at `/usr/bin/sandbox-exec` on macOS, this is reliable. If not found, fall back to shell-only enforcement (graceful degradation). **Confidence: HIGH** |

### CLI Flag Parsing

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `flag` | stdlib | Parse CLI flags | Matches flox-bwrap precedent exactly. No external CLI framework (cobra, urfave/cli). `flag` is sufficient for the sandflox flag surface: `--net`, `--profile <name>`, `--policy <path>`, `--debug`, `--requisites <file>`. Implements `flag.Value` interface for repeatable flags if needed. **Confidence: HIGH** |

### File Operations

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `os` | stdlib | File I/O, environment variables, process info | Read policy.toml, write cache files, read requisites files, get HOME/cwd |
| `os/user` | stdlib | Get home directory | `os.UserHomeDir()` for resolving `~/` paths in policy |
| `path/filepath` | stdlib | Path manipulation | `filepath.Abs()`, `filepath.Join()`, `filepath.EvalSymlinks()` for resolving relative/tilde paths in policy.toml |
| `bufio` | stdlib | Line-by-line file reading | Parse requisites.txt files (one tool per line) |
| `strings` | stdlib | String manipulation | SBPL generation, TOML parsing, path manipulation |
| `fmt` | stdlib | Formatted output | Diagnostic `[sandflox]` messages to stderr, SBPL generation |

### Shell Enforcement Artifacts

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go `os.WriteFile` / `os.MkdirAll` | stdlib | Generate shell enforcement scripts | The binary generates `fs-filter.sh`, `usercustomize.py`, `entrypoint.sh`, and config cache files. These are text files written to `.flox/cache/sandflox/`. The Go binary takes ownership of ALL artifact generation that the current Bash+Python does. **Confidence: HIGH** |
| Go `os.Symlink` | stdlib | Build filtered bin directory | Create symlinks from `$FLOX_ENV/bin/<tool>` into the sandflox bin directory for requisites filtering. The Go binary can do this during setup before exec-ing into sandbox-exec. **Confidence: MEDIUM** -- depends on whether the binary handles this pre-exec or delegates to entrypoint.sh |

### Build System

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Nix `buildGoModule` | nixpkgs stable | Build the Go binary in a Nix sandbox | The proven pattern from flox-bwrap. Handles Go compilation, vendoring, and packaging in one derivation. `vendorHash = null` when there are zero external Go dependencies (our case). **Confidence: HIGH** |
| `lib.fileset.toSource` | nixpkgs stable | Select Go source files for the Nix build | Prevents cache invalidation by only including relevant source files. Pattern from flox-bwrap: explicitly list `.go` files and `go.mod`. **Confidence: HIGH** |
| `-ldflags "-X main.var=value"` | Go linker | Inject build-time paths | Inject Nix store paths at build time (e.g., path to `flox` binary if needed). flox-bwrap uses this to inject the bwrap path. sandflox may not need this since `sandbox-exec` is a system binary at a fixed path (`/usr/bin/sandbox-exec`), but the pattern should be available. **Confidence: HIGH** |

### Distribution

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `flox build` | Flox 1.10+ | Build the package locally | Runs the `.flox/pkgs/sandflox.nix` expression. Produces a `result-sandflox` symlink. **Confidence: HIGH** |
| `flox publish` | Flox 1.10+ | Publish to FloxHub | `flox publish sandflox` uploads the built package. Requires: git repo, clean working tree, current commit pushed to remote. Published under user/org namespace. **Confidence: HIGH** |
| `flox install` | Flox 1.10+ | Install into any environment | Users run `flox install <owner>/sandflox` to add the binary to their environment. **Confidence: HIGH** |

## Go Stdlib Package Summary

Every package the binary needs, zero external dependencies:

| Package | Purpose |
|---------|---------|
| `bufio` | Line-by-line file reading (requisites, TOML) |
| `flag` | CLI argument parsing |
| `fmt` | Formatted I/O, stderr diagnostics |
| `os` | File I/O, env vars, process |
| `os/exec` | Subprocess for nix-store queries, LookPath |
| `os/user` | Home directory resolution |
| `path/filepath` | Path joining, resolution, symlink eval |
| `regexp` | TOML parser (section/key-value regex) |
| `strings` | String operations, SBPL building |
| `syscall` | `syscall.Exec` for process replacement |

## Nix Build Expression

Exact pattern for `.flox/pkgs/sandflox.nix`:

```nix
{ lib, buildGoModule }:

buildGoModule {
  pname = "sandflox";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../..;
    fileset = lib.fileset.unions [
      ../../go.mod
      ../../main.go
      ../../config.go
      ../../policy.go
      ../../sbpl.go
      ../../shell.go
      ../../sandbox.go
    ];
  };

  # null because zero external Go dependencies
  vendorHash = null;

  # No build-time path injection needed -- sandbox-exec is a
  # system binary at /usr/bin/sandbox-exec (not in Nix store).
  # If we later need to inject paths:
  # ldflags = [ "-X main.someVar=${someNixPackage}/bin/something" ];

  meta = with lib; {
    description = "macOS-native sandbox for AI coding agents via Flox";
    homepage = "https://github.com/jhogan/sandflox";
    license = licenses.mit;
    mainProgram = "sandflox";
    platforms = platforms.darwin;
  };
}
```

**Key differences from flox-bwrap's Nix expression:**
- `platforms = platforms.darwin` -- sandflox is macOS-only
- No `bubblewrap` input -- we use macOS `sandbox-exec` (system binary, not Nix-packaged)
- No `ldflags` needed initially -- `sandbox-exec` lives at `/usr/bin/sandbox-exec` always
- Same `lib.fileset.toSource` pattern for hermetic source selection

## Flox Manifest for Development + Build

The manifest needs two concerns: (1) the existing sandflox enforcement environment and (2) Go build tooling.

For the Go binary **build environment**, a separate or modified manifest with:

```toml
[install]
go.pkg-path = "go"

[build.sandflox]
# Nix expression build -- defined in .flox/pkgs/sandflox.nix
```

The `[build.sandflox]` entry without a `command` field tells Flox to look for `.flox/pkgs/sandflox.nix`. The `go` install provides the development toolchain.

## Build and Publish Workflow

```bash
# 1. Development: edit Go source files
#    go.mod has zero requires, just module path + go version

# 2. Local build test
flox build sandflox
# Creates result-sandflox/ symlink with the binary

# 3. Run the built binary
./result-sandflox/bin/sandflox --debug

# 4. Publish (requires clean git, pushed to remote)
git add . && git commit -m "..."
git push
flox publish sandflox
# or: flox publish -o <org> sandflox

# 5. Users install it
flox install jhogan/sandflox
# Then: sandflox -- claude-code
```

## File Layout

```
sandflox/
  go.mod                    # module github.com/jhogan/sandflox; go 1.24
  main.go                   # entrypoint: ParseConfig, Validate, run()
  config.go                 # Config struct, flag parsing, validation
  policy.go                 # TOML parser, Policy struct, resolution logic
  sbpl.go                   # SBPL profile generation
  shell.go                  # Shell enforcement artifact generation
  sandbox.go                # sandbox-exec invocation, syscall.Exec
  policy.toml               # declarative security policy (user-facing)
  requisites.txt            # binary whitelist (user-facing)
  .flox/
    pkgs/
      sandflox.nix          # Nix build expression (buildGoModule)
    env/
      manifest.toml         # Flox manifest (go in [install], sandflox in [build])
```

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Language | Go | Rust | flox-bwrap establishes Go as the Flox tool convention. Rust adds compile time, Nix complexity (`buildRustPackage` + cargo hashes), and deviates from the reference. Go's stdlib is sufficient for every sandflox need |
| Language | Go | Bash (keep current) | Cannot `flox publish` a Bash script as a standalone package. Python dependency for TOML parsing is fragile. Not a single artifact |
| TOML parsing | Custom subset parser | `BurntSushi/toml` | Violates zero-external-dep constraint. Would require `vendorHash` in Nix, adds supply chain surface. Our TOML subset is trivial (~5 features) |
| TOML parsing | Custom subset parser | `encoding/json` (rewrite policy as JSON) | Breaks backward compatibility with existing `policy.toml` files. TOML is more human-friendly for a config file. Comments matter for policy documentation |
| CLI framework | `flag` stdlib | `cobra` / `urfave/cli` | External dependency. `flag` handles sandflox's flag surface. flox-bwrap uses `flag` |
| Process exec | `syscall.Exec` | `os/exec.Command` + Wait | Creates a child process. Breaks signal forwarding semantics. flox-bwrap uses `syscall.Exec` for good reason -- the Go binary should vanish from the process tree |
| Build system | `buildGoModule` Nix | `go build` in manifest `[build.sandflox].command` | Manifest command builds are simpler but less reproducible. Nix expression builds with `buildGoModule` handle vendoring, cross-compilation, and cache correctly. flox-bwrap uses this pattern |

## sandbox-exec Deprecation Risk Assessment

**Status:** `sandbox-exec` is officially deprecated by Apple (man page marked deprecated since macOS 10.x). However:

- **It still ships** in macOS 15 Sequoia (2024) and will ship in macOS 26 Tahoe (2025)
- **Major products use it:** OpenAI Codex, Anthropic Claude Code, Google Chrome all ship sandbox-exec profiles
- **No replacement exists** for process-level sandboxing on macOS. Apple's new Containerization framework (WWDC 2025) is for Linux containers in VMs, not macOS process sandboxing
- **Apple uses it internally:** App Store sandboxing still runs on the Seatbelt kernel extension that sandbox-exec invokes
- **Practical risk:** LOW for the next 2-3 years. Apple cannot remove it without breaking their own App Store sandbox. If they eventually restrict it to entitled processes, sandflox can degrade gracefully to shell-only enforcement (which is already the documented behavior when sandbox-exec is unavailable)

**Confidence: MEDIUM** -- deprecation status is factual, but future Apple decisions are inherently unpredictable.

## Go Version Notes

| Version | Status | Notes |
|---------|--------|-------|
| Go 1.24.13 | Current stable (Feb 2026) | Supported through May 2026. Features: Swiss Table maps, os.Root, generic type aliases. Available in Flox catalog as `go` |
| Go 1.25.x | Released Aug 2025 | GOMAXPROCS cgroup awareness, experimental JSON v2, DWARF 5 debug info. flox-bwrap's go.mod says `go 1.25.5` |
| Go 1.26.x | Released ~Feb 2026 | Latest stable. May not yet be in Flox catalog |

**Recommendation:** Set `go 1.24` in go.mod for maximum compatibility. The binary uses only stdlib features available since Go 1.13+ (flag, os, syscall, etc.). No features from 1.24+ are required but 1.24 is the safe baseline for Flox builds.

## Sources

- [Go 1.24 Release Notes](https://go.dev/doc/go1.24) -- Go version features and release date (HIGH confidence)
- [Go 1.25 Release Notes](https://go.dev/doc/go1.25) -- Go 1.25 features (HIGH confidence)
- [Go Release History](https://go.dev/doc/devel/release) -- Version timeline (HIGH confidence)
- [Flox Build and Publish Tutorial](https://flox.dev/docs/tutorials/build-and-publish/) -- flox build/publish workflow (HIGH confidence)
- [Flox Go Language Guide](https://flox.dev/docs/languages/go/) -- Go + Flox integration (HIGH confidence)
- [Flox Nix Expression Builds](https://flox.dev/docs/concepts/nix-expression-builds/) -- .flox/pkgs/ convention (HIGH confidence)
- [Flox Publishing Concepts](https://flox.dev/docs/concepts/publishing/) -- Publish requirements and commands (HIGH confidence)
- [nixpkgs Go documentation](https://ryantm.github.io/nixpkgs/languages-frameworks/go/) -- buildGoModule reference (HIGH confidence)
- [flox-bwrap source](https://github.com/devusb/flox-bwrap) -- Reference implementation, inspected directly (HIGH confidence)
- [HackTricks macOS Sandbox](https://angelica.gitbook.io/hacktricks/macos-hardening/macos-security-and-privilege-escalation/macos-security-protections/macos-sandbox) -- SBPL documentation (MEDIUM confidence, community-sourced)
- [sandbox-exec deprecation discussion](https://github.com/openai/codex/issues/215) -- Deprecation status context (MEDIUM confidence)
- [Apple Containerization WWDC 2025](https://appleinsider.com/articles/25/06/09/sorry-docker-macos-26-adds-native-support-for-linux-containers) -- macOS 26 containers (HIGH confidence, multiple sources)
- [BurntSushi/toml](https://github.com/BurntSushi/toml) -- Go TOML library (not used, but evaluated) (HIGH confidence)
- [pelletier/go-toml](https://github.com/pelletier/go-toml) -- Go TOML library (not used, but evaluated) (HIGH confidence)
