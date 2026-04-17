package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── extractSubcommand Tests ─────────────────────────────

func TestExtractSubcommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "validate alone",
			args:     []string{"validate"},
			wantCmd:  "validate",
			wantArgs: []string{},
		},
		{
			name:     "debug before validate",
			args:     []string{"--debug", "validate"},
			wantCmd:  "validate",
			wantArgs: []string{"--debug"},
		},
		{
			name:     "validate then debug",
			args:     []string{"validate", "--debug"},
			wantCmd:  "validate",
			wantArgs: []string{"--debug"},
		},
		{
			name:     "double-dash then echo hello",
			args:     []string{"--", "echo", "hello"},
			wantCmd:  "",
			wantArgs: []string{"--", "echo", "hello"},
		},
		{
			name:     "unknown first arg",
			args:     []string{"foo", "bar"},
			wantCmd:  "",
			wantArgs: []string{"foo", "bar"},
		},
		{
			name:     "empty args",
			args:     []string{},
			wantCmd:  "",
			wantArgs: []string{},
		},
		{
			name:     "status alone",
			args:     []string{"status"},
			wantCmd:  "status",
			wantArgs: []string{},
		},
		{
			name:     "elevate alone",
			args:     []string{"elevate"},
			wantCmd:  "elevate",
			wantArgs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmd, gotArgs := extractSubcommand(tt.args)
			if gotCmd != tt.wantCmd {
				t.Errorf("extractSubcommand(%v) cmd = %q, want %q", tt.args, gotCmd, tt.wantCmd)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Errorf("extractSubcommand(%v) args len = %d, want %d: got %v", tt.args, len(gotArgs), len(tt.wantArgs), gotArgs)
				return
			}
			for i, a := range gotArgs {
				if a != tt.wantArgs[i] {
					t.Errorf("extractSubcommand(%v) args[%d] = %q, want %q", tt.args, i, a, tt.wantArgs[i])
				}
			}
		})
	}
}

// ── Flag position equivalence ───────────────────────────

func TestSubcommandFlagPosition(t *testing.T) {
	// Both "--debug validate" and "validate --debug" should produce
	// flags.Debug=true after extractSubcommand + ParseFlags
	cases := [][]string{
		{"--debug", "validate"},
		{"validate", "--debug"},
	}
	for _, args := range cases {
		_, remaining := extractSubcommand(args)
		flags, _ := ParseFlags(remaining)
		if !flags.Debug {
			t.Errorf("ParseFlags after extractSubcommand(%v): expected Debug=true, got false", args)
		}
	}
}

// ── Validate output tests ───────────────────────────────

// setupValidateProject creates a temp dir with a valid policy.toml and
// requisites.txt for validate testing.
func setupValidateProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	policy := `[meta]
version = "2"
profile = "default"

[network]
mode = "blocked"
allow-localhost = true

[filesystem]
mode = "workspace"
writable = [".", "/tmp"]
read-only = [".git/"]
denied = ["~/.ssh/", "~/.aws/"]

[profiles.default]
requisites = "requisites.txt"
network = "blocked"
filesystem = "workspace"
`
	if err := os.WriteFile(filepath.Join(dir, "policy.toml"), []byte(policy), 0644); err != nil {
		t.Fatal(err)
	}

	requisites := "bash\nsh\ngit\ncoreutils\ncurl\n"
	if err := os.WriteFile(filepath.Join(dir, "requisites.txt"), []byte(requisites), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestValidateOutput(t *testing.T) {
	dir := setupValidateProject(t)

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	flags := &CLIFlags{
		PolicyPath: filepath.Join(dir, "policy.toml"),
	}

	code := runValidateWithExitCode(flags)
	if code != 0 {
		t.Fatalf("runValidate returned exit code %d, want 0. stderr: %s", code, buf.String())
	}

	output := buf.String()

	// Must contain policy valid line
	if !strings.Contains(output, "[sandflox] Policy: ") || !strings.Contains(output, "(valid)") {
		t.Errorf("expected '[sandflox] Policy: ... (valid)' in output, got:\n%s", output)
	}

	// Must contain profile/network/filesystem summary
	if !strings.Contains(output, "[sandflox] Profile: default | Network: blocked | Filesystem: workspace") {
		t.Errorf("expected profile summary line in output, got:\n%s", output)
	}

	// Must contain tools count
	if !strings.Contains(output, "[sandflox] Tools: 5 (from requisites.txt)") {
		t.Errorf("expected tools count line in output, got:\n%s", output)
	}

	// Must contain denied paths count
	if !strings.Contains(output, "[sandflox] Denied paths: 2") {
		t.Errorf("expected denied paths count in output, got:\n%s", output)
	}
}

func TestValidateDebugOutput(t *testing.T) {
	dir := setupValidateProject(t)

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	flags := &CLIFlags{
		PolicyPath: filepath.Join(dir, "policy.toml"),
		Debug:      true,
	}

	code := runValidateWithExitCode(flags)
	if code != 0 {
		t.Fatalf("runValidate (debug) returned exit code %d, want 0. stderr: %s", code, buf.String())
	}

	output := buf.String()

	// Debug output must contain requisites line
	if !strings.Contains(output, "[sandflox] Requisites:") {
		t.Errorf("expected debug requisites line, got:\n%s", output)
	}

	// Debug output must contain sbpl line
	if !strings.Contains(output, "[sandflox] sbpl:") {
		t.Errorf("expected debug sbpl line, got:\n%s", output)
	}
}

func TestValidateNoPolicyExitsWithError(t *testing.T) {
	dir := t.TempDir() // empty dir -- no policy.toml

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	flags := &CLIFlags{
		PolicyPath: filepath.Join(dir, "policy.toml"),
	}

	code := runValidateWithExitCode(flags)
	if code != 1 {
		t.Errorf("runValidate with no policy should return exit code 1, got %d", code)
	}

	output := buf.String()
	if !strings.Contains(output, "[sandflox] ERROR:") {
		t.Errorf("expected error message in output, got:\n%s", output)
	}
}

// ── Status output tests ─────────────────────────────────

func TestStatusOutput(t *testing.T) {
	// Create a project with cached state via WriteCache
	projectDir := t.TempDir()
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")

	// Write requisites file in project dir
	reqContent := "bash\nsh\ngit\ncoreutils\n"
	if err := os.WriteFile(filepath.Join(projectDir, "requisites.txt"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	config := &ResolvedConfig{
		Profile:        "default",
		NetMode:        "blocked",
		FsMode:         "workspace",
		Requisites:     "requisites.txt",
		AllowLocalhost: true,
		Denied:         []string{"/home/user/.ssh/", "/home/user/.aws/"},
	}

	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache error: %v", err)
	}

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	code := runStatusWithExitCode(cacheDir)
	if code != 0 {
		t.Fatalf("runStatus returned exit code %d, want 0. stderr: %s", code, buf.String())
	}

	output := buf.String()

	if !strings.Contains(output, "[sandflox] Profile: default | Network: blocked | Filesystem: workspace") {
		t.Errorf("expected profile summary line in output, got:\n%s", output)
	}

	if !strings.Contains(output, "[sandflox] Tools: 4 | Denied paths: 2") {
		t.Errorf("expected tools/denied line in output, got:\n%s", output)
	}
}

func TestStatusDebugOutput(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")

	reqContent := "bash\nsh\n"
	if err := os.WriteFile(filepath.Join(projectDir, "requisites.txt"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	config := &ResolvedConfig{
		Profile:        "default",
		NetMode:        "blocked",
		FsMode:         "workspace",
		Requisites:     "requisites.txt",
		AllowLocalhost: true,
		Writable:       []string{"/project"},
		ReadOnly:       []string{"/project/.git/"},
		Denied:         []string{"/home/.ssh/"},
	}

	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatalf("WriteCache error: %v", err)
	}

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	code := runStatusDebugWithExitCode(cacheDir)
	if code != 0 {
		t.Fatalf("runStatus (debug) returned exit code %d, want 0. stderr: %s", code, buf.String())
	}

	output := buf.String()

	if !strings.Contains(output, "[sandflox] Requisites:") {
		t.Errorf("expected requisites line in debug output, got:\n%s", output)
	}
	if !strings.Contains(output, "[sandflox] Allow localhost: true") {
		t.Errorf("expected allow-localhost line in debug output, got:\n%s", output)
	}
	if !strings.Contains(output, "[sandflox] Writable paths:") {
		t.Errorf("expected writable paths line in debug output, got:\n%s", output)
	}
}

func TestStatusNoCache(t *testing.T) {
	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	code := runStatusWithExitCode("")
	if code != 1 {
		t.Errorf("runStatus with no cache should return exit code 1, got %d", code)
	}

	output := buf.String()
	if !strings.Contains(output, "Not in a sandflox session") {
		t.Errorf("expected 'Not in a sandflox session' in output, got:\n%s", output)
	}
}

// ── Elevate detection tests ──────────────────────────────

func TestElevateAlreadySandboxed(t *testing.T) {
	t.Setenv("SANDFLOX_ENABLED", "1")
	t.Setenv("FLOX_ENV", "/some/path")

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	msg, code := checkElevatePrereqs()
	if code != 0 {
		t.Errorf("expected exit code 0 for already-sandboxed, got %d", code)
	}
	if !strings.Contains(msg, "Already sandboxed") {
		t.Errorf("expected 'Already sandboxed' message, got %q", msg)
	}
}

func TestElevateNoFlox(t *testing.T) {
	t.Setenv("SANDFLOX_ENABLED", "")
	t.Setenv("FLOX_ENV", "")

	msg, code := checkElevatePrereqs()
	if code != 1 {
		t.Errorf("expected exit code 1 for no flox session, got %d", code)
	}
	if !strings.Contains(msg, "Not in a flox session") {
		t.Errorf("expected 'Not in a flox session' message, got %q", msg)
	}
}

func TestElevateHasFloxEnv(t *testing.T) {
	t.Setenv("SANDFLOX_ENABLED", "")
	t.Setenv("FLOX_ENV", "/some/flox/env")

	msg, code := checkElevatePrereqs()
	if msg != "" {
		t.Errorf("expected empty message for valid flox session, got %q (code=%d)", msg, code)
	}
}

func TestElevateNoPolicyExits(t *testing.T) {
	dir := t.TempDir() // empty dir -- no policy.toml
	t.Setenv("SANDFLOX_ENABLED", "")
	t.Setenv("FLOX_ENV", "/some/flox/env")
	t.Setenv("FLOX_ENV_CACHE", "")

	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	flags := &CLIFlags{
		PolicyPath: filepath.Join(dir, "policy.toml"),
	}

	code := runElevateWithExitCode(flags)
	if code != 1 {
		t.Errorf("runElevate with no policy should return exit code 1, got %d", code)
	}

	output := buf.String()
	if !strings.Contains(output, "[sandflox] ERROR:") {
		t.Errorf("expected error message in output, got:\n%s", output)
	}
}

func TestDiscoverCacheDir(t *testing.T) {
	// Test fallback path: cwd-relative
	projectDir := t.TempDir()
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")

	// Write requisites and config for WriteCache
	if err := os.WriteFile(filepath.Join(projectDir, "requisites.txt"), []byte("bash\n"), 0644); err != nil {
		t.Fatal(err)
	}
	config := &ResolvedConfig{
		Profile:    "default",
		NetMode:    "blocked",
		FsMode:     "workspace",
		Requisites: "requisites.txt",
	}
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		t.Fatal(err)
	}

	flags := &CLIFlags{PolicyPath: filepath.Join(projectDir, "policy.toml")}

	got := discoverCacheDir(flags)
	if got != cacheDir {
		t.Errorf("discoverCacheDir fallback: got %q, want %q", got, cacheDir)
	}

	// Test env var path
	envCacheDir := t.TempDir()
	sandfloxCache := filepath.Join(envCacheDir, "sandflox")
	if err := os.MkdirAll(sandfloxCache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sandfloxCache, "config.json"), []byte(`{"profile":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FLOX_ENV_CACHE", envCacheDir)
	got = discoverCacheDir(flags)
	if got != sandfloxCache {
		t.Errorf("discoverCacheDir env var: got %q, want %q", got, sandfloxCache)
	}
}
