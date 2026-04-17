# Phase 4: Security Hardening - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — discuss skipped)

<domain>
## Phase Boundary

sandflox scrubs the environment before sandbox entry so sensitive credentials and configuration do not leak into the agent's execution context. This phase adds env-var sanitization (blocklist/allowlist), Python safety flags (PYTHONDONTWRITEBYTECODE, ensurepip blocking), and ensures essential vars ($HOME, $TERM, $USER, $SHELL) pass through.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions. Key constraints from prior phases:
- Zero external Go deps (stdlib only)
- Platform split via build tags (darwin/other)
- ResolvedConfig drives all generated artifacts
- Env scrubbing happens before exec into sandbox-exec (pre-sandbox entry)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `main.go` — current execution pipeline: parse policy -> resolve config -> write cache -> write shell artifacts -> exec
- `exec_darwin.go` — `execWithKernelEnforcement` builds sandbox-exec argv and calls `syscall.Exec`
- `config.go` / `ResolvedConfig` — carries all policy-derived state
- `templates/entrypoint.sh.tmpl` — already sets `PYTHONDONTWRITEBYTECODE=1` and `PYTHON_NOPIP=1`

### Established Patterns
- Generated artifacts live in `.flox/cache/sandflox/`
- Shell-tier enforcement via Go template rendering
- Build tags for platform-specific code
- Unit tests use golden-file comparisons and table-driven subtests

### Integration Points
- Env scrubbing must happen in `main.go` or `exec_darwin.go` before `syscall.Exec`
- May need to modify the environment passed to `syscall.Exec` (currently inherits parent env)
- `entrypoint.sh.tmpl` already handles some Python env vars — coordinate to avoid duplication

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase. Refer to ROADMAP phase description and success criteria (SEC-01, SEC-02, SEC-03).

</specifics>

<deferred>
## Deferred Ideas

None — infrastructure phase.

</deferred>
