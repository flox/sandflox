# Phase 5: Subcommands - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

sandflox gains three subcommands that let users inspect and control sandbox behavior without modifying policy.toml: `validate` (dry-run policy inspection), `status` (live enforcement state from cache), and `elevate` (re-exec an existing flox session under sandbox-exec). These are user-facing CLI commands that follow existing diagnostic patterns.

</domain>

<decisions>
## Implementation Decisions

### CLI Routing & Subcommand Design
- Positional first-arg routing: check `os.Args[1]` against known subcommands (`validate`, `status`, `elevate`) before flag parsing. Matches Go CLI conventions (`go build`, `flox activate`)
- Both flag positions work: `sandflox --debug validate` and `sandflox validate --debug` are equivalent. Parse global flags, strip subcommand, re-parse remainder
- Unknown first args are NOT treated as subcommands: they route to the default exec pipeline (backward-compatible with `sandflox -- CMD` behavior)
- No subcommand-specific flags in v1: global flags (`--debug`, `--profile`, `--policy`, `--net`, `--requisites`) apply to all subcommands

### Output Format & Verbosity
- `validate` and `status` use plain text with `[sandflox]` prefix, matching existing diagnostic output style (Phase 1 CORE-07)
- `validate` summary: profile, network mode, filesystem mode, tool count from requisites, denied path count. `--debug` adds full path lists and SBPL rule count (reuses `emitDiagnostics` logic)
- `status` reads from cache files (`config.json`, `net-mode.txt`, `active-profile.txt`) — shows "live" cached state
- `status` outside a sandbox: error "Not in a sandflox session — no cached state found. Run `sandflox` first." Exit 1

### Elevate Mechanics
- Detect existing sandbox via `SANDFLOX_ENABLED=1` env var (already set by entrypoint.sh from Phase 3). If present → print "already sandboxed" and exit 0
- Detect flox session via `FLOX_ENV` env var. If missing → error "Not in a flox session. Run `flox activate` first." Exit 1
- `elevate` accepts the same flags as default mode: `--profile`, `--policy`, `--net`, `--debug` all work. Runs full pipeline (parse policy, resolve config, write cache, generate artifacts) but skips `flox activate` in argv
- Re-exec: `sandbox-exec -f <sbpl> -D ... bash --rcfile <entrypoint> -i` — wraps current shell under sandbox-exec. Flox activate skipped since already inside it. Uses `syscall.Exec` for clean PID replacement

### Claude's Discretion
- Internal code organization (new files vs extending main.go) at Claude's discretion
- Error message phrasing within `[sandflox]` convention
- Test strategy for subcommands (unit vs integration split)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `main.go` — current pipeline: parse flags → find policy → resolve config → write cache → generate shell → emit diagnostics → exec. Subcommands can intercept after flag parsing
- `cli.go` — `ParseFlags` using `flag.FlagSet` with `ContinueOnError`. Subcommand routing inserts before this
- `config.go` — `ResolveConfig`, `ParseRequisites` — reusable by validate for policy inspection
- `emitDiagnostics` in `main.go` — validate can share this logic for consistent output
- `exec_darwin.go` — `buildSandboxExecArgv` — elevate needs a variant that skips `flox activate`
- `cache.go` — `WriteCache` writes config.json, path lists — status reads these back
- `env.go` — `BuildSanitizedEnv` — elevate reuses for environment scrubbing

### Established Patterns
- Package-level `stderr io.Writer` for testable output
- `[sandflox]` prefixed messages to stderr
- `syscall.Exec` for process replacement (no child processes)
- Build tags for platform-specific code (`exec_darwin.go` / `exec_other.go`)
- Table-driven subtests and golden-file comparisons in tests

### Integration Points
- `main()` in `main.go` — subcommand routing inserts at the top, before step 1
- `CLIFlags` / `ParseFlags` — may need modification to handle subcommand extraction
- `exec_darwin.go` — elevate needs a `buildElevateArgv` or a modified `buildSandboxExecArgv` that omits `flox activate`
- `exec_other.go` — elevate on non-darwin needs a stub (warn: "elevate requires macOS sandbox-exec")

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond success criteria — open to standard approaches following codebase conventions.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
