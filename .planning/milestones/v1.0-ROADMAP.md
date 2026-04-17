# Roadmap: sandflox

## Overview

sandflox is a brownfield rewrite: a working 500-line Bash+Python prototype being replaced by a zero-dependency Go binary distributed as a Flox package. The roadmap follows the dependency chain -- policy parsing feeds kernel enforcement, kernel enforcement feeds shell enforcement, and distribution packages the result. The Nix build is validated in Phase 1 (not deferred) so build failures surface before 2000 lines of Go exist. Security hardening and subcommands follow the core enforcement layers as focused passes.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Go Scaffold, Policy Engine, and Build Validation** - Stand up the Go binary with TOML parsing, CLI flags, config resolution, and prove it builds via Nix
- [x] **Phase 2: Kernel Enforcement (SBPL + sandbox-exec)** - Generate SBPL profiles from policy and exec into sandbox-exec for filesystem/network isolation (completed 2026-04-16)
- [ ] **Phase 3: Shell Enforcement Artifacts** - Generate PATH wipe, requisites bin, function armor, fs-filter wrappers, Python enforcement, and breadcrumb cleanup
- [ ] **Phase 4: Security Hardening** - Scrub environment variables before sandbox entry, blocking sensitive vars and setting Python safety flags
- [ ] **Phase 5: Subcommands** - Add validate, status, and elevate subcommands for policy inspection and re-exec elevation
- [ ] **Phase 6: Distribution and Polish** - Publish to FloxHub, verify flox install, and finalize hermetic Nix build expression

## Phase Details

### Phase 1: Go Scaffold, Policy Engine, and Build Validation
**Goal**: A Go binary that parses policy.toml, resolves profiles with CLI flag overrides, caches resolved config, emits diagnostics, and builds successfully via `flox build`
**Depends on**: Nothing (first phase)
**Requirements**: CORE-01, CORE-02, CORE-03, CORE-04, CORE-05, CORE-06, CORE-07, DIST-01, DIST-04
**Success Criteria** (what must be TRUE):
  1. Running `flox build` produces a sandflox binary with zero external Go dependencies
  2. Running `sandflox --debug` with a valid policy.toml prints resolved profile, filesystem mode, and network mode to stderr with `[sandflox]` prefix
  3. Running `sandflox --profile minimal --net` correctly overrides policy.toml values (CLI flags win over file settings)
  4. Running sandflox with a malformed policy.toml produces a clear error message identifying the parse failure
  5. Resolved config, path lists, and generated artifacts are written to `.flox/cache/sandflox/`
**Plans**: 3 plans

Plans:
- [x] 01-01-PLAN.md -- Project scaffold, TOML subset parser, and policy validation
- [x] 01-02-PLAN.md -- Config resolution, CLI flags, cache writer, diagnostics, and main entry point
- [x] 01-03-PLAN.md -- Nix build expression and flox build validation

### Phase 2: Kernel Enforcement (SBPL + sandbox-exec)
**Goal**: sandflox wraps flox activate under sandbox-exec with a generated SBPL profile that enforces filesystem modes, network modes, and denied paths at the kernel level
**Depends on**: Phase 1
**Requirements**: KERN-01, KERN-02, KERN-03, KERN-04, KERN-05, KERN-06, KERN-07, KERN-08
**Success Criteria** (what must be TRUE):
  1. Running `sandflox` (no args) drops into an interactive sandboxed shell where writes outside the workspace are kernel-blocked
  2. Running `sandflox -- echo hello` executes the command under sandbox-exec and exits cleanly
  3. With `network.mode = "blocked"`, network requests (e.g., `curl`) fail at the kernel level, but localhost connections succeed when `allow-localhost = true`
  4. Denied paths listed in policy.toml are inaccessible (read or write) from within the sandbox
  5. The process tree shows clean exec replacement (no intermediate sandflox parent process)
**Plans**: 3 plans

Plans:
- [x] 02-01-PLAN.md -- SBPL generator (GenerateSBPL + WriteSBPL in sbpl.go with table-driven unit tests)
- [x] 02-02-PLAN.md -- sandbox-exec wrapper (exec_darwin.go + exec_other.go + main.go wiring with --debug SBPL diagnostic)
- [x] 02-03-PLAN.md -- Integration tests (real sandbox-exec subprocess) + manual verification checkpoint

### Phase 3: Shell Enforcement Artifacts
**Goal**: sandflox generates and applies all shell-tier enforcement -- PATH restriction, requisites filtering, function armor, filesystem write wrappers, Python enforcement, and breadcrumb cleanup -- so agents cannot reach tools or discover escape vectors
**Depends on**: Phase 1
**Requirements**: SHELL-01, SHELL-02, SHELL-03, SHELL-04, SHELL-05, SHELL-06, SHELL-07, SHELL-08
**Success Criteria** (what must be TRUE):
  1. Inside the sandbox, `which pip` (or any armored package manager) returns nothing, and attempting to run it prints `[sandflox] BLOCKED: ...`
  2. Inside the sandbox, `echo $PATH` shows only the requisites-filtered symlink bin directory (not the full `$FLOX_ENV/bin`)
  3. Running `cp /etc/hosts /tmp/stolen` from inside the sandbox is intercepted by fs-filter with a `[sandflox] BLOCKED:` message explaining the denial
  4. Python code running `open('/etc/passwd', 'w')` inside the sandbox is blocked by the usercustomize.py enforcement
  5. Environment variables `FLOX_ENV_PROJECT`, `FLOX_ENV_DIRS`, and `FLOX_PATH_PATCHED` are not visible inside the sandbox
**Plans**: 3 plans

Plans:
- [x] 03-01-PLAN.md -- Shell-tier generators: embedded templates (entrypoint.sh/fs-filter.sh/usercustomize.py) + shell.go (Generate* + WriteShellArtifacts + shellquote) + unit tests (SHELL-01..SHELL-08 at unit level)
- [x] 03-02-PLAN.md -- Runtime wiring: WriteShellArtifacts in main.go, buildSandboxExecArgv rewire for D-01/D-02 (bash --rcfile / bash -c source), update Phase 2 argv tests, add net-blocked.flag writer to cache.go
- [x] 03-03-PLAN.md -- Subprocess integration tests: shell_integration_test.go covering SHELL-01..SHELL-08 end-to-end under real flox + sandbox-exec + bash

### Phase 4: Security Hardening
**Goal**: sandflox scrubs the environment before sandbox entry so sensitive credentials and configuration do not leak into the agent's execution context
**Depends on**: Phase 2
**Requirements**: SEC-01, SEC-02, SEC-03
**Success Criteria** (what must be TRUE):
  1. Inside the sandbox, `echo $AWS_SECRET_ACCESS_KEY` and `echo $GITHUB_TOKEN` are empty even when set in the parent shell
  2. Inside the sandbox, `echo $HOME` and `echo $TERM` are correctly set (allowlisted vars pass through)
  3. Inside the sandbox, `python3 -c "import ensurepip"` fails and `PYTHONDONTWRITEBYTECODE` is set to `1`
**Plans**: 2 plans

Plans:
- [x] 04-01-PLAN.md -- Env sanitization engine (BuildSanitizedEnv + allowlist/blocklist constants) + policy [security] section + config EnvPassthrough + unit tests
- [x] 04-02-PLAN.md -- Wire into exec paths (exec_darwin.go, main.go, exec_other.go) + --debug env diagnostic + integration tests proving env scrubbing end-to-end

### Phase 5: Subcommands
**Goal**: Users can inspect policy without executing (validate), check enforcement state (status), and elevate an existing flox session into the sandbox (elevate)
**Depends on**: Phase 3
**Requirements**: CMD-01, CMD-02, CMD-03
**Success Criteria** (what must be TRUE):
  1. Running `sandflox validate` with a valid policy.toml prints what would be enforced (profile, modes, denied paths, tool count) without launching a sandbox
  2. Running `sandflox status` from inside a sandboxed session reports the active profile, blocked paths, allowed tools, and network mode from cached state
  3. Running `sandflox elevate` from within a plain `flox activate` session re-execs the shell under sandbox-exec, and running it again inside an already-sandboxed session prints a "already sandboxed" message instead of nesting
**Plans**: 2 plans

Plans:
- [x] 05-01-PLAN.md -- Subcommand routing (extractSubcommand + main.go rewire) + validate handler (CMD-01) + status handler with ReadCache (CMD-02)
- [x] 05-02-PLAN.md -- Elevate subcommand (CMD-03): buildElevateArgv, elevateExec, re-entry/session detection, platform stubs

### Phase 6: Distribution and Polish
**Goal**: sandflox is published to FloxHub and installable into any Flox environment with a hermetic, reproducible Nix build
**Depends on**: Phase 4, Phase 5
**Requirements**: DIST-02, DIST-03, DIST-05
**Success Criteria** (what must be TRUE):
  1. Running `flox publish` successfully uploads sandflox to FloxHub
  2. In a fresh Flox environment, running `flox install sandflox` makes the `sandflox` command available
  3. The Nix expression uses `lib.fileset.toSource` for hermetic source selection and `-trimpath` in build flags (no build path leaks in binary)
**Plans**: 1 plan

Plans:
- [x] 06-01-PLAN.md -- Fix env.json blocker, push to remote, build, publish to FloxHub, verify flox install

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6
Note: Phase 3 depends on Phase 1 (not Phase 2), so Phases 2 and 3 could theoretically run in parallel, but sequential execution is simpler.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Go Scaffold, Policy Engine, and Build Validation | 0/3 | Planning complete | - |
| 2. Kernel Enforcement (SBPL + sandbox-exec) | 3/3 | Complete   | 2026-04-16 |
| 3. Shell Enforcement Artifacts | 0/3 | Planning complete | - |
| 4. Security Hardening | 0/2 | Planning complete | - |
| 5. Subcommands | 0/2 | Planning complete | - |
| 6. Distribution and Polish | 0/1 | Planning complete | - |
