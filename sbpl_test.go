// sbpl_test.go — Table-driven unit tests for GenerateSBPL and WriteSBPL.
//
// Assertions are stdlib-only: strings.Contains plus a tiny helper pair.
// Tests build a *ResolvedConfig literal directly (no ResolveConfig call)
// so each test is hermetic. Home is hardcoded to "/Users/x" so expected
// substrings are stable across machines.
//
// Test names must match .planning/phases/02-kernel-enforcement-sbpl-sandbox-exec/02-VALIDATION.md
// per-task map (TestGenerateSBPL_*, TestWriteSBPL).

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Assertion Helpers ───────────────────────────────────

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected output to NOT contain %q, got:\n%s", substr, s)
	}
}

// ── Test 1: KERN-02 — (allow default) baseline across all modes ──

func TestGenerateSBPL_AllowDefaultBaseline(t *testing.T) {
	combos := []struct {
		name    string
		fsMode  string
		netMode string
	}{
		{"workspace_blocked", "workspace", "blocked"},
		{"workspace_unrestricted", "workspace", "unrestricted"},
		{"strict_blocked", "strict", "blocked"},
		{"strict_unrestricted", "strict", "unrestricted"},
		{"permissive_blocked", "permissive", "blocked"},
		{"permissive_unrestricted", "permissive", "unrestricted"},
	}

	for _, c := range combos {
		t.Run(c.name, func(t *testing.T) {
			cfg := &ResolvedConfig{
				FsMode:  c.fsMode,
				NetMode: c.netMode,
			}
			out := GenerateSBPL(cfg, "/Users/x")

			assertContains(t, out, "(version 1)")
			assertContains(t, out, "(allow default)")

			// (version 1) must come before (allow default)
			vIdx := strings.Index(out, "(version 1)")
			aIdx := strings.Index(out, "(allow default)")
			if vIdx < 0 || aIdx < 0 || vIdx > aIdx {
				t.Errorf("expected (version 1) before (allow default), got:\n%s", out)
			}
		})
	}
}

// ── Test 2: KERN-01 — full canonical workspace+blocked profile shape ──

func TestGenerateSBPL_WorkspaceBlocked(t *testing.T) {
	cfg := &ResolvedConfig{
		Profile:        "default",
		FsMode:         "workspace",
		NetMode:        "blocked",
		AllowLocalhost: true,
		Writable:       []string{"/Users/x/proj", "/private/tmp"},
		ReadOnly:       []string{"/Users/x/proj/.git/", "/Users/x/proj/policy.toml"},
		Denied:         []string{"/Users/x/.ssh/", "/Users/x/.aws/"},
	}
	out := GenerateSBPL(cfg, "/Users/x")

	assertContains(t, out, "(version 1)")
	assertContains(t, out, "(allow default)")

	// Denied pairs (trailing slash stripped)
	assertContains(t, out, `(deny file-read* (subpath "/Users/x/.ssh"))`)
	assertContains(t, out, `(deny file-write* (subpath "/Users/x/.ssh"))`)
	assertContains(t, out, `(deny file-read* (subpath "/Users/x/.aws"))`)
	assertContains(t, out, `(deny file-write* (subpath "/Users/x/.aws"))`)

	// Filesystem workspace block
	assertContains(t, out, ";; ── Filesystem writes (workspace) ──")
	assertContains(t, out, "(deny file-write*)")
	assertContains(t, out, `(subpath (param "PROJECT"))`)
	assertContains(t, out, `(subpath "/private/tmp")`)
	assertContains(t, out, `(subpath "/private/var/folders")`)
	assertContains(t, out, `(subpath "/dev")`)
	assertContains(t, out, `(subpath (param "FLOX_CACHE"))`)
	assertContains(t, out, `(subpath "/Users/x/.config/flox")`)
	assertContains(t, out, `(subpath "/Users/x/.local/share/flox")`)

	// Read-only overrides: subpath vs literal
	assertContains(t, out, `(deny file-write* (subpath "/Users/x/proj/.git"))`)
	assertContains(t, out, `(deny file-write* (literal "/Users/x/proj/policy.toml"))`)

	// Network
	assertContains(t, out, ";; ── Network (blocked) ──")
	assertContains(t, out, "(deny network*)")
	assertContains(t, out, "(allow network* (remote unix-socket))")
	assertContains(t, out, `(allow network* (remote ip "localhost:*"))`)
}

// ── Test 3: KERN-03 — denied paths with trailing-slash stripping ──

func TestGenerateSBPL_DeniedPaths(t *testing.T) {
	// With non-empty Denied: pairs emitted, trailing slash stripped,
	// Flox-required overrides block appears.
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
		Denied:  []string{"/Users/x/.ssh/", "/Users/x/.aws/"},
	}
	out := GenerateSBPL(cfg, "/Users/x")

	assertContains(t, out, ";; ── Denied paths (sensitive data) ──")
	// Trailing slash IS stripped
	assertContains(t, out, `(deny file-read* (subpath "/Users/x/.ssh"))`)
	assertContains(t, out, `(deny file-write* (subpath "/Users/x/.ssh"))`)
	assertContains(t, out, `(deny file-read* (subpath "/Users/x/.aws"))`)
	assertContains(t, out, `(deny file-write* (subpath "/Users/x/.aws"))`)
	// Must NOT contain the with-slash form
	assertNotContains(t, out, `(subpath "/Users/x/.ssh/")`)
	assertNotContains(t, out, `(subpath "/Users/x/.aws/")`)

	// Flox-required overrides appears when Denied is non-empty
	assertContains(t, out, ";; ── Flox-required overrides ──")
	assertContains(t, out, `(allow file-read* (subpath (param "FLOX_CACHE")))`)
	assertContains(t, out, `(allow file-read* (subpath "/Users/x/.config/flox"))`)
	assertContains(t, out, `(allow file-write* (subpath "/Users/x/.config/flox"))`)

	// With Denied empty: BOTH blocks are absent (matches bash
	// conditional at sandflox.bash:209 — the Flox-required overrides
	// block is inside the same `if [ -s denied-paths.txt ]` guard)
	cfgEmpty := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
		Denied:  nil,
	}
	outEmpty := GenerateSBPL(cfgEmpty, "/Users/x")
	assertNotContains(t, outEmpty, "Denied paths (sensitive data)")
	assertNotContains(t, outEmpty, "Flox-required overrides")
}

// ── Test 4: KERN-07 — localhost rule toggle ──

func TestGenerateSBPL_LocalhostAllowed(t *testing.T) {
	t.Run("allow_localhost_true", func(t *testing.T) {
		cfg := &ResolvedConfig{
			FsMode:         "workspace",
			NetMode:        "blocked",
			AllowLocalhost: true,
		}
		out := GenerateSBPL(cfg, "/Users/x")
		assertContains(t, out, `(allow network* (remote ip "localhost:*"))`)
	})

	t.Run("allow_localhost_false", func(t *testing.T) {
		cfg := &ResolvedConfig{
			FsMode:         "workspace",
			NetMode:        "blocked",
			AllowLocalhost: false,
		}
		out := GenerateSBPL(cfg, "/Users/x")
		assertNotContains(t, out, "localhost:*")
	})
}

// ── Test 5: KERN-08 — unix socket always allowed in blocked mode ──

func TestGenerateSBPL_UnixSocketAlwaysAllowed(t *testing.T) {
	for _, allowLocalhost := range []bool{true, false} {
		cfg := &ResolvedConfig{
			FsMode:         "workspace",
			NetMode:        "blocked",
			AllowLocalhost: allowLocalhost,
		}
		out := GenerateSBPL(cfg, "/Users/x")
		if !strings.Contains(out, "(allow network* (remote unix-socket))") {
			t.Errorf("unix-socket rule missing when AllowLocalhost=%v:\n%s",
				allowLocalhost, out)
		}
	}
}

// ── Test 6: KERN-02 — parameter substitution strings ──

func TestGenerateSBPL_ParameterSubstitution(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out := GenerateSBPL(cfg, "/Users/x")

	// These are the param references the -D flags will resolve in
	// Plan 02-02 (sandbox-exec -D PROJECT=... -D FLOX_CACHE=...)
	assertContains(t, out, `(param "PROJECT")`)
	assertContains(t, out, `(param "FLOX_CACHE")`)
}

// ── Test 7: KERN-01 — WriteSBPL file behavior ──

func TestWriteSBPL(t *testing.T) {
	tmpDir := t.TempDir()
	content := "(version 1)\n"

	path, err := WriteSBPL(tmpDir, content)
	if err != nil {
		t.Fatalf("WriteSBPL returned error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "sandflox.sb")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	// File mode bits are 0644
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("expected mode 0644, got %o", perm)
	}

	// File content equals input
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != content {
		t.Errorf("expected content %q, got %q", content, string(got))
	}

	// WriteSBPL with non-existent nested cacheDir creates the directory
	nested := filepath.Join(tmpDir, "a", "b", "c")
	nestedPath, err := WriteSBPL(nested, content)
	if err != nil {
		t.Fatalf("WriteSBPL into nested dir: %v", err)
	}
	if _, err := os.Stat(nestedPath); err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
}

// ── Test 8: D-04 — read-only subpath vs literal ──

func TestGenerateSBPL_ReadOnlySubpathVsLiteral(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:   "workspace",
		NetMode:  "blocked",
		ReadOnly: []string{"/p/.git/", "/p/.env"},
	}
	out := GenerateSBPL(cfg, "/Users/x")

	// Trailing slash input -> subpath (with slash stripped)
	assertContains(t, out, `(deny file-write* (subpath "/p/.git"))`)
	// No trailing slash -> literal form (exact file)
	assertContains(t, out, `(deny file-write* (literal "/p/.env"))`)

	// Must NOT mix the forms
	assertNotContains(t, out, `(deny file-write* (literal "/p/.git"))`)
	assertNotContains(t, out, `(deny file-write* (subpath "/p/.env"))`)
}

// ── Test 9: KERN-01 — strict mode shape ──

func TestGenerateSBPL_StrictMode(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "strict",
		NetMode: "blocked",
	}
	out := GenerateSBPL(cfg, "/Users/x")

	assertContains(t, out, ";; ── Filesystem writes (strict — deny most writes) ──")
	assertContains(t, out, "(deny file-write*)")
	assertContains(t, out, `(subpath "/private/tmp")`)
	assertContains(t, out, `(subpath "/private/var/folders")`)
	assertContains(t, out, `(subpath "/dev")`)
	assertContains(t, out, `(subpath (param "FLOX_CACHE"))`)
	assertContains(t, out, `(subpath "/Users/x/.config/flox")`)
	assertContains(t, out, `(subpath "/Users/x/.local/share/flox")`)

	// Strict MUST NOT allow project writes
	assertNotContains(t, out, `(subpath (param "PROJECT"))`)
	assertNotContains(t, out, ";; ── Filesystem writes (workspace) ──")
}

// ── Test 10: KERN-01 — permissive mode + unrestricted network ──

func TestGenerateSBPL_PermissiveMode(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "permissive",
		NetMode: "unrestricted",
	}
	out := GenerateSBPL(cfg, "/Users/x")

	// No filesystem write restrictions
	assertNotContains(t, out, "(deny file-write*)")
	assertNotContains(t, out, ";; ── Filesystem writes")

	// Network: deny rule absent, but header comment present
	assertNotContains(t, out, "(deny network*)")
	assertContains(t, out, ";; ── Network (unrestricted) ──")
	assertNotContains(t, out, "(allow network* (remote unix-socket))")
}
