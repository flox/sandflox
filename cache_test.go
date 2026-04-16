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
