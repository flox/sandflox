# Phase 1: Go Scaffold, Policy Engine, and Build Validation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 01-go-scaffold-policy-engine-and-build-validation
**Areas discussed:** TOML parser strategy, Go project layout, Phase 1 binary behavior, Coexistence strategy

---

## TOML Parser Strategy

### Q1: How much TOML should the Go parser support?

| Option | Description | Selected |
|--------|-------------|----------|
| Exact subset only | Support only what policy.toml v2 uses: sections, dotted sections, strings, booleans, string arrays, comments. Reject anything else with a clear error. ~200 lines of Go. | ✓ |
| Broad TOML subset | Support the above plus multiline strings, integers, and inline tables. More future-proof but adds complexity (~400 lines). | |
| You decide | Claude picks the right scope. | |

**User's choice:** Exact subset only (Recommended)
**Notes:** None

### Q2: How should the parser handle unknown or invalid values?

| Option | Description | Selected |
|--------|-------------|----------|
| Strict with clear errors | Validate all parsed values against known enums. Reject with [sandflox] ERROR identifying the bad value and line. | ✓ |
| Parse-only, validate later | Parser accepts any syntactically valid TOML subset. Validation happens in the config resolution step. | |
| You decide | Claude picks the validation approach. | |

**User's choice:** Strict with clear errors (Recommended)
**Notes:** None

### Q3: Should the parser support version negotiation for future policy formats?

| Option | Description | Selected |
|--------|-------------|----------|
| Check version = "2", reject others | Hard error if meta.version is not "2". Simple and safe. | ✓ |
| Warn on unknown versions, parse anyway | Print a warning but try to parse. More permissive. | |
| You decide | Claude picks the versioning strategy. | |

**User's choice:** Check version = "2", reject others (Recommended)
**Notes:** None

---

## Go Project Layout

### Q4: How should the Go source be organized?

| Option | Description | Selected |
|--------|-------------|----------|
| Flat main package | All .go files in root directory under package main. Matches flox-bwrap pattern. Files like main.go, policy.go, sbpl.go, shell.go, cli.go. | ✓ |
| cmd/ + internal/ packages | Standard Go project layout: cmd/sandflox/main.go, internal/policy/, internal/sbpl/, etc. | |
| You decide | Claude picks layout based on complexity growth. | |

**User's choice:** Flat main package (Recommended)
**Notes:** None

### Q5: Where should Go source files live relative to the existing bash scripts?

| Option | Description | Selected |
|--------|-------------|----------|
| Root alongside existing files | Go files at project root next to bash sandflox, policy.toml, test scripts. go.mod at root. | ✓ |
| Dedicated src/ or go/ subdirectory | Separate Go source from bash files. | |

**User's choice:** Root alongside existing files (Recommended)
**Notes:** None

---

## Phase 1 Binary Behavior

### Q6: What should running 'sandflox' do in Phase 1?

| Option | Description | Selected |
|--------|-------------|----------|
| Parse, cache, diagnose, then exec flox activate | Binary does Phase 1 work then falls through to exec flox activate without enforcement. Usable immediately. Phase 2 inserts sandbox-exec before the exec. | ✓ |
| Parse, cache, diagnose, then exit | Binary does Phase 1 work and exits. No flox activate. Essentially a dry-run tool until Phase 2. | |
| You decide | Claude picks the behavior. | |

**User's choice:** Parse, cache, diagnose, then exec flox activate (Recommended)
**Notes:** None

### Q7: Should the binary distinguish interactive vs non-interactive mode in Phase 1?

| Option | Description | Selected |
|--------|-------------|----------|
| Both modes in Phase 1 | Handle 'sandflox' (interactive) and 'sandflox -- CMD' (non-interactive) from the start. The mode split is in arg parsing. | ✓ |
| Interactive only, defer -- CMD | Only handle 'sandflox' in Phase 1. Add the -- CMD split later. | |
| You decide | Claude picks based on Phase 2 needs. | |

**User's choice:** Both modes in Phase 1
**Notes:** User provided important architectural context: flox-bwrap's flow is "call flox -> pull env -> read metadata -> build sandbox spec -> pass spec to sandbox and start it up. It runs flox activate inside the sandbox." sandflox should mirror this pattern.

### Q8: Should the Go binary discover the Flox environment at startup or assume project directory?

| Option | Description | Selected |
|--------|-------------|----------|
| Project-dir assumption | Assume sandflox runs from a directory with policy.toml and .flox/. Reads $FLOX_ENV from environment or resolves from .flox/. | ✓ |
| Explicit flox env resolution | Call out to 'flox' to resolve and activate the environment first (like flox-bwrap). | |
| You decide | Claude picks based on flox-bwrap patterns. | |

**User's choice:** Project-dir assumption (Recommended)
**Notes:** None

---

## Coexistence Strategy

### Q9: How should the Go binary coexist with the existing bash sandflox script?

| Option | Description | Selected |
|--------|-------------|----------|
| Rename bash script, Go binary takes 'sandflox' name | Move bash script to 'sandflox.bash'. Go binary builds as 'sandflox'. Clean break. | ✓ |
| Go binary builds as 'sandflox-go' until feature-complete | Keep bash 'sandflox' as working tool. Go binary as 'sandflox-go' during development. | |
| You decide | Claude picks based on development workflow. | |

**User's choice:** Rename bash script, Go binary takes 'sandflox' name (Recommended)
**Notes:** None

### Q10: What happens to the existing manifest.toml hooks and profile scripts?

| Option | Description | Selected |
|--------|-------------|----------|
| Strip to minimal build manifest in Phase 1 | Replace 418-line manifest.toml with minimal one: just 'go' in [install] (DIST-04). Old manifest preserved as 'manifest.toml.v2-bash'. | ✓ |
| Keep existing manifest, add Go build alongside | Keep current manifest with enforcement hooks. Add Go to [install]. | |
| You decide | Claude picks based on DIST-04 and Phase 2-3 needs. | |

**User's choice:** Strip to minimal build manifest in Phase 1 (Recommended)
**Notes:** None

### Q11: Should existing test suites be preserved or replaced?

| Option | Description | Selected |
|--------|-------------|----------|
| Preserve as reference, write Go tests | Keep bash test scripts as reference. Write Go test files that verify the same behaviors. | ✓ |
| Adapt bash tests to call Go binary | Modify existing bash test scripts to invoke the Go binary. | |
| You decide | Claude picks the testing strategy. | |

**User's choice:** Preserve as reference, write Go tests (Recommended)
**Notes:** None

---

## Claude's Discretion

- CLI flag parsing implementation details
- Cache file format and layout
- Diagnostic message formatting details
- Go file organization within the flat package
- Nix expression details for buildGoModule

## Deferred Ideas

None -- discussion stayed within phase scope
