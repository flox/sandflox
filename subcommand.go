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

// ── Cache Discovery ─────────────────────────────────────

// discoverCacheDir finds the sandflox cache directory containing config.json.
// Tries FLOX_ENV_CACHE first (set inside a flox activate session), then
// falls back to resolving from the project directory.
// Returns "" if no cache is found.
func discoverCacheDir(flags *CLIFlags) string {
	// 1. Try FLOX_ENV_CACHE env var (set inside `flox activate`)
	if envCache := os.Getenv("FLOX_ENV_CACHE"); envCache != "" {
		candidate := filepath.Join(envCache, "sandflox")
		if _, err := os.Stat(filepath.Join(candidate, "config.json")); err == nil {
			return candidate
		}
	}

	// 2. Fall back to project dir relative path
	projectDir := resolveProjectDir(flags)
	candidate := filepath.Join(projectDir, ".flox", "cache", "sandflox")
	if _, err := os.Stat(filepath.Join(candidate, "config.json")); err == nil {
		return candidate
	}

	return ""
}

// ── Status Handler ──────────────────────────────────────

// runStatus reads cached enforcement state and prints a summary.
// Exits 0 on success, 1 if not in a sandflox session.
func runStatus(flags *CLIFlags) {
	cacheDir := discoverCacheDir(flags)
	os.Exit(runStatusInternal(cacheDir, flags.Debug))
}

// runStatusWithExitCode is the testable core of runStatus.
// Takes the resolved cacheDir (or "" if no cache found).
func runStatusWithExitCode(cacheDir string) int {
	return runStatusInternal(cacheDir, false)
}

// runStatusDebugWithExitCode is the testable debug variant.
func runStatusDebugWithExitCode(cacheDir string) int {
	return runStatusInternal(cacheDir, true)
}

// runStatusInternal implements the status logic for both normal and debug modes.
func runStatusInternal(cacheDir string, debug bool) int {
	if cacheDir == "" {
		fmt.Fprintf(stderr, "[sandflox] Not in a sandflox session -- no cached state found. Run 'sandflox' first.\n")
		return 1
	}

	config, err := ReadCache(cacheDir)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	// Count tools from cached requisites
	tools, err := ParseRequisites(filepath.Join(cacheDir, "requisites.txt"))
	toolCount := 0
	if err == nil {
		toolCount = len(tools)
	}

	// Print status summary
	fmt.Fprintf(stderr, "[sandflox] Profile: %s | Network: %s | Filesystem: %s\n",
		config.Profile, config.NetMode, config.FsMode)
	fmt.Fprintf(stderr, "[sandflox] Tools: %d | Denied paths: %d\n",
		toolCount, len(config.Denied))

	if debug {
		fmt.Fprintf(stderr, "[sandflox] Requisites: %s\n", config.Requisites)
		fmt.Fprintf(stderr, "[sandflox] Allow localhost: %v\n", config.AllowLocalhost)
		fmt.Fprintf(stderr, "[sandflox] Writable paths: %v\n", config.Writable)
		fmt.Fprintf(stderr, "[sandflox] Read-only paths: %v\n", config.ReadOnly)
		fmt.Fprintf(stderr, "[sandflox] Denied paths: %v\n", config.Denied)
	}

	return 0
}

// ── Elevate Handler ─────────────────────────────────────

// checkElevatePrereqs checks whether the elevate subcommand can proceed.
// Returns ("", -1) if all checks pass and execution should continue.
// Returns (message, exitCode) if a check stops execution.
//
// Testable: does not call os.Exit. runElevate calls os.Exit based on
// the returned values.
func checkElevatePrereqs() (string, int) {
	// 1. Re-entry detection: already sandboxed
	if os.Getenv("SANDFLOX_ENABLED") == "1" {
		return "[sandflox] Already sandboxed -- nothing to do.\n", 0
	}
	// 2. Flox session detection: not in a flox session
	if os.Getenv("FLOX_ENV") == "" {
		return "[sandflox] Not in a flox session. Run `flox activate` first.\n", 1
	}
	return "", -1
}

// runElevate re-execs the current shell under sandbox-exec for kernel
// enforcement. Detects re-entry (already sandboxed) and missing flox
// session. Uses os.Exit -- see runElevateWithExitCode for testable variant.
func runElevate(flags *CLIFlags) {
	os.Exit(runElevateWithExitCode(flags))
}

// runElevateWithExitCode is the testable core of runElevate. Returns the
// exit code instead of calling os.Exit.
func runElevateWithExitCode(flags *CLIFlags) int {
	// 1-2. Prereq checks (re-entry + flox session)
	if msg, code := checkElevatePrereqs(); msg != "" {
		fmt.Fprint(stderr, msg)
		return code
	}

	// 3. Read FLOX_ENV_CACHE early (Pitfall 6: capture before env manipulation)
	floxEnvCache := os.Getenv("FLOX_ENV_CACHE")

	// 4. Standard pipeline: resolve project dir, parse policy
	projectDir := resolveProjectDir(flags)

	policyPath := filepath.Join(projectDir, "policy.toml")
	if flags.PolicyPath != "" {
		policyPath = flags.PolicyPath
	}

	policy, err := ParsePolicy(policyPath)
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: %v\n", err)
		return 1
	}

	// 5. Resolve config
	config := ResolveConfig(policy, flags, projectDir)

	// 6. Determine cache dir
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")
	if floxEnvCache != "" {
		cacheDir = filepath.Join(floxEnvCache, "sandflox")
	}

	// 7. Write cache artifacts
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	// 8. Generate shell enforcement artifacts
	if err := WriteShellArtifacts(cacheDir, config); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	entrypointPath := filepath.Join(cacheDir, "entrypoint.sh")

	// 9. Diagnostics
	emitDiagnostics(config, projectDir, flags.Debug)

	// 10. Call platform-specific exec (does not return on success)
	elevateExec(config, projectDir, entrypointPath)

	// If we reach here on darwin, elevateExec already called os.Exit(1)
	// On non-darwin, elevateExec prints error and exits -- but just in case:
	return 1
}
