package main

// ── Policy Types ────────────────────────────────────────

// Policy represents a parsed policy.toml v2 configuration.
type Policy struct {
	Meta       MetaSection
	Network    NetworkSection
	Filesystem FilesystemSection
	Profiles   map[string]ProfileSection
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

// ParsePolicy reads policy.toml and returns a validated Policy.
// Returns an error with context for any parse or validation failure.
func ParsePolicy(path string) (*Policy, error) {
	// Stub -- will be implemented in GREEN phase
	return nil, nil
}
