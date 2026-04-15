# Testing Patterns

**Analysis Date:** 2026-04-15

## Test Framework

**Runner:**
- Pure Bash -- no external test framework
- Custom pass/fail/skip counters with helper functions
- Exit code `1` on any failure, `0` on all-pass

**Assertion Library:**
- Hand-rolled helpers defined at the top of each test script
- Three test scripts with slightly different helper styles (see patterns below)

**Run Commands:**
```bash
# Full kernel + shell enforcement tests (v2 policy tests)
./sandflox -- bash test-policy.sh

# Legacy sandbox verification (shell enforcement only)
./sandflox -- bash test-sandbox.sh

# Original v1 verification (expects interactive activation)
flox activate -- bash verify-sandbox.sh

# Shell-only mode (kernel tests auto-skip)
flox activate -- bash test-policy.sh
```

## Test File Organization

**Location:**
- All test files live at the project root alongside `sandflox` and `policy.toml`
- No `tests/` directory -- flat layout

**Naming:**
- `test-*.sh`: Active test suites
- `verify-sandbox.sh`: Legacy verification script

**Files:**
- `test-policy.sh` (302 lines): Comprehensive v2 policy enforcement tests -- kernel, shell, profiles, backward compat
- `test-sandbox.sh` (95 lines): Shell enforcement tests -- PATH restriction, function armor, breadcrumbs, escape vectors
- `verify-sandbox.sh` (111 lines): Original v1 verification -- similar to `test-sandbox.sh` but with different helper style

## Test Structure

**Suite Organization (test-policy.sh):**
```bash
#!/usr/bin/env bash
set -uo pipefail

# Helper definitions
pass=0; fail=0; skip=0
ok()   { echo "  PASS: $1"; pass=$((pass + 1)); }
bad()  { echo "  FAIL: $1"; fail=$((fail + 1)); }
skp()  { echo "  SKIP: $1"; skip=$((skip + 1)); }

# Banner
echo "========================================"
echo " sandflox v2 — policy enforcement tests"
echo "========================================"

# Test sections with headers
echo "[Kernel/macOS] Kernel-level enforcement"
# ... tests using ok()/bad()/skp() ...

echo "[Shell] Shell-level enforcement (defense-in-depth)"
# ... tests ...

echo "[Profiles] Profile configuration"
# ... tests ...

echo "[Backward] Core sandbox enforcement (v1 compatibility)"
# ... tests ...

# Results summary
echo "========================================"
echo " Results: $pass passed, $fail failed, $skip skipped"
echo "========================================"
[ $fail -gt 0 ] && exit 1 || exit 0
```

**Suite Organization (verify-sandbox.sh -- alternate style):**
```bash
# Inverse assertion: command succeeding means FAIL
check() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "  FAIL: $desc"
    fail=$((fail + 1))
  else
    echo "  PASS: $desc"
    pass=$((pass + 1))
  fi
}

# Positive assertion: command succeeding means PASS
check_exists() {
  local desc="$1"; local cmd="$2"
  if command -v "$cmd" >/dev/null 2>&1; then
    echo "  PASS: $desc"
    pass=$((pass + 1))
  else
    echo "  FAIL: $desc"
    fail=$((fail + 1))
  fi
}
```

**Test Sections (labeled categories):**
- `[Kernel/macOS]` -- Kernel-level sandbox-exec enforcement
- `[Shell]` -- Shell-level defense-in-depth (fs-filter, curl removal, python write blocking)
- `[Profiles]` -- Profile switching (minimal/default/full)
- `[Backward]` -- v1 backward compatibility
- `[Layer 1]` -- PATH restriction (blocked and allowed tools)
- `[Layer 2]` -- Function armor
- `[Layer 3]` -- Breadcrumb cleanup
- `[Escape vector tests]` -- Agent escape prevention

## Test Patterns

**Pattern 1: Command presence/absence testing**
```bash
# Tool should NOT be in PATH
for cmd in flox nix nix-env brew pip pip3 npm docker; do
  if type -P "$cmd" >/dev/null 2>&1; then
    bad "$cmd binary found in PATH"
  else
    ok "$cmd not in PATH"
  fi
done

# Tool SHOULD be in PATH
for cmd in python3 git jq grep sed awk bash ls cat echo; do
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "$cmd available"
  else
    bad "$cmd not found"
  fi
done
```

**Pattern 2: Function armor exit code testing**
```bash
for cmd in flox nix brew pip pip3 npm cargo docker; do
  $cmd 2>/dev/null
  rc=$?
  if [ $rc -eq 126 ]; then
    ok "$cmd function armor returns 126"
  elif [ $rc -eq 127 ]; then
    ok "$cmd not found (127)"
  else
    bad "$cmd returned $rc (expected 126 or 127)"
  fi
done
```

**Pattern 3: Filesystem write enforcement testing**
```bash
# Write outside workspace should fail
if echo "test" > /etc/sandflox-test 2>/dev/null; then
  bad "sandbox-exec allows write outside workspace (/etc)"
  rm -f /etc/sandflox-test 2>/dev/null
else
  ok "sandbox-exec blocks write outside workspace (/etc)"
fi

# Write inside workspace should succeed
_sfx_ws_test="./sandflox-ws-test-$$"
if echo "test" > "$_sfx_ws_test" 2>/dev/null; then
  ok "sandbox-exec allows write within workspace"
  rm -f "$_sfx_ws_test" 2>/dev/null
else
  bad "sandbox-exec blocks write within workspace"
fi
```

**Pattern 4: Network enforcement testing**
```bash
if [ "$_sfx_net_mode" = "blocked" ]; then
  if curl -s --connect-timeout 2 https://example.com >/dev/null 2>&1; then
    bad "sandbox-exec allows network when mode=blocked"
  else
    ok "sandbox-exec blocks network when mode=blocked"
  fi
else
  skp "network mode is not blocked — skipping network kernel test"
fi
```

**Pattern 5: Python-level enforcement testing**
```bash
_sfx_py_result=$(python3 -c "
try:
    open('/etc/sandflox-test', 'w')
    print('ALLOWED')
except PermissionError as e:
    if 'sandflox' in str(e):
        print('BLOCKED')
    else:
        print('EPERM')
except Exception:
    print('ERROR')
" 2>/dev/null)
if [ "$_sfx_py_result" = "BLOCKED" ]; then
  ok "python open() blocked for writes outside workspace"
fi
```

**Pattern 6: Localhost socket testing (distinguishing EPERM from ECONNREFUSED)**
```bash
python3 -c "
import socket, errno
try:
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(1)
    s.connect(('127.0.0.1', 1))
except ConnectionRefusedError:
    pass  # refused means socket was allowed
except PermissionError:
    raise  # EPERM means sandbox blocked it
except OSError as e:
    if e.errno == errno.EACCES:
        raise
    pass  # other errors are fine
finally:
    s.close()
"
```

**Pattern 7: Conditional skipping based on enforcement tier**
```bash
if [ "$_sfx_platform" != "Darwin" ] || [ $_sfx_kernel -eq 0 ]; then
  skp "sandbox-exec not active — run via ./sandflox to test kernel enforcement"
else
  # kernel-dependent tests here
fi
```

**Pattern 8: Environment variable assertions**
```bash
if [ -z "${FLOX_ENV_PROJECT:-}" ]; then
  ok "FLOX_ENV_PROJECT is unset"
else
  bad "FLOX_ENV_PROJECT still set"
fi

if [ "${SANDFLOX_ENABLED:-}" = "1" ]; then
  ok "SANDFLOX_ENABLED=1"
else
  bad "SANDFLOX_ENABLED not set"
fi
```

## Mocking

**No mocking framework.** Tests operate against the real sandbox environment. They test actual enforcement by:
- Attempting blocked operations and checking for failure
- Attempting allowed operations and checking for success
- Running inside the activated sandbox, not in a mock environment

The tests are inherently integration tests -- they verify the combined behavior of all enforcement layers.

## Fixtures and Factories

**Test Data:**
- Tests create temporary files using `$$` (PID) for uniqueness: `/tmp/sandflox-kernel-test-$$`, `./sandflox-ws-test-$$`
- Cleanup is inline with `rm -f` after assertions
- No shared fixture files or factory functions

## Coverage

**Requirements:** No formal coverage target. No coverage tooling.

**What is tested (by file):**

`test-policy.sh` (v2 comprehensive):
- Kernel/macOS: `/tmp` write allowed, `/etc` write blocked, network blocking, denied paths (`~/.ssh`), workspace writes, localhost socket
- Shell: `fs-filter.sh` denied path enforcement, curl removal when `network=blocked`, Python `open()` write blocking
- Profiles: Tool count verification, profile-specific tool availability (curl, git, cp per profile)
- Backward: PATH exclusion for all package managers, function armor exit codes, breadcrumb cleanup, `python3 -m pip` blocking

`test-sandbox.sh` (shell enforcement):
- Layer 1: PATH restriction for 10 blocked tools and 11 allowed tools, conditional curl handling
- Layer 2: Function armor exit code for 9 package managers
- Layer 3: Breadcrumb cleanup (3 env vars)
- Escape vectors: `python3 -m pip`, `python3 -m ensurepip`

`verify-sandbox.sh` (v1 legacy):
- PATH restriction (9 blocked, 10 allowed)
- Function armor (10 commands)
- Breadcrumb cleanup (3 env vars)

## Test Types

**Unit Tests:**
- None. The codebase has no isolated unit tests for individual functions.

**Integration Tests:**
- All three test scripts are integration tests that run inside an activated sandbox
- They test the full enforcement stack: kernel + shell + Python layers together
- Must be invoked via `./sandflox -- bash <test>` for kernel-level tests or `flox activate -- bash <test>` for shell-only

**E2E Tests:**
- The test scripts themselves are effectively E2E tests -- they validate the user-facing behavior of the sandbox from an agent's perspective
- They simulate what an agent would try to do (install packages, write to sensitive paths, access network)

**Smoke Tests:**
- `verify-sandbox.sh` serves as a quick smoke test for v1 enforcement

## Known Gaps

**Linux Kernel Enforcement:**
- `test-policy.sh` kernel tests are macOS-only (`[Kernel/macOS]` section). No equivalent `[Kernel/Linux]` section exists for bwrap testing.
- The `_sfx_generate_bwrap()` function in `sandflox` has no dedicated tests.

**Read-Only Path Enforcement:**
- Read-only paths (`.git/`, `.flox/env/`, `policy.toml`, `requisites.txt`) declared in `policy.toml` have no direct test assertions verifying write denial. The `fs-filter.sh` wraps commands but tests only check denied paths, not read-only overrides.

**Profile Switching at Runtime:**
- Tests check the currently active profile but do not test switching profiles (e.g., running `SANDFLOX_PROFILE=minimal ./sandflox` vs `SANDFLOX_PROFILE=full ./sandflox` in the same test run).

**TOML Parser Fallback:**
- The inline minimal TOML parser (`_tomllib` class) used when `tomllib` and `tomli` are unavailable has no dedicated unit tests. It is only tested implicitly when the full stack runs on Python < 3.11.

**fs-filter.sh Edge Cases:**
- Shell write wrappers (`cp`, `mv`, `mkdir`, etc.) only check the last argument as the target. Multi-target commands or flag-after-target patterns are not tested.

**Network Allow-Localhost:**
- The localhost socket test is a best-effort test (port 1 connection) and may not reliably distinguish sandbox blocking from other socket errors on all systems.

**Generated Artifact Correctness:**
- No tests validate the generated SBPL profile (`sandflox.sb`) syntax or correctness independent of sandbox-exec execution.
- No tests validate the generated bwrap argument list.

**usercustomize.py:**
- The Python `builtins.open` wrapper (`_sandflox_open`) is only tested via the `test-policy.sh` Python write test. No tests cover edge cases like relative paths, symlinks, or binary mode opens.

**Concurrent / Multi-Session:**
- No tests verify behavior when multiple sandflox sessions run simultaneously.

**No CI Pipeline:**
- Tests are manual-only. No CI/CD integration (GitHub Actions, etc.) is configured.

---

*Testing analysis: 2026-04-15*
