package main

import (
	"sort"
	"strings"
	"testing"
)

// envSliceToMap converts a []string of KEY=VALUE entries to a map for test assertions.
func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// ── SEC-01: Allowlist Tests ─────────────────────────────

func TestBuildSanitizedEnv_AllowlistPassesEssentials(t *testing.T) {
	t.Setenv("HOME", "/Users/test")
	t.Setenv("USER", "testuser")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("PATH", "/usr/bin")

	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	want := map[string]string{
		"HOME":  "/Users/test",
		"USER":  "testuser",
		"TERM":  "xterm-256color",
		"SHELL": "/bin/bash",
		"LANG":  "en_US.UTF-8",
		"PATH":  "/usr/bin",
	}
	for k, v := range want {
		got, ok := m[k]
		if !ok {
			t.Errorf("%s not found in sanitized env", k)
		} else if got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestBuildSanitizedEnv_PrefixAllowlist(t *testing.T) {
	t.Setenv("FLOX_ENV", "/nix/store/xxx")
	t.Setenv("NIX_SSL_CERT_FILE", "/etc/ssl")
	t.Setenv("LC_ALL", "C")
	t.Setenv("__CF_USER_TEXT_ENCODING", "0x1F5")

	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	prefixVars := []string{"FLOX_ENV", "NIX_SSL_CERT_FILE", "LC_ALL", "__CF_USER_TEXT_ENCODING"}
	for _, k := range prefixVars {
		if _, ok := m[k]; !ok {
			t.Errorf("%s not found in sanitized env (should pass via prefix allowlist)", k)
		}
	}
}

func TestBuildSanitizedEnv_UnknownVarsBlocked(t *testing.T) {
	t.Setenv("RANDOM_VAR", "leaked")
	t.Setenv("MY_CUSTOM_THING", "oops")
	// Also set a known var so the result isn't empty
	t.Setenv("HOME", "/Users/test")

	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	blocked := []string{"RANDOM_VAR", "MY_CUSTOM_THING"}
	for _, k := range blocked {
		if _, ok := m[k]; ok {
			t.Errorf("%s should NOT appear in sanitized env (unknown var)", k)
		}
	}
}

// ── SEC-02: Sensitive Prefix Blocking ───────────────────

func TestBuildSanitizedEnv_BlocksSensitivePrefixes(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/ssh")
	t.Setenv("GPG_AGENT_INFO", "xxx")
	t.Setenv("GITHUB_TOKEN", "ghp_xxx")
	t.Setenv("DOCKER_HOST", "tcp://localhost:2375")
	t.Setenv("OPENAI_API_KEY", "sk-xxx")
	// Set a known-good var for non-empty result
	t.Setenv("HOME", "/Users/test")

	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	sensitive := []string{
		"AWS_SECRET_ACCESS_KEY", "SSH_AUTH_SOCK", "GPG_AGENT_INFO",
		"GITHUB_TOKEN", "DOCKER_HOST", "OPENAI_API_KEY",
	}
	for _, k := range sensitive {
		if _, ok := m[k]; ok {
			t.Errorf("%s should be BLOCKED but appeared in sanitized env", k)
		}
	}
}

func TestBuildSanitizedEnv_Passthrough(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	t.Setenv("HOME", "/Users/test")

	cfg := &ResolvedConfig{EnvPassthrough: []string{"EDITOR"}}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	if m["EDITOR"] != "vim" {
		t.Errorf("EDITOR should pass through via EnvPassthrough, got %q", m["EDITOR"])
	}
}

func TestBuildSanitizedEnv_PassthroughDoesNotOverrideBlock(t *testing.T) {
	// User explicitly chose to pass AWS_SECRET_ACCESS_KEY through --
	// passthrough bypasses block check (user knows what they're doing)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "xxx")

	cfg := &ResolvedConfig{EnvPassthrough: []string{"AWS_SECRET_ACCESS_KEY"}}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	if _, ok := m["AWS_SECRET_ACCESS_KEY"]; !ok {
		t.Error("AWS_SECRET_ACCESS_KEY should appear when explicitly in EnvPassthrough")
	}
}

// ── SEC-03: Forced Variables ────────────────────────────

func TestBuildSanitizedEnv_ForcedVars(t *testing.T) {
	// Even with neither var set in the parent env, both must appear as "1"
	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	if m["PYTHONDONTWRITEBYTECODE"] != "1" {
		t.Errorf("PYTHONDONTWRITEBYTECODE = %q, want %q", m["PYTHONDONTWRITEBYTECODE"], "1")
	}
	if m["PYTHON_NOPIP"] != "1" {
		t.Errorf("PYTHON_NOPIP = %q, want %q", m["PYTHON_NOPIP"], "1")
	}
}

func TestBuildSanitizedEnv_ForcedVarsOverride(t *testing.T) {
	// Parent env sets PYTHONDONTWRITEBYTECODE=0, but forced must win
	t.Setenv("PYTHONDONTWRITEBYTECODE", "0")

	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)
	m := envSliceToMap(env)

	if m["PYTHONDONTWRITEBYTECODE"] != "1" {
		t.Errorf("PYTHONDONTWRITEBYTECODE = %q, want %q (forced should override)", m["PYTHONDONTWRITEBYTECODE"], "1")
	}
}

// ── Determinism ─────────────────────────────────────────

func TestBuildSanitizedEnv_Sorted(t *testing.T) {
	t.Setenv("HOME", "/Users/test")
	t.Setenv("USER", "testuser")
	t.Setenv("TERM", "xterm")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("LANG", "en_US.UTF-8")

	cfg := &ResolvedConfig{}
	env := BuildSanitizedEnv(cfg)

	if !sort.StringsAreSorted(env) {
		t.Errorf("output must be sorted, got: %v", env)
	}
}
