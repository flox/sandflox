#!/usr/bin/env bash
# verify-sandbox.sh — Run inside an activated sandflox environment
# to confirm all enforcement layers are working.

set -euo pipefail

pass=0
fail=0

check() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "  FAIL: $desc"
    fail=$((fail + 1))
  else
    echo "  PASS: $desc"
    pass=$((pass + 1))
  fi
}

check_exists() {
  local desc="$1"
  local cmd="$2"
  if command -v "$cmd" >/dev/null 2>&1; then
    echo "  PASS: $desc"
    pass=$((pass + 1))
  else
    echo "  FAIL: $desc"
    fail=$((fail + 1))
  fi
}

echo "========================================"
echo " sandflox verification"
echo "========================================"
echo ""

echo "[Layer 1] PATH restriction — blocked tools should not resolve"
check "flox not in PATH"           command -v flox
check "nix not in PATH"            command -v nix
check "nix-env not in PATH"        command -v nix-env
check "nix-store not in PATH"      command -v nix-store
check "brew not in PATH"           command -v brew
check "pip not in PATH"            command -v pip
check "pip3 not in PATH"           command -v pip3
check "npm not in PATH"            command -v npm
check "docker not in PATH"         command -v docker
echo ""

echo "[Layer 1] PATH restriction — allowed tools should resolve"
check_exists "python3 available"   python3
check_exists "git available"       git
check_exists "curl available"      curl
check_exists "jq available"        jq
check_exists "grep available"      grep
check_exists "sed available"       sed
check_exists "awk available"       awk
check_exists "bash available"      bash
check_exists "ls available"        ls
check_exists "cat available"       cat
echo ""

echo "[Layer 2] Function armor — package managers return 126"
for cmd in flox nix nix-env brew apt pip pip3 npm cargo docker; do
  $cmd 2>/dev/null
  rc=$?
  if [ $rc -eq 126 ]; then
    echo "  PASS: $cmd returns 126 (blocked)"
    pass=$((pass + 1))
  else
    echo "  FAIL: $cmd returned $rc (expected 126)"
    fail=$((fail + 1))
  fi
done
echo ""

echo "[Layer 3] Breadcrumb cleanup"
if [ -z "${FLOX_ENV_PROJECT:-}" ]; then
  echo "  PASS: FLOX_ENV_PROJECT is unset"
  pass=$((pass + 1))
else
  echo "  FAIL: FLOX_ENV_PROJECT is still set: $FLOX_ENV_PROJECT"
  fail=$((fail + 1))
fi

if [ -z "${FLOX_ENV_DIRS:-}" ]; then
  echo "  PASS: FLOX_ENV_DIRS is unset"
  pass=$((pass + 1))
else
  echo "  FAIL: FLOX_ENV_DIRS is still set"
  fail=$((fail + 1))
fi

if [ "${SANDFLOX_ENABLED:-}" = "1" ]; then
  echo "  PASS: SANDFLOX_ENABLED=1"
  pass=$((pass + 1))
else
  echo "  FAIL: SANDFLOX_ENABLED not set"
  fail=$((fail + 1))
fi
echo ""

echo "========================================"
echo " Results: $pass passed, $fail failed"
echo "========================================"

if [ $fail -gt 0 ]; then
  exit 1
fi
