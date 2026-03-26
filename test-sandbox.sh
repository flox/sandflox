#!/usr/bin/env bash
# test-sandbox.sh — comprehensive sandbox verification
# Run with: flox activate -- bash test-sandbox.sh  (tests Layer 1 only)
# Or:       flox activate  then  bash test-sandbox.sh  (tests all layers)

pass=0; fail=0
ok()   { echo "  PASS: $1"; pass=$((pass + 1)); }
bad()  { echo "  FAIL: $1"; fail=$((fail + 1)); }

echo "========================================"
echo " sandflox verification"
echo "========================================"
echo ""

echo "[Layer 1] PATH restriction — blocked tools"
for cmd in flox nix nix-env nix-store brew pip pip3 npm docker podman; do
  if command -v "$cmd" >/dev/null 2>&1; then
    bad "$cmd found at $(command -v $cmd)"
  else
    ok "$cmd not in PATH"
  fi
done
echo ""

echo "[Layer 1] PATH restriction — allowed tools"
for cmd in python3 git curl jq grep sed awk bash ls cat echo; do
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "$cmd available"
  else
    bad "$cmd not found"
  fi
done
echo ""

echo "[Layer 2] Function armor (interactive shells only)"
for cmd in flox nix brew apt pip pip3 npm cargo docker; do
  output=$($cmd 2>&1)
  rc=$?
  if [ $rc -eq 126 ]; then
    ok "$cmd returns 126 (function armor)"
  elif [ $rc -eq 127 ]; then
    ok "$cmd returns 127 (not found — Layer 1 caught it)"
  else
    bad "$cmd returned $rc"
  fi
done
echo ""

echo "[Layer 3] Breadcrumb cleanup"
if [ -z "${FLOX_ENV_PROJECT:-}" ]; then
  ok "FLOX_ENV_PROJECT is unset"
else
  bad "FLOX_ENV_PROJECT still set: $FLOX_ENV_PROJECT"
fi
if [ -z "${FLOX_ENV_DIRS:-}" ]; then
  ok "FLOX_ENV_DIRS is unset"
else
  bad "FLOX_ENV_DIRS still set: $FLOX_ENV_DIRS"
fi
if [ "${SANDFLOX_ENABLED:-}" = "1" ]; then
  ok "SANDFLOX_ENABLED=1"
else
  bad "SANDFLOX_ENABLED not set"
fi
echo ""

echo "[Escape vector tests]"
# Can the agent bootstrap pip?
if python3 -m pip --version >/dev/null 2>&1; then
  bad "python3 -m pip works (agent can install packages)"
else
  ok "python3 -m pip not available"
fi
# Can the agent use ensurepip?
if python3 -m ensurepip --version >/dev/null 2>&1; then
  bad "python3 -m ensurepip works (agent can bootstrap pip)"
else
  ok "python3 -m ensurepip not available"
fi
echo ""

echo "========================================"
echo " Results: $pass passed, $fail failed"
echo "========================================"
[ $fail -gt 0 ] && exit 1 || exit 0
