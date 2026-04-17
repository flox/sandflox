# Milestones

## v1.0 sandflox Go Binary (Shipped: 2026-04-17)

**Phases completed:** 6 phases, 14 plans, 21 tasks

**Key accomplishments:**

- Zero-dependency Go module with custom TOML subset parser validating policy.toml v2 schema (14 passing tests)
- Full pipeline from CLI flags through profile resolution, cache artifact writing, and syscall.Exec into flox activate with 35 new tests
- Nix buildGoModule expression with hermetic fileset source selection, verified flox build producing static Mach-O binary that parses policy.toml
- One-liner:
- One-liner:
- 5 subprocess-based integration tests prove kernel-level enforcement via real `sandbox-exec` invocations: write-blocking, localhost semantics, unix-socket allow, denied-path denial, and full-binary wrap -- manual TTY verification skipped per user decision.
- Go text/template generators for entrypoint.sh (PATH wipe + armor + breadcrumbs), fs-filter.sh (write-command wrappers with improved prefix matching), and usercustomize.py (ensurepip block + builtins.open wrapper) -- all 11 unit tests pass, zero third-party deps
- Wired WriteShellArtifacts into main.go and rewired buildSandboxExecArgv for D-01 (bash --rcfile entrypoint.sh -i) and D-02 (bash -c 'source entrypoint.sh && exec "$@"') dispatch -- all five Phase 2 argv tests updated, net-blocked.flag toggle test added, zero regressions
- 9 subprocess integration tests covering SHELL-01 through SHELL-08 via real sandflox binary + flox activate + sandbox-exec -- 6 PASS / 3 SKIP in dev env, plus 2 template bug fixes (export -f and bash builtin) discovered during testing
- Allowlist-based env filtering with 20+ blocked credential prefixes, policy-driven passthrough, and forced Python safety vars
- BuildSanitizedEnv wired into both exec paths with 4 integration tests proving credentials scrubbed, essentials preserved, and Python safety flags forced in real sandbox
- Subcommand routing with extractSubcommand + validate (policy dry-run) and status (cached state reader) handlers using WithExitCode testable pattern
- `sandflox elevate` re-execs existing flox sessions under sandbox-exec with re-entry detection, flox session detection, and 13-element argv without flox activate
- sandflox published to FloxHub via `flox publish` and verified installable via `flox install 8BitTacoSupreme/sandflox` with functional --help and all subcommands

---
