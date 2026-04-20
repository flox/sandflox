// shell_test.go — Unit tests for shell-tier enforcement generators.
//
// Assertions use assertContains / assertNotContains from sbpl_test.go
// (same package main). Test names match 03-VALIDATION.md per-task map
// (SHELL-01 through SHELL-08).
//
// All tests build a minimal *ResolvedConfig literal — no file I/O,
// no subprocess, no external deps. Tests run in <1s.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ── Test 1: SHELL-01 — PATH wipe in entrypoint ─────────

func TestGenerateEntrypoint_PathExport(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
		Writable: []string{"/proj"},
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}

	assertContains(t, out, `export PATH="$_sfx_bin"`)
	assertContains(t, out, `_sfx_bin="${FLOX_ENV_CACHE}/sandflox/bin"`)
	assertContains(t, out, `rm -rf "$_sfx_bin"`)
}

// ── Test 2: SHELL-03 — armor function generation ────────

func TestGenerateEntrypoint_ArmorFunctions(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}

	// Each armored command has a function definition
	for _, name := range ArmoredCommands {
		assertContains(t, out, "unalias "+name+" 2>/dev/null; "+name+"() { _sandflox_blocked "+name+"; }")
	}

	// Helper function body
	assertContains(t, out, `[sandflox] BLOCKED: $1 is not available. Environment is immutable.`)
	assertContains(t, out, `return 126`)

	// Single export -f statement with _sandflox_blocked + all 26 names
	assertContains(t, out, "export -f _sandflox_blocked")

	// Verify all 26 names appear after "export -f _sandflox_blocked"
	exportIdx := strings.Index(out, "export -f _sandflox_blocked")
	if exportIdx < 0 {
		t.Fatal("export -f _sandflox_blocked not found")
	}
	exportLine := out[exportIdx:]
	// Find end of line (or next newline)
	if nlIdx := strings.Index(exportLine, "\n"); nlIdx >= 0 {
		exportLine = exportLine[:nlIdx]
	}
	for _, name := range ArmoredCommands {
		if !strings.Contains(exportLine, name) {
			t.Errorf("export -f line missing armor name %q", name)
		}
	}
}

// ── Test 3: SHELL-06 — breadcrumb unset ─────────────────

func TestGenerateEntrypoint_BreadcrumbUnset(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}

	target := "unset FLOX_ENV_PROJECT FLOX_ENV_DIRS FLOX_PATH_PATCHED"
	assertContains(t, out, target)

	// Exactly one occurrence
	count := strings.Count(out, target)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of breadcrumb unset, got %d", count)
	}
}

// ── Test 4: SHELL-05 — Python env exports in entrypoint ─

func TestGenerateEntrypoint_PythonEnvExports(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}

	assertContains(t, out, `export PYTHONUSERBASE="${FLOX_ENV_CACHE}/sandflox-python"`)
	assertContains(t, out, `export ENABLE_USER_SITE=1`)
	assertContains(t, out, `export PYTHONPATH="${FLOX_ENV_CACHE}/sandflox-python:${PYTHONPATH:-}"`)
	assertContains(t, out, `export PYTHON_NOPIP=1`)
	assertContains(t, out, `export PYTHONDONTWRITEBYTECODE=1`)
}

// ── Test 5: SHELL-07 — curl removal is runtime-gated ────

func TestGenerateEntrypoint_CurlRemovalGated(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}

	// Runtime gate uses the net-blocked.flag file
	assertContains(t, out, `if [ -f "${FLOX_ENV_CACHE}/sandflox/net-blocked.flag" ]; then`)
	assertContains(t, out, `rm -f "${_sfx_bin}/curl"`)
}

// ── Test 6: SHELL-04 — fs-filter wrappers ───────────────

func TestGenerateFsFilter_Wrappers(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:   "workspace",
		NetMode:  "blocked",
		Writable: []string{"/proj"},
		ReadOnly: []string{"/proj/.git/"},
		Denied:   []string{"/proj/.env"},
	}
	out, err := GenerateFsFilter(cfg)
	if err != nil {
		t.Fatalf("GenerateFsFilter error: %v", err)
	}

	// Check function defined
	assertContains(t, out, "_sfx_check_write_target()")

	// Check case statements present for each path type
	assertContains(t, out, `case "$resolved" in`)

	// Each write command gets a wrapper
	for _, cmd := range WriteCmds {
		assertContains(t, out, `export _sfx_real_`+cmd+`="$(command -v `+cmd+` 2>/dev/null)"`)
		assertContains(t, out, cmd+"() {")
		assertContains(t, out, "export -f "+cmd)
	}

	// Improved prefix matching: dual alternatives (exact + subpath)
	assertContains(t, out, "'/proj'|'/proj'/*")

	// Permissive mode branch: no enforcement
	cfg2 := &ResolvedConfig{
		FsMode:  "permissive",
		NetMode: "blocked",
	}
	out2, err := GenerateFsFilter(cfg2)
	if err != nil {
		t.Fatalf("GenerateFsFilter permissive error: %v", err)
	}
	assertContains(t, out2, "permissive mode, no enforcement")
	assertNotContains(t, out2, `case "$resolved"`)
}

// ── Test 7: SHELL-05 — usercustomize blocks ensurepip ───

func TestGenerateUsercustomize_BlocksEnsurepip(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateUsercustomize(cfg)
	if err != nil {
		t.Fatalf("GenerateUsercustomize error: %v", err)
	}

	assertContains(t, out, `sys.modules["ensurepip"] = _blocked`)
	assertContains(t, out, `raise SystemExit("[sandflox] BLOCKED: ensurepip is disabled in this sandbox")`)
	assertContains(t, out, `types.ModuleType("ensurepip")`)
}

// ── Test 8: SHELL-05 — usercustomize wraps open() ──────

func TestGenerateUsercustomize_WrapsOpen(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateUsercustomize(cfg)
	if err != nil {
		t.Fatalf("GenerateUsercustomize error: %v", err)
	}

	assertContains(t, out, `builtins.open = _sandflox_open`)
	assertContains(t, out, `def _sandflox_open(file, mode="r"`)

	// Runtime state file references
	assertContains(t, out, "fs-mode.txt")
	assertContains(t, out, "writable-paths.txt")
	assertContains(t, out, "denied-paths.txt")

	// All three PermissionError messages with [sandflox] BLOCKED: prefix
	assertContains(t, out, `PermissionError(f"[sandflox] BLOCKED: write to '{file}' is denied by policy")`)
	assertContains(t, out, `PermissionError(f"[sandflox] BLOCKED: filesystem is read-only (strict mode)")`)
	assertContains(t, out, `PermissionError(f"[sandflox] BLOCKED: write to '{file}' outside workspace policy")`)
}

// ── Test 9: SHELL-08 — BLOCKED message format ──────────

func TestGenerate_BlockedMessagesFormat(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:   "workspace",
		NetMode:  "blocked",
		Writable: []string{"/proj"},
		ReadOnly: []string{"/proj/.git/"},
		Denied:   []string{"/proj/.env"},
	}

	entrypoint, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}

	fsFilter, err := GenerateFsFilter(cfg)
	if err != nil {
		t.Fatalf("GenerateFsFilter error: %v", err)
	}

	usercustomize, err := GenerateUsercustomize(cfg)
	if err != nil {
		t.Fatalf("GenerateUsercustomize error: %v", err)
	}

	combined := entrypoint + "\n" + fsFilter + "\n" + usercustomize

	// Count all BLOCKED messages
	re := regexp.MustCompile(`\[sandflox\] BLOCKED: .+`)
	matches := re.FindAllString(combined, -1)
	if len(matches) < 7 {
		t.Errorf("expected >= 7 BLOCKED messages across all artifacts, got %d:\n%s",
			len(matches), strings.Join(matches, "\n"))
	}

	// Every BLOCKED line uses the canonical [sandflox] BLOCKED: prefix
	for _, m := range matches {
		if !strings.HasPrefix(m, "[sandflox] BLOCKED:") {
			t.Errorf("BLOCKED message missing canonical prefix: %q", m)
		}
	}
}

// ── Test 10: shellquote escaping ────────────────────────

func TestShellquote_EscapesSingleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"don't", `'don'\''t'`},
		{"", "''"},
		{"a'b'c", `'a'\''b'\''c'`},
	}

	for _, tt := range tests {
		got := shellquote(tt.input)
		if got != tt.expected {
			t.Errorf("shellquote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ── Test 11: WriteShellArtifacts writes all three files ─

func TestWriteShellArtifacts_WritesAllThreeFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, ".flox", "cache", "sandflox")

	cfg := &ResolvedConfig{
		FsMode:   "workspace",
		NetMode:  "blocked",
		Writable: []string{"/proj"},
	}

	err := WriteShellArtifacts(cacheDir, cfg)
	if err != nil {
		t.Fatalf("WriteShellArtifacts error: %v", err)
	}

	// Check entrypoint.sh exists with 0755
	entrypointPath := filepath.Join(cacheDir, "entrypoint.sh")
	info, err := os.Stat(entrypointPath)
	if err != nil {
		t.Fatalf("entrypoint.sh not found: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0755 {
		t.Errorf("entrypoint.sh mode = %o, want 0755", perm)
	}

	// Check fs-filter.sh exists with 0644
	fsFilterPath := filepath.Join(cacheDir, "fs-filter.sh")
	info, err = os.Stat(fsFilterPath)
	if err != nil {
		t.Fatalf("fs-filter.sh not found: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("fs-filter.sh mode = %o, want 0644", perm)
	}

	// Check usercustomize.py exists with 0644 (in sibling dir)
	pyPath := filepath.Join(tmpDir, ".flox", "cache", "sandflox-python", "usercustomize.py")
	info, err = os.Stat(pyPath)
	if err != nil {
		t.Fatalf("usercustomize.py not found: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("usercustomize.py mode = %o, want 0644", perm)
	}

	// Verify entrypoint.sh has content
	data, err := os.ReadFile(entrypointPath)
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	if len(data) == 0 {
		t.Error("entrypoint.sh is empty")
	}
	assertContains(t, string(data), `export PATH="$_sfx_bin"`)
}

// ── Test 12: Bug 2 — physical path resolution (pwd -P) ──

func TestGenerateFsFilter_PhysicalPathResolution(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:   "workspace",
		NetMode:  "blocked",
		Writable: []string{"/proj"},
	}
	out, err := GenerateFsFilter(cfg)
	if err != nil {
		t.Fatalf("GenerateFsFilter error: %v", err)
	}
	assertContains(t, out, "pwd -P")
	assertNotContains(t, out, "&& pwd)")
}

// ── Test 13: SANDFLOX_ENABLED=1 exported in entrypoint ───

func TestGenerateEntrypoint_SetsSandfloxEnabled(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}
	assertContains(t, out, `export SANDFLOX_ENABLED=1`)
}

// ── Test 14: Bug 3 — sandflox auto-include in entrypoint ─

func TestGenerateEntrypoint_SandfloxAutoInclude(t *testing.T) {
	cfg := &ResolvedConfig{
		FsMode:  "workspace",
		NetMode: "blocked",
	}
	out, err := GenerateEntrypoint(cfg)
	if err != nil {
		t.Fatalf("GenerateEntrypoint error: %v", err)
	}
	assertContains(t, out, `"${FLOX_ENV}/bin/sandflox"`)
	assertContains(t, out, `"${_sfx_bin}/sandflox"`)
}
