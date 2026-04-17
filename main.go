package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// ── Package-level variables ─────────────────────────────

// Version is set at build time via ldflags.
var Version = "dev"

// stderr is the writer for diagnostic output. Package-level variable
// so tests can redirect it to a buffer without subprocess spawning.
var stderr io.Writer = os.Stderr

// ── Main Entry Point ────────────────────────────────────

func main() {
	// 0. Extract subcommand (if any) before flag parsing
	subcmd, remaining := extractSubcommand(os.Args[1:])

	// 1. Parse CLI flags from remaining args
	flags, userArgs := ParseFlags(remaining)

	// 2. Route to subcommand handler or continue default pipeline
	switch subcmd {
	case "validate":
		runValidate(flags)
	case "status":
		runStatus(flags)
	case "elevate":
		runElevate(flags)
	default:
		// Original pipeline (steps 2-8)
		runDefault(flags, userArgs)
	}
}

// runDefault executes the original sandflox pipeline: parse policy,
// resolve config, write cache, generate shell artifacts, emit diagnostics,
// and exec into flox activate wrapped in kernel enforcement.
func runDefault(flags *CLIFlags, userArgs []string) {
	// 2. Determine project directory
	projectDir := resolveProjectDir(flags)

	// 3. Find and parse policy.toml
	policyPath := filepath.Join(projectDir, "policy.toml")
	if flags.PolicyPath != "" {
		policyPath = flags.PolicyPath
	}

	policy, err := ParsePolicy(policyPath)
	if err != nil {
		// If policy.toml does not exist, fall through to bare flox activate
		if os.IsNotExist(unwrapPathError(err)) {
			fmt.Fprintf(stderr, "[sandflox] No policy.toml found -- falling back to flox activate\n")
			execFlox(nil, userArgs)
			return
		}
		// Parse error -- report and exit
		fmt.Fprintf(stderr, "%v\n", err)
		os.Exit(1)
	}

	// 4. Resolve config (profile + merge + CLI overrides)
	config := ResolveConfig(policy, flags, projectDir)

	// 5. Write cache artifacts
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")
	if err := WriteCache(cacheDir, config, projectDir); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		os.Exit(1)
	}

	// 6. Generate shell enforcement artifacts (D-04: regenerate every run)
	if err := WriteShellArtifacts(cacheDir, config); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		os.Exit(1)
	}
	entrypointPath := filepath.Join(cacheDir, "entrypoint.sh")

	// 7. Emit diagnostics to stderr
	emitDiagnostics(config, projectDir, flags.Debug)

	// 8. Exec into flox activate, wrapped in kernel enforcement when available
	execWithKernelEnforcement(config, projectDir, entrypointPath, userArgs)
}

// ── Project Directory Resolution ────────────────────────

// resolveProjectDir determines the project directory.
// If --policy is specified, uses its parent directory.
// Otherwise uses the current working directory.
func resolveProjectDir(flags *CLIFlags) string {
	if flags.PolicyPath != "" {
		absPath, err := filepath.Abs(flags.PolicyPath)
		if err != nil {
			fmt.Fprintf(stderr, "[sandflox] ERROR: cannot resolve policy path: %v\n", err)
			os.Exit(1)
		}
		return filepath.Dir(absPath)
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	return dir
}

// ── Diagnostics ─────────────────────────────────────────

// emitDiagnostics writes [sandflox] prefixed diagnostic messages to stderr.
// Always emits the summary line. Debug mode adds verbose details including
// the generated SBPL file path and rule count (D-07).
func emitDiagnostics(config *ResolvedConfig, projectDir string, debug bool) {
	// Always: summary line matching existing bash format
	fmt.Fprintf(stderr, "[sandflox] Profile: %s | Network: %s | Filesystem: %s\n",
		config.Profile, config.NetMode, config.FsMode)

	if debug {
		fmt.Fprintf(stderr, "[sandflox] Requisites: %s\n", config.Requisites)
		fmt.Fprintf(stderr, "[sandflox] Allow localhost: %v\n", config.AllowLocalhost)
		fmt.Fprintf(stderr, "[sandflox] Writable paths: %v\n", config.Writable)
		fmt.Fprintf(stderr, "[sandflox] Read-only paths: %v\n", config.ReadOnly)
		fmt.Fprintf(stderr, "[sandflox] Denied paths: %v\n", config.Denied)

		// D-07: SBPL diagnostic -- path and rule count. The actual .sb
		// file is written by execWithKernelEnforcement (darwin) via
		// WriteSBPL; here we just recompute the content to count rules.
		home, _ := os.UserHomeDir()
		sbplContent := GenerateSBPL(config, home)
		sbplPath := filepath.Join(projectDir, ".flox", "cache", "sandflox", "sandflox.sb")
		ruleCount := strings.Count(sbplContent, "\n(deny ") + strings.Count(sbplContent, "\n(allow ")
		fmt.Fprintf(stderr, "[sandflox] sbpl: %s (%d rules)\n", sbplPath, ruleCount)

		// Env scrubbing diagnostic: counts only (values are secrets).
		env := BuildSanitizedEnv(config)
		parentCount := len(os.Environ())
		passedCount := len(env)
		forcedCount := len(forcedVars)
		blockedCount := parentCount - passedCount + forcedCount
		fmt.Fprintf(stderr, "[sandflox] Env: %d vars passed, %d blocked, %d forced\n",
			passedCount, blockedCount, forcedCount)
	}
}

// ── Exec into flox activate ─────────────────────────────

// execFlox replaces the current process with flox activate using syscall.Exec.
// If userArgs is empty, starts interactive mode. Otherwise wraps the command
// in non-interactive mode with --. When cfg is non-nil, the process inherits
// a sanitized environment via BuildSanitizedEnv; when nil (no policy.toml),
// the parent environment passes through unchanged.
func execFlox(cfg *ResolvedConfig, userArgs []string) {
	floxPath, err := exec.LookPath("flox")
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: flox not found in PATH\n")
		os.Exit(1)
	}

	// Build argv: interactive or non-interactive
	argv := []string{"flox", "activate"}
	if len(userArgs) > 0 {
		argv = append(argv, "--")
		argv = append(argv, userArgs...)
	}

	// Sanitize environment when policy is available; pass through unchanged
	// when no policy.toml was found (cfg == nil). Per Research Open Question 1:
	// graceful degradation preserves existing behavior without a policy file.
	env := os.Environ()
	if cfg != nil {
		env = BuildSanitizedEnv(cfg)
	}

	// Replace this process with flox -- does not return on success
	execErr := syscall.Exec(floxPath, argv, env)
	// If we get here, exec failed
	fmt.Fprintf(stderr, "[sandflox] ERROR: exec failed: %v\n", execErr)
	os.Exit(1)
}

// ── Helpers ─────────────────────────────────────────────

// unwrapPathError extracts the underlying error from os.PathError for
// os.IsNotExist checks that work with wrapped errors.
func unwrapPathError(err error) error {
	if err == nil {
		return nil
	}
	// The [sandflox] ERROR: wrapper means os.IsNotExist won't work directly.
	// Check if the error message indicates file not found.
	for e := err; e != nil; {
		if pe, ok := e.(*os.PathError); ok {
			return pe
		}
		if u, ok := e.(interface{ Unwrap() error }); ok {
			e = u.Unwrap()
		} else {
			break
		}
	}
	return err
}
