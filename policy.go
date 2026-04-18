package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"strings"
)

// ── Embedded Default Policy ─────────────────────────────

//go:embed policy.toml
var embeddedDefaultPolicy []byte

// ── Policy Types ────────────────────────────────────────

// Policy represents a parsed policy.toml v2 configuration.
type Policy struct {
	Meta       MetaSection
	Network    NetworkSection
	Filesystem FilesystemSection
	Security   SecuritySection
	Profiles   map[string]ProfileSection
}

// SecuritySection holds the [security] table values.
type SecuritySection struct {
	EnvPassthrough []string
}

// MetaSection holds the [meta] table values.
type MetaSection struct {
	Version string
	Profile string
}

// NetworkSection holds the [network] table values.
type NetworkSection struct {
	Mode           string // "blocked" | "unrestricted"
	AllowLocalhost bool
}

// FilesystemSection holds the [filesystem] table values.
type FilesystemSection struct {
	Mode     string   // "permissive" | "workspace" | "strict"
	Writable []string
	ReadOnly []string
	Denied   []string
}

// ProfileSection holds a [profiles.*] table values.
type ProfileSection struct {
	Requisites string
	Network    string
	Filesystem string
}

// ── TOML Subset Parser ─────────────────────────────────
// Handles only the features policy.toml v2 uses:
//   - [sections] and [dotted.sections]
//   - key = "string" (with inline comments)
//   - key = true / false
//   - key = ["array", "of", "strings"]
//   - # full-line comments
//   - blank lines
// Rejects unsupported TOML: [[array.of.tables]], inline tables {}, multiline strings.

// ParsePolicy reads a policy.toml file at path and returns a validated Policy.
// All errors are prefixed with [sandflox] ERROR: per project convention.
func ParsePolicy(path string) (*Policy, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("[sandflox] ERROR: cannot open policy file: %w", err)
	}
	defer f.Close()

	// Intermediate representation: nested map from TOML sections
	sections := make(map[string]map[string]interface{})
	var currentSection string

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip blank lines and full-line comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Reject unsupported TOML: array of tables [[...]]
		if strings.HasPrefix(trimmed, "[[") {
			return nil, fmt.Errorf("[sandflox] ERROR: line %d: unsupported TOML feature: array of tables ([[...]])", lineNum)
		}

		// Section header: [section] or [dotted.section]
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := trimmed[1 : len(trimmed)-1]
			sectionName = strings.TrimSpace(sectionName)
			if sectionName == "" {
				return nil, fmt.Errorf("[sandflox] ERROR: line %d: empty section header", lineNum)
			}
			currentSection = sectionName
			if _, exists := sections[currentSection]; !exists {
				sections[currentSection] = make(map[string]interface{})
			}
			continue
		}

		// Key = value pair
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			return nil, fmt.Errorf("[sandflox] ERROR: line %d: expected key = value, got %q", lineNum, trimmed)
		}

		key := strings.TrimSpace(trimmed[:eqIdx])
		rawValue := strings.TrimSpace(trimmed[eqIdx+1:])

		if key == "" {
			return nil, fmt.Errorf("[sandflox] ERROR: line %d: empty key", lineNum)
		}

		value, err := parseValue(rawValue, lineNum)
		if err != nil {
			return nil, err
		}

		if currentSection == "" {
			// Top-level keys outside any section -- treat as root
			if _, exists := sections[""]; !exists {
				sections[""] = make(map[string]interface{})
			}
			sections[""][key] = value
		} else {
			sections[currentSection][key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("[sandflox] ERROR: reading policy file: %w", err)
	}

	// Map intermediate representation into typed Policy struct
	policy, err := mapToPolicy(sections)
	if err != nil {
		return nil, err
	}

	// Validate
	if err := validatePolicy(policy); err != nil {
		return nil, err
	}

	return policy, nil
}

// ParsePolicyBytes parses a policy.toml from raw bytes. The filename
// parameter is used only in error messages. This avoids writing a temp
// file when using the embedded default policy.
func ParsePolicyBytes(data []byte, filename string) (*Policy, error) {
	sections := make(map[string]map[string]interface{})
	var currentSection string

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "[[") {
			return nil, fmt.Errorf("[sandflox] ERROR: %s line %d: unsupported TOML feature: array of tables ([[...]])", filename, lineNum)
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := trimmed[1 : len(trimmed)-1]
			sectionName = strings.TrimSpace(sectionName)
			if sectionName == "" {
				return nil, fmt.Errorf("[sandflox] ERROR: %s line %d: empty section header", filename, lineNum)
			}
			currentSection = sectionName
			if _, exists := sections[currentSection]; !exists {
				sections[currentSection] = make(map[string]interface{})
			}
			continue
		}

		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			return nil, fmt.Errorf("[sandflox] ERROR: %s line %d: expected key = value, got %q", filename, lineNum, trimmed)
		}

		key := strings.TrimSpace(trimmed[:eqIdx])
		rawValue := strings.TrimSpace(trimmed[eqIdx+1:])

		if key == "" {
			return nil, fmt.Errorf("[sandflox] ERROR: %s line %d: empty key", filename, lineNum)
		}

		value, err := parseValue(rawValue, lineNum)
		if err != nil {
			return nil, err
		}

		if currentSection == "" {
			if _, exists := sections[""]; !exists {
				sections[""] = make(map[string]interface{})
			}
			sections[""][key] = value
		} else {
			sections[currentSection][key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("[sandflox] ERROR: reading %s: %w", filename, err)
	}

	policy, err := mapToPolicy(sections)
	if err != nil {
		return nil, err
	}

	if err := validatePolicy(policy); err != nil {
		return nil, err
	}

	return policy, nil
}

// DefaultPolicy returns the embedded default policy parsed from the
// built-in policy.toml. Used by `sandflox init` and `sandflox prepare`
// fallback when no policy.toml is found.
func DefaultPolicy() (*Policy, error) {
	return ParsePolicyBytes(embeddedDefaultPolicy, "embedded:policy.toml")
}

// parseValue parses a TOML value from the right side of key = value.
// Handles: quoted strings, booleans, string arrays. Rejects inline tables.
func parseValue(raw string, lineNum int) (interface{}, error) {
	if raw == "" {
		return nil, fmt.Errorf("[sandflox] ERROR: line %d: empty value", lineNum)
	}

	// Quoted string: find closing quote, ignore trailing comment
	if strings.HasPrefix(raw, "\"") {
		closeIdx := strings.Index(raw[1:], "\"")
		if closeIdx < 0 {
			return nil, fmt.Errorf("[sandflox] ERROR: line %d: unterminated string", lineNum)
		}
		return raw[1 : closeIdx+1], nil
	}

	// String array: ["a", "b", "c"]
	if strings.HasPrefix(raw, "[") {
		closeIdx := strings.Index(raw, "]")
		if closeIdx < 0 {
			return nil, fmt.Errorf("[sandflox] ERROR: line %d: unterminated array", lineNum)
		}
		inner := raw[1:closeIdx]
		if strings.TrimSpace(inner) == "" {
			return []string{}, nil
		}
		parts := strings.Split(inner, ",")
		var result []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// Each element should be a quoted string
			if strings.HasPrefix(part, "\"") && strings.HasSuffix(part, "\"") {
				result = append(result, part[1:len(part)-1])
			} else {
				return nil, fmt.Errorf("[sandflox] ERROR: line %d: array element must be a quoted string, got %q", lineNum, part)
			}
		}
		return result, nil
	}

	// Reject inline tables
	if strings.HasPrefix(raw, "{") {
		return nil, fmt.Errorf("[sandflox] ERROR: line %d: unsupported TOML feature: inline table ({...})", lineNum)
	}

	// Reject multiline strings
	if strings.HasPrefix(raw, "'''") || strings.HasPrefix(raw, "\"\"\"") {
		return nil, fmt.Errorf("[sandflox] ERROR: line %d: unsupported TOML feature: multiline string", lineNum)
	}

	// Strip inline comment for bare values (booleans, etc.)
	bare := raw
	if idx := strings.Index(bare, "#"); idx >= 0 {
		bare = strings.TrimSpace(bare[:idx])
	}

	// Booleans
	if bare == "true" {
		return true, nil
	}
	if bare == "false" {
		return false, nil
	}

	// Bare unquoted value -- return as string
	return bare, nil
}

// mapToPolicy converts the intermediate section map into a typed Policy struct.
func mapToPolicy(sections map[string]map[string]interface{}) (*Policy, error) {
	p := &Policy{
		Profiles: make(map[string]ProfileSection),
	}

	// [meta]
	if meta, ok := sections["meta"]; ok {
		if v, ok := meta["version"]; ok {
			p.Meta.Version = toString(v)
		}
		if v, ok := meta["profile"]; ok {
			p.Meta.Profile = toString(v)
		}
	}

	// [network]
	if net, ok := sections["network"]; ok {
		if v, ok := net["mode"]; ok {
			p.Network.Mode = toString(v)
		}
		if v, ok := net["allow-localhost"]; ok {
			p.Network.AllowLocalhost = toBool(v)
		}
	}

	// [filesystem]
	if fs, ok := sections["filesystem"]; ok {
		if v, ok := fs["mode"]; ok {
			p.Filesystem.Mode = toString(v)
		}
		if v, ok := fs["writable"]; ok {
			p.Filesystem.Writable = toStringSlice(v)
		}
		if v, ok := fs["read-only"]; ok {
			p.Filesystem.ReadOnly = toStringSlice(v)
		}
		if v, ok := fs["denied"]; ok {
			p.Filesystem.Denied = toStringSlice(v)
		}
	}

	// [security]
	if sec, ok := sections["security"]; ok {
		if v, ok := sec["env-passthrough"]; ok {
			p.Security.EnvPassthrough = toStringSlice(v)
		}
	}

	// [profiles.*] -- dotted sections
	for sectionName, kvs := range sections {
		if !strings.HasPrefix(sectionName, "profiles.") {
			continue
		}
		profileName := sectionName[len("profiles."):]
		ps := ProfileSection{}
		if v, ok := kvs["requisites"]; ok {
			ps.Requisites = toString(v)
		}
		if v, ok := kvs["network"]; ok {
			ps.Network = toString(v)
		}
		if v, ok := kvs["filesystem"]; ok {
			ps.Filesystem = toString(v)
		}
		p.Profiles[profileName] = ps
	}

	return p, nil
}

// validatePolicy checks all enum values and required fields.
func validatePolicy(p *Policy) error {
	// Version must be "2" (D-03: hard error, no warn-and-continue)
	if p.Meta.Version == "" {
		return fmt.Errorf("[sandflox] ERROR: missing required field: meta.version")
	}
	if p.Meta.Version != "2" {
		return fmt.Errorf("[sandflox] ERROR: unsupported policy version %q (expected \"2\")", p.Meta.Version)
	}

	// Network mode validation (if set)
	if p.Network.Mode != "" {
		if !isValidNetworkMode(p.Network.Mode) {
			return fmt.Errorf("[sandflox] ERROR: invalid network mode %q (expected \"blocked\" or \"unrestricted\")", p.Network.Mode)
		}
	}

	// Filesystem mode validation (if set)
	if p.Filesystem.Mode != "" {
		if !isValidFilesystemMode(p.Filesystem.Mode) {
			return fmt.Errorf("[sandflox] ERROR: invalid filesystem mode %q (expected \"permissive\", \"workspace\", or \"strict\")", p.Filesystem.Mode)
		}
	}

	// Validate profile-level network and filesystem modes
	for name, profile := range p.Profiles {
		if profile.Network != "" && !isValidNetworkMode(profile.Network) {
			return fmt.Errorf("[sandflox] ERROR: invalid network mode %q in profile %q (expected \"blocked\" or \"unrestricted\")", profile.Network, name)
		}
		if profile.Filesystem != "" && !isValidFilesystemMode(profile.Filesystem) {
			return fmt.Errorf("[sandflox] ERROR: invalid filesystem mode %q in profile %q (expected \"permissive\", \"workspace\", or \"strict\")", profile.Filesystem, name)
		}
	}

	return nil
}

func isValidNetworkMode(mode string) bool {
	return mode == "blocked" || mode == "unrestricted"
}

func isValidFilesystemMode(mode string) bool {
	return mode == "permissive" || mode == "workspace" || mode == "strict"
}

// ── Type conversion helpers ─────────────────────────────

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	// Fallback for string "true"/"false"
	if s, ok := v.(string); ok {
		return s == "true"
	}
	return false
}

func toStringSlice(v interface{}) []string {
	if ss, ok := v.([]string); ok {
		return ss
	}
	return nil
}
