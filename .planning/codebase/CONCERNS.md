# Technical Concerns

**Analysis Date:** 2026-04-15

## Security Concerns

### fs-filter.sh only checks the last argument (HIGH)
- **Severity:** High
- **Files:** `sandflox` (lines 207-214 — fs-filter.sh generation in manifest.toml hook), `.flox/cache/sandflox/fs-filter.sh` (lines 33-37)
- **Details:** The shell-level `fs-filter.sh` wrappers for `cp`, `mv`, `ln`, `tee`, etc. only validate the *last* argument (`${!#}`) as the write target. This is incorrect for several commands:
  - `cp source dest` — last arg is destination (correct)
  - `cp source1 source2 dest/` — last arg is destination (correct)
  - `mv source dest` — last arg is destination (correct)
  - `tee file1 file2 file3` — ALL arguments are write targets, not just the last one. An agent could `echo secrets | tee /tmp/ok ~/.ssh/exfil` and only `/tmp/ok` is checked.
  - `ln -sf target linkname` — last arg is correct, but `ln target` (single-arg form) uses the current directory. The filter doesn't handle single-arg `ln`.
  - `rm -rf dir1 dir2 dir3` — ALL arguments are targets, not just the last one.
- **Impact:** Shell-level enforcement is bypassable for multi-target commands. Kernel enforcement still blocks at the syscall level, but `flox activate` users (without `./sandflox`) lose protection.
- **Fix approach:** Iterate over all non-flag arguments for `rm`, `rmdir`, `tee`. For `cp` and `mv`, check the last arg (destination). For `ln`, handle the single-argument form.

### Inline TOML parser does not handle edge cases (MEDIUM)
- **Severity:** Medium
- **Files:** `sandflox` (lines 54-98), `.flox/env/manifest.toml` (lines 61-112)
- **Details:** The fallback `_tomllib` mini-parser (for Python <3.11 without `tomli`) has limitations:
  - No support for multiline strings (triple-quoted `'''` or `"""`)
  - No support for inline tables `{key = "val"}`
  - No support for arrays of tables `[[section]]`
  - No support for dotted keys `a.b = "val"` outside section headers
  - Quoted strings with escaped quotes (`\"`) will break the `raw.index('"', 1)` logic
  - The parser is duplicated identically in both `sandflox` (line 54) and `manifest.toml` hook (line 70) — two copies to maintain
- **Impact:** If a user adds complex TOML to `policy.toml` (e.g., inline tables, multiline strings), the parser silently produces wrong results rather than failing. This could lead to a permissive security posture by accident.
- **Fix approach:** Either (1) require `tomli` as a dependency for Python <3.11, or (2) add validation that the parser result matches expected schema, or (3) extract the parser into a single shared `.py` file.

### Python `builtins.open` monkey-patch is bypassable (MEDIUM)
- **Severity:** Medium
- **Files:** `.flox/cache/sandflox-python/usercustomize.py` (lines 37-55)
- **Details:** The `_sandflox_open` wrapper replaces `builtins.open`, but Python has other ways to write files:
  - `os.open()` + `os.write()` — raw file descriptors
  - `pathlib.Path.write_text()` / `write_bytes()` — uses `open()` internally on CPython but the import could be cached before monkey-patch
  - `io.open()` — the real underlying open
  - `subprocess.Popen("echo data > file", shell=True)` — subprocess escape
  - `ctypes` to call libc `fopen`/`write` directly
  - `os.rename()`, `os.link()`, `os.symlink()` — filesystem mutation without `open()`
- **Impact:** A sophisticated agent could bypass Python-level write restrictions. Kernel enforcement (sandbox-exec/bwrap) remains the real barrier; this is defense-in-depth only.
- **Fix approach:** Document clearly that the Python wrapper is "best-effort, agent-friendly" and not a security boundary. The kernel tier is the actual enforcement. Optionally, also patch `os.open`, `os.rename`, `os.symlink`, `os.link`, `pathlib.Path.write_text`, `pathlib.Path.write_bytes`.

### macOS `sandbox-exec` is deprecated (MEDIUM)
- **Severity:** Medium
- **Files:** `sandflox` (lines 190-291, 466-480)
- **Details:** Apple deprecated `sandbox-exec` and the Seatbelt SBPL profile format. It still works on macOS Sequoia (15.x) and earlier, but Apple could remove it in a future release. The tool generates SBPL profiles at runtime (`_sfx_generate_sbpl`). There is no fallback if `sandbox-exec` disappears — the script falls back to shell-only enforcement (line 468-469).
- **Impact:** A future macOS version could silently remove kernel enforcement, leaving only shell-level protection. No warning mechanism for deprecation detection beyond "command not found."
- **Fix approach:** Monitor Apple's deprecation timeline. Investigate alternatives: macOS Endpoint Security framework (requires entitlements), App Sandbox (requires .app bundle), or a custom kext/system extension. For now, document the risk and ensure `flox activate` shell-level enforcement remains robust as the fallback.

### `eval echo "~"` used for home directory expansion (LOW)
- **Severity:** Low
- **Files:** `sandflox` (lines 194, 298, 475)
- **Details:** `eval echo "~"` is used instead of `$HOME` or `~` directly in multiple places. While controlled here (no user input in the eval), `eval` is generally dangerous. If any path variable ever leaked into an eval context, it could execute arbitrary code.
- **Impact:** Low risk currently — the eval operates on a literal tilde. But it's a hazardous pattern that could become exploitable if the code evolves.
- **Fix approach:** Replace all `eval echo "~"` with `"$HOME"` throughout `sandflox`.

### Denied paths list does not cover all sensitive locations (LOW)
- **Severity:** Low
- **Files:** `policy.toml` (line 23)
- **Details:** The default denied paths are `~/.ssh/`, `~/.gnupg/`, `~/.aws/`, `~/.config/gcloud/`, `~/.config/gh/`. Missing notable sensitive directories:
  - `~/.kube/` (Kubernetes credentials)
  - `~/.docker/` (Docker credentials/config.json)
  - `~/.azure/` (Azure CLI credentials)
  - `~/.config/op/` (1Password CLI)
  - `~/.npmrc` (npm auth tokens — file, not directory)
  - `~/.netrc` (HTTP auth credentials)
  - `~/.config/hub/` (legacy GitHub CLI)
  - `~/Library/Keychains/` (macOS keychain — though likely OS-protected)
- **Impact:** Agents with workspace or permissive filesystem access could read credentials from undeniable paths. Kernel tier `(allow default)` baseline means reads are allowed unless explicitly denied.
- **Fix approach:** Expand the default denied list in `policy.toml`. Consider adding a `denied-files` array for individual files like `~/.npmrc`, `~/.netrc`.

## Technical Debt

### TOML parser duplicated between `sandflox` and `manifest.toml` (HIGH)
- **Severity:** High
- **Files:** `sandflox` (lines 54-98), `.flox/env/manifest.toml` (lines 61-112)
- **Details:** The entire fallback TOML parser (45 lines of Python) and the policy resolution logic (~30 lines) are copy-pasted identically in two locations:
  1. The `sandflox` wrapper script (for kernel enforcement generation)
  2. The `manifest.toml` `on-activate` hook (for shell-level enforcement staging)
- Both parse `policy.toml`, resolve profiles, compute writable/denied paths, and write cache files. Any bug fix or feature addition must be applied to both copies.
- **Impact:** Divergence between the two copies could cause kernel and shell enforcement to disagree on policy interpretation.
- **Fix approach:** Extract the shared Python logic into a single file (e.g., `_sandflox_parse_policy.py`) that both `sandflox` and the hook invoke.

### Entrypoint script duplicates profile.common logic (MEDIUM)
- **Severity:** Medium
- **Files:** `sandflox` (lines 358-438 — entrypoint.sh generation), `.flox/env/manifest.toml` (lines 318-412 — profile.common)
- **Details:** The generated `entrypoint.sh` (for `./sandflox -- CMD` non-interactive mode) replicates requisites filtering, function armor, fs-filter sourcing, and breadcrumb cleanup from `profile.common`. These are nearly identical blocks maintained in two places.
- **Impact:** Adding a new blocked command or changing the armor list requires updating three locations: `sandflox` entrypoint generation, `manifest.toml` profile.common, and the wrapper's own entrypoint template.
- **Fix approach:** Have the entrypoint source a shared script rather than embedding a copy. The `profile.common` already generates the right state; the entrypoint could `source` it or share a common script.

### Function armor list is hardcoded in three places (MEDIUM)
- **Severity:** Medium
- **Files:** `sandflox` (lines 396-426), `.flox/env/manifest.toml` (lines 364-400), `.flox/cache/sandflox/entrypoint.sh` (lines 33-67)
- **Details:** The list of 27 blocked commands (`flox`, `nix`, `brew`, `pip`, etc.) is hardcoded identically in three locations. Adding or removing a blocked command requires editing all three.
- **Impact:** Risk of inconsistency if one copy is updated but others are not. The entrypoint is generated at runtime from the `sandflox` script, so the wrapper and entrypoint are technically one source — but `manifest.toml` is a separate copy.
- **Fix approach:** Generate the armor list from a single source (e.g., a text file like `blocked-commands.txt`) or have both the entrypoint and profile source a shared armor script.

### No version pinning for sandflox script itself (LOW)
- **Severity:** Low
- **Files:** `sandflox`, `policy.toml` (line 9: `version = "2"`)
- **Details:** While `policy.toml` has a `version = "2"` field, the `sandflox` script does not check this version or validate compatibility. A v3 policy format could be silently misinterpreted by a v2 parser.
- **Impact:** Future policy format changes could cause silent misconfiguration.
- **Fix approach:** Add a version check in the Python policy parser: reject or warn if `meta.version` is not `"2"`.

## Performance Risks

### Six separate `python3` invocations during policy parsing (LOW)
- **Severity:** Low
- **Files:** `sandflox` (lines 43-183)
- **Details:** The `sandflox` wrapper invokes `python3` at least 6 times during startup:
  1. Parse policy.toml and emit JSON (line 43)
  2. Extract `profile` from JSON (line 133)
  3. Extract `net_mode` from JSON (line 134)
  4. Extract `fs_mode` from JSON (line 135)
  5. Extract `requisites` from JSON (line 136)
  6. Extract `allow_localhost` from JSON (line 137)
  7. Resolve paths and write cache files (line 154)
- Each `python3` invocation has ~50-100ms startup overhead. Total: ~350-700ms added to sandbox launch.
- **Impact:** Noticeable delay when launching the sandbox. Not a runtime concern (one-time cost), but it adds up for rapid iteration workflows.
- **Fix approach:** Consolidate into a single `python3` invocation that outputs all fields at once (e.g., as shell `export` statements that can be `eval`'d, or write all values to separate files in one pass). Lines 133-137 could use a single `python3 -c` call that extracts all fields.

### Requisites symlink directory rebuilt on every activation (LOW)
- **Severity:** Low
- **Files:** `.flox/env/manifest.toml` (lines 327-341), `sandflox` entrypoint (lines 5-18)
- **Details:** Every `flox activate` or `./sandflox -- CMD` invocation deletes and recreates the `$FLOX_ENV_CACHE/sandflox/bin` directory, iterating over `requisites.txt` and symlinking each allowed binary.
- **Impact:** Adds ~50-100ms per activation. Minor, but could be optimized with a cache-validity check (e.g., compare mtime of requisites.txt against the bin directory).
- **Fix approach:** Check if `bin/` is already populated and `requisites.txt` hasn't changed before rebuilding.

## Maintainability Issues

### `sandflox` is a 500-line monolithic bash script (MEDIUM)
- **Severity:** Medium
- **Files:** `sandflox` (501 lines)
- **Details:** The script handles policy parsing (Python), SBPL generation (bash+heredoc), bwrap flag generation (bash), entrypoint generation (bash heredoc), and execution logic in a single file. The embedded Python strings with shell variable interpolation (`'$_sfx_policy'`, `'$_sfx_dir'`) make the code hard to read, test, and debug.
- **Impact:** Modifying any one concern (e.g., adding a new filesystem mode) requires careful navigation of the monolith and understanding of the bash-Python interleaving.
- **Fix approach:** Extract Python logic into standalone `.py` files. Extract SBPL generation into a template or generator function. Keep `sandflox` as a thin orchestrator that calls these components.

### `manifest.toml` on-activate hook is 275 lines of embedded Python (MEDIUM)
- **Severity:** Medium
- **Files:** `.flox/env/manifest.toml` (lines 40-313)
- **Details:** The `on-activate` hook contains a large embedded Python script (~225 lines) within TOML triple-quoted strings. This is not lintable, not independently testable, and errors are hard to debug (Python tracebacks reference `<string>` not a filename).
- **Impact:** Bugs in the hook require activating a flox environment to reproduce. No unit testing is possible for the policy parsing logic within the hook.
- **Fix approach:** Move the Python policy parsing to a standalone script (e.g., `_sfx_stage_policy.py`) and call it from the hook: `python3 "$FLOX_ENV_PROJECT/_sfx_stage_policy.py"`.

### Three overlapping test scripts with duplicated test logic (LOW)
- **Severity:** Low
- **Files:** `test-policy.sh` (302 lines), `test-sandbox.sh` (95 lines), `verify-sandbox.sh` (111 lines)
- **Details:** Three separate test scripts exist with significant overlap:
  - `test-sandbox.sh` — Layer 1/2/3 tests + escape vectors
  - `verify-sandbox.sh` — Layer 1/2/3 tests (subset of test-sandbox.sh)
  - `test-policy.sh` — Kernel + shell + profile + backward compat tests
  - `test-sandbox.sh` and `verify-sandbox.sh` test nearly identical things (PATH restriction, function armor, breadcrumbs) with different helper function styles (`ok`/`bad` vs `check`/`check_exists`).
- **Impact:** Bug fixes to test logic may not propagate to all three files. Confusion about which test script to run.
- **Fix approach:** Consolidate into a single `test-sandflox.sh` that covers all tiers, or have `test-sandbox.sh` and `verify-sandbox.sh` source shared test helpers.

### No shellcheck or linting for bash scripts (LOW)
- **Severity:** Low
- **Files:** `sandflox`, `test-policy.sh`, `test-sandbox.sh`, `verify-sandbox.sh`
- **Details:** No CI or local linting configuration exists. The bash scripts use some patterns that `shellcheck` would flag (e.g., `eval echo "~"`, unquoted variables in some contexts). No `.shellcheckrc` or CI pipeline definition.
- **Impact:** Style inconsistencies and potential bugs go undetected until runtime.
- **Fix approach:** Add a `.shellcheckrc` and run `shellcheck` on all `.sh` files and the `sandflox` script.

## Missing Infrastructure

### No CI/CD pipeline (MEDIUM)
- **Impact:** Tests must be run manually. No automated verification that changes don't break sandbox enforcement. For a security tool, this is a notable gap — a broken test that passes locally but fails on a different OS version would go unnoticed.
- **What should exist:** A GitHub Actions workflow that runs `test-policy.sh` and `test-sandbox.sh` on macOS and Linux runners.

### No automated testing on Linux (MEDIUM)
- **Impact:** The Linux/bwrap code path (`_sfx_generate_bwrap`, lines 295-351) has no test coverage beyond manual execution. The bwrap flag generation handles `--ro-bind`, `--bind`, `--tmpfs`, `--unshare-net`, `--unshare-pid` but is untested in CI.
- **What should exist:** A Linux CI job (or container-based local test) that exercises bwrap enforcement.

### No policy.toml schema validation (LOW)
- **Impact:** A typo in `policy.toml` (e.g., `mode = "workpace"` instead of `"workspace"`) falls through to the `workspace|*` case in SBPL generation (line 245: `workspace|*)`), silently applying workspace mode. Network mode typos similarly fall through to `blocked|*)` (line 279).
- **What should exist:** Validate parsed policy values against known enums (`blocked`/`unrestricted` for network, `permissive`/`workspace`/`strict` for filesystem) and fail with a clear error on unknown values.

### No `--help` or `--version` flags (LOW)
- **Impact:** Users must read the README to understand options. No way to check which version of sandflox is running.
- **What should exist:** `./sandflox --help` and `./sandflox --version` flags.

## Recommended Improvements

### Priority 1 (Address Soon)

- **Fix fs-filter.sh multi-argument handling:** The `tee` and `rm` wrappers must check ALL arguments, not just the last one. This is a security bypass in the shell enforcement tier. Files: `sandflox` (fs-filter generation in the manifest.toml hook), `.flox/env/manifest.toml`.

- **Extract duplicated TOML parser into shared module:** The 45-line fallback TOML parser and 30-line policy resolution logic exist in two identical copies. Extract to `_sfx_parse_policy.py` and call from both `sandflox` and the `manifest.toml` hook. This eliminates the highest-risk maintenance burden.

- **Add policy.toml schema validation:** Reject unknown `mode` values for network and filesystem sections. A typo like `mode = "workpace"` currently silently applies the wrong policy. Add an `assert` or explicit check in the Python parser.

### Priority 2 (Address Eventually)

- **Consolidate python3 invocations in `sandflox`:** Replace the 6+ separate `python3 -c` calls (lines 133-137) with a single invocation that writes all needed values. This cuts 300-500ms from startup.

- **Expand default denied paths in `policy.toml`:** Add `~/.kube/`, `~/.docker/`, `~/.azure/`, `~/.netrc`, `~/.npmrc` to the default deny list. These contain credentials that agents should not access.

- **Consolidate test scripts:** Merge `test-sandbox.sh` and `verify-sandbox.sh` into `test-policy.sh` (or a new unified `test-sandflox.sh`). Eliminate duplicated test helper functions.

- **Add CI pipeline:** Create a GitHub Actions workflow running tests on macOS and Linux. For a security-focused tool, automated verification is essential.

### Priority 3 (Nice to Have)

- **Replace `eval echo "~"` with `$HOME`:** Minor cleanup across `sandflox` (lines 194, 298, 475). Eliminates a hazardous pattern.

- **Add `--help` and `--version` to `sandflox`:** Improve discoverability. `--version` could read from `policy.toml` meta section or a dedicated constant.

- **Add shellcheck linting:** Create `.shellcheckrc`, run `shellcheck sandflox test-*.sh verify-*.sh`. Fix flagged issues.

- **Investigate `sandbox-exec` deprecation path:** Research Apple's Endpoint Security framework or App Sandbox as future alternatives. Document the migration plan before Apple removes `sandbox-exec`.

- **Optimize requisites symlink rebuild:** Cache-check the `bin/` directory against `requisites.txt` mtime before rebuilding on every activation.

---

*Concerns audit: 2026-04-15*
