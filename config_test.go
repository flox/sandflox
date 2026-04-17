package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── Profile Resolution Tests ────────────────────────────

func TestProfileResolutionEnvVar(t *testing.T) {
	// SANDFLOX_PROFILE env var takes precedence over policy.Meta.Profile
	t.Setenv("SANDFLOX_PROFILE", "minimal")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "default"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"minimal": {
				Requisites: "requisites-minimal.txt",
				Network:    "blocked",
				Filesystem: "strict",
			},
		},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.Profile != "minimal" {
		t.Errorf("expected profile='minimal' from env var, got %q", config.Profile)
	}
}

func TestProfileResolutionPolicyFile(t *testing.T) {
	// When no SANDFLOX_PROFILE env var, profile comes from policy.Meta.Profile
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "full"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"full": {
				Requisites: "requisites-full.txt",
				Network:    "unrestricted",
				Filesystem: "permissive",
			},
		},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.Profile != "full" {
		t.Errorf("expected profile='full' from policy file, got %q", config.Profile)
	}
}

func TestProfileResolutionDefault(t *testing.T) {
	// When no env var and no policy.Meta.Profile, falls back to "default"
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles:   map[string]ProfileSection{},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.Profile != "default" {
		t.Errorf("expected profile='default' as fallback, got %q", config.Profile)
	}
}

func TestProfileMergeNetworkOverride(t *testing.T) {
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "full"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"full": {
				Network:    "unrestricted",
				Filesystem: "",
			},
		},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.NetMode != "unrestricted" {
		t.Errorf("expected net_mode='unrestricted' from profile override, got %q", config.NetMode)
	}
}

func TestProfileMergeFilesystemOverride(t *testing.T) {
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "minimal"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"minimal": {
				Filesystem: "strict",
			},
		},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.FsMode != "strict" {
		t.Errorf("expected fs_mode='strict' from profile override, got %q", config.FsMode)
	}
}

func TestProfileMergeRequisites(t *testing.T) {
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "minimal"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"minimal": {
				Requisites: "requisites-minimal.txt",
			},
		},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.Requisites != "requisites-minimal.txt" {
		t.Errorf("expected requisites='requisites-minimal.txt', got %q", config.Requisites)
	}
}

func TestProfileMergeNoOverride(t *testing.T) {
	// Profile with empty Network/Filesystem inherits top-level values
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "plain"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"plain": {
				Requisites: "requisites.txt",
			},
		},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.NetMode != "blocked" {
		t.Errorf("expected net_mode='blocked' (inherited from top-level), got %q", config.NetMode)
	}
	if config.FsMode != "workspace" {
		t.Errorf("expected fs_mode='workspace' (inherited from top-level), got %q", config.FsMode)
	}
}

// ── EnvPassthrough Resolution Tests ─────────────────────

func TestResolveConfig_EnvPassthrough(t *testing.T) {
	t.Setenv("SANDFLOX_PROFILE", "")

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "default"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Security:   SecuritySection{EnvPassthrough: []string{"EDITOR"}},
		Profiles:   map[string]ProfileSection{},
	}
	flags := &CLIFlags{}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if len(config.EnvPassthrough) != 1 || config.EnvPassthrough[0] != "EDITOR" {
		t.Errorf("EnvPassthrough = %v, want [EDITOR]", config.EnvPassthrough)
	}
}

// ── Path Resolution Tests ───────────────────────────────

func TestResolvePathTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := ResolvePath("~/.ssh/", "/project")
	expected := home + "/.ssh/"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestResolvePathRelative(t *testing.T) {
	result := ResolvePath(".", "/my/project")
	if result != "/my/project" {
		t.Errorf("expected '/my/project', got %q", result)
	}
}

func TestResolvePathTmpDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only test")
	}
	result := ResolvePath("/tmp", "/project")
	if result != "/private/tmp" {
		t.Errorf("expected '/private/tmp', got %q", result)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	result := ResolvePath("/etc/hosts", "/project")
	if result != "/etc/hosts" {
		t.Errorf("expected '/etc/hosts', got %q", result)
	}
}

func TestResolvePathTrailingSlash(t *testing.T) {
	result := ResolvePath(".git/", "/my/project")
	if result != "/my/project/.git/" {
		t.Errorf("expected '/my/project/.git/', got %q", result)
	}
}

// ── Requisites Parser Tests ─────────────────────────────

func TestParseRequisites(t *testing.T) {
	// Create a temp requisites file
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "requisites.txt")
	content := `# Core shell utilities
bash
sh
env

# Text processing
grep
sed

# Blank lines above and below are skipped

git
`
	if err := os.WriteFile(reqPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tools, err := ParseRequisites(reqPath)
	if err != nil {
		t.Fatalf("ParseRequisites error: %v", err)
	}

	expected := []string{"bash", "sh", "env", "grep", "sed", "git"}
	if len(tools) != len(expected) {
		t.Fatalf("expected %d tools, got %d: %v", len(expected), len(tools), tools)
	}
	for i, want := range expected {
		if tools[i] != want {
			t.Errorf("tool[%d]: expected %q, got %q", i, want, tools[i])
		}
	}

	// Verify comments and blank lines were skipped
	for _, tool := range tools {
		if strings.HasPrefix(tool, "#") {
			t.Errorf("tool should not start with #: %q", tool)
		}
		if tool == "" {
			t.Error("empty tool name should not appear")
		}
	}
}
