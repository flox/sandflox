package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Cache Writer Tests ──────────────────────────────────

// testCacheSetup creates a temp directory with a requisites file for cache tests.
func testCacheSetup(t *testing.T) (cacheDir, projectDir string) {
	t.Helper()
	projectDir = t.TempDir()
	cacheDir = filepath.Join(projectDir, ".flox", "cache", "sandflox")

	// Write a minimal requisites file in the project dir
	reqContent := "bash\nsh\ngit\n"
	if err := os.WriteFile(filepath.Join(projectDir, "requisites.txt"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}
	return cacheDir, projectDir
}

func TestCacheWriteCreatesDir(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}

	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache error: %v", err)
	}

	// Cache directory should exist
	info, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("cache dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("cache path is not a directory")
	}
}

func TestCacheWriteNetMode(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}
	WriteCache(cacheDir, config, projectDir)

	data, err := os.ReadFile(filepath.Join(cacheDir, "net-mode.txt"))
	if err != nil {
		t.Fatalf("cannot read net-mode.txt: %v", err)
	}
	if string(data) != "blocked\n" {
		t.Errorf("expected 'blocked\\n', got %q", string(data))
	}
}

func TestCacheWriteFsMode(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}
	WriteCache(cacheDir, config, projectDir)

	data, err := os.ReadFile(filepath.Join(cacheDir, "fs-mode.txt"))
	if err != nil {
		t.Fatalf("cannot read fs-mode.txt: %v", err)
	}
	if string(data) != "workspace\n" {
		t.Errorf("expected 'workspace\\n', got %q", string(data))
	}
}

func TestCacheWriteActiveProfile(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}
	WriteCache(cacheDir, config, projectDir)

	data, err := os.ReadFile(filepath.Join(cacheDir, "active-profile.txt"))
	if err != nil {
		t.Fatalf("cannot read active-profile.txt: %v", err)
	}
	if string(data) != "default\n" {
		t.Errorf("expected 'default\\n', got %q", string(data))
	}
}

func TestCacheWriteNetBlockedFlag(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}
	WriteCache(cacheDir, config, projectDir)

	data, err := os.ReadFile(filepath.Join(cacheDir, "net-blocked.flag"))
	if err != nil {
		t.Fatalf("cannot read net-blocked.flag: %v", err)
	}
	if string(data) != "1\n" {
		t.Errorf("expected '1\\n', got %q", string(data))
	}
}

func TestCacheWriteNoNetBlockedFlag(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "full",
		NetMode:    "unrestricted",
		FsMode:     "permissive",
		Requisites: "requisites.txt",
	}
	WriteCache(cacheDir, config, projectDir)

	_, err := os.Stat(filepath.Join(cacheDir, "net-blocked.flag"))
	if !os.IsNotExist(err) {
		t.Error("net-blocked.flag should not exist when network is unrestricted")
	}
}

func TestCacheWritePathLists(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
		Writable:   []string{"/project", "/private/tmp"},
		ReadOnly:   []string{"/project/.flox/env/", "/project/.git/"},
		Denied:     []string{"/home/user/.ssh/", "/home/user/.aws/"},
	}
	WriteCache(cacheDir, config, projectDir)

	// Check writable-paths.txt
	data, _ := os.ReadFile(filepath.Join(cacheDir, "writable-paths.txt"))
	lines := strings.TrimSpace(string(data))
	if lines != "/project\n/private/tmp" {
		t.Errorf("writable-paths.txt: expected '/project\\n/private/tmp', got %q", lines)
	}

	// Check read-only-paths.txt
	data, _ = os.ReadFile(filepath.Join(cacheDir, "read-only-paths.txt"))
	lines = strings.TrimSpace(string(data))
	if lines != "/project/.flox/env/\n/project/.git/" {
		t.Errorf("read-only-paths.txt: got %q", lines)
	}

	// Check denied-paths.txt
	data, _ = os.ReadFile(filepath.Join(cacheDir, "denied-paths.txt"))
	lines = strings.TrimSpace(string(data))
	if lines != "/home/user/.ssh/\n/home/user/.aws/" {
		t.Errorf("denied-paths.txt: got %q", lines)
	}
}

func TestCacheWriteConfigJSON(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:        "default",
		NetMode:        "blocked",
		FsMode:         "workspace",
		Requisites:     "requisites.txt",
		AllowLocalhost: true,
	}
	WriteCache(cacheDir, config, projectDir)

	data, err := os.ReadFile(filepath.Join(cacheDir, "config.json"))
	if err != nil {
		t.Fatalf("cannot read config.json: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}

	if parsed["profile"] != "default" {
		t.Errorf("expected profile='default', got %v", parsed["profile"])
	}
	if parsed["net_mode"] != "blocked" {
		t.Errorf("expected net_mode='blocked', got %v", parsed["net_mode"])
	}
	if parsed["fs_mode"] != "workspace" {
		t.Errorf("expected fs_mode='workspace', got %v", parsed["fs_mode"])
	}
}

// ── Net-blocked.flag toggle test (D-04 stale cache purge) ──

func TestCacheWriteNetBlockedFlagToggle(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)
	flagPath := filepath.Join(cacheDir, "net-blocked.flag")

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}

	// Step 1: blocked -> flag must exist
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache (blocked): %v", err)
	}
	if _, err := os.Stat(flagPath); os.IsNotExist(err) {
		t.Fatal("net-blocked.flag should exist after blocked write")
	}

	// Step 2: flip to unrestricted -> flag must be REMOVED
	config.NetMode = "unrestricted"
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache (unrestricted): %v", err)
	}
	if _, err := os.Stat(flagPath); !os.IsNotExist(err) {
		t.Fatal("net-blocked.flag should be removed after unrestricted write")
	}

	// Step 3: flip back to blocked -> flag must be RECREATED
	config.NetMode = "blocked"
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache (blocked again): %v", err)
	}
	if _, err := os.Stat(flagPath); os.IsNotExist(err) {
		t.Fatal("net-blocked.flag should be recreated after second blocked write")
	}

	// Step 4: idempotency -- calling blocked again should not error
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache (blocked idempotent): %v", err)
	}
	if _, err := os.Stat(flagPath); os.IsNotExist(err) {
		t.Fatal("net-blocked.flag should still exist after idempotent write")
	}
}

// ── ReadCache Tests ──────────────────────────────────────

func TestReadCacheRoundTrip(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	original := &ResolvedConfig{
		Profile:        "default",
		NetMode:        "blocked",
		FsMode:         "workspace",
		Requisites:     "requisites.txt",
		AllowLocalhost: true,
		Writable:       []string{"/project", "/private/tmp"},
		ReadOnly:       []string{"/project/.git/"},
		Denied:         []string{"/home/user/.ssh/", "/home/user/.aws/"},
	}

	if err := WriteCache(cacheDir, original, projectDir); err != nil {
		t.Fatalf("WriteCache error: %v", err)
	}

	got, err := ReadCache(cacheDir)
	if err != nil {
		t.Fatalf("ReadCache error: %v", err)
	}

	if got.Profile != original.Profile {
		t.Errorf("Profile: got %q, want %q", got.Profile, original.Profile)
	}
	if got.NetMode != original.NetMode {
		t.Errorf("NetMode: got %q, want %q", got.NetMode, original.NetMode)
	}
	if got.FsMode != original.FsMode {
		t.Errorf("FsMode: got %q, want %q", got.FsMode, original.FsMode)
	}
	if got.AllowLocalhost != original.AllowLocalhost {
		t.Errorf("AllowLocalhost: got %v, want %v", got.AllowLocalhost, original.AllowLocalhost)
	}
	if len(got.Writable) != len(original.Writable) {
		t.Errorf("Writable: got %v, want %v", got.Writable, original.Writable)
	}
	if len(got.ReadOnly) != len(original.ReadOnly) {
		t.Errorf("ReadOnly: got %v, want %v", got.ReadOnly, original.ReadOnly)
	}
	if len(got.Denied) != len(original.Denied) {
		t.Errorf("Denied: got %v, want %v", got.Denied, original.Denied)
	}
}

func TestReadCacheMissing(t *testing.T) {
	_, err := ReadCache("/tmp/nonexistent-sandflox-test-" + t.Name())
	if err == nil {
		t.Fatal("expected error for missing cache dir, got nil")
	}
	if !strings.Contains(err.Error(), "cannot read cached config") {
		t.Errorf("expected error containing 'cannot read cached config', got: %v", err)
	}
}

func TestReadCacheCorrupt(t *testing.T) {
	dir := t.TempDir()
	// Write garbage to config.json
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("not valid json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadCache(dir)
	if err == nil {
		t.Fatal("expected error for corrupt config.json, got nil")
	}
	if !strings.Contains(err.Error(), "corrupt cached config") {
		t.Errorf("expected error containing 'corrupt cached config', got: %v", err)
	}
}

func TestCacheWriteRequisites(t *testing.T) {
	cacheDir, projectDir := testCacheSetup(t)

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}
	WriteCache(cacheDir, config, projectDir)

	// The cache should have a copy of the requisites file
	data, err := os.ReadFile(filepath.Join(cacheDir, "requisites.txt"))
	if err != nil {
		t.Fatalf("cannot read cached requisites.txt: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "bash") {
		t.Error("cached requisites.txt should contain 'bash'")
	}
	if !strings.Contains(content, "git") {
		t.Error("cached requisites.txt should contain 'git'")
	}
}

// ── Embedded Requisites Fallback Tests ───────────────────

func TestWriteCacheFallsBackToEmbeddedRequisites(t *testing.T) {
	// Create a project dir WITHOUT any requisites file on disk
	projectDir := t.TempDir()
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt", // not on disk, must use embedded
	}

	err := WriteCache(cacheDir, config, projectDir)
	if err != nil {
		t.Fatalf("WriteCache should succeed with embedded fallback, got: %v", err)
	}

	// Verify the cached requisites.txt has content from the embedded file
	data, err := os.ReadFile(filepath.Join(cacheDir, "requisites.txt"))
	if err != nil {
		t.Fatalf("cannot read cached requisites.txt: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "bash") {
		t.Error("embedded requisites.txt should contain 'bash'")
	}
	if !strings.Contains(content, "git") {
		t.Error("embedded requisites.txt should contain 'git'")
	}
	if !strings.Contains(content, "curl") {
		t.Error("embedded requisites.txt should contain 'curl'")
	}
}

func TestWriteCacheFallsBackToEmbeddedMinimal(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")

	config := &ResolvedConfig{
		Profile:    "minimal",
		NetMode:    "blocked",
		FsMode:     "strict",
		Requisites: "requisites-minimal.txt",
	}

	err := WriteCache(cacheDir, config, projectDir)
	if err != nil {
		t.Fatalf("WriteCache should succeed with embedded minimal fallback, got: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cacheDir, "requisites.txt"))
	if err != nil {
		t.Fatalf("cannot read cached requisites.txt: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "bash") {
		t.Error("embedded requisites-minimal.txt should contain 'bash'")
	}
	// minimal profile should NOT have git as a standalone tool
	// (but grep/egrep/fgrep are present — check line-by-line)
	hasGitLine := false
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "git" {
			hasGitLine = true
			break
		}
	}
	if hasGitLine {
		t.Error("embedded requisites-minimal.txt should NOT contain 'git' as a tool")
	}
}

func TestWriteCacheErrorsOnUnknownRequisites(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")

	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "nonexistent-requisites.txt",
	}

	err := WriteCache(cacheDir, config, projectDir)
	if err == nil {
		t.Fatal("WriteCache should error on unknown requisites file not found on disk or embedded")
	}
	if !strings.Contains(err.Error(), "cannot read requisites file") {
		t.Errorf("expected 'cannot read requisites file' in error, got: %v", err)
	}
}
