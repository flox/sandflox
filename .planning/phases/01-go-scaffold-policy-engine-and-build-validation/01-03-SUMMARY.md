---
phase: 01-go-scaffold-policy-engine-and-build-validation
plan: 03
subsystem: build
tags: [nix, flox-build, buildGoModule, fileset, distribution]

# Dependency graph
requires:
  - "01-01: Go module scaffold, go.mod, zero-dependency structure"
  - "01-02: All Go source files (config.go, cli.go, cache.go, main.go)"
provides:
  - "Nix build expression (.flox/pkgs/sandflox.nix) using buildGoModule"
  - "Verified flox build producing working Mach-O binary"
  - "Hermetic source selection via lib.fileset.toSource"
  - "Version injection via ldflags (-X main.Version=0.1.0)"
affects: [phase-06]

# Tech tracking
tech-stack:
  added: [nix-buildGoModule, lib-fileset-toSource]
  patterns: [hermetic-nix-build, cgo-disabled-static-binary, trimpath-reproducible-build]

key-files:
  created:
    - .flox/pkgs/sandflox.nix
    - .gitignore
  modified:
    - .flox/env/manifest.lock

key-decisions:
  - "vendorHash = null because zero external Go dependencies"
  - "lib.fileset.toSource includes only .go files and go.mod -- runtime config excluded from build inputs"
  - "CGO_ENABLED=0 via env attribute (not buildFlags) for static binary"
  - "-trimpath in buildFlags (not ldflags) for reproducible builds without local path leaks"
  - "doCheck = false since tests need runtime files not available in Nix sandbox"
  - "platforms = platforms.darwin since sandflox is macOS-only"

patterns-established:
  - "Nix expression in .flox/pkgs/ directory for flox build discovery"
  - "lib.fileset.toSource with fileFilter for Go-only hermetic source selection"
  - "result-* symlinks gitignored (Nix build output)"

requirements-completed: [DIST-01]

# Metrics
duration: 4min
completed: 2026-04-16
---

# Phase 01 Plan 03: Nix Build Expression and flox build Validation Summary

**Nix buildGoModule expression with hermetic fileset source selection, verified flox build producing static Mach-O binary that parses policy.toml**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-16T02:21:00Z
- **Completed:** 2026-04-16T02:26:03Z
- **Tasks:** 2 (1 auto + 1 checkpoint)
- **Files modified:** 3

## Accomplishments
- Nix build expression using buildGoModule with vendorHash = null (zero external deps verified)
- Hermetic source selection via lib.fileset.toSource includes only .go files and go.mod
- flox build produces a working static Mach-O binary at result-sandflox/bin/sandflox
- Binary correctly parses policy.toml, resolves profiles, writes cache artifacts, and emits [sandflox] diagnostics
- Human-verified: all Phase 1 success criteria confirmed (build, debug output, CLI overrides, error handling, cache writing)

## Task Commits

Each task was committed atomically:

1. **Task 1: Create Nix build expression and verify flox build** - `587060e` (feat)
2. **Task 2: Human verification of complete Phase 1 binary** - checkpoint approved (no commit needed)

## Files Created/Modified
- `.flox/pkgs/sandflox.nix` - Nix build expression with buildGoModule, fileset, ldflags version injection (34 lines)
- `.flox/env/manifest.lock` - Regenerated lockfile for current minimal manifest
- `.gitignore` - Added result-* pattern for Nix build output symlinks

## Decisions Made
- vendorHash = null confirmed correct for zero external Go dependencies (go.mod has no require directives)
- lib.fileset.toSource with fileFilter selects only .go files + go.mod -- policy.toml and requisites*.txt are runtime config, not build inputs
- CGO_ENABLED=0 set via `env.CGO_ENABLED` attribute (not in buildFlags) for clean Nix expression
- -trimpath placed in buildFlags (compiler flag) rather than ldflags (linker flag) for correct Go build behavior
- doCheck = false because Go tests reference policy.toml which is not available in the Nix build sandbox
- platforms restricted to darwin since sandflox targets macOS only

## Deviations from Plan

None -- plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None -- the Nix build expression is complete and produces a fully functional binary.

## Next Phase Readiness
- Phase 1 is fully complete: all 5 roadmap success criteria verified
  1. `flox build` produces binary with zero external Go deps
  2. `sandflox --debug` prints resolved profile, fs mode, net mode with [sandflox] prefix
  3. `sandflox --profile minimal --net` correctly overrides policy values
  4. Malformed policy.toml produces clear error message
  5. Resolved config and path lists written to .flox/cache/sandflox/
- Ready for Phase 2 (kernel enforcement) or Phase 3 (shell enforcement artifacts)
- Nix expression pattern established for Phase 6 distribution work

---
*Phase: 01-go-scaffold-policy-engine-and-build-validation*
*Completed: 2026-04-16*
