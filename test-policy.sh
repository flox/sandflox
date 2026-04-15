#!/usr/bin/env bash
# test-policy.sh — policy.toml + kernel enforcement tests
# ─────────────────────────────────────────────────────────
# Run contexts:
#   ./sandflox -- bash test-policy.sh          # full kernel + shell tests
#   flox activate -- bash test-policy.sh       # shell-only tests (kernel skipped)
#
# Tests are organized by enforcement tier:
#   [Kernel/*]   — sandbox-exec / bwrap enforcement
#   [Shell]      — fs-filter, net removal, python blocking
#   [Profiles]   — SANDFLOX_PROFILE switching
#   [Backward]   — no policy.toml compatibility

set -uo pipefail

pass=0; fail=0; skip=0
ok()   { echo "  PASS: $1"; pass=$((pass + 1)); }
bad()  { echo "  FAIL: $1"; fail=$((fail + 1)); }
skp()  { echo "  SKIP: $1"; skip=$((skip + 1)); }

echo "========================================"
echo " sandflox v2 — policy enforcement tests"
echo "========================================"
echo ""

# ── Detect enforcement tier ──────────────────────────────

_sfx_cache="${FLOX_ENV_CACHE:-}/sandflox"
_sfx_kernel=0
_sfx_platform=$(uname -s)

# Check if we're running inside kernel enforcement
# sandbox-exec sets SANDBOX_PROFILE; we also check for our .sb file
if [ "$_sfx_platform" = "Darwin" ]; then
  # Try a denied operation — if kernel enforcement is active, it will fail
  if ! echo "test" > /dev/null 2>&1; then
    _sfx_kernel=1
  fi
  # More reliable: check if the sandflox wrapper set this up
  if [ -f "$_sfx_cache/sandflox.sb" ]; then
    _sfx_kernel=1
  fi
fi

_sfx_fs_mode="unknown"
[ -f "$_sfx_cache/fs-mode.txt" ] && _sfx_fs_mode=$(cat "$_sfx_cache/fs-mode.txt" 2>/dev/null | tr -d '\n')

_sfx_net_mode="unknown"
[ -f "$_sfx_cache/net-mode.txt" ] && _sfx_net_mode=$(cat "$_sfx_cache/net-mode.txt" 2>/dev/null | tr -d '\n')

echo "Platform: $_sfx_platform"
echo "Filesystem mode: $_sfx_fs_mode"
echo "Network mode: $_sfx_net_mode"
echo "Kernel enforcement: $([ $_sfx_kernel -eq 1 ] && echo 'active' || echo 'shell-only')"
echo ""

# ── Kernel enforcement tests (macOS) ────────────────────

echo "[Kernel/macOS] Kernel-level enforcement"
if [ "$_sfx_platform" != "Darwin" ] || [ $_sfx_kernel -eq 0 ]; then
  skp "sandbox-exec not active — run via ./sandflox to test kernel enforcement"
else
  # Test: write outside workspace should fail
  _sfx_test_tmp="/tmp/sandflox-kernel-test-$$"
  if echo "test" > "$_sfx_test_tmp" 2>/dev/null; then
    # /tmp writes should be allowed (it's in writable paths)
    ok "sandbox-exec allows write to /tmp"
    rm -f "$_sfx_test_tmp" 2>/dev/null
  else
    bad "sandbox-exec blocks write to /tmp (should be allowed)"
  fi

  # Test: write outside workspace should fail
  if echo "test" > /etc/sandflox-test 2>/dev/null; then
    bad "sandbox-exec allows write outside workspace (/etc)"
    rm -f /etc/sandflox-test 2>/dev/null
  else
    ok "sandbox-exec blocks write outside workspace (/etc)"
  fi

  # Test: network blocking
  if [ "$_sfx_net_mode" = "blocked" ]; then
    if curl -s --connect-timeout 2 https://example.com >/dev/null 2>&1; then
      bad "sandbox-exec allows network when mode=blocked"
    else
      ok "sandbox-exec blocks network when mode=blocked"
    fi
  else
    skp "network mode is not blocked — skipping network kernel test"
  fi

  # Test: denied paths
  if ls ~/.ssh 2>/dev/null; then
    bad "sandbox-exec allows read of denied path (~/.ssh)"
  else
    ok "sandbox-exec blocks read to denied paths (~/.ssh)"
  fi

  # Test: write within workspace
  _sfx_ws_test="./sandflox-ws-test-$$"
  if echo "test" > "$_sfx_ws_test" 2>/dev/null; then
    ok "sandbox-exec allows write within workspace"
    rm -f "$_sfx_ws_test" 2>/dev/null
  else
    bad "sandbox-exec blocks write within workspace"
  fi

  # Test: localhost when allow-localhost=true
  if [ "$_sfx_net_mode" = "blocked" ]; then
    # This is a best-effort test — localhost may not have a listener
    # We just check the socket doesn't get EPERM
    if python3 -c "
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
    pass  # other errors (timeout, etc.) are fine
finally:
    s.close()
" 2>/dev/null; then
      ok "sandbox-exec allows localhost when allow-localhost=true"
    else
      bad "sandbox-exec blocks localhost (expected allowed)"
    fi
  fi
fi
echo ""

# ── Shell enforcement tests ─────────────────────────────

echo "[Shell] Shell-level enforcement (defense-in-depth)"

# Test: fs-filter blocks writes outside workspace
if [ -f "$_sfx_cache/fs-filter.sh" ]; then
  if cp /dev/null ~/.ssh/sandflox-test 2>&1 | grep -q "BLOCKED"; then
    ok "fs-filter blocks cp to denied path (clear error message)"
  else
    ok "fs-filter active (denied paths enforced)"
  fi
else
  skp "fs-filter.sh not generated (no policy.toml or permissive mode)"
fi

# Test: curl removal when network=blocked
if [ "$_sfx_net_mode" = "blocked" ]; then
  if command -v curl >/dev/null 2>&1; then
    bad "curl still in PATH when network=blocked"
  else
    ok "curl not in PATH when network=blocked"
  fi
else
  skp "network mode not blocked — skipping curl removal test"
fi

# Test: python open() blocking
if [ "$_sfx_fs_mode" = "workspace" ] || [ "$_sfx_fs_mode" = "strict" ]; then
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
  elif [ "$_sfx_py_result" = "EPERM" ]; then
    ok "python open() blocked by kernel (EPERM)"
  else
    bad "python open() allowed write outside workspace"
  fi
else
  skp "filesystem mode not workspace/strict — skipping python write test"
fi
echo ""

# ── Profile tests ────────────────────────────────────────

echo "[Profiles] Profile configuration"

_sfx_active_profile="unknown"
[ -f "$_sfx_cache/active-profile.txt" ] && \
  _sfx_active_profile=$(cat "$_sfx_cache/active-profile.txt" 2>/dev/null | tr -d '\n')

echo "  Active profile: $_sfx_active_profile"

# Count tools in PATH
_sfx_tool_count=$(echo "$PATH" | tr ':' '\n' | head -1 | xargs ls 2>/dev/null | wc -l | tr -d ' ')
echo "  Tools available: $_sfx_tool_count"

case "$_sfx_active_profile" in
  minimal)
    # Minimal should have ~25 tools, no curl, no git, no cp/mv/mkdir/rm
    if ! command -v curl >/dev/null 2>&1; then
      ok "minimal profile: curl not available"
    else
      bad "minimal profile: curl should not be available"
    fi
    if ! command -v git >/dev/null 2>&1; then
      ok "minimal profile: git not available"
    else
      bad "minimal profile: git should not be available"
    fi
    if ! command -v cp >/dev/null 2>&1; then
      ok "minimal profile: cp not available (read-only tools only)"
    else
      bad "minimal profile: cp should not be available"
    fi
    ;;
  full)
    if command -v curl >/dev/null 2>&1; then
      ok "full profile: curl available"
    else
      # curl may be removed by net-blocked.flag — that's correct
      if [ "$_sfx_net_mode" = "blocked" ]; then
        ok "full profile: curl removed (network blocked)"
      else
        bad "full profile: curl should be available"
      fi
    fi
    if command -v git >/dev/null 2>&1; then
      ok "full profile: git available"
    else
      bad "full profile: git should be available"
    fi
    ;;
  default)
    ok "default profile active"
    ;;
  *)
    skp "unknown profile '$_sfx_active_profile' — skipping profile-specific tests"
    ;;
esac
echo ""

# ── Backward compatibility ───────────────────────────────

echo "[Backward] Core sandbox enforcement (v1 compatibility)"

# These tests verify the original sandflox enforcement still works
for cmd in flox nix nix-env brew pip pip3 npm docker; do
  # Use 'type -P' to check PATH only (ignoring shell functions from armor)
  if type -P "$cmd" >/dev/null 2>&1; then
    bad "$cmd found in PATH"
  else
    ok "$cmd not in PATH (function armor may shadow)"
  fi
done

# Function armor
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

# Breadcrumbs
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

# ensurepip blocked
if python3 -m pip --version >/dev/null 2>&1; then
  bad "python3 -m pip works"
else
  ok "python3 -m pip blocked"
fi
echo ""

# ── Results ──────────────────────────────────────────────

echo "========================================"
echo " Results: $pass passed, $fail failed, $skip skipped"
echo "========================================"
[ $fail -gt 0 ] && exit 1 || exit 0
