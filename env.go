package main

import (
	"os"
	"sort"
	"strings"
)

// ── Environment Variable Sanitization ────────────────────

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
	// sandflox's own vars (set by manifest.toml [vars])
	"SANDFLOX_ENABLED", "SANDFLOX_MODE", "SANDFLOX_PROFILE",
}

// allowedPrefixes lists env var prefixes that pass through. Vars matching
// any prefix are included UNLESS they also match a blocked pattern.
var allowedPrefixes = []string{
	"FLOX_", // Flox runtime vars (breadcrumbs cleaned by entrypoint.sh)
	"NIX_",  // Nix store operation vars
	"__",    // macOS framework internals (__CF_*, __XPC_*, etc.)
	"LC_",   // Locale categories (LC_ALL, LC_CTYPE, etc.)
	"XPC_",  // macOS XPC service vars
}

// blockedPrefixes lists env var prefixes that are always blocked,
// even if they match an allowed prefix. Defense-in-depth against
// credential leakage from the parent shell.
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
// parent value. SEC-03 defense-in-depth: Python safety flags survive
// even if entrypoint.sh is bypassed.
var forcedVars = map[string]string{
	"PYTHONDONTWRITEBYTECODE": "1",
	"PYTHON_NOPIP":           "1",
}

// envToMap converts a []string of KEY=VALUE entries to a map.
// Uses SplitN(_, "=", 2) to correctly handle values containing '='.
func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// isBlocked returns true if key matches any blocked prefix or exact name.
func isBlocked(key string) bool {
	for _, prefix := range blockedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	for _, exact := range blockedExact {
		if key == exact {
			return true
		}
	}
	return false
}

// BuildSanitizedEnv constructs a filtered environment for the sandboxed
// process. Only allowlisted variables pass through; sensitive credentials
// are blocked by default. User-configured passthrough vars from
// policy.toml [security] env-passthrough bypass the block check.
//
// Phases:
//  1. Exact allowlist (POSIX essentials, sandflox vars)
//  2. Prefix allowlist (FLOX_*, NIX_*, LC_*, etc.) minus blocked
//  3. User-configured passthrough (bypasses block check)
//  4. Forced vars (PYTHONDONTWRITEBYTECODE, PYTHON_NOPIP)
//
// Output is sorted for deterministic --debug output and test assertions.
func BuildSanitizedEnv(cfg *ResolvedConfig) []string {
	envMap := envToMap(os.Environ())
	seen := make(map[string]bool)
	var result []string

	// Phase 1: exact allowlist -- POSIX essentials, sandflox vars
	for _, key := range defaultAllowlist {
		if isBlocked(key) {
			continue
		}
		if val, ok := envMap[key]; ok && !seen[key] {
			result = append(result, key+"="+val)
			seen[key] = true
		}
	}

	// Phase 2: prefix allowlist -- FLOX_*, NIX_*, LC_*, __*, XPC_*
	for _, prefix := range allowedPrefixes {
		for key, val := range envMap {
			if strings.HasPrefix(key, prefix) && !isBlocked(key) && !seen[key] {
				result = append(result, key+"="+val)
				seen[key] = true
			}
		}
	}

	// Phase 3: user-configured passthrough -- bypasses block check
	if cfg != nil {
		for _, key := range cfg.EnvPassthrough {
			if val, ok := envMap[key]; ok && !seen[key] {
				result = append(result, key+"="+val)
				seen[key] = true
			}
		}
	}

	// Phase 4: forced vars -- override any existing value
	for key, val := range forcedVars {
		entry := key + "=" + val
		if seen[key] {
			// Replace existing entry
			for i, e := range result {
				if strings.HasPrefix(e, key+"=") {
					result[i] = entry
					break
				}
			}
		} else {
			result = append(result, entry)
			seen[key] = true
		}
	}

	// Sort for deterministic output (Pitfall 5)
	sort.Strings(result)
	return result
}
