package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ── Resolved Configuration ──────────────────────────────

// ResolvedConfig holds the fully resolved configuration after applying
// profile resolution, profile merges, and CLI overrides.
type ResolvedConfig struct {
	Profile        string   `json:"profile"`
	NetMode        string   `json:"net_mode"`
	FsMode         string   `json:"fs_mode"`
	Requisites     string   `json:"requisites"`
	AllowLocalhost bool     `json:"allow_localhost"`
	Writable       []string `json:"writable"`
	ReadOnly       []string `json:"read_only"`
	Denied         []string `json:"denied"`
	EnvPassthrough []string `json:"env_passthrough"`
}

// ResolveConfig resolves the active configuration by applying three-level
// profile precedence (SANDFLOX_PROFILE env > policy.Meta.Profile > "default"),
// merging profile overrides with top-level settings, and applying CLI flag
// overrides on top.
func ResolveConfig(policy *Policy, flags *CLIFlags, projectDir string) *ResolvedConfig {
	// 1. Determine profile name: env var > policy file > "default"
	profileName := os.Getenv("SANDFLOX_PROFILE")
	if profileName == "" {
		profileName = policy.Meta.Profile
	}
	if profileName == "" {
		profileName = "default"
	}

	// 2. CLI --profile overrides everything (including env var)
	if flags.Profile != "" {
		profileName = flags.Profile
	}

	// 3. Look up profile section; use empty if not found
	profile := policy.Profiles[profileName]

	// 4. Merge: profile overrides top-level modes
	netMode := policy.Network.Mode
	if profile.Network != "" {
		netMode = profile.Network
	}

	fsMode := policy.Filesystem.Mode
	if profile.Filesystem != "" {
		fsMode = profile.Filesystem
	}

	// 5. CLI --net flag forces unrestricted
	if flags.Net {
		netMode = "unrestricted"
	}

	// 6. Requisites: CLI > profile > default
	requisites := "requisites.txt"
	if profile.Requisites != "" {
		requisites = profile.Requisites
	}
	if flags.Requisites != "" {
		requisites = flags.Requisites
	}

	// 7. Resolve all paths
	writable := resolvePaths(policy.Filesystem.Writable, projectDir)
	readOnly := resolvePaths(policy.Filesystem.ReadOnly, projectDir)
	denied := resolvePaths(policy.Filesystem.Denied, projectDir)

	return &ResolvedConfig{
		Profile:        profileName,
		NetMode:        netMode,
		FsMode:         fsMode,
		Requisites:     requisites,
		AllowLocalhost: policy.Network.AllowLocalhost,
		Writable:       writable,
		ReadOnly:       readOnly,
		Denied:         denied,
	}
}

// ── Path Resolution ─────────────────────────────────────

// ResolvePath resolves a single path from policy.toml to an absolute path.
// Expands ~ to $HOME, resolves relative paths against projectDir, and
// canonicalizes /tmp to /private/tmp on macOS. Preserves trailing slash
// (directory indicator) through filepath.Clean.
func ResolvePath(p string, projectDir string) string {
	isDir := strings.HasSuffix(p, "/")

	// Expand ~ to home directory
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[2:])
	} else if !filepath.IsAbs(p) {
		// Relative paths (including ".") resolve against project dir
		p = filepath.Join(projectDir, p)
	}

	p = filepath.Clean(p)

	// macOS: /tmp is a symlink to /private/tmp
	if runtime.GOOS == "darwin" && (p == "/tmp" || strings.HasPrefix(p, "/tmp/")) {
		p = "/private" + p
	}

	// Restore trailing slash for directory paths
	if isDir {
		p += "/"
	}

	return p
}

// resolvePaths resolves a slice of paths using ResolvePath.
func resolvePaths(paths []string, projectDir string) []string {
	if paths == nil {
		return nil
	}
	resolved := make([]string, len(paths))
	for i, p := range paths {
		resolved[i] = ResolvePath(p, projectDir)
	}
	return resolved
}

// ── Requisites Parser ───────────────────────────────────

// ParseRequisites reads a requisites file and returns tool names.
// Skips blank lines and lines starting with #. Takes first
// whitespace-delimited token from each line (handles trailing comments).
func ParseRequisites(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tools []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Take first whitespace-delimited token
		tool := strings.Fields(line)[0]
		tools = append(tools, tool)
	}
	return tools, scanner.Err()
}
