package main

import "testing"

// ── CLI Flag Parsing Tests ──────────────────────────────

func TestCLIFlagProfile(t *testing.T) {
	flags, _ := ParseFlags([]string{"--profile", "minimal"})
	if flags.Profile != "minimal" {
		t.Errorf("expected Profile='minimal', got %q", flags.Profile)
	}
}

func TestCLIFlagNet(t *testing.T) {
	flags, _ := ParseFlags([]string{"--net"})
	if !flags.Net {
		t.Error("expected Net=true, got false")
	}
}

func TestCLIFlagDebug(t *testing.T) {
	flags, _ := ParseFlags([]string{"--debug"})
	if !flags.Debug {
		t.Error("expected Debug=true, got false")
	}
}

func TestCLIFlagPolicy(t *testing.T) {
	flags, _ := ParseFlags([]string{"--policy", "/path/to/policy.toml"})
	if flags.PolicyPath != "/path/to/policy.toml" {
		t.Errorf("expected PolicyPath='/path/to/policy.toml', got %q", flags.PolicyPath)
	}
}

func TestCLIFlagRequisites(t *testing.T) {
	flags, _ := ParseFlags([]string{"--requisites", "requisites-full.txt"})
	if flags.Requisites != "requisites-full.txt" {
		t.Errorf("expected Requisites='requisites-full.txt', got %q", flags.Requisites)
	}
}

func TestCLISeparator(t *testing.T) {
	flags, remaining := ParseFlags([]string{"--debug", "--", "echo", "hello"})
	if !flags.Debug {
		t.Error("expected Debug=true before --")
	}
	if len(remaining) != 2 || remaining[0] != "echo" || remaining[1] != "hello" {
		t.Errorf("expected remaining=[echo, hello], got %v", remaining)
	}
}

func TestCLINoFlags(t *testing.T) {
	flags, remaining := ParseFlags([]string{})
	if flags.Profile != "" || flags.PolicyPath != "" || flags.Net || flags.Debug || flags.Requisites != "" {
		t.Error("expected all flags to be zero values")
	}
	if len(remaining) != 0 {
		t.Errorf("expected no remaining args, got %v", remaining)
	}
}

func TestCLIOverrideProfile(t *testing.T) {
	// CLI --profile should set the profile, which ResolveConfig respects
	flags, _ := ParseFlags([]string{"--profile", "full"})

	policy := &Policy{
		Meta:    MetaSection{Version: "2", Profile: "default"},
		Network: NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles: map[string]ProfileSection{
			"full": {
				Requisites: "requisites-full.txt",
				Network:    "unrestricted",
				Filesystem: "permissive",
			},
		},
	}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.Profile != "full" {
		t.Errorf("expected profile='full', got %q", config.Profile)
	}
	if config.NetMode != "unrestricted" {
		t.Errorf("expected net_mode='unrestricted', got %q", config.NetMode)
	}
}

func TestCLIOverrideNet(t *testing.T) {
	flags, _ := ParseFlags([]string{"--net"})

	policy := &Policy{
		Meta:       MetaSection{Version: "2", Profile: "default"},
		Network:    NetworkSection{Mode: "blocked"},
		Filesystem: FilesystemSection{Mode: "workspace"},
		Profiles:   map[string]ProfileSection{},
	}

	config := ResolveConfig(policy, flags, "/tmp/test")
	if config.NetMode != "unrestricted" {
		t.Errorf("expected net_mode='unrestricted' with --net flag, got %q", config.NetMode)
	}
}
