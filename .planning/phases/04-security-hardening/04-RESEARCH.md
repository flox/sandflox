# Phase 4: Security Hardening - Research

**Researched:** 2026-04-16
**Domain:** Environment variable sanitization, Go process exec environment control
**Confidence:** HIGH

## Summary

Phase 4 adds environment variable scrubbing to sandflox so that sensitive credentials (AWS keys, GitHub tokens, SSH agent sockets, GPG config, etc.) do not leak into the sandboxed agent session. The implementation is straightforward: before `syscall.Exec` replaces the sandflox process with the sandbox, the env slice must be filtered to an allowlist of safe variables plus Flox-required variables.

There are exactly two `os.Environ()` call sites that pass the unfiltered environment to `syscall.Exec`: `main.go:145` (the `execFlox` fallback path) and `exec_darwin.go:126` (the primary kernel enforcement path). Both must be replaced with a filtered env slice. SEC-03 (Python safety flags) is already handled by `entrypoint.sh.tmpl` -- the phase just needs an integration test proving the flags survive the exec boundary.

**Primary recommendation:** Create a `BuildSanitizedEnv()` function in a new `env.go` file that constructs a clean `[]string` from `os.Environ()` by matching against an allowlist (exact names + prefix patterns). Wire it into both exec call sites. Add a `[security]` section to policy.toml with `env-passthrough` for user-configurable additions. The allowlist, blocklist patterns, and forced-set vars are constants in Go code.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- all implementation choices are at Claude's discretion (infrastructure phase).

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions. Key constraints from prior phases:
- Zero external Go deps (stdlib only)
- Platform split via build tags (darwin/other)
- ResolvedConfig drives all generated artifacts
- Env scrubbing happens before exec into sandbox-exec (pre-sandbox entry)

### Deferred Ideas (OUT OF SCOPE)
None -- infrastructure phase.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SEC-01 | sandflox scrubs environment variables before passing them into the sandbox -- only allowlisted vars pass through (HOME, USER, TERM, SHELL, LANG, PATH, plus Flox-required vars) | Env allowlist pattern in `BuildSanitizedEnv()` function; wired into both `syscall.Exec` call sites |
| SEC-02 | sandflox blocks sensitive env vars by default (AWS_*, SSH_*, GPG_*, GITHUB_TOKEN, etc.) with a configurable allowlist in policy.toml | Blocked prefix patterns as Go constants; `[security] env-passthrough` array in policy.toml for user overrides |
| SEC-03 | sandflox sets `PYTHONDONTWRITEBYTECODE=1` and `PYTHON_NOPIP=1` inside the sandbox | Already implemented in `entrypoint.sh.tmpl` lines 63-64; phase adds integration test proving values survive exec |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go, stdlib only, zero external deps
- **Platform:** macOS only (Darwin) -- sandbox-exec is macOS-specific
- **Naming:** `_sfx_` prefix for internal vars, `SCREAMING_SNAKE_CASE` for exports, `[sandflox]` prefix for all messages
- **Error handling:** `[sandflox] ERROR:` prefix on stderr, exit 1 on hard errors
- **Diagnostics:** Package-level `stderr io.Writer` for testable output
- **Testing:** Table-driven subtests, golden-file comparisons where applicable, stdlib `testing` only
- **Code style:** Section separators `# -- Name ---`, inline comments explain *why*
- **Build tags:** `//go:build darwin` / `//go:build !darwin` for platform split

## Architecture Patterns

### Where Env Scrubbing Fits in the Pipeline

The existing main.go pipeline:
```
1. ParseFlags
2. resolveProjectDir
3. ParsePolicy
4. ResolveConfig
5. WriteCache
6. WriteShellArtifacts
7. emitDiagnostics
8. execWithKernelEnforcement -> syscall.Exec(sbxPath, argv, os.Environ())
```

Env scrubbing must happen at step 8, right before `syscall.Exec`. The function takes a `*ResolvedConfig` (for policy-driven passthrough vars) and returns a `[]string` in the `KEY=VALUE` format that `syscall.Exec` expects.

### Two Exec Call Sites

Both must use the sanitized env:

1. **`exec_darwin.go:126`** -- Primary path: `syscall.Exec(sbxPath, argv, os.Environ())` in `execWithKernelEnforcement`
2. **`main.go:145`** -- Fallback path: `syscall.Exec(floxPath, argv, os.Environ())` in `execFlox`

### Recommended File Structure
```
sandflox/
  env.go              # BuildSanitizedEnv() + constants (allowlist, blocked prefixes, forced vars)
  env_test.go         # Unit tests for env filtering logic
  exec_darwin.go      # Modified: pass sanitized env to syscall.Exec
  main.go             # Modified: pass sanitized env to execFlox
  config.go           # Modified: ResolvedConfig gains EnvPassthrough field
  policy.go           # Modified: parse [security] section
  policy_test.go      # New tests for [security] parsing
  policy.toml         # New [security] section
```

### Pattern 1: Allowlist-Based Environment Filtering
**What:** Construct env from scratch using an allowlist, rather than removing blocklisted items. This is defense-in-depth: unknown vars are blocked by default.
**When to use:** Always -- allowlist is more secure than blocklist alone. New sensitive vars (e.g., a future `AZURE_KEY`) are automatically excluded without code changes.

```go
// env.go
func BuildSanitizedEnv(cfg *ResolvedConfig) []string {
    parentEnv := os.Environ()
    envMap := envToMap(parentEnv)

    var result []string

    // 1. Pass through allowlisted vars (exact match)
    for _, key := range defaultAllowlist {
        if val, ok := envMap[key]; ok {
            result = append(result, key+"="+val)
        }
    }

    // 2. Pass through allowlisted prefixes (FLOX_*, LC_*, etc.)
    for _, prefix := range allowedPrefixes {
        for key, val := range envMap {
            if strings.HasPrefix(key, prefix) && !isBlocked(key) {
                result = append(result, key+"="+val)
            }
        }
    }

    // 3. Pass through user-configured passthrough vars
    for _, key := range cfg.EnvPassthrough {
        if val, ok := envMap[key]; ok {
            result = append(result, key+"="+val)
        }
    }

    // 4. Force-set sandflox vars
    for key, val := range forcedVars {
        result = append(result, key+"="+val)
    }

    return result
}
```

### Pattern 2: Policy-Driven Passthrough Configuration
**What:** `[security]` section in policy.toml allows users to explicitly pass additional env vars through the filter.
**When to use:** When a user needs a non-default env var inside the sandbox (e.g., a custom `EDITOR` or `PAGER`).

```toml
# policy.toml (new section)
[security]
env-passthrough = ["EDITOR", "PAGER", "COLORTERM"]
```

### Anti-Patterns to Avoid
- **Blocklist-only filtering:** Never filter by removing known-bad vars. The universe of secret-carrying env vars is unbounded. A new cloud provider SDK could introduce `NEWCLOUD_SECRET_KEY` tomorrow and the blocklist wouldn't catch it.
- **Mutating `os.Environ()` in place:** Go's `os.Environ()` returns a copy, but `os.Unsetenv()` or `os.Setenv()` would affect the sandflox process itself. Build a fresh slice instead.
- **Filtering after exec:** The entrypoint.sh already does breadcrumb cleanup (`unset FLOX_ENV_PROJECT` etc.), but this runs INSIDE the sandbox. The agent could read `/proc/self/environ` before entrypoint.sh runs on Linux (not applicable to macOS, but defense-in-depth still matters). Filter BEFORE exec.

## Standard Stack

No new libraries. This phase uses only Go stdlib:

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `os` | stdlib | `os.Environ()`, env var access | Process environment |
| `strings` | stdlib | Prefix matching, string manipulation | Env var key parsing |
| `syscall` | stdlib | `syscall.Exec` env parameter | Already used in exec paths |
| `sort` | stdlib | Deterministic env ordering for tests | Reproducible output |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Env var parsing | Custom `KEY=VALUE` parser | `strings.SplitN(entry, "=", 2)` | Handles values containing `=` correctly |
| Prefix matching | Regex-based pattern matcher | `strings.HasPrefix(key, prefix)` | Simpler, faster, no regex overhead |
| Env dedup | Manual dedup loop | Build into a `map[string]string` first | Map lookup is O(1), prevents duplicate keys |

## Common Pitfalls

### Pitfall 1: Breaking flox activate by stripping required Flox vars
**What goes wrong:** Flox sets `FLOX_ENV`, `FLOX_ENV_CACHE`, `FLOX_ENV_LIB`, `FLOX_ENV_SHARE`, `NIX_SSL_CERT_FILE`, and other vars that `flox activate` depends on. Stripping these causes cryptic flox failures.
**Why it happens:** Overly aggressive allowlist that only includes obvious vars like `HOME`, `PATH`, `SHELL`.
**How to avoid:** Use prefix-based allowlisting for `FLOX_*` vars (then the entrypoint.sh breadcrumb cleanup handles the sensitive ones). Also allow `NIX_*` vars since Nix store operations need them. Also allow `__*` (Nix internal double-underscore vars).
**Warning signs:** `flox activate` fails with "environment not found" or Nix store path errors.

### Pitfall 2: Breaking sandbox-exec by stripping macOS system vars
**What goes wrong:** `sandbox-exec` and macOS system frameworks need certain vars to function: `TMPDIR`, `__CF_USER_TEXT_ENCODING`, `XPC_SERVICE_NAME`, `SHELL`, `HOME`, `USER`, `LOGNAME`.
**Why it happens:** These vars look "unnecessary" but macOS system calls and framework initializers read them.
**How to avoid:** Include macOS system vars (`TMPDIR`, `__CF_*`, `XPC_*`) in the allowlist. The `__*` prefix allowlist handles most of these.
**Warning signs:** `sandbox-exec` crashes or hangs instead of starting the sandbox.

### Pitfall 3: Env var values containing equals signs
**What goes wrong:** A naive `strings.Split(entry, "=")` produces wrong results for vars like `PROMPT_COMMAND='if [ $? = 0 ]; then ...'`.
**Why it happens:** Splitting on all `=` characters instead of just the first one.
**How to avoid:** Use `strings.SplitN(entry, "=", 2)` which splits only on the first `=`.
**Warning signs:** Garbled env var values or missing vars inside the sandbox.

### Pitfall 4: SEC-03 appearing as duplicate work
**What goes wrong:** Developer adds `PYTHONDONTWRITEBYTECODE=1` and `PYTHON_NOPIP=1` to the Go env builder, but they're already set in `entrypoint.sh.tmpl:63-64`. The Go-level forced vars and shell-level exports could conflict or cause confusion.
**Why it happens:** SEC-03 is already implemented in the shell tier; the phase success criteria just needs verification.
**How to avoid:** For SEC-03, the implementation approach should be: (a) ensure the Go env builder does NOT strip `PYTHONDONTWRITEBYTECODE` or `PYTHON_NOPIP` if they're already set, and (b) optionally force-set them in the Go env builder for defense-in-depth (belt AND suspenders -- both shell and Go set them). The integration test is what matters.
**Warning signs:** Test sees the var empty because the Go layer stripped it before the shell layer could set it.

### Pitfall 5: Non-deterministic test output from env ordering
**What goes wrong:** `os.Environ()` returns vars in an unspecified order. `BuildSanitizedEnv()` iterates a map. Test assertions on the result depend on ordering.
**Why it happens:** Go map iteration order is randomized.
**How to avoid:** Sort the output `[]string` slice before returning from `BuildSanitizedEnv()`. This makes tests deterministic and also makes `--debug` output reproducible.
**Warning signs:** Flaky test failures.

### Pitfall 6: exec_other.go (non-darwin) also needs env scrubbing
**What goes wrong:** The `exec_other.go` stub calls `execFlox(userArgs)`, which passes `os.Environ()`. If env scrubbing only happens in `exec_darwin.go`, the fallback path on non-darwin leaks secrets.
**Why it happens:** Forgetting that `execFlox()` in `main.go` is the common fallback for both darwin and non-darwin.
**How to avoid:** Make `execFlox()` accept a `[]string` env parameter (or call `BuildSanitizedEnv()` internally). Since both `exec_darwin.go` and `exec_other.go` call `execFlox()` as their fallback, fixing `execFlox()` covers both paths.
**Warning signs:** Integration tests pass on macOS but secrets leak on non-darwin fallback.

### Pitfall 7: PATH must be set correctly for flox to find itself
**What goes wrong:** If `BuildSanitizedEnv()` strips or overwrites `PATH`, the `flox` binary (resolved via `exec.LookPath`) might not match what the child process sees.
**Why it happens:** `syscall.Exec` uses the env you pass, not the current process's PATH. If PATH is missing from the filtered env, the child process has no PATH.
**How to avoid:** Always include `PATH` in the allowlist. The entrypoint.sh will overwrite it with the sandflox bin directory anyway, but `flox activate`'s hook needs PATH to find binaries during activation. Let the parent's PATH pass through; the shell tier handles the final wipe.
**Warning signs:** `flox activate` fails with "command not found" errors during hook execution.

## Code Examples

### Core Env Filtering Function (Recommended Implementation)

```go
// env.go -- Environment variable sanitization (SEC-01, SEC-02).

package main

import (
    "os"
    "sort"
    "strings"
)

// defaultAllowlist contains exact env var names that always pass through
// the sandbox boundary. These are essential for shell operation, locale,
// terminal handling, and user identity.
var defaultAllowlist = []string{
    // POSIX essentials
    "HOME", "USER", "LOGNAME", "SHELL", "TERM", "PATH",
    // Locale
    "LANG", "LANGUAGE",
    // Terminal
    "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION",
    // macOS system
    "TMPDIR",
    // Sandflox's own vars (set by manifest.toml [vars])
    "SANDFLOX_ENABLED", "SANDFLOX_MODE", "SANDFLOX_PROFILE",
}

// allowedPrefixes lists env var prefixes that pass through. Vars matching
// any prefix are included UNLESS they also match blockedExact.
var allowedPrefixes = []string{
    "FLOX_",   // Flox runtime vars (breadcrumbs cleaned by entrypoint.sh)
    "NIX_",    // Nix store operation vars
    "__",      // macOS framework internals (__CF_*, __XPC_*, etc.)
    "LC_",     // Locale categories (LC_ALL, LC_CTYPE, etc.)
    "XPC_",    // macOS XPC service vars
}

// blockedPrefixes lists env var prefixes that are always blocked,
// even if they match an allowed prefix. Defense-in-depth.
var blockedPrefixes = []string{
    "AWS_", "AZURE_", "GCP_", "GCLOUD_",
    "SSH_", "GPG_",
    "DOCKER_", "KUBE",
    "OPENAI_", "ANTHROPIC_", "MISTRAL_",
    "GITHUB_", "GITLAB_", "BITBUCKET_",
    "HOMEBREW_",
    "NPM_", "YARN_", "CARGO_",
    "DATABASE_", "DB_", "REDIS_", "MONGO",
    "STRIPE_", "TWILIO_", "SENDGRID_",
    "SLACK_", "DISCORD_",
}

// blockedExact lists exact env var names that are always blocked.
var blockedExact = []string{
    "GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN",
    "HF_TOKEN", "HUGGING_FACE_HUB_TOKEN",
    "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
    "SECRET_KEY", "API_KEY", "API_SECRET",
    "PASSWORD", "PASSWD",
}

// forcedVars are always set in the sanitized env, overriding any
// parent value. SEC-03 defense-in-depth.
var forcedVars = map[string]string{
    "PYTHONDONTWRITEBYTECODE": "1",
    "PYTHON_NOPIP":           "1",
}

// BuildSanitizedEnv constructs a filtered environment for the sandboxed
// process. Only allowlisted variables pass through; sensitive credentials
// are blocked by default. User-configured passthrough vars from
// policy.toml [security] env-passthrough are also included.
func BuildSanitizedEnv(cfg *ResolvedConfig) []string {
    // ... implementation
}
```

### Policy Schema Extension

```toml
# policy.toml -- new [security] section
[security]
env-passthrough = []   # additional env vars to pass through (user override)
```

### Policy Parsing Extension

```go
// In policy.go, add SecuritySection to Policy struct:
type SecuritySection struct {
    EnvPassthrough []string
}

type Policy struct {
    Meta       MetaSection
    Network    NetworkSection
    Filesystem FilesystemSection
    Security   SecuritySection     // new
    Profiles   map[string]ProfileSection
}

// In mapToPolicy, add:
if sec, ok := sections["security"]; ok {
    if v, ok := sec["env-passthrough"]; ok {
        p.Security.EnvPassthrough = toStringSlice(v)
    }
}
```

### ResolvedConfig Extension

```go
// In config.go, add to ResolvedConfig:
type ResolvedConfig struct {
    // ... existing fields ...
    EnvPassthrough []string `json:"env_passthrough"`
}

// In ResolveConfig, add:
config.EnvPassthrough = policy.Security.EnvPassthrough
```

### Modified Exec Call Sites

```go
// exec_darwin.go -- replace os.Environ() with sanitized env
func execWithKernelEnforcement(cfg *ResolvedConfig, ...) {
    // ... existing code ...
    env := BuildSanitizedEnv(cfg)
    execErr := syscall.Exec(sbxPath, argv, env)
    // ...
}

// main.go -- execFlox takes config for env filtering
func execFlox(cfg *ResolvedConfig, userArgs []string) {
    // ... existing code ...
    env := BuildSanitizedEnv(cfg)
    execErr := syscall.Exec(floxPath, argv, env)
    // ...
}
```

### Unit Test Pattern

```go
// env_test.go
func TestBuildSanitizedEnv_BlocksSensitiveVars(t *testing.T) {
    // Set sensitive vars in test env
    t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCY")
    t.Setenv("GITHUB_TOKEN", "ghp_xxxxxxxxxxxxxxxxxxxx")
    t.Setenv("HOME", "/Users/test")
    t.Setenv("TERM", "xterm-256color")

    cfg := &ResolvedConfig{EnvPassthrough: nil}
    env := BuildSanitizedEnv(cfg)
    envMap := envSliceToMap(env)

    // Sensitive vars must be absent
    if _, ok := envMap["AWS_SECRET_ACCESS_KEY"]; ok {
        t.Error("AWS_SECRET_ACCESS_KEY should be blocked")
    }
    if _, ok := envMap["GITHUB_TOKEN"]; ok {
        t.Error("GITHUB_TOKEN should be blocked")
    }

    // Essential vars must be present
    if envMap["HOME"] != "/Users/test" {
        t.Errorf("HOME should pass through, got %q", envMap["HOME"])
    }
    if envMap["TERM"] != "xterm-256color" {
        t.Errorf("TERM should pass through, got %q", envMap["TERM"])
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `os.Environ()` passed verbatim to `syscall.Exec` | Filtered env via `BuildSanitizedEnv()` | This phase | Parent shell secrets no longer leak into sandbox |
| No `[security]` section in policy.toml | `[security] env-passthrough = [...]` | This phase | Users can explicitly pass needed vars through |
| PYTHONDONTWRITEBYTECODE set only in entrypoint.sh | Set in both Go env builder AND entrypoint.sh | This phase | Defense-in-depth: vars survive even if entrypoint.sh is bypassed |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (go1.26.0) |
| Config file | None -- `go test` uses defaults |
| Quick run command | `go test ./... -count=1` |
| Full suite command | `go test -tags integration ./... -count=1 -timeout 120s` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SEC-01 | Allowlisted vars pass, others blocked | unit | `go test ./... -run TestBuildSanitizedEnv -count=1` | Wave 0 |
| SEC-01 | Inside sandbox, `echo $HOME` works, `echo $AWS_SECRET_ACCESS_KEY` empty | integration | `go test -tags integration ./... -run TestEnvScrubbing -count=1 -timeout 120s` | Wave 0 |
| SEC-02 | Sensitive prefix patterns (AWS_*, SSH_*, etc.) blocked | unit | `go test ./... -run TestBuildSanitizedEnv_BlocksSensitive -count=1` | Wave 0 |
| SEC-02 | env-passthrough in policy.toml parsed and respected | unit | `go test ./... -run TestParsePolicy_SecuritySection -count=1` | Wave 0 |
| SEC-02 | Passthrough override allows specific vars through | unit | `go test ./... -run TestBuildSanitizedEnv_Passthrough -count=1` | Wave 0 |
| SEC-03 | PYTHONDONTWRITEBYTECODE=1 inside sandbox | integration | `go test -tags integration ./... -run TestEnvScrubbing_PythonFlags -count=1 -timeout 120s` | Wave 0 |
| SEC-03 | PYTHON_NOPIP=1 inside sandbox | integration | Same as above | Wave 0 |
| SEC-03 | `import ensurepip` fails inside sandbox | integration | Already exists: `TestShellEnforces_EnsurepipBlocked` | Exists |

### Sampling Rate
- **Per task commit:** `go test ./... -count=1`
- **Per wave merge:** `go test -tags integration ./... -count=1 -timeout 120s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `env.go` -- `BuildSanitizedEnv()` function + constants
- [ ] `env_test.go` -- Unit tests for env filtering (SEC-01, SEC-02)
- [ ] Integration test in `shell_integration_test.go` or new `env_integration_test.go` -- SEC-01 inside real sandbox
- [ ] `policy_test.go` additions -- `[security]` section parsing tests

## Open Questions

1. **Should `execFlox` signature change?**
   - What we know: `execFlox()` is called from `main.go` (no-policy fallback) and from `exec_other.go` (non-darwin fallback). Currently takes only `userArgs`.
   - What's unclear: Should it take a `*ResolvedConfig` parameter? In the no-policy-found fallback, there's no config -- we'd need a default config.
   - Recommendation: Change `execFlox()` signature to `execFlox(cfg *ResolvedConfig, userArgs []string)`. For the no-policy fallback, create a minimal `defaultSanitizedConfig()` that has empty passthrough. This ensures env scrubbing happens on ALL paths. If cfg is nil, fall back to `os.Environ()` (graceful degradation, matching existing behavior for edge cases).

2. **Should forced vars (`PYTHONDONTWRITEBYTECODE`, `PYTHON_NOPIP`) be set in Go OR in entrypoint.sh, or both?**
   - What we know: `entrypoint.sh.tmpl` already exports both vars. Adding them in Go means they're set before the shell even runs.
   - Recommendation: Set in BOTH places (defense-in-depth). Go-level ensures they survive even if entrypoint.sh fails to source. Shell-level ensures they're visible to interactive sessions that might somehow bypass Go-level vars. The success criteria just says "inside the sandbox, these are set" -- both approaches satisfy it.

3. **Should `--debug` log which env vars were scrubbed?**
   - What we know: `--debug` already logs profile, paths, SBPL rule count.
   - Recommendation: Yes -- add a debug line: `[sandflox] Env: N vars passed, M blocked (K forced)`. Do NOT log the actual values (they're secrets), but logging counts is safe and useful for debugging. Optionally log the NAMES of blocked vars at debug level (names are not secrets, values are).

## Sources

### Primary (HIGH confidence)
- Codebase analysis: `exec_darwin.go`, `main.go`, `config.go`, `policy.go`, `shell.go`, `templates/entrypoint.sh.tmpl`
- Go stdlib docs: `os.Environ()`, `syscall.Exec`, `strings.SplitN`, `sort.Strings` -- verified via training knowledge (stable APIs, no recent changes)
- Existing integration tests: `exec_integration_test.go`, `shell_integration_test.go` -- confirm subprocess test pattern

### Secondary (MEDIUM confidence)
- `.planning/research/FEATURES.md` -- env var scrubbing value analysis and competitive landscape
- `.planning/codebase/INTEGRATIONS.md` -- Flox env var inventory (FLOX_ENV, FLOX_ENV_CACHE, NIX_SSL_CERT_FILE, etc.)
- Phase 3 STATE.md decisions -- entrypoint.sh template patterns, breadcrumb cleanup approach

### Tertiary (LOW confidence)
- macOS framework var requirements (`__CF_*`, `XPC_*`) -- based on training knowledge of macOS internals, should be verified by integration test (if stripping these causes sandbox-exec to fail, the test catches it)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- stdlib only, no new deps, well-understood Go APIs
- Architecture: HIGH -- two clear call sites, pure function pattern matches existing codebase style
- Pitfalls: HIGH -- identified from codebase analysis (Flox var requirements, PATH handling, `strings.SplitN` for `=` in values)
- Policy schema extension: HIGH -- TOML parser already handles `[section]` + string arrays; `[security]` follows same pattern as `[network]` and `[filesystem]`

**Research date:** 2026-04-16
**Valid until:** 2026-05-16 (stable -- Go stdlib, no moving targets)
