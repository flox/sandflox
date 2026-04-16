package main

import "flag"

// ── CLI Flags ────────────────────────────────────────────

// CLIFlags holds parsed command-line flags for sandflox.
type CLIFlags struct {
	Profile    string
	PolicyPath string
	Net        bool
	Debug      bool
	Requisites string
}

// ParseFlags parses sandflox CLI flags from args and returns the flags
// plus any remaining arguments (everything after -- or the first non-flag arg).
// Uses flag.ContinueOnError so tests can exercise error paths.
func ParseFlags(args []string) (*CLIFlags, []string) {
	flags := &CLIFlags{}
	fs := flag.NewFlagSet("sandflox", flag.ContinueOnError)
	fs.StringVar(&flags.Profile, "profile", "", "Override active profile")
	fs.StringVar(&flags.PolicyPath, "policy", "", "Path to policy.toml")
	fs.BoolVar(&flags.Net, "net", false, "Override network to unrestricted")
	fs.BoolVar(&flags.Debug, "debug", false, "Emit verbose diagnostics")
	fs.StringVar(&flags.Requisites, "requisites", "", "Override requisites file")
	fs.Parse(args)
	return flags, fs.Args()
}
