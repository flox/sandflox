#!/usr/bin/env bash
# sandflox — kernel-enforced sandbox wrapper for flox activate
# ──────────────────────────────────────────────────────────────
# Reads policy.toml, generates platform-specific kernel enforcement
# (sandbox-exec on macOS, bwrap on Linux), then activates flox inside.
#
# Usage:
#   ./sandflox                         # interactive shell
#   ./sandflox -- bash agent-script.sh # run a command
#   SANDFLOX_PROFILE=minimal ./sandflox -- python3 agent.py
#
# Without this wrapper, `flox activate` still provides shell-level
# enforcement (PATH wipe, function armor, requisites filtering).
# This wrapper adds kernel-level enforcement on top.

set -euo pipefail

_sfx_dir="$(cd "$(dirname "$0")" && pwd)"
_sfx_policy="$_sfx_dir/policy.toml"
_sfx_cache="$_sfx_dir/.flox/cache/sandflox"

# ── Preflight ────────────────────────────────────────────

if [ ! -f "$_sfx_policy" ]; then
  echo "[sandflox] No policy.toml found — falling back to shell-only enforcement" >&2
  exec flox activate "$@"
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "[sandflox] ERROR: python3 required to parse policy.toml" >&2
  exit 1
fi

if ! command -v flox >/dev/null 2>&1; then
  echo "[sandflox] ERROR: flox not found in PATH" >&2
  exit 1
fi

mkdir -p "$_sfx_cache"

# ── Parse policy.toml ───────────────────────────────────

_sfx_config=$(python3 -c "
import os, sys, json

# tomllib is Python 3.11+; fall back to tomli (pip) or bundled parser
try:
    import tomllib
except ModuleNotFoundError:
    try:
        import tomli as tomllib
    except ModuleNotFoundError:
        # Minimal TOML parser for the subset policy.toml uses
        import re
        class _tomllib:
            @staticmethod
            def load(f):
                return _tomllib.loads(f.read().decode())
            @staticmethod
            def _parse_value(raw):
                raw = raw.strip()
                # Quoted string — find closing quote, ignore rest (inline comments)
                if raw.startswith('\"'):
                    end = raw.index('\"', 1)
                    return raw[1:end]
                # Array
                if raw.startswith('['):
                    end = raw.index(']')
                    inner = raw[1:end]
                    return [x.strip().strip('\"') for x in inner.split(',') if x.strip()]
                # Strip inline comments for bare values
                if '#' in raw:
                    raw = raw[:raw.index('#')].strip()
                if raw == 'true':
                    return True
                if raw == 'false':
                    return False
                return raw
            @staticmethod
            def loads(s):
                result = {}
                current = result
                for line in s.split('\n'):
                    line = line.strip()
                    if not line or line.startswith('#'):
                        continue
                    m = re.match(r'^\[([^\]]+)\]$', line)
                    if m:
                        path = m.group(1).split('.')
                        current = result
                        for p in path:
                            current = current.setdefault(p, {})
                        continue
                    m = re.match(r'^([^=]+?)\s*=\s*(.+)$', line)
                    if m:
                        key = m.group(1).strip()
                        current[key] = _tomllib._parse_value(m.group(2))
                return result
        tomllib = _tomllib

with open('$_sfx_policy', 'rb') as f:
    policy = tomllib.load(f)

# Resolve profile: env var > meta.profile > 'default'
profile_name = os.environ.get('SANDFLOX_PROFILE', policy.get('meta', {}).get('profile', 'default'))
profiles = policy.get('profiles', {})
profile = profiles.get(profile_name, {})

# Profile overrides top-level settings
net_mode = profile.get('network', policy.get('network', {}).get('mode', 'blocked'))
fs_mode = profile.get('filesystem', policy.get('filesystem', {}).get('mode', 'workspace'))
requisites = profile.get('requisites', 'requisites.txt')

# Top-level filesystem/network config (profile doesn't override these details)
allow_localhost = policy.get('network', {}).get('allow-localhost', True)
writable = policy.get('filesystem', {}).get('writable', ['.', '/tmp'])
read_only = policy.get('filesystem', {}).get('read-only', [])
denied = policy.get('filesystem', {}).get('denied', [])

config = {
    'profile': profile_name,
    'net_mode': net_mode,
    'fs_mode': fs_mode,
    'requisites': requisites,
    'allow_localhost': allow_localhost,
    'writable': writable,
    'read_only': read_only,
    'denied': denied,
}
print(json.dumps(config))
")

_sfx_profile=$(echo "$_sfx_config" | python3 -c "import sys,json; print(json.load(sys.stdin)['profile'])")
_sfx_net_mode=$(echo "$_sfx_config" | python3 -c "import sys,json; print(json.load(sys.stdin)['net_mode'])")
_sfx_fs_mode=$(echo "$_sfx_config" | python3 -c "import sys,json; print(json.load(sys.stdin)['fs_mode'])")
_sfx_requisites=$(echo "$_sfx_config" | python3 -c "import sys,json; print(json.load(sys.stdin)['requisites'])")
_sfx_allow_localhost=$(echo "$_sfx_config" | python3 -c "import sys,json; print(json.load(sys.stdin)['allow_localhost'])")

echo "[sandflox] Profile: $_sfx_profile | Network: $_sfx_net_mode | Filesystem: $_sfx_fs_mode" >&2

# Stage the resolved requisites file for the hook to pick up
if [ -f "$_sfx_dir/$_sfx_requisites" ]; then
  cp "$_sfx_dir/$_sfx_requisites" "$_sfx_cache/requisites.txt"
fi

# Write resolved config for shell-level enforcement scripts
echo "$_sfx_net_mode" > "$_sfx_cache/net-mode.txt"
echo "$_sfx_fs_mode" > "$_sfx_cache/fs-mode.txt"

# Write config JSON for Python to read (avoids shell quoting issues)
echo "$_sfx_config" > "$_sfx_cache/config.json"

# Resolve writable/denied paths to absolute and write to files
python3 -c "
import json, os, sys

with open('$_sfx_cache/config.json') as f:
    config = json.load(f)
project = '$_sfx_dir'
home = os.path.expanduser('~')

def resolve(p):
    is_dir = p.endswith('/')
    p = p.replace('~/', home + '/')
    if p.startswith('.') or not p.startswith('/'):
        p = os.path.join(project, p)
    p = os.path.normpath(p)
    if is_dir:
        p += '/'
    return p

with open('$_sfx_cache/writable-paths.txt', 'w') as f:
    for p in config['writable']:
        f.write(resolve(p) + '\n')

with open('$_sfx_cache/read-only-paths.txt', 'w') as f:
    for p in config['read_only']:
        f.write(resolve(p) + '\n')

with open('$_sfx_cache/denied-paths.txt', 'w') as f:
    for p in config['denied']:
        f.write(resolve(p) + '\n')
"

# ── Platform detection ───────────────────────────────────

_sfx_platform=$(uname -s)

# ── macOS: Generate SBPL profile + sandbox-exec ──────────

_sfx_generate_sbpl() {
  local sbpl="$_sfx_cache/sandflox.sb"
  local home
  home=$(eval echo "~")

  # Strategy: start from (allow default), then deny what the policy restricts.
  # deny-default + explicit whitelisting is too fragile for flox (nix daemon,
  # macOS frameworks, system libs all need unpredictable read paths).

  cat > "$sbpl" << 'SBPL_HEADER'
(version 1)

;; sandflox — generated SBPL profile
;; Baseline: allow everything, then restrict per policy.toml
(allow default)
SBPL_HEADER

  # Denied read paths (sensitive directories)
  if [ -s "$_sfx_cache/denied-paths.txt" ]; then
    echo "" >> "$sbpl"
    echo ";; ── Denied paths (sensitive data) ──" >> "$sbpl"
    while IFS= read -r dpath; do
      [ -z "$dpath" ] && continue
      dpath="${dpath%/}"
      echo "(deny file-read* (subpath \"$dpath\"))" >> "$sbpl"
      echo "(deny file-write* (subpath \"$dpath\"))" >> "$sbpl"
    done < "$_sfx_cache/denied-paths.txt"

    # Re-allow paths flox needs inside denied trees
    echo "" >> "$sbpl"
    echo ";; ── Flox-required overrides ──" >> "$sbpl"
    echo "(allow file-read* (subpath (param \"FLOX_CACHE\")))" >> "$sbpl"
    echo "(allow file-read* (subpath \"$home/.config/flox\"))" >> "$sbpl"
    echo "(allow file-write* (subpath \"$home/.config/flox\"))" >> "$sbpl"
  fi

  # Filesystem write restrictions based on mode
  case "$_sfx_fs_mode" in
    permissive)
      # No write restrictions
      ;;
    strict)
      echo "" >> "$sbpl"
      echo ";; ── Filesystem writes (strict — deny most writes) ──" >> "$sbpl"
      echo "(deny file-write*)" >> "$sbpl"
      # Re-allow essential writes for shell/flox operation
      echo "(allow file-write*" >> "$sbpl"
      echo "  (subpath \"/private/tmp\")" >> "$sbpl"
      echo "  (subpath \"/private/var/folders\")" >> "$sbpl"
      echo "  (subpath \"/dev\")" >> "$sbpl"
      echo "  (subpath (param \"FLOX_CACHE\"))" >> "$sbpl"
      echo "  (subpath \"$home/.config/flox\")" >> "$sbpl"
      echo "  (subpath \"$home/.local/share/flox\"))" >> "$sbpl"
      ;;
    workspace|*)
      echo "" >> "$sbpl"
      echo ";; ── Filesystem writes (workspace) ──" >> "$sbpl"
      echo "(deny file-write*)" >> "$sbpl"
      # Re-allow: project dir, tmp, flox state dirs
      echo "(allow file-write*" >> "$sbpl"
      echo "  (subpath (param \"PROJECT\"))" >> "$sbpl"
      echo "  (subpath \"/private/tmp\")" >> "$sbpl"
      echo "  (subpath \"/private/var/folders\")" >> "$sbpl"
      echo "  (subpath \"/dev\")" >> "$sbpl"
      echo "  (subpath (param \"FLOX_CACHE\"))" >> "$sbpl"
      echo "  (subpath \"$home/.config/flox\")" >> "$sbpl"
      echo "  (subpath \"$home/.local/share/flox\"))" >> "$sbpl"

      # Read-only overrides within the writable project dir
      echo "" >> "$sbpl"
      echo ";; Read-only overrides within project" >> "$sbpl"
      while IFS= read -r ropath; do
        [ -z "$ropath" ] && continue
        if [[ "$ropath" == */ ]]; then
          echo "(deny file-write* (subpath \"${ropath%/}\"))" >> "$sbpl"
        else
          echo "(deny file-write* (literal \"$ropath\"))" >> "$sbpl"
        fi
      done < "$_sfx_cache/read-only-paths.txt"
      ;;
  esac

  # Network rules
  echo "" >> "$sbpl"
  case "$_sfx_net_mode" in
    unrestricted)
      echo ";; ── Network (unrestricted) ──" >> "$sbpl"
      ;;
    blocked|*)
      echo ";; ── Network (blocked) ──" >> "$sbpl"
      echo "(deny network*)" >> "$sbpl"
      # Always allow unix sockets (nix daemon, local IPC)
      echo "(allow network* (remote unix-socket))" >> "$sbpl"
      if [ "$_sfx_allow_localhost" = "True" ]; then
        echo "(allow network* (remote ip \"localhost:*\"))" >> "$sbpl"
      fi
      ;;
  esac

  echo "$sbpl"
}

# ── Linux: Generate bwrap flags ──────────────────────────

_sfx_generate_bwrap() {
  local -a bwrap_args=()
  local home
  home=$(eval echo "~")

  # Base: read-only bind the whole filesystem
  bwrap_args+=(--ro-bind / /)

  # Writable paths
  case "$_sfx_fs_mode" in
    permissive)
      # Rebind root as read-write
      bwrap_args=(--bind / /)
      ;;
    workspace)
      while IFS= read -r wpath; do
        [ -z "$wpath" ] && continue
        bwrap_args+=(--bind "$wpath" "$wpath")
      done < "$_sfx_cache/writable-paths.txt"
      bwrap_args+=(--tmpfs /tmp)
      ;;
    strict)
      bwrap_args+=(--tmpfs /tmp)
      ;;
  esac

  # Read-only overrides
  while IFS= read -r ropath; do
    [ -z "$ropath" ] && continue
    [ -e "$ropath" ] && bwrap_args+=(--ro-bind "$ropath" "$ropath")
  done < "$_sfx_cache/read-only-paths.txt"

  # Denied paths — mount tmpfs over them to hide contents
  while IFS= read -r dpath; do
    [ -z "$dpath" ] && continue
    [ -e "$dpath" ] && bwrap_args+=(--tmpfs "$dpath")
  done < "$_sfx_cache/denied-paths.txt"

  # Network
  case "$_sfx_net_mode" in
    blocked)
      bwrap_args+=(--unshare-net)
      # Note: bwrap --unshare-net blocks all network including localhost
      # allow-localhost has no effect with bwrap (would need slirp4netns)
      ;;
  esac

  # Process isolation + essential mounts
  bwrap_args+=(
    --unshare-pid
    --die-with-parent
    --proc /proc
    --dev /dev
  )

  printf '%s\n' "${bwrap_args[@]}"
}

# ── Entrypoint generation ────────────────────────────────
# flox profile.common only runs for interactive shells.
# For `-- CMD` mode, generate an entrypoint that applies
# requisites filtering and function armor before the command.

_sfx_entrypoint="$_sfx_cache/entrypoint.sh"
cat > "$_sfx_entrypoint" << 'ENTRYEOF'
#!/usr/bin/env bash
# sandflox entrypoint — applies profile setup for non-interactive mode
_sfx_req="${FLOX_ENV_CACHE}/sandflox/requisites.txt"
if [ -f "$_sfx_req" ]; then
  _sfx_bin="${FLOX_ENV_CACHE}/sandflox/bin"
  rm -rf "$_sfx_bin"
  mkdir -p "$_sfx_bin"
  _sfx_count=0
  while IFS= read -r _sfx_line || [ -n "$_sfx_line" ]; do
    case "$_sfx_line" in \#*|"") continue ;; esac
    _sfx_tool="${_sfx_line%% *}"
    _sfx_tool="${_sfx_tool%%	*}"
    [ -z "$_sfx_tool" ] && continue
    if [ -x "${FLOX_ENV}/bin/$_sfx_tool" ]; then
      ln -sf "${FLOX_ENV}/bin/$_sfx_tool" "${_sfx_bin}/$_sfx_tool"
      _sfx_count=$((_sfx_count + 1))
    fi
  done < "$_sfx_req"
  if [ -f "${FLOX_ENV_CACHE}/sandflox/net-blocked.flag" ]; then
    if [ -L "${_sfx_bin}/curl" ]; then
      rm -f "${_sfx_bin}/curl"
      _sfx_count=$((_sfx_count - 1))
    fi
  fi
  export PATH="$_sfx_bin"
  _sfx_profile_name="default"
  [ -f "${FLOX_ENV_CACHE}/sandflox/active-profile.txt" ] && \
    _sfx_profile_name=$(cat "${FLOX_ENV_CACHE}/sandflox/active-profile.txt" 2>/dev/null | tr -d '\n')
  echo "[sandflox] Sandbox active: $_sfx_count tools (profile: $_sfx_profile_name)" >&2
fi

# Function armor
_sandflox_blocked() {
  echo "[sandflox] BLOCKED: $1 is not available. Environment is immutable." >&2
  return 126
}
flox()      { _sandflox_blocked flox; }
nix()       { _sandflox_blocked nix; }
nix-env()   { _sandflox_blocked nix-env; }
nix-store() { _sandflox_blocked nix-store; }
nix-shell() { _sandflox_blocked nix-shell; }
nix-build() { _sandflox_blocked nix-build; }
apt()       { _sandflox_blocked apt; }
apt-get()   { _sandflox_blocked apt-get; }
yum()       { _sandflox_blocked yum; }
dnf()       { _sandflox_blocked dnf; }
brew()      { _sandflox_blocked brew; }
snap()      { _sandflox_blocked snap; }
flatpak()   { _sandflox_blocked flatpak; }
pip()       { _sandflox_blocked pip; }
pip3()      { _sandflox_blocked pip3; }
npm()       { _sandflox_blocked npm; }
npx()       { _sandflox_blocked npx; }
yarn()      { _sandflox_blocked yarn; }
pnpm()      { _sandflox_blocked pnpm; }
cargo()     { _sandflox_blocked cargo; }
go()        { _sandflox_blocked go; }
gem()       { _sandflox_blocked gem; }
composer()  { _sandflox_blocked composer; }
uv()        { _sandflox_blocked uv; }
docker()    { _sandflox_blocked docker; }
podman()    { _sandflox_blocked podman; }
export -f _sandflox_blocked \
  flox nix nix-env nix-store nix-shell nix-build \
  apt apt-get yum dnf brew snap flatpak \
  pip pip3 npm npx yarn pnpm cargo go gem composer uv \
  docker podman

# Source fs-filter if available
if [ -f "${FLOX_ENV_CACHE}/sandflox/fs-filter.sh" ]; then
  . "${FLOX_ENV_CACHE}/sandflox/fs-filter.sh"
fi

# Breadcrumb cleanup
unset FLOX_ENV_PROJECT FLOX_ENV_DIRS FLOX_PATH_PATCHED
unset _sfx_bin _sfx_req _sfx_count _sfx_line _sfx_tool _sfx_profile_name

exec "$@"
ENTRYEOF

# ── Execute ──────────────────────────────────────────────

# Export profile for the hook to pick up
export SANDFLOX_PROFILE="$_sfx_profile"

# Build the flox activate command
# For interactive: flox activate (profile will run)
# For -- CMD: flox activate -- bash entrypoint.sh CMD (entrypoint applies profile)
_sfx_flox_args=()
if [[ "$*" == *"--"* ]]; then
  # Extract everything after --
  _sfx_found_sep=0
  _sfx_user_cmd=()
  for _sfx_arg in "$@"; do
    if [ "$_sfx_found_sep" -eq 1 ]; then
      _sfx_user_cmd+=("$_sfx_arg")
    elif [ "$_sfx_arg" = "--" ]; then
      _sfx_found_sep=1
    fi
  done
  _sfx_flox_args=(-- bash "$_sfx_entrypoint" "${_sfx_user_cmd[@]}")
else
  _sfx_flox_args=("$@")
fi

case "$_sfx_platform" in
  Darwin)
    if ! command -v sandbox-exec >/dev/null 2>&1; then
      echo "[sandflox] WARNING: sandbox-exec not found — falling back to shell-only" >&2
      exec flox activate "${_sfx_flox_args[@]}"
    fi

    _sfx_sbpl=$(_sfx_generate_sbpl)
    echo "[sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)" >&2

    _sfx_home=$(eval echo '~')
    exec sandbox-exec -f "$_sfx_sbpl" \
      -D "PROJECT=$_sfx_dir" \
      -D "HOME=$_sfx_home" \
      -D "FLOX_CACHE=$_sfx_home/.cache/flox" \
      flox activate "${_sfx_flox_args[@]}"
    ;;

  Linux)
    if ! command -v bwrap >/dev/null 2>&1; then
      echo "[sandflox] WARNING: bwrap not found — falling back to shell-only" >&2
      exec flox activate "${_sfx_flox_args[@]}"
    fi

    mapfile -t _sfx_bwrap_args < <(_sfx_generate_bwrap)
    echo "[sandflox] Kernel enforcement: bwrap (bubblewrap namespaces)" >&2

    exec bwrap "${_sfx_bwrap_args[@]}" \
      -- flox activate "${_sfx_flox_args[@]}"
    ;;

  *)
    echo "[sandflox] WARNING: unsupported platform '$_sfx_platform' — shell-only" >&2
    exec flox activate "${_sfx_flox_args[@]}"
    ;;
esac
