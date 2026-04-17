// subcommand.go -- Subcommand routing and handlers for validate, status, elevate.
//
// extractSubcommand scans args for known subcommand names and separates
// the subcommand from the remaining flags/args. This enables both
// `sandflox --debug validate` and `sandflox validate --debug` to work
// identically -- the subcommand name is removed, and remaining args
// (including flags) are passed to ParseFlags.
//
// Handlers (runValidate, runStatus, runElevate) are read-only operations
// that reuse the existing pipeline stages (policy parsing, config
// resolution, cache writing, diagnostics) without launching a sandbox.

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// ── Known Subcommands ───────────────────────────────────

// knownSubcommands maps subcommand names to true. Any arg matching a key
// is treated as a subcommand and routed to its handler. Unknown first
// args fall through to the default exec pipeline (backward compat).
var knownSubcommands = map[string]bool{
	"validate": true,
	"status":   true,
	"elevate":  true,
}

// ── Subcommand Extraction ───────────────────────────────

// extractSubcommand scans args for the first element matching a known
// subcommand name. Returns the subcommand (or "") and the remaining args
// with the subcommand stripped out.
//
// Scanning all positions (not just first) enables `--debug validate` to
// find "validate" at index 1. Args like "--" that happen to precede a
// known subcommand name are NOT treated as delimiters -- but "--" itself
// is never a subcommand, so ["--", "echo", "hello"] returns ("", [...]).
//
// If no known subcommand is found, returns ("", args) unchanged.
func extractSubcommand(args []string) (string, []string) {
	for i, arg := range args {
		if knownSubcommands[arg] {
			remaining := make([]string, 0, len(args)-1)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+1:]...)
			return arg, remaining
		}
		// Stop scanning at "--" -- everything after is user command args,
		// not subcommands.
		if arg == "--" {
			break
		}
	}
	return "", args
}

// ── Validate Handler ────────────────────────────────────

// runValidate parses and validates policy.toml, then prints a summary
// without launching a sandbox. Exits 0 on success, 1 on error.
// Uses os.Exit -- see runValidateWithExitCode for a testable variant.
func runValidate(flags *CLIFlags) {
	os.Exit(runValidateWithExitCode(flags))
}

// runValidateWithExitCode is the testable core of runValidate. Returns
// the exit code instead of calling os.Exit, so tests can capture output
// via the package-level stderr writer.
func runValidateWithExitCode(flags *CLIFlags) int {
	// 1. Determine project directory
	projectDir := resolveProjectDir(flags)

	// 2. Find and parse policy.toml
	policyPath := filepath.Join(projectDir, "policy.toml")
	if flags.PolicyPath != "" {
		policyPath = flags.PolicyPath
	}

	policy, err := ParsePolicy(policyPath)
	if err != nil {
		// Unlike main's default pipeline, validate does NOT fall through
		// to bare flox activate on missing policy. It reports the error.
		fmt.Fprintf(stderr, "[sandflox] ERROR: %v\n", err)
		return 1
	}

	// 3. Resolve config
	config := ResolveConfig(policy, flags, projectDir)

	// 4. Write cache artifacts (so generated files are available for counting)
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	// 5. Generate shell enforcement artifacts
	if err := WriteShellArtifacts(cacheDir, config); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	// 6. Count tools from cached requisites
	tools, err := ParseRequisites(filepath.Join(cacheDir, "requisites.txt"))
	toolCount := 0
	if err == nil {
		toolCount = len(tools)
	}

	// 7. Print summary to stderr
	fmt.Fprintf(stderr, "[sandflox] Policy: %s (valid)\n", filepath.Base(policyPath))
	fmt.Fprintf(stderr, "[sandflox] Profile: %s | Network: %s | Filesystem: %s\n",
		config.Profile, config.NetMode, config.FsMode)
	fmt.Fprintf(stderr, "[sandflox] Tools: %d (from %s)\n", toolCount, config.Requisites)
	fmt.Fprintf(stderr, "[sandflox] Denied paths: %d\n", len(config.Denied))

	// 8. Debug mode: emit full diagnostics
	if flags.Debug {
		emitDiagnostics(config, projectDir, true)
	}

	return 0
}

// ── Status Handler (stub -- implemented in Task 2) ──────

// runStatus reads cached enforcement state and prints a summary.
// Implemented in Task 2.
func runStatus(flags *CLIFlags) {
	fmt.Fprintf(stderr, "[sandflox] ERROR: status not yet implemented\n")
	os.Exit(1)
}

// ── Elevate Handler (stub -- future plan) ───────────────

// runElevate re-execs the current shell under sandbox-exec.
// Placeholder for future implementation.
func runElevate(flags *CLIFlags) {
	fmt.Fprintf(stderr, "[sandflox] ERROR: elevate not yet implemented\n")
	os.Exit(1)
}
