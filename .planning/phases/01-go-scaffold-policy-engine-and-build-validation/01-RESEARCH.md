# Phase 1: Go Scaffold, Policy Engine, and Build Validation - Research

**Researched:** 2026-04-15
**Domain:** Go binary scaffold, TOML subset parsing, Nix buildGoModule, Flox build system
**Confidence:** HIGH

## Summary

Phase 1 builds the foundation: a zero-dependency Go binary that parses `policy.toml`, resolves profiles with CLI flag overrides, writes resolved config to cache, emits diagnostics, and builds via `flox build` using a Nix expression. The binary then execs into `flox activate` (without sandbox-exec wrapping -- that is Phase 2).

The existing Bash+Python implementation (501 lines in `sandflox`, 418 lines in `manifest.toml`) provides an exact behavioral specification. The Go binary replicates the policy parsing, profile resolution, path resolution, and cache artifact generation -- but eliminates the Python dependency, the duplicated TOML parser, and the six separate `python3` invocations. The Go binary handles everything in a single process with zero startup overhead.

The Nix build uses `buildGoModule` with `vendorHash = null` (no external Go deps means no vendor directory needed). The Flox manifest becomes minimal: just `go` in `[install]` for development, with a `.flox/pkgs/sandflox.nix` expression using `lib.fileset.toSource` for hermetic source selection. Go 1.26.1 is available in the Flox catalog; Go 1.26.0 is currently installed on this machine.

**Primary recommendation:** Use flat `package main` layout at project root (matching flox-bwrap pattern), custom ~200-line TOML subset parser in Go stdlib, Go `flag` package for CLI with `--` separator handling, and `syscall.Exec` for clean process replacement into `flox activate`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Custom Go subset parser supporting only what policy.toml v2 uses: sections, dotted sections (`[profiles.minimal]`), string values, booleans, string arrays, and comments. Reject unsupported TOML features with clear errors. Target ~200 lines of Go.
- **D-02:** Strict validation of all parsed values against known enums: network mode (`blocked`/`unrestricted`), filesystem mode (`permissive`/`workspace`/`strict`), policy version (`"2"`). Reject unknown values with `[sandflox] ERROR:` identifying the bad value and its location.
- **D-03:** Hard error if `meta.version` is not `"2"`. No warn-and-continue. Future policy format changes get a clear "unsupported policy version" error rather than silent misparse.
- **D-04:** Flat main package -- all `.go` files in project root under `package main`. Matches flox-bwrap pattern. Files like `main.go`, `policy.go`, `sbpl.go`, `shell.go`, `cli.go`. No `cmd/` or `internal/` overhead.
- **D-05:** Go source files at project root alongside existing bash scripts, policy.toml, requisites files, and test scripts. `go.mod` at root. Nix build expression picks `.go` files from root.
- **D-06:** Running `sandflox` in Phase 1 does: parse policy.toml -> resolve profile (env var > policy > default) -> merge CLI flag overrides -> write cache artifacts to `.flox/cache/sandflox/` -> emit `[sandflox]` diagnostics to stderr -> exec `flox activate`. Phase 2 inserts `sandbox-exec` before the final exec.
- **D-07:** Both interactive (`sandflox` -> `flox activate`) and non-interactive (`sandflox -- CMD` -> `flox activate -- CMD`) modes implemented in Phase 1. The mode split lives in arg parsing, which is Phase 1 scope.
- **D-08:** Project-dir assumption for Flox environment discovery. The binary assumes it's running from (or is pointed to via `--policy`) a directory with `policy.toml` and `.flox/`. Reads `$FLOX_ENV` from environment or resolves from `.flox/`.
- **D-09:** The overall flow mirrors flox-bwrap's architecture: resolve environment -> read policy/metadata -> build sandbox spec -> exec sandbox wrapping flox activate. Phase 1 covers the first half (resolve + read + cache). Phase 2 adds the sandbox wrapping.
- **D-10:** Rename existing bash script to `sandflox.bash` (preserved as reference artifact). Go binary builds as `sandflox`. Clean break from Phase 1 onward -- the Go binary IS the product.
- **D-11:** Replace the 418-line `manifest.toml` with a minimal build manifest: just `go` in `[install]`, no hooks or profile scripts (per DIST-04). Old manifest preserved as `manifest.toml.v2-bash` for reference. The Go binary generates enforcement artifacts itself.
- **D-12:** Preserve existing bash test scripts as reference (for behavior documentation). Write Go test files (`policy_test.go`, etc.) that verify the same behaviors. The bash tests document expected behavior; Go tests replicate it.

### Claude's Discretion
- CLI flag parsing implementation details (Go `flag` package usage, flag naming)
- Cache file format and layout (JSON, text files, directory structure in `.flox/cache/sandflox/`)
- Diagnostic message formatting details beyond the `[sandflox]` prefix convention
- Go file organization within the flat package (which types/functions go in which `.go` file)
- Nix expression details for `buildGoModule` (vendorHash, ldflags, fileset selection)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CORE-01 | Single Go binary with zero external dependencies (stdlib only) | `go.mod` with no `require` directives; `vendorHash = null` in Nix; Go `flag`, `os`, `path/filepath`, `encoding/json`, `strings`, `bufio`, `fmt`, `syscall` packages only |
| CORE-02 | Parse policy.toml v2 schema using custom Go TOML subset parser | Custom ~200-line parser for sections, dotted sections, strings, booleans, string arrays, comments; see Architecture Patterns section |
| CORE-03 | Profile resolution via precedence: `$SANDFLOX_PROFILE` > `policy.toml [meta] profile` > `"default"` | Direct port of existing Python logic at `sandflox` lines 104-107; Go `os.Getenv` + map lookups |
| CORE-04 | Merge profile overrides with top-level `[network]` and `[filesystem]` settings | Profile's `network`/`filesystem` keys override top-level `.mode`; requisites file from profile; see Code Examples |
| CORE-05 | CLI flags `--net`, `--profile`, `--policy`, `--debug`, `--requisites` override policy values | Go `flag` package; `--` separator natively supported; flags applied after profile resolution |
| CORE-06 | Write resolved config, path lists, generated artifacts to `.flox/cache/sandflox/` | 10 cache files; see Cache Artifact Layout; `os.MkdirAll`, `os.WriteFile` |
| CORE-07 | Emit `[sandflox]` prefixed diagnostics to stderr | `fmt.Fprintf(os.Stderr, "[sandflox] ...")` for all diagnostics; `--debug` flag gates verbose output |
| DIST-01 | Build via `flox build` using `.flox/pkgs/sandflox.nix` with `buildGoModule` and `vendorHash = null` | Nix expression with `lib.fileset.toSource` for hermetic source; see Standard Stack / Nix Expression |
| DIST-04 | Minimal build manifest -- only `go` in `[install]`, no hooks or profile scripts | New `manifest.toml` replaces 418-line original; ~15 lines total; old preserved as `manifest.toml.v2-bash` |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.26.1 (Flox catalog) / 1.26.0 (installed) | Language runtime and stdlib | Zero external deps constraint (CORE-01); `go.mod` declares `go 1.22` or later for broad compat |
| `flag` (stdlib) | -- | CLI flag parsing | Native `--` separator support; no external dep needed |
| `encoding/json` (stdlib) | -- | Serialize resolved config to `config.json` | Standard JSON output format matching existing cache layout |
| `os` / `path/filepath` (stdlib) | -- | File I/O, path resolution, env vars | Cross-platform path handling; `filepath.Abs`, `filepath.Join` |
| `syscall` (stdlib) | -- | `syscall.Exec` for process replacement | Clean exec into `flox activate` with no child process |
| `bufio` / `strings` (stdlib) | -- | TOML parser internals, requisites file parsing | Line-by-line file processing |
| Nix `buildGoModule` | nixpkgs (via Flox) | Nix build function for Go projects | Standard nixpkgs pattern for Go; `vendorHash = null` for zero-dep projects |
| Nix `lib.fileset.toSource` | nixpkgs (via Flox) | Hermetic source filtering | Includes only `.go`, `go.mod`, `policy.toml`, `requisites*.txt` -- rebuild only when relevant files change |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `fmt` (stdlib) | -- | Formatted output to stderr | All `[sandflox]` diagnostic messages |
| `os/exec` (stdlib) | -- | Locate `flox` binary | `exec.LookPath("flox")` for preflight check |
| `testing` (stdlib) | -- | Go test framework | `policy_test.go`, `cli_test.go` unit tests |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Custom TOML parser | `github.com/BurntSushi/toml` | Adds external dependency, violating CORE-01; full TOML spec is overkill for policy.toml's 43-line subset |
| `flag` (stdlib) | `github.com/spf13/pflag` | POSIX-style flags; violates CORE-01; `flag` handles `--` natively |
| `encoding/json` for config output | Raw text files only | JSON config is the established format from existing implementation; keep for compatibility |

**Installation:**
```bash
# Development environment (manifest.toml [install]):
go.pkg-path = "go"

# No Go dependencies to install -- stdlib only
# go.mod has zero require directives
```

**Version verification:**
- Go 1.26.1 available in Flox catalog (`flox show go` confirms `go@1.26.1` as latest)
- Go 1.26.0 currently installed on dev machine
- `go.mod` should declare minimum `go 1.22` for broad compatibility (all stdlib APIs used are stable since Go 1.0+)

## Architecture Patterns

### Recommended Project Structure (Phase 1 files)
```
sandflox/
├── main.go                     # Entry point: CLI parsing, orchestration, exec
├── policy.go                   # TOML parser, policy types, validation
├── policy_test.go              # TOML parser + policy resolution tests
├── config.go                   # Profile resolution, merge logic, path resolution
├── config_test.go              # Profile merge + path resolution tests
├── cache.go                    # Cache artifact writer (all 10 files)
├── cli.go                      # Flag definitions, --debug, --profile, --net, --policy
├── cli_test.go                 # CLI flag override tests
├── go.mod                      # Module declaration (zero require directives)
├── .flox/
│   ├── env/
│   │   ├── manifest.toml       # MINIMAL: just go in [install] (DIST-04)
│   │   └── manifest.lock       # Auto-generated by Flox
│   └── pkgs/
│       └── sandflox.nix        # buildGoModule expression (DIST-01)
├── sandflox.bash               # Renamed from sandflox (reference artifact)
├── manifest.toml.v2-bash       # Renamed from .flox/env/manifest.toml (reference)
├── policy.toml                 # Unchanged -- the spec the parser targets
├── requisites.txt              # Unchanged
├── requisites-minimal.txt      # Unchanged
├── requisites-full.txt         # Unchanged
├── test-policy.sh              # Preserved as behavior reference
├── test-sandbox.sh             # Preserved as behavior reference
└── verify-sandbox.sh           # Preserved as behavior reference
```

### Pattern 1: Custom TOML Subset Parser
**What:** A ~200-line Go parser that handles only the TOML features `policy.toml` uses: `[sections]`, `[dotted.sections]`, `key = "string"`, `key = true/false`, `key = ["array", "of", "strings"]`, and `# comments`. Rejects anything else with a clear error.
**When to use:** Parsing `policy.toml` -- the only TOML file the binary reads.
**Example:**
```go
// policy.go -- TOML subset parser

type Policy struct {
    Meta       MetaSection
    Network    NetworkSection
    Filesystem FilesystemSection
    Profiles   map[string]ProfileSection
}

type MetaSection struct {
    Version string
    Profile string
}

type NetworkSection struct {
    Mode           string // "blocked" | "unrestricted"
    AllowLocalhost bool
}

type FilesystemSection struct {
    Mode     string   // "permissive" | "workspace" | "strict"
    Writable []string
    ReadOnly []string
    Denied   []string
}

type ProfileSection struct {
    Requisites string // filename
    Network    string // overrides NetworkSection.Mode
    Filesystem string // overrides FilesystemSection.Mode
}

// ParsePolicy reads policy.toml and returns a validated Policy.
// Returns an error with line number and context for any parse failure.
func ParsePolicy(path string) (*Policy, error) {
    // 1. Read file line by line
    // 2. Track current section path (e.g., ["profiles", "minimal"])
    // 3. For each line:
    //    - Skip blank/comment lines
    //    - Match [section] or [dotted.section] headers
    //    - Match key = value pairs
    //    - Parse value as string, bool, or string array
    //    - Reject unknown TOML features (inline tables, multiline strings, etc.)
    // 4. Map parsed key-value pairs into typed struct fields
    // 5. Validate: meta.version == "2", enum values, etc.
    return nil, nil
}
```

### Pattern 2: Profile Resolution with CLI Override
**What:** Three-level precedence (env var > policy file > default) followed by CLI flag override layer.
**When to use:** Determining the active configuration before writing cache.
**Example:**
```go
// config.go -- profile resolution

type ResolvedConfig struct {
    Profile        string
    NetMode        string
    FsMode         string
    Requisites     string
    AllowLocalhost bool
    Writable       []string
    ReadOnly       []string
    Denied         []string
}

func ResolveConfig(policy *Policy, flags *CLIFlags) *ResolvedConfig {
    // 1. Determine profile name: SANDFLOX_PROFILE env > policy.Meta.Profile > "default"
    profileName := os.Getenv("SANDFLOX_PROFILE")
    if profileName == "" {
        profileName = policy.Meta.Profile
    }
    if profileName == "" {
        profileName = "default"
    }

    // 2. Look up profile section
    profile := policy.Profiles[profileName]

    // 3. Merge: profile overrides top-level
    netMode := policy.Network.Mode
    if profile.Network != "" {
        netMode = profile.Network
    }
    fsMode := policy.Filesystem.Mode
    if profile.Filesystem != "" {
        fsMode = profile.Filesystem
    }

    // 4. CLI flags override everything
    if flags.Profile != "" {
        profileName = flags.Profile
        // re-lookup profile and re-merge...
    }
    if flags.Net {
        netMode = "unrestricted"
    }

    // 5. Return resolved config
    return &ResolvedConfig{...}
}
```

### Pattern 3: syscall.Exec for Process Replacement
**What:** Replace the Go process with `flox activate` using `syscall.Exec` -- no child process, no wait.
**When to use:** Final step after all config resolution and cache writing.
**Example:**
```go
// main.go -- exec into flox activate

func execFlox(args []string) error {
    floxPath, err := exec.LookPath("flox")
    if err != nil {
        return fmt.Errorf("[sandflox] ERROR: flox not found in PATH")
    }

    // Build argv: ["flox", "activate", ...userArgs]
    argv := []string{"flox", "activate"}
    argv = append(argv, args...)

    // Replace this process with flox
    return syscall.Exec(floxPath, argv, os.Environ())
}
```

### Pattern 4: Path Resolution (~ and . expansion)
**What:** Resolve `~` to `$HOME`, `.` and relative paths to project directory, preserve trailing `/` for directories, use `/private/tmp` on macOS.
**When to use:** When writing `writable-paths.txt`, `read-only-paths.txt`, `denied-paths.txt`.
**Example:**
```go
// config.go -- path resolution

func resolvePath(p string, projectDir string) string {
    isDir := strings.HasSuffix(p, "/")
    home, _ := os.UserHomeDir()

    // Expand ~ to home directory
    if strings.HasPrefix(p, "~/") {
        p = filepath.Join(home, p[2:])
    } else if !filepath.IsAbs(p) {
        // Relative paths resolve against project dir
        p = filepath.Join(projectDir, p)
    }

    p = filepath.Clean(p)

    // macOS: /tmp is a symlink to /private/tmp
    if runtime.GOOS == "darwin" && (p == "/tmp" || strings.HasPrefix(p, "/tmp/")) {
        p = "/private" + p
    }

    if isDir {
        p += "/"
    }
    return p
}
```

### Anti-Patterns to Avoid
- **Using `os/exec.Command` instead of `syscall.Exec`:** Creates a child process; sandflox should replace itself entirely. The Go process must not remain as a parent.
- **Importing any external Go module:** Violates CORE-01. The zero-dependency constraint is absolute. If you need TOML parsing, build it; don't import it.
- **Using `log` package for diagnostics:** The `[sandflox]` prefix convention predates Go; use direct `fmt.Fprintf(os.Stderr, ...)` to match the existing format exactly.
- **Hardcoding cache paths:** Use `projectDir + "/.flox/cache/sandflox/"` pattern. The project dir comes from the binary's location or `--policy` flag, never hardcoded.
- **Ignoring the existing Python parser's behavior:** The Go parser must produce byte-identical cache files. Use the existing `sandflox` bash script and `test-policy.sh` as the behavioral specification.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Full TOML spec parser | Complete TOML v1.0 parser | Custom subset parser (~200 lines) targeting only policy.toml features | policy.toml uses <10% of TOML spec; full parser would be 1000+ lines and still violate zero-dep constraint |
| JSON serialization | Custom JSON emitter | `encoding/json` stdlib | Standard, correct, handles escaping |
| Path operations | String concatenation for paths | `path/filepath` stdlib | Handles separators, cleaning, and abs resolution correctly |
| CLI flag parsing | Custom argv parser | `flag` stdlib | Handles `--` separator, error messages, usage text natively |
| Process exec | `os/exec.Command` with Wait | `syscall.Exec` | Clean process replacement, no zombie processes |

**Key insight:** The zero-external-dependency constraint does not mean building everything from scratch. Go's stdlib covers JSON, CLI flags, path manipulation, file I/O, and process exec. The only custom code needed is the TOML subset parser and the sandflox-specific business logic.

## Common Pitfalls

### Pitfall 1: vendorHash mismatch with zero dependencies
**What goes wrong:** Setting `vendorHash` to a hash value when there are no dependencies causes build failures. With zero external Go deps, there is no vendor directory and no go.sum to hash.
**Why it happens:** Nix expects to fetch and hash vendor dependencies; with none, the hash is meaningless.
**How to avoid:** Set `vendorHash = null;` in the Nix expression. This tells `buildGoModule` to skip dependency fetching entirely.
**Warning signs:** `flox build` fails with hash mismatch errors mentioning an empty vendor directory.

### Pitfall 2: TOML parser handling of inline comments
**What goes wrong:** The value `mode = "blocked"  # "unrestricted" | "blocked"` must parse as `"blocked"`, not `"blocked"  # "unrestricted" | "blocked"`.
**Why it happens:** Naive string splitting on `=` captures everything after the equals sign.
**How to avoid:** For quoted strings, find the closing quote and ignore everything after it. For bare values (booleans), strip inline comments (everything after `#` that is not inside a string).
**Warning signs:** Values contain `#` characters or trailing whitespace.

### Pitfall 3: Go `flag` package stops at first non-flag argument
**What goes wrong:** `sandflox --debug -- echo hello` must parse `--debug` as a flag and pass `echo hello` as the command. But `sandflox echo hello` (without `--`) would try to parse `echo` as a flag.
**Why it happens:** Go's `flag` package stops parsing at the first non-flag argument OR at `--`. After `flag.Parse()`, remaining args are in `flag.Args()`.
**How to avoid:** Design the CLI so all sandflox flags come before `--`, and everything after `--` is the user command. The `flag` package handles this natively -- `flag.Args()` returns everything after `--`.
**Warning signs:** Flag parsing errors when passing commands through to flox.

### Pitfall 4: /tmp vs /private/tmp on macOS
**What goes wrong:** Writing `/tmp` to path lists, but macOS `/tmp` is a symlink to `/private/tmp`. SBPL rules using `/tmp` won't match actual file operations using `/private/tmp`.
**Why it happens:** macOS has this symlink for historical reasons; `filepath.EvalSymlinks("/tmp")` returns `/private/tmp`.
**How to avoid:** In path resolution, detect macOS (`runtime.GOOS == "darwin"`) and canonicalize `/tmp` to `/private/tmp`. The existing Python implementation does this.
**Warning signs:** SBPL rules that reference `/tmp` fail to allow writes that should succeed.

### Pitfall 5: Trailing slash semantics in path lists
**What goes wrong:** `denied = ["~/.ssh/"]` means "the entire directory tree", while `read-only = ["policy.toml"]` means "this specific file". SBPL uses `subpath` for directories and `literal` for files.
**Why it happens:** The trailing `/` convention is a sandflox policy convention, not a filesystem convention.
**How to avoid:** Preserve trailing `/` through path resolution. After `filepath.Clean` (which strips trailing slashes), re-append `/` if the original path had one.
**Warning signs:** Denied directories are only partially denied; individual files within them are still accessible.

### Pitfall 6: Nix build source filtering must include non-Go files
**What goes wrong:** Using `lib.fileset.toSource` to include only `.go` files, but the binary also needs `policy.toml` and `requisites*.txt` at test time. More critically, `go.mod` must be included for `buildGoModule`.
**Why it happens:** Over-aggressive source filtering in the Nix expression.
**How to avoid:** The fileset must include `*.go`, `go.mod`, and any test fixture files. For the Nix build, `policy.toml` and `requisites*.txt` are NOT needed in the build (they're runtime config), but they ARE needed for `go test` if tests reference them. Embed test fixtures directly in Go test files using string literals instead.
**Warning signs:** `flox build` fails with "go.mod not found" or test failures about missing fixture files.

### Pitfall 7: manifest.toml replacement breaks existing flox environment
**What goes wrong:** Replacing the 418-line `manifest.toml` with a minimal one causes `flox activate` to fail because the lockfile references packages that no longer exist in the manifest.
**Why it happens:** `manifest.lock` is tied to the manifest's `[install]` section.
**How to avoid:** After replacing `manifest.toml`, delete `manifest.lock` and run `flox install` or `flox activate` to regenerate it. Alternatively, `flox lock` to force a new lockfile.
**Warning signs:** `flox activate` errors about missing packages or hash mismatches.

### Pitfall 8: go.mod file must be git-tracked for flox build
**What goes wrong:** `flox build` using Nix expressions requires all source files to be tracked by git. If `go.mod` or `.go` files are not `git add`ed, the Nix build silently excludes them.
**Why it happens:** Nix's source filtering respects `.gitignore` and only includes git-tracked files when using local source paths.
**How to avoid:** Run `git add go.mod *.go .flox/pkgs/sandflox.nix` before `flox build`. All Nix expression files and Go source files must be at least staged.
**Warning signs:** `flox build` fails with mysterious "file not found" errors or empty source directory.

## Code Examples

Verified patterns from official sources and existing implementation:

### Cache Artifact Layout (matching existing implementation)
```go
// cache.go -- write all cache artifacts

func WriteCache(cacheDir string, config *ResolvedConfig) error {
    if err := os.MkdirAll(cacheDir, 0755); err != nil {
        return fmt.Errorf("[sandflox] ERROR: cannot create cache dir: %w", err)
    }

    // Individual text files (matching existing cache layout)
    files := map[string]string{
        "net-mode.txt":        config.NetMode + "\n",
        "fs-mode.txt":         config.FsMode + "\n",
        "active-profile.txt":  config.Profile + "\n",
    }
    for name, content := range files {
        path := filepath.Join(cacheDir, name)
        if err := os.WriteFile(path, []byte(content), 0644); err != nil {
            return fmt.Errorf("[sandflox] ERROR: cannot write %s: %w", name, err)
        }
    }

    // Net-blocked flag (presence = blocked)
    flagPath := filepath.Join(cacheDir, "net-blocked.flag")
    if config.NetMode == "blocked" {
        if err := os.WriteFile(flagPath, []byte("1\n"), 0644); err != nil {
            return err
        }
    } else {
        os.Remove(flagPath) // clean up if mode changed
    }

    // Path lists
    writePathList(filepath.Join(cacheDir, "writable-paths.txt"), config.Writable)
    writePathList(filepath.Join(cacheDir, "read-only-paths.txt"), config.ReadOnly)
    writePathList(filepath.Join(cacheDir, "denied-paths.txt"), config.Denied)

    // config.json (full resolved config as JSON)
    jsonData, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return fmt.Errorf("[sandflox] ERROR: cannot marshal config: %w", err)
    }
    return os.WriteFile(filepath.Join(cacheDir, "config.json"), jsonData, 0644)
}

func writePathList(path string, paths []string) error {
    var buf strings.Builder
    for _, p := range paths {
        buf.WriteString(p)
        buf.WriteByte('\n')
    }
    return os.WriteFile(path, []byte(buf.String()), 0644)
}
```

### Requisites File Parser
```go
// config.go -- parse requisites file (one tool name per line, # comments)

func ParseRequisites(path string) ([]string, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var tools []string
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        // Take first whitespace-delimited token (handles trailing comments)
        tool := strings.Fields(line)[0]
        tools = append(tools, tool)
    }
    return tools, scanner.Err()
}
```

### Diagnostic Output (matching existing format)
```go
// main.go -- stderr diagnostics

func emitDiagnostics(config *ResolvedConfig, debug bool) {
    // Always emit the summary line (matches existing bash output)
    fmt.Fprintf(os.Stderr, "[sandflox] Profile: %s | Network: %s | Filesystem: %s\n",
        config.Profile, config.NetMode, config.FsMode)

    if debug {
        fmt.Fprintf(os.Stderr, "[sandflox] Requisites: %s\n", config.Requisites)
        fmt.Fprintf(os.Stderr, "[sandflox] Allow localhost: %v\n", config.AllowLocalhost)
        fmt.Fprintf(os.Stderr, "[sandflox] Writable paths: %v\n", config.Writable)
        fmt.Fprintf(os.Stderr, "[sandflox] Denied paths: %v\n", config.Denied)
        fmt.Fprintf(os.Stderr, "[sandflox] Cache dir: %s\n", config.CacheDir)
    }
}
```

### Nix Build Expression
```nix
# .flox/pkgs/sandflox.nix
{ buildGoModule, lib }:

buildGoModule {
  pname = "sandflox";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../../.;
    fileset = lib.fileset.unions [
      (lib.fileset.fileFilter (file: lib.hasSuffix ".go" file.name) ../../.)
      ../../go.mod
    ];
  };

  vendorHash = null;  # zero external dependencies

  ldflags = [
    "-s" "-w"
    "-trimpath"
    "-X main.Version=0.1.0"
  ];

  meta = with lib; {
    description = "macOS-native sandbox for AI coding agents";
    license = licenses.mit;
    platforms = platforms.darwin;
  };
}
```

### Minimal Manifest (DIST-04)
```toml
# .flox/env/manifest.toml -- minimal build manifest
# The Go binary (sandflox) handles all enforcement.
# This manifest only provides the Go compiler for development.
schema-version = "1.10.0"

[install]
go.pkg-path = "go"

[vars]

[hook]

[profile]

[services]
[include]
[build]
[options]
```

### CLI Flag Definitions
```go
// cli.go -- flag definitions

type CLIFlags struct {
    Profile    string
    PolicyPath string
    Net        bool
    Debug      bool
    Requisites string
}

func ParseFlags(args []string) (*CLIFlags, []string) {
    flags := &CLIFlags{}
    fs := flag.NewFlagSet("sandflox", flag.ExitOnError)

    fs.StringVar(&flags.Profile, "profile", "", "Override active profile")
    fs.StringVar(&flags.PolicyPath, "policy", "", "Path to policy.toml")
    fs.BoolVar(&flags.Net, "net", false, "Override network to unrestricted")
    fs.BoolVar(&flags.Debug, "debug", false, "Emit verbose diagnostics")
    fs.StringVar(&flags.Requisites, "requisites", "", "Override requisites file")

    fs.Parse(args)

    // Everything after flags (or after --) is the user command
    return flags, fs.Args()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Bash + inline Python TOML parser | Go binary with custom TOML subset parser | Phase 1 (this phase) | Eliminates Python dependency, 6x startup overhead, duplicated parser |
| 418-line manifest.toml with hooks/profile | Minimal manifest + Go binary generates artifacts | Phase 1 (this phase) | Single source of truth for enforcement logic |
| `vendorSha256` in Nix | `vendorHash` in Nix | nixpkgs 2023 | Old attribute name deprecated; use `vendorHash` |
| `go mod vendor` + vendored tree | `vendorHash = null` for zero-dep projects | -- | No vendor directory needed when there are no dependencies |

**Deprecated/outdated:**
- `vendorSha256` in Nix `buildGoModule` -- replaced by `vendorHash`
- `sandbox-exec` on macOS -- deprecated by Apple but still functional through macOS 15.x (Sequoia). No replacement API available. Phase 2 concern, not Phase 1.

## Open Questions

1. **Go module name**
   - What we know: Module name should match project identity. Likely `github.com/jhogan/sandflox` or just `sandflox`.
   - What's unclear: Whether a simple module name like `sandflox` works with `buildGoModule` or if it needs a full path.
   - Recommendation: Use `sandflox` as the module name since this is a standalone binary with zero dependencies. No imports, no resolution needed. Verify with `go mod init sandflox && flox build`.

2. **`-trimpath` in ldflags vs CGO_ENABLED**
   - What we know: `-trimpath` should be a build flag (`-gcflags -trimpath` or `go build -trimpath`), not an ldflag. Also, `CGO_ENABLED=0` ensures a static binary.
   - What's unclear: Whether `buildGoModule` handles `-trimpath` as a top-level attribute or if it must be in `buildFlags`.
   - Recommendation: Use `buildFlags = [ "-trimpath" ];` and `CGO_ENABLED = "0";` in the Nix expression rather than putting `-trimpath` in `ldflags`.

3. **Requisites file copying to cache**
   - What we know: The existing implementation copies the active requisites file to `$cache/requisites.txt`. The Go binary should do the same.
   - What's unclear: Whether Phase 1 should also set up the symlink bin directory or defer that to Phase 3 (shell enforcement).
   - Recommendation: Phase 1 copies the requisites file to cache (CORE-06 scope). Symlink bin directory creation is Phase 3 (SHELL-02 scope).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build + development | Yes | 1.26.0 (installed) / 1.26.1 (catalog) | -- |
| Flox | Build, distribution, runtime | Yes | 1.11.2 | -- |
| sandbox-exec | Phase 2 (not Phase 1) | Yes | macOS built-in | -- |
| git | Source tracking for Nix builds | Yes | (system) | -- |
| Nix (via Flox) | `flox build` backend | Yes | (managed by Flox) | -- |

**Missing dependencies with no fallback:** None. All Phase 1 dependencies are available.

**Missing dependencies with fallback:** None.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + `go test` |
| Config file | None needed -- Go testing is built-in |
| Quick run command | `go test ./... -count=1 -short` |
| Full suite command | `go test ./... -count=1 -v` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CORE-01 | Zero external deps in go.mod | unit | `go mod tidy && test -z "$(grep 'require' go.mod)"` or verify in test | Wave 0 |
| CORE-02 | Parse policy.toml v2 schema | unit | `go test ./... -run TestParsePolicy -v` | Wave 0 |
| CORE-03 | Profile resolution precedence | unit | `go test ./... -run TestProfileResolution -v` | Wave 0 |
| CORE-04 | Profile merges with top-level settings | unit | `go test ./... -run TestProfileMerge -v` | Wave 0 |
| CORE-05 | CLI flags override policy values | unit | `go test ./... -run TestCLIOverride -v` | Wave 0 |
| CORE-06 | Cache artifacts written correctly | unit | `go test ./... -run TestCacheWrite -v` | Wave 0 |
| CORE-07 | Diagnostic output to stderr | unit | `go test ./... -run TestDiagnostics -v` | Wave 0 |
| DIST-01 | `flox build` produces binary | integration | `flox build && test -x result-sandflox/bin/sandflox` | Wave 0 |
| DIST-04 | Minimal manifest, no hooks | manual-only | Verify `manifest.toml` has no `[hook]` or `[profile]` content | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1 -v && flox build`
- **Phase gate:** Full suite green + `flox build` produces working binary before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `policy_test.go` -- TOML parser tests, validation tests, edge cases (covers CORE-02, CORE-03, CORE-04)
- [ ] `cli_test.go` -- CLI flag parsing, `--` handling, override precedence (covers CORE-05)
- [ ] `cache_test.go` -- Cache artifact output format verification (covers CORE-06)
- [ ] `go.mod` -- Module declaration with zero external dependencies (covers CORE-01)
- [ ] `.flox/pkgs/sandflox.nix` -- Nix build expression (covers DIST-01)

## Sources

### Primary (HIGH confidence)
- Existing `sandflox` bash script (501 lines) -- exact behavioral specification for Go rewrite
- Existing `policy.toml` (43 lines) -- the parser target and primary test fixture
- Existing `manifest.toml` (418 lines) -- reference for hook/profile logic being replaced
- Existing `test-policy.sh` and `test-sandbox.sh` -- expected behavior documentation
- Go stdlib documentation (`flag`, `os`, `encoding/json`, `syscall`) -- verified APIs
- `flox show go` -- confirmed Go 1.26.1 available in catalog
- `flox build --help` -- confirmed Nix expression build support via `.flox/pkgs/`
- [Flox Go language docs](https://flox.dev/docs/languages/go/) -- build patterns, `-trimpath` requirement
- [Flox Nix expression builds](https://flox.dev/docs/concepts/nix-expression-builds/) -- `.flox/pkgs/` conventions

### Secondary (MEDIUM confidence)
- [nixpkgs Go docs](https://ryantm.github.io/nixpkgs/languages-frameworks/go/) -- `buildGoModule` with `vendorHash = null`
- [lib.fileset.toSource](https://johns.codes/blog/efficient-nix-derivations-with-file-sets) -- file set patterns for Go source filtering
- [Go by Example: Exec'ing Processes](https://gobyexample.com/execing-processes) -- `syscall.Exec` pattern verification
- [Flox build examples](https://github.com/flox/flox-build-examples) -- `quotes-app-go` reference

### Tertiary (LOW confidence)
- flox-bwrap reference (mentioned in CONTEXT.md but GitHub URL returned 404; could not verify actual source code)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- Go stdlib APIs are stable and well-documented; Nix `buildGoModule` is the standard pattern
- Architecture: HIGH -- directly porting known-working Python logic to Go with locked decisions
- Pitfalls: HIGH -- most pitfalls identified from actual existing implementation behavior and known Nix/macOS quirks
- Nix expression: MEDIUM -- `lib.fileset.toSource` pattern verified but exact interaction with `buildGoModule` for local source not fully tested

**Research date:** 2026-04-15
**Valid until:** 2026-05-15 (stable domain; Go stdlib and Nix patterns change slowly)
