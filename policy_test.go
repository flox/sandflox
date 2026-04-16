package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParsePolicyToml parses the real policy.toml and verifies all fields.
func TestParsePolicyToml(t *testing.T) {
	// Find policy.toml relative to the test working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(wd, "policy.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("policy.toml not found -- skipping real-file test")
	}

	p, err := ParsePolicy(path)
	if err != nil {
		t.Fatalf("ParsePolicy(%q) returned error: %v", path, err)
	}

	// Meta section
	if p.Meta.Version != "2" {
		t.Errorf("Meta.Version = %q, want %q", p.Meta.Version, "2")
	}
	if p.Meta.Profile != "default" {
		t.Errorf("Meta.Profile = %q, want %q", p.Meta.Profile, "default")
	}

	// Network section
	if p.Network.Mode != "blocked" {
		t.Errorf("Network.Mode = %q, want %q", p.Network.Mode, "blocked")
	}
	if p.Network.AllowLocalhost != true {
		t.Errorf("Network.AllowLocalhost = %v, want true", p.Network.AllowLocalhost)
	}

	// Filesystem section
	if p.Filesystem.Mode != "workspace" {
		t.Errorf("Filesystem.Mode = %q, want %q", p.Filesystem.Mode, "workspace")
	}
	if len(p.Filesystem.Writable) != 2 || p.Filesystem.Writable[0] != "." || p.Filesystem.Writable[1] != "/tmp" {
		t.Errorf("Filesystem.Writable = %v, want [. /tmp]", p.Filesystem.Writable)
	}

	// Check read-only contains specific entries
	roWant := map[string]bool{".flox/env/": true, ".git/": true}
	for _, ro := range p.Filesystem.ReadOnly {
		delete(roWant, ro)
	}
	if len(roWant) > 0 {
		t.Errorf("Filesystem.ReadOnly missing entries: %v (got %v)", roWant, p.Filesystem.ReadOnly)
	}

	// Check denied contains specific entries
	deniedWant := map[string]bool{"~/.ssh/": true, "~/.gnupg/": true}
	for _, d := range p.Filesystem.Denied {
		delete(deniedWant, d)
	}
	if len(deniedWant) > 0 {
		t.Errorf("Filesystem.Denied missing entries: %v (got %v)", deniedWant, p.Filesystem.Denied)
	}

	// Profile sections
	if p.Profiles == nil {
		t.Fatal("Profiles is nil")
	}

	minimal, ok := p.Profiles["minimal"]
	if !ok {
		t.Fatal("missing profile 'minimal'")
	}
	if minimal.Requisites != "requisites-minimal.txt" {
		t.Errorf("minimal.Requisites = %q, want %q", minimal.Requisites, "requisites-minimal.txt")
	}
	if minimal.Network != "blocked" {
		t.Errorf("minimal.Network = %q, want %q", minimal.Network, "blocked")
	}
	if minimal.Filesystem != "strict" {
		t.Errorf("minimal.Filesystem = %q, want %q", minimal.Filesystem, "strict")
	}

	def, ok := p.Profiles["default"]
	if !ok {
		t.Fatal("missing profile 'default'")
	}
	if def.Requisites != "requisites.txt" {
		t.Errorf("default.Requisites = %q, want %q", def.Requisites, "requisites.txt")
	}

	full, ok := p.Profiles["full"]
	if !ok {
		t.Fatal("missing profile 'full'")
	}
	if full.Network != "unrestricted" {
		t.Errorf("full.Network = %q, want %q", full.Network, "unrestricted")
	}
}

// TestParseInlineComments verifies that inline comments are stripped from values.
func TestParseInlineComments(t *testing.T) {
	input := `[meta]
version = "2"

[network]
mode = "blocked"  # "unrestricted" | "blocked"
`
	p, err := parsePolicyFromString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Network.Mode != "blocked" {
		t.Errorf("Network.Mode = %q, want %q", p.Network.Mode, "blocked")
	}
}

// TestParseBooleans verifies boolean parsing for true and false.
func TestParseBooleans(t *testing.T) {
	input := `[meta]
version = "2"

[network]
mode = "blocked"
allow-localhost = true
`
	p, err := parsePolicyFromString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Network.AllowLocalhost != true {
		t.Errorf("AllowLocalhost = %v, want true", p.Network.AllowLocalhost)
	}

	input2 := `[meta]
version = "2"

[network]
mode = "blocked"
allow-localhost = false
`
	p2, err := parsePolicyFromString(input2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p2.Network.AllowLocalhost != false {
		t.Errorf("AllowLocalhost = %v, want false", p2.Network.AllowLocalhost)
	}
}

// TestParseStringArrays verifies parsing of string arrays.
func TestParseStringArrays(t *testing.T) {
	input := `[meta]
version = "2"

[filesystem]
mode = "workspace"
writable = [".", "/tmp"]
denied = ["~/.ssh/", "~/.gnupg/", "~/.aws/", "~/.config/gcloud/", "~/.config/gh/"]
`
	p, err := parsePolicyFromString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Filesystem.Writable) != 2 {
		t.Fatalf("Writable len = %d, want 2", len(p.Filesystem.Writable))
	}
	if p.Filesystem.Writable[0] != "." || p.Filesystem.Writable[1] != "/tmp" {
		t.Errorf("Writable = %v, want [. /tmp]", p.Filesystem.Writable)
	}
	if len(p.Filesystem.Denied) != 5 {
		t.Fatalf("Denied len = %d, want 5", len(p.Filesystem.Denied))
	}
}

// TestParseErrorBadVersion verifies that version "1" is rejected.
func TestParseErrorBadVersion(t *testing.T) {
	input := `[meta]
version = "1"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for version 1, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported policy version") {
		t.Errorf("error = %q, want to contain 'unsupported policy version'", err.Error())
	}
}

// TestParseErrorBadVersion3 verifies that version "3" is rejected.
func TestParseErrorBadVersion3(t *testing.T) {
	input := `[meta]
version = "3"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for version 3, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported policy version") {
		t.Errorf("error = %q, want to contain 'unsupported policy version'", err.Error())
	}
}

// TestParseErrorMissingVersion verifies that missing [meta] section causes an error.
func TestParseErrorMissingVersion(t *testing.T) {
	input := `[network]
mode = "blocked"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "version") {
		t.Errorf("error = %q, want to contain 'version'", err.Error())
	}
}

// TestParseErrorBadNetworkMode verifies that invalid network mode is rejected.
func TestParseErrorBadNetworkMode(t *testing.T) {
	input := `[meta]
version = "2"

[network]
mode = "partial"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for bad network mode, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "partial") {
		t.Errorf("error = %q, want to contain 'partial'", errStr)
	}
	if !strings.Contains(strings.ToLower(errStr), "network") {
		t.Errorf("error = %q, want to contain 'network'", errStr)
	}
}

// TestParseErrorBadFilesystemMode verifies that invalid filesystem mode is rejected.
func TestParseErrorBadFilesystemMode(t *testing.T) {
	input := `[meta]
version = "2"

[filesystem]
mode = "readonly"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for bad filesystem mode, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "readonly") {
		t.Errorf("error = %q, want to contain 'readonly'", errStr)
	}
	if !strings.Contains(strings.ToLower(errStr), "filesystem") {
		t.Errorf("error = %q, want to contain 'filesystem'", errStr)
	}
}

// TestParseBlankLinesAndComments verifies that blank lines and comments parse cleanly.
func TestParseBlankLinesAndComments(t *testing.T) {
	input := `
# This is a comment

# Another comment
[meta]
version = "2"  # inline comment
profile = "default"

# comment between sections

[network]
mode = "blocked"

`
	p, err := parsePolicyFromString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Meta.Version != "2" {
		t.Errorf("Meta.Version = %q, want %q", p.Meta.Version, "2")
	}
	if p.Meta.Profile != "default" {
		t.Errorf("Meta.Profile = %q, want %q", p.Meta.Profile, "default")
	}
}

// TestParseDottedSections verifies that [profiles.minimal] creates a nested map entry.
func TestParseDottedSections(t *testing.T) {
	input := `[meta]
version = "2"

[profiles.minimal]
requisites = "requisites-minimal.txt"
network = "blocked"
filesystem = "strict"

[profiles.full]
requisites = "requisites-full.txt"
network = "unrestricted"
filesystem = "permissive"
`
	p, err := parsePolicyFromString(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Profiles == nil {
		t.Fatal("Profiles is nil")
	}
	minimal, ok := p.Profiles["minimal"]
	if !ok {
		t.Fatal("missing profile 'minimal'")
	}
	if minimal.Requisites != "requisites-minimal.txt" {
		t.Errorf("minimal.Requisites = %q, want %q", minimal.Requisites, "requisites-minimal.txt")
	}
	if minimal.Network != "blocked" {
		t.Errorf("minimal.Network = %q, want %q", minimal.Network, "blocked")
	}
	if minimal.Filesystem != "strict" {
		t.Errorf("minimal.Filesystem = %q, want %q", minimal.Filesystem, "strict")
	}

	full, ok := p.Profiles["full"]
	if !ok {
		t.Fatal("missing profile 'full'")
	}
	if full.Network != "unrestricted" {
		t.Errorf("full.Network = %q, want %q", full.Network, "unrestricted")
	}
	if full.Filesystem != "permissive" {
		t.Errorf("full.Filesystem = %q, want %q", full.Filesystem, "permissive")
	}
}

// TestParseErrorUnsupportedTOML verifies that unsupported TOML features are rejected.
func TestParseErrorUnsupportedTOML(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "array of tables",
			input: `[meta]
version = "2"

[[array.of.tables]]
name = "bad"
`,
		},
		{
			name: "inline table",
			input: `[meta]
version = "2"

[network]
extra = {inline = "table"}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePolicyFromString(tt.input)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// TestParseErrorBadProfileNetworkMode verifies that invalid network mode in profiles is rejected.
func TestParseErrorBadProfileNetworkMode(t *testing.T) {
	input := `[meta]
version = "2"

[profiles.bad]
requisites = "requisites.txt"
network = "partial"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for bad profile network mode, got nil")
	}
	if !strings.Contains(err.Error(), "partial") {
		t.Errorf("error = %q, want to contain 'partial'", err.Error())
	}
}

// TestParseErrorBadProfileFilesystemMode verifies that invalid filesystem mode in profiles is rejected.
func TestParseErrorBadProfileFilesystemMode(t *testing.T) {
	input := `[meta]
version = "2"

[profiles.bad]
requisites = "requisites.txt"
filesystem = "readonly"
`
	_, err := parsePolicyFromString(input)
	if err == nil {
		t.Fatal("expected error for bad profile filesystem mode, got nil")
	}
	if !strings.Contains(err.Error(), "readonly") {
		t.Errorf("error = %q, want to contain 'readonly'", err.Error())
	}
}

// parsePolicyFromString is a test helper that writes input to a temp file and parses it.
func parsePolicyFromString(input string) (*Policy, error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-policy.toml")
	if err := os.WriteFile(tmpFile, []byte(input), 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)
	return ParsePolicy(tmpFile)
}
