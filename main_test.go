package main

import (
	"bytes"
	"strings"
	"testing"
)

// ── Diagnostics Tests ───────────────────────────────────

func TestDiagnosticsBasicFormat(t *testing.T) {
	// Capture stderr output by temporarily replacing package-level writer
	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	config := &ResolvedConfig{
		Profile: "default",
		NetMode: "blocked",
		FsMode:  "workspace",
	}
	emitDiagnostics(config, false)

	output := buf.String()
	expected := "[sandflox] Profile: default | Network: blocked | Filesystem: workspace"
	if !strings.Contains(output, expected) {
		t.Errorf("expected output to contain %q, got %q", expected, output)
	}

	// Debug-only lines should NOT appear
	if strings.Contains(output, "Requisites:") {
		t.Error("non-debug output should not contain Requisites line")
	}
}

func TestDiagnosticsDebugOutput(t *testing.T) {
	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	config := &ResolvedConfig{
		Profile:        "default",
		NetMode:        "blocked",
		FsMode:         "workspace",
		Requisites:     "requisites.txt",
		AllowLocalhost: true,
		Writable:       []string{"/project", "/private/tmp"},
		ReadOnly:       []string{"/project/.git/"},
		Denied:         []string{"/home/user/.ssh/"},
	}
	emitDiagnostics(config, true)

	output := buf.String()

	// Should contain the summary line
	if !strings.Contains(output, "[sandflox] Profile: default | Network: blocked | Filesystem: workspace") {
		t.Error("debug output should still contain the summary line")
	}

	// Should contain debug-only lines
	if !strings.Contains(output, "Requisites: requisites.txt") {
		t.Error("debug output should contain requisites")
	}
	if !strings.Contains(output, "Allow localhost: true") {
		t.Error("debug output should contain allow localhost")
	}
	if !strings.Contains(output, "Writable paths:") {
		t.Error("debug output should contain writable paths")
	}
	if !strings.Contains(output, "Denied paths:") {
		t.Error("debug output should contain denied paths")
	}
}

func TestDiagnosticsMinimalProfile(t *testing.T) {
	var buf bytes.Buffer
	origStderr := stderr
	stderr = &buf
	defer func() { stderr = origStderr }()

	config := &ResolvedConfig{
		Profile: "minimal",
		NetMode: "blocked",
		FsMode:  "strict",
	}
	emitDiagnostics(config, false)

	output := buf.String()
	if !strings.Contains(output, "Profile: minimal") {
		t.Errorf("expected 'Profile: minimal' in output, got %q", output)
	}
	if !strings.Contains(output, "Filesystem: strict") {
		t.Errorf("expected 'Filesystem: strict' in output, got %q", output)
	}
}
