# Domain Pitfalls

**Domain:** macOS sandbox-exec Go binary (Flox-distributed CLI)
**Researched:** 2026-04-15

---

## Critical Pitfalls

Mistakes that cause rewrites, security bypasses, or broken distribution.

---

### Pitfall 1: SBPL Profile Blocks Nix Store and Flox Daemon Communication

**What goes wrong:** A `(deny default)` (whitelist) SBPL profile blocks reads from `/nix/store`, the Nix daemon unix socket, Flox cache directories, and macOS system frameworks. The sandboxed `flox activate` silently fails or hangs because it cannot reach the Nix daemon or read store paths.

**Why it happens:** The Nix/Flox ecosystem requires access to an unpredictable and constantly-changing set of paths: `/nix/store/<hash>-*` for every package and transitive dependency, the Nix daemon socket at `/nix/var/nix/daemon-socket/socket`, `~/.cache/flox/`, `~/.local/share/flox/`, `~/.config/flox/`, and macOS system frameworks under `/System/Library/` and `/usr/lib/`. A deny-default profile requires enumerating all of these -- impossible to maintain as packages change.

**Consequences:** Flox activation fails inside the sandbox. The binary appears to work outside sandbox-exec but breaks when actually sandboxed. This manifests as cryptic errors from the Nix daemon or missing library errors, not as clear sandbox violations.

**Prevention:**
- Use `(allow default)` baseline and deny specific operations (the current v2 approach). This is not a compromise -- it is the correct design for wrapping developer toolchains. OpenAI Codex took the same approach for their Seatbelt sandbox.
- Explicitly allow `/nix/store` read access: `(allow file-read* (subpath "/nix/store"))`.
- Always allow unix sockets: `(allow network* (remote unix-socket))` for Nix daemon IPC.
- Test the generated SBPL by running `flox activate -- echo ok` under sandbox-exec before any other tests.

**Detection:** `flox activate` hangs or exits with "nix daemon connection error" inside sandbox. Check system log with `log stream --predicate 'eventMessage contains "sandbox"' --level debug`.

**Phase:** Phase 1 (Core Go binary). SBPL generation must be correct from the first working build.

---

### Pitfall 2: sandbox-exec Deprecation Creates a Ticking Clock

**What goes wrong:** Apple deprecated `sandbox-exec` circa 2016. While it still works on macOS Sequoia 15.x (and Apple uses it internally for system daemons), Apple could remove it in a future macOS release without notice. The SBPL profile language is undocumented for third-party use.

**Why it happens:** Apple wants developers to use the App Sandbox (requires entitlements + .app bundle) or the Endpoint Security framework (requires an Apple-signed system extension). Neither of these works for a CLI tool distributed via Flox. There is no announced removal timeline, and `sandbox-exec` is deeply entrenched in macOS internals (Chromium, Firefox, and Apple's own daemons use it), but the risk is nonzero.

**Consequences:** A future macOS update silently removes kernel enforcement, leaving only shell-level protection. Users on the new macOS version have a degraded security posture without knowing it.

**Prevention:**
- The existing graceful degradation (fall back to shell-only when sandbox-exec is missing) is the correct pattern. Preserve this in the Go binary.
- Add a version-check mechanism: at startup, verify `sandbox-exec` exists and probe that a minimal profile works (`sandbox-exec -p '(version 1)(allow default)' /usr/bin/true`). Log a clear warning if the probe fails.
- Monitor Apple's WWDC announcements and macOS release notes annually.
- Document the Endpoint Security framework as the long-term migration path (requires a signed system extension and Apple Developer Program membership).
- Consider `alcoholless` (NTT Labs' open-source sandbox alternative) as an intermediate fallback if Apple removes sandbox-exec before Endpoint Security becomes viable for CLI tools.

**Detection:** `command -v sandbox-exec` returns empty after macOS upgrade. The probe command above fails.

**Phase:** Phase 1 (Core Go binary) for graceful degradation. Phase 3+ for monitoring/migration research.

---

### Pitfall 3: Go TOML Parsing Without External Dependencies

**What goes wrong:** Go's standard library has no TOML parser. The project constraint is zero external Go dependencies (matching flox-bwrap). Building a custom TOML parser replicates the same bug class as the current Python `_tomllib` mini-parser: silent misparses of edge cases (multiline strings, inline tables, escaped quotes, dotted keys). A policy misparse leads to a permissive security posture.

**Why it happens:** The `policy.toml` schema is simple today (flat sections, string values, string arrays, booleans), but TOML has many edge cases. A hand-rolled parser that works for the current schema will silently break when users add perfectly valid TOML that the parser does not handle.

**Consequences:** A `policy.toml` with inline tables, multiline strings, or escaped characters is silently misparsed. Denied paths are missed, filesystem mode defaults to permissive, network mode defaults to unrestricted. The sandbox appears to work but enforces the wrong policy.

**Prevention:**
- Write a purpose-built parser for the exact `policy.toml` v2 schema, NOT a general TOML parser. The schema uses only: sections `[a.b]`, string values `key = "val"`, string arrays `key = ["a", "b"]`, booleans `key = true/false`. No multiline strings, no inline tables, no dotted keys.
- Add strict validation: reject any TOML construct the parser does not handle (e.g., triple-quoted strings, `{`, `[[`). Fail loudly with `[sandflox] ERROR: unsupported TOML syntax in policy.toml` rather than silently misparsing.
- Validate parsed values against known enums: filesystem mode must be `permissive|workspace|strict`, network mode must be `blocked|unrestricted`. Reject unknown values instead of falling through to a default.
- Add a `policy.toml` validation test that covers every supported construct and explicitly tests rejection of unsupported constructs.
- If the zero-dependency constraint is ever relaxed, `github.com/BurntSushi/toml` is the correct choice (TOML v1.1, reflection-based like encoding/json, well-maintained).

**Detection:** Parse a policy.toml with `mode = "workpace"` (typo). If the binary does not error, validation is missing.

**Phase:** Phase 1 (Core Go binary). The parser is the foundation of everything else.

---

### Pitfall 4: macOS Sandbox Blocks sysctl on Apple Silicon (ARM64)

**What goes wrong:** macOS sandbox profiles can block access to hardware-capability sysctl keys like `hw.optional.neon`, `machdep.ptrauth_enabled`, and others. Programs that probe CPU features at startup (Qt, some JIT runtimes, `uv`, Node.js) crash or abort when running inside a sandbox-exec profile that does not explicitly allow sysctl access.

**Why it happens:** Apple's sandbox restricts `sysctl` operations by default in tighter profiles. With `(allow default)`, sysctl access is permitted, but if the profile is ever tightened (e.g., moving toward deny-default for specific operations), this breaks tools that depend on CPU feature detection.

**Consequences:** Tools installed in the Flox environment crash inside the sandbox on Apple Silicon Macs. The crash message is opaque ("Incompatible processor", "illegal instruction", panic). Users blame sandflox.

**Prevention:**
- Maintain `(allow default)` as the baseline. This inherently allows sysctl access.
- If future profiles restrict `sysctl`, explicitly add: `(allow sysctl-read)`.
- Test on both Intel and Apple Silicon Macs. The sysctl issue is ARM64-specific.
- Include `python3`, `node` (if installed), and `git` in smoke tests under the sandbox on Apple Silicon.

**Detection:** Tools crash with SIGILL or "Incompatible processor" only when run under sandbox-exec on Apple Silicon.

**Phase:** Phase 2 (Testing). Must be part of the Apple Silicon test matrix.

---

### Pitfall 5: syscall.Exec on macOS Can SIGILL Due to Preemption Signals

**What goes wrong:** Go's `syscall.Exec` on macOS can cause the newly exec'd process to receive SIGILL (illegal instruction), crashing the launched process. This is a macOS kernel bug where a signal delivered during `execve` corrupts the new process's signal handling.

**Why it happens:** Go's runtime uses preemption signals (SIGURG) for goroutine scheduling. If a preemption signal is pending at the exact moment `syscall.Exec` calls `execve`, macOS mishandles it and the new process gets SIGILL. This was a release-blocker for Go 1.16 and was fixed by using `execLock` to suppress preemption signals during exec.

**Consequences:** `sandflox` occasionally crashes `flox activate` or `sandbox-exec` at launch. The crash is intermittent and difficult to reproduce, appearing as a random SIGILL in the child process.

**Prevention:**
- Use Go 1.16+ (the fix was backported to 1.15.6). Any modern Go version (1.21+) includes the fix.
- Verify the Go version in the Nix expression: `buildGoModule` should use a Go version >= 1.21.
- If users report intermittent SIGILL at launch, check the Go version first.
- Do not add goroutine work (timers, goroutine spawning) between argument parsing and the `syscall.Exec` call. Keep the hot path to exec as simple as possible.

**Detection:** Intermittent SIGILL crashes in `flox activate` or `sandbox-exec` when launched by the Go binary.

**Phase:** Phase 1 (Core Go binary). Set Go version constraint in the Nix expression.

---

## Moderate Pitfalls

---

### Pitfall 6: buildGoModule vendorHash Management for Zero-Dependency Projects

**What goes wrong:** `buildGoModule` requires a `vendorHash` parameter. For a zero-dependency project (empty `go.sum`), setting `vendorHash` incorrectly causes the build to fail. Setting it to `null` tells Nix to use the vendored `vendor/` directory, which does not exist. Leaving it as an empty string or a sha256 hash of nothing requires the exact right incantation.

**Why it happens:** `buildGoModule` always runs a vendor phase. With no dependencies, the vendor output is an empty directory, but Nix still hashes it. The hash of "empty vendor" is a specific value that must be provided, and it changes when Go toolchain versions change the vendor directory format.

**Prevention:**
- For a zero-dependency Go module, use `vendorHash = null;` in the Nix expression. This skips the vendor fetch phase entirely and uses the source repository's (nonexistent) vendor directory, which is the correct behavior for no-dep projects.
- Alternative: create an empty `vendor/` directory and `vendor/modules.txt` in the repo. This makes `vendorHash = null;` work unambiguously.
- Reference the flox-bwrap Nix expression for the exact pattern.
- Test `flox build` early and often. The vendorHash mismatch error is clear ("hash mismatch") but confusing if you do not know the pattern.

**Detection:** `flox build` fails with "hash mismatch in fixed-output derivation" during the vendor phase.

**Phase:** Phase 1 (Core Go binary). The Nix expression is the first thing to get right.

---

### Pitfall 7: Environment Variable Handling Differences Between Bash and Go

**What goes wrong:** The current Bash script inherits and manipulates the shell environment seamlessly (PATH wipe, variable unsetting, function exports). Go's `syscall.Exec` replaces the process, but environment setup before exec requires explicit construction. Subtle differences in how Go handles environment variables vs. Bash lead to missing variables, duplicate entries, or leaked state.

**Why it happens:** Go's `os.Environ()` returns a `[]string` of `KEY=VALUE` pairs. Appending a variable that already exists creates a duplicate (both values present, behavior is platform-dependent). The current Bash script uses `export PATH=`, `unset VAR`, and `export -f func` -- none of which have direct Go equivalents for the exec'd process.

**Consequences:**
- PATH not properly wiped: agent sees system binaries.
- `FLOX_ENV_PROJECT` not unset: agent discovers escape paths.
- Duplicate environment entries cause unpredictable behavior in the child process.
- Function armor (`export -f`) cannot be set from Go -- it is a bash-only feature.

**Prevention:**
- Build the environment explicitly: start from `os.Environ()`, filter out variables to remove, deduplicate, then set the result on `syscall.Exec`'s `env` parameter.
- Write a helper function `buildEnv(remove []string, set map[string]string) []string` that handles deduplication.
- For function armor: generate a bash init script that the entrypoint sources, rather than trying to `export -f` from Go.
- For PATH: set `PATH=$SANDFLOX_BIN_DIR` explicitly in the env slice.
- Test: after exec, `env | sort` should show no duplicates and no leaked variables.

**Detection:** Run `env | grep FLOX_ENV_PROJECT` inside the sandbox. If it has a value, breadcrumb cleanup failed.

**Phase:** Phase 1 (Core Go binary). Environment construction is core to the exec chain.

---

### Pitfall 8: Shell Enforcement Cannot Be Set From Go (Function Armor, fs-filter)

**What goes wrong:** The current shell enforcement (function armor shadowing 27 package managers, fs-filter wrappers for `cp`/`mv`/`rm`/etc.) relies on `export -f` to inject bash functions into the child shell. Go cannot set bash functions in a child process -- `export -f` is a bashism that modifies the current shell's function table, not an environment variable.

**Why it happens:** Bash's `export -f func_name` creates environment variables like `BASH_FUNC_func_name%%=() { body; }`. Go could theoretically set these env vars, but it is fragile, version-dependent (bash changed the format after ShellShock), and breaks in non-bash shells (zsh, fish).

**Consequences:** If the Go binary simply execs `sandbox-exec ... flox activate`, the interactive shell gets function armor from `profile.common` (because it sources the manifest profile). But `sandflox -- CMD` mode skips profile.common, and the generated entrypoint.sh must be the sole source of shell enforcement. If entrypoint.sh generation is wrong, function armor and fs-filter are missing in non-interactive mode.

**Prevention:**
- Generate `entrypoint.sh` from Go (write to cache dir) that contains all shell enforcement: function armor, fs-filter wrappers, PATH setup, breadcrumb cleanup.
- For interactive mode: rely on `profile.common` in the manifest (existing behavior).
- For non-interactive mode: exec `sandbox-exec ... flox activate -- bash entrypoint.sh CMD`.
- The Go binary's job is to **generate** shell scripts, not to **be** the shell enforcement. The binary is the orchestrator; bash scripts are the enforcement layer.
- Consolidate the function armor list into a single source of truth (e.g., a Go constant array) that generates both entrypoint.sh and is documented.

**Detection:** Run `sandflox -- type flox` inside the sandbox. If it does not print the blocked function, function armor is missing.

**Phase:** Phase 1 (Core Go binary). Entrypoint generation is part of the core exec chain.

---

### Pitfall 9: /tmp vs /private/tmp Symlink on macOS

**What goes wrong:** On macOS, `/tmp` is a symlink to `/private/tmp`. An SBPL rule allowing writes to `(subpath "/tmp")` does not match actual filesystem operations that resolve to `/private/tmp`. The sandbox denies writes to temporary files, breaking tools that use `/tmp`.

**Why it happens:** macOS uses the `/private` prefix for several system directories (`/tmp -> /private/tmp`, `/var -> /private/var`, `/etc -> /private/etc`). Sandbox-exec resolves paths before matching, so the SBPL must use the canonical path.

**Consequences:** Tools that write to `/tmp` or `$TMPDIR` (which is under `/private/var/folders/`) get EPERM. Build tools, test frameworks, and many CLI tools fail inside the sandbox.

**Prevention:**
- Always use `/private/tmp` and `/private/var/folders` in SBPL rules (the current v2 script already does this correctly).
- When translating user-specified paths in `policy.toml` (e.g., `writable = ["/tmp"]`), resolve symlinks before generating SBPL: use Go's `filepath.EvalSymlinks()`.
- Also allow `$TMPDIR` paths, which are under `/private/var/folders/<user>/`.
- Test: `sandbox-exec -f profile.sb touch /tmp/test-sandflox` should succeed.

**Detection:** Tools fail with "Operation not permitted" when writing to `/tmp` inside the sandbox.

**Phase:** Phase 1 (Core Go binary). Path resolution in SBPL generation.

---

### Pitfall 10: sandbox-exec Child Process Inheritance Gotchas

**What goes wrong:** Processes forked by a sandboxed process inherit the sandbox profile. But the inheritance model has edge cases: `process-exec` rules in the profile control which binaries can be exec'd, and a too-restrictive profile blocks `flox`, `bash`, `git`, or other tools the agent needs. Conversely, the sandbox cannot apply a *different* profile to child processes.

**Why it happens:** sandbox-exec applies one profile to the initial process. All children inherit it. There is no per-child-process policy. The `(allow process-exec)` / `(deny process-exec)` operations control which executables can run, but with `(allow default)`, all process-exec is allowed.

**Consequences:** With `(allow default)`, this is not a problem (all children inherit the same permissive-with-denials profile). But if the profile is ever tightened to restrict `process-exec`, tools in `/nix/store` may be blocked. The binary paths in `/nix/store` include content hashes that change on every package update, making `(allow process-exec (literal "/nix/store/..."))` rules unmaintainable.

**Prevention:**
- Use `(allow process-exec)` globally (implied by `(allow default)`). Do not attempt per-binary exec filtering in SBPL -- use shell-level enforcement (PATH restriction, requisites filtering) for that purpose.
- Document this design decision: kernel tier controls *what the system allows* (writes, network), shell tier controls *what tools are reachable* (PATH, functions). Do not mix the two.
- If future hardening requires exec filtering, use `(allow process-exec (subpath "/nix/store"))` as a broad allow.

**Detection:** Tools in the Flox environment fail with "spawn: Operation not permitted" inside the sandbox.

**Phase:** Phase 1 (Core Go binary). SBPL design.

---

### Pitfall 11: Go Build Leaks Absolute Paths into Binary

**What goes wrong:** Go embeds absolute source file paths in compiled binaries (for stack traces, debugging). When built via `buildGoModule` in Nix, these paths are Nix store paths (e.g., `/nix/store/abc123-sandflox-src/main.go`). This leaks build environment information and can cause reproducibility issues.

**Why it happens:** Go's default build behavior includes source paths in the binary's DWARF debugging info and runtime panic traces. `buildGoModule` in nixpkgs strips some of this, but not all.

**Consequences:** Binary contains Nix store paths that differ across builds, breaking bit-for-bit reproducibility. Stack traces show long Nix store paths that are hard to read.

**Prevention:**
- Use `-trimpath` in Go build flags: `buildFlags = [ "-trimpath" ];` in the Nix expression. This replaces absolute paths with module-relative paths.
- Combine with `-ldflags "-s -w"` to strip debugging symbols and reduce binary size.
- Use `-ldflags "-X main.Version=${version}"` for build-time version injection (the flox-bwrap pattern).

**Detection:** Run `strings sandflox | grep /nix/store`. If paths appear, `-trimpath` is missing.

**Phase:** Phase 1 (Core Go binary). Set in the Nix expression from the start.

---

### Pitfall 12: flox publish Clean Build Reveals Hidden Dependencies

**What goes wrong:** `flox publish` clones the repo to a temp location and performs a clean build. The build succeeds locally because the dev environment has tools/files available that are not declared in the Nix expression. The clean build fails because those implicit dependencies are missing.

**Why it happens:** Local `flox build` runs inside the developer's environment, which may have `go`, `git`, system headers, or other tools available. `flox publish` uses `sandbox = "pure"` mode, which strips everything not explicitly declared.

**Consequences:** `flox build` works locally, `flox publish` fails. The error messages from the Nix sandbox are often opaque ("No such file or directory" for a missing tool, or "cannot find package" for a missing Go dependency).

**Prevention:**
- Test with `flox build` in a clean environment before attempting `flox publish`.
- Ensure the Nix expression declares ALL build inputs (Go compiler, git if needed for version stamps).
- For a zero-dependency Go project, ensure `go.mod` and `go.sum` are committed and up-to-date.
- Use `flox build` early in development -- do not wait until the binary is "done" to try building.

**Detection:** `flox publish` fails with build errors that do not reproduce locally.

**Phase:** Phase 1 (Core Go binary). Set up the Nix expression and test `flox build` first.

---

## Minor Pitfalls

---

### Pitfall 13: macOS DNS Resolution Broken with CGO_ENABLED=0

**What goes wrong:** Go binaries compiled with `CGO_ENABLED=0` on macOS use Go's pure DNS resolver, which does not support macOS-specific DNS configuration (VPN split-DNS, `/etc/resolver/` directory, mDNS). Network operations may fail or resolve to wrong addresses.

**Why it happens:** macOS DNS resolution uses `libSystem` (the system C library). With cgo disabled, Go falls back to reading `/etc/resolv.conf`, which does not capture macOS's full DNS configuration.

**Prevention:**
- sandflox itself does not need DNS resolution (it execs local processes). This is a non-issue for the binary itself.
- However, if `CGO_ENABLED=0` is set in the Nix expression, document that this is intentional and does not affect sandboxed processes (which are separate processes with their own DNS resolution).
- Go 1.20+ improved macOS DNS by making direct syscalls even without cgo, but the improvement is partial.

**Detection:** DNS lookups fail from within sandflox's Go code. Since sandflox does not make network calls, this is unlikely to manifest.

**Phase:** Phase 1 (informational). Include `CGO_ENABLED = "0"` in the Nix expression comment explaining why it is safe.

---

### Pitfall 14: fs-filter.sh Only Checks Last Argument (Inherited Bug)

**What goes wrong:** The shell-level `fs-filter.sh` wrappers for `tee`, `rm`, `rmdir` only validate the last argument (`${!#}`) as the write target. For multi-target commands (`tee file1 file2`, `rm file1 file2`), only the last target is checked. An agent can bypass shell enforcement by placing the restricted path as a non-final argument.

**Why it happens:** The original Bash implementation took a shortcut for `cp` and `mv` (where the last arg is the destination), then applied the same pattern to all commands.

**Consequences:** An agent can `echo secrets | tee /tmp/allowed ~/.ssh/exfil` and only `/tmp/allowed` is checked. The kernel tier (sandbox-exec) still blocks the write to `~/.ssh/` via denied path rules, but the shell error message is wrong (no `[sandflox] BLOCKED:` feedback), and `flox activate` users without `./sandflox` lose protection entirely.

**Prevention:**
- In the Go binary's entrypoint.sh generation, fix the fs-filter logic:
  - `tee`, `rm`, `rmdir`: check ALL non-flag arguments.
  - `cp`, `mv`: check the last argument (destination).
  - `ln`: handle both `ln target linkname` (check linkname) and `ln target` (check current dir).
- Add test cases: `tee /tmp/ok ~/.ssh/test` should be blocked, `rm ~/.ssh/key1 ~/.ssh/key2` should be blocked.

**Detection:** `echo test | tee /tmp/ok /restricted/path` -- if only `/tmp/ok` is validated, the bug is present.

**Phase:** Phase 1 (Core Go binary). Fix during entrypoint.sh generation rewrite.

---

### Pitfall 15: Python usercustomize.py Injection Path

**What goes wrong:** The current implementation injects `usercustomize.py` via `PYTHONPATH` and `PYTHONUSERBASE`. If the sandboxed Python process modifies `sys.path` or runs with `-S` (no site), the monkey-patch is not loaded and Python write enforcement is bypassed.

**Why it happens:** `usercustomize.py` is loaded by the `site` module during Python startup. If `site` is disabled (`-S` flag) or `PYTHONPATH` is cleared, the patch is never applied.

**Prevention:**
- Document that Python enforcement is "best-effort, agent-friendly" -- not a security boundary. The kernel tier is the real enforcement.
- Use `PYTHONSTARTUP` as an additional injection vector (works even with `-S` in some configurations).
- In the Go binary, generate `usercustomize.py` from a Go template rather than a heredoc-in-heredoc-in-TOML (the current triple-nesting).
- Consider also patching `os.open`, `os.rename`, `os.symlink`, `pathlib.Path.write_text/write_bytes` for more complete coverage.

**Detection:** Run `python3 -S -c "open('/restricted/file', 'w')"` inside the sandbox. If the Python monkey-patch does not trigger, the kernel tier should still block it.

**Phase:** Phase 2 (Shell enforcement). Improve during entrypoint generation rewrite.

---

### Pitfall 16: Manifest Becomes Minimal but Must Still Work Standalone

**What goes wrong:** When the Go binary becomes the entrypoint, the manifest (`manifest.toml`) should become minimal (just `[install]` packages). But users who `flox activate` without `sandflox` still need shell enforcement. If the manifest hooks are removed entirely, `flox activate` provides zero protection.

**Why it happens:** The current architecture has shell enforcement in both the manifest (profile.common) AND the sandflox entrypoint. The Go rewrite aims to move everything into the binary, but the manifest is the only enforcement for users who run `flox activate` directly.

**Consequences:** Users who `flox activate` without sandflox get a completely unprotected environment. No PATH wipe, no function armor, no fs-filter.

**Prevention:**
- Keep minimal shell enforcement in the manifest even after the Go rewrite: at minimum, PATH wipe (`export PATH="$FLOX_ENV/bin"`) and breadcrumb cleanup (`unset FLOX_ENV_PROJECT`).
- Move the heavy enforcement (policy parsing, SBPL generation, requisites filtering, function armor, fs-filter) into the Go binary.
- Document clearly: `flox activate` = light shell protection, `sandflox` = full kernel + shell protection.
- Test both paths: `flox activate` alone and `sandflox` as entrypoint.

**Detection:** Run `flox activate` without sandflox, then `which brew`. If it returns a path, shell enforcement is missing.

**Phase:** Phase 2 (Manifest simplification). Do not strip manifest enforcement until the Go binary fully replaces it.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Phase 1: Go binary + SBPL generation | Nix store / Flox daemon blocked by SBPL (#1) | Use `(allow default)` baseline, test `flox activate` under sandbox |
| Phase 1: Go binary + Nix expression | vendorHash for zero-dep project (#6) | Use `vendorHash = null;`, test `flox build` immediately |
| Phase 1: Go binary + TOML parsing | Silent misparse of policy.toml (#3) | Purpose-built parser with strict validation, reject unknown constructs |
| Phase 1: Go binary + syscall.Exec | SIGILL on macOS (#5) | Require Go >= 1.21, minimize work before exec |
| Phase 1: Go binary + environment | Env var leaks and duplicates (#7) | Build env explicitly, deduplicate, test with `env \| sort` |
| Phase 1: Go binary + entrypoint | Function armor missing in -- CMD mode (#8) | Generate entrypoint.sh from Go, test `sandflox -- type flox` |
| Phase 1: Go binary + paths | /tmp vs /private/tmp (#9) | Use `filepath.EvalSymlinks()`, test writes to /tmp |
| Phase 1: Go binary + build | Path leaks in binary (#11) | Use `-trimpath` in build flags from day one |
| Phase 1: Go binary + distribution | Clean build fails (#12) | Test `flox build` before `flox publish` |
| Phase 2: Testing | Apple Silicon sysctl blocked (#4) | Test on ARM64, keep `(allow default)` |
| Phase 2: Shell enforcement | fs-filter last-arg bug (#14) | Fix during entrypoint.sh rewrite |
| Phase 2: Shell enforcement | Python bypass (#15) | Document as defense-in-depth, improve patches |
| Phase 2: Manifest | Standalone `flox activate` loses protection (#16) | Keep minimal enforcement in manifest |
| Phase 3+: Long-term | sandbox-exec removal (#2) | Monitor Apple, research Endpoint Security |

---

## Sources

- [macOS Sandbox - HackTricks](https://angelica.gitbook.io/hacktricks/macos-hardening/macos-security-and-privilege-escalation/macos-security-protections/macos-sandbox) -- SBPL reference, sandbox internals
- [How to build a replacement for sandbox-exec? - Apple Developer Forums](https://developer.apple.com/forums/thread/661939) -- Apple's position on sandbox-exec deprecation
- [Sandboxing on macOS - Mark Rowe](https://bdash.net.nz/posts/sandboxing-on-macos/) -- Comprehensive sandbox-exec analysis
- [macOS sandbox-exec HN discussion (2025)](https://news.ycombinator.com/item?id=47101200) -- Community experience reports
- [Go Issue #41702: syscall.Exec SIGILL on macOS](https://github.com/golang/go/issues/41702) -- SIGILL bug details and fix
- [OpenAI Codex Seatbelt Policy](https://github.com/openai/codex/blob/main/codex-rs/core/src/seatbelt_base_policy.sbpl) -- Reference SBPL implementation
- [OpenAI Codex macOS sysctl sandbox issue](https://github.com/openai/codex/issues/7099) -- ARM64 sysctl blocking
- [Codex seatbelt split filesystem policies PR](https://github.com/openai/codex/pull/13448) -- SBPL design lessons
- [NixOS/nixpkgs Go documentation](https://github.com/NixOS/nixpkgs/blob/master/doc/languages-frameworks/go.section.md) -- buildGoModule reference
- [Nix macOS sandbox issues](https://discourse.nixos.org/t/nix-macos-sandbox-issues-in-nix-2-4-and-later/17475) -- Nix + macOS sandbox conflicts
- [Flox Build and Publish docs](https://flox.dev/docs/tutorials/build-and-publish/) -- flox publish clean build behavior
- [Go macOS DNS resolution changes](https://danp.net/posts/macos-dns-change-in-go-1-20/) -- CGO_ENABLED=0 DNS implications
- [Issue(s) with flox publish - Flox Forum](https://discourse.flox.dev/t/issue-s-with-flox-publish/1152) -- flox publish gotchas
- [Alcoholless sandbox alternative](https://medium.com/nttlabs/alcoholless-a-lightweight-security-sandbox-for-macos-programs-homebrew-ai-agents-etc-ccf0d1927301) -- sandbox-exec alternative research
- [uv panics in sandbox-exec](https://github.com/astral-sh/uv/issues/17799) -- Real-world sandbox-exec breakage

---

*Pitfalls audit: 2026-04-15*
