# sandflox — Development Context

## TESTPLAN Audit (2026-04-17)

Walked every flox-sbx test (13-28) against the Go implementation.
Findings organized by severity.

### Code Bugs

**B1 (CRITICAL): `sandflox elevate` can never fire from flox-sbx.**
The manifest sets `SANDFLOX_ENABLED=1` on `flox activate`, so the re-entry
check in `checkElevatePrereqs()` bails immediately with "Already sandboxed."
Tests 20-26 all fail. Fix: use a separate `SANDFLOX_SANDBOX` env var set
only at the `syscall.Exec` boundary.

**B2 (minor): `elevateExec` writes SBPL to `projectDir/.flox/cache/` but
`runElevateWithExitCode` writes other artifacts to `$FLOX_ENV_CACHE/`.**
Works by coincidence for project-local envs but diverges for remote ones.

### Test Bugs (commands that won't behave as expected)

- **T1**: `which` not in requisites — use `command -v` instead
- **T2**: `touch` not a wrapped command — use `mkdir` or `tee`
- **T3**: `echo bad > policy.toml` bypasses fs-filter (shell redirection) — use `tee`
- **T4**: `echo test > ./testfile` same issue (weak test)
- **T5**: `cp /etc/hosts /tmp/hosts-copy` succeeds at both tiers (`/tmp` is writable) — use a non-writable target
- **T6**: `touch .git/pwned` not wrapped (same as T2)
- **T7**: `.git/x` path resolution fails if `.git/` doesn't exist in consumer env

### Wording Issues

- **W1**: Test 14 message says "Environment is immutable.", not "exit 126"
- **W2**: Test 15 expected messages don't match actual fs-filter format
- **W3**: Test 16 Python message has trailing "policy"
- **W4**: Test 20 missing the diagnostics line that prints first

### Design Questions

- **D1**: Should `touch` be added to `WriteCmds` in `shell.go`?
- **D2**: Shell redirections (`>`, `>>`) are inherently unwrappable at the shell tier — document this explicitly
- **D3**: `.git/` path doesn't exist in the flox-sbx consumer env — either remove from consumer policy or note in tests
