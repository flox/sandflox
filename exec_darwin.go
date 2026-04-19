//go:build darwin

// exec_darwin.go -- sandbox-exec wrapping for macOS kernel enforcement.
// Mirrors sandflox.bash interactive (D-01) and non-interactive (D-02) patterns.
//
// execWithKernelEnforcement wraps `flox activate` under Apple sandbox-exec using
// syscall.Exec. PID is preserved (no child process), so the process tree shows
// sandbox-exec in place of sandflox.
//
// On missing sandbox-exec: warn and fall through to plain execFlox (D-05).
// On missing flox: hard error -- cannot continue without flox.
// On syscall.Exec failure: hard error with [sandflox] ERROR prefix.
//
// Argv shape (D-01, interactive -- 16 elements):
//   sandbox-exec -f <sbpl> -D PROJECT=... -D HOME=... -D FLOX_CACHE=... <flox-abs> activate -- bash --rcfile <entrypoint> -i
//
// Argv shape (D-02, non-interactive -- 16 + len(userArgs) elements):
//   sandbox-exec -f <sbpl> -D PROJECT=... -D HOME=... -D FLOX_CACHE=... <flox-abs> activate -- bash -c 'source <entrypoint> && exec "$@"' -- CMD ARGS...

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// ── Pure argv builder (testable) ────────────────────────

// buildSandboxExecArgv constructs the argv slice for invoking sandbox-exec.
// Pure function -- no I/O, no syscalls. Used by execWithKernelEnforcement at
// runtime AND by exec_test.go for argv-shape assertions.
//
// Interactive (D-01, len(userArgs)==0) -- 16 elements:
//
//	[0..8]   sandbox-exec scaffold: sandbox-exec -f <sbpl> -D PROJECT=... -D HOME=... -D FLOX_CACHE=...
//	[9..10]  floxAbsPath, "activate"
//	[11]     "--"                    (flox activate argv boundary)
//	[12..15] "bash", "--rcfile", entrypointPath, "-i"
//
// Non-interactive (D-02, len(userArgs)>0) -- 16 + len(userArgs) elements:
//
//	[0..10]  same scaffold
//	[11]     "--"                    (flox activate argv boundary)
//	[12..13] "bash", "-c"
//	[14]     "source <entrypointPath> && exec \"$@\""  (single argv element)
//	[15]     "--"                    (bash $0/$@ boundary)
//	[16..]   userArgs...
func buildSandboxExecArgv(sbplPath, projectDir, home, floxCachePath, floxAbsPath, entrypointPath string, userArgs []string) []string {
	base := []string{
		"sandbox-exec",
		"-f", sbplPath,
		"-D", "PROJECT=" + projectDir,
		"-D", "HOME=" + home,
		"-D", "FLOX_CACHE=" + floxCachePath,
		floxAbsPath, "activate",
		"--",
	}
	if len(userArgs) == 0 {
		// D-01: interactive mode
		return append(base, "bash", "--rcfile", entrypointPath, "-i")
	}
	// D-02: non-interactive mode
	payload := "source " + shellquote(entrypointPath) + ` && exec "$@"`
	argv := append(base, "bash", "-c", payload, "--")
	return append(argv, userArgs...)
}

// ── Main entry point ────────────────────────────────────

// execWithKernelEnforcement wraps flox activate under sandbox-exec on darwin.
// Replaces the current process via syscall.Exec (PID preserved).
//
// Flow:
//  1. LookPath("sandbox-exec"); on miss, warn + fall through to execFlox (D-05)
//  2. UserHomeDir(); on error, hard error
//  3. GenerateSBPL -> WriteSBPL into .flox/cache/sandflox/sandflox.sb
//  4. LookPath("flox") -- must be absolute in argv (Pitfall 6)
//  5. Build argv via buildSandboxExecArgv with entrypointPath
//  6. syscall.Exec -- does not return on success; error path is hard error (Pitfall 1)
func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, cacheDir string, entrypointPath string, userArgs []string) {
	// 1. Preflight: sandbox-exec availability.
	// Use absolute path -- sandbox-exec is a macOS system binary at a
	// fixed location; LookPath may fail if PATH is already restricted.
	sbxPath := "/usr/bin/sandbox-exec"
	if _, err := os.Stat(sbxPath); err != nil {
		fmt.Fprintf(stderr, "[sandflox] WARNING: sandbox-exec not found -- falling back to shell-only\n")
		execFlox(cfg, userArgs)
		return
	}

	// 2. Resolve home directory
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	// 3. Generate and write SBPL profile
	sbplContent := GenerateSBPL(cfg, home)
	sbplPath, err := WriteSBPL(cacheDir, sbplContent)
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: %v\n", err)
		os.Exit(1)
	}

	// 4. Flox absolute path (Pitfall 6 -- argv must not be relative "flox")
	floxAbsPath, err := exec.LookPath("flox")
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: flox not found in PATH\n")
		os.Exit(1)
	}

	// 5. Compute FLOX_CACHE path (matches sandflox.bash:479)
	floxCachePath := filepath.Join(home, ".cache", "flox")

	// 6. Diagnostic line (matches sandflox.bash:472-473)
	fmt.Fprintf(stderr, "[sandflox] Kernel enforcement: sandbox-exec (macOS Seatbelt)\n")

	// 7. Build argv and exec
	argv := buildSandboxExecArgv(sbplPath, projectDir, home, floxCachePath, floxAbsPath, entrypointPath, userArgs)

	// Sanitize environment before exec -- replaces os.Environ() with
	// filtered allowlist (SEC-01/02/03).
	env := BuildSanitizedEnv(cfg)
	env = append(env, "SANDFLOX_SANDBOX=1")

	// syscall.Exec replaces the current process; does not return on success.
	// If we get here, exec failed (Pitfall 1 -- never fall through after exec).
	execErr := syscall.Exec(sbxPath, argv, env)
	fmt.Fprintf(stderr, "[sandflox] ERROR: sandbox-exec failed: %v\n", execErr)
	os.Exit(1)
}

// ── Elevate argv builder (testable) ─────────────────────

// buildElevateArgv constructs the argv slice for sandbox-exec wrapping
// the current shell WITHOUT flox activate. Used by `sandflox elevate`
// when the user is already inside `flox activate` and wants to add
// kernel enforcement in-place.
//
// Pure function -- no I/O, no syscalls. Returns exactly 12 elements:
//
//	[0..8]   sandbox-exec -f <sbpl> -D PROJECT=... -D HOME=... -D FLOX_CACHE=...
//	[9..11]  "bash", "--rcfile", entrypointPath, "-i"
//
// Differs from buildSandboxExecArgv by omitting flox activate -- the
// user is already in a flox session.
func buildElevateArgv(sbplPath, projectDir, home, floxCachePath, entrypointPath string) []string {
	return []string{
		"sandbox-exec",
		"-f", sbplPath,
		"-D", "PROJECT=" + projectDir,
		"-D", "HOME=" + home,
		"-D", "FLOX_CACHE=" + floxCachePath,
		"bash", "--rcfile", entrypointPath, "-i",
	}
}

// ── Elevate exec ────────────────────────────────────────

// elevateExec re-execs the current shell under sandbox-exec on darwin.
// Unlike execWithKernelEnforcement, elevate does NOT wrap flox activate
// (user is already in a flox session). Requires sandbox-exec -- no
// fallback, since elevate's purpose is kernel enforcement.
//
// Flow:
//  1. LookPath("sandbox-exec"); hard error on miss (no fallback)
//  2. UserHomeDir
//  3. GenerateSBPL -> WriteSBPL
//  4. Build argv via buildElevateArgv
//  5. Sanitize env via BuildSanitizedEnv
//  6. syscall.Exec (does not return on success)
func elevateExec(cfg *ResolvedConfig, projectDir, cacheDir, entrypointPath string) {
	// 1. Preflight: sandbox-exec is mandatory for elevate.
	// Use absolute path -- inside a sandflox session, PATH is restricted
	// to the symlink bin and /usr/bin is not on PATH.
	sbxPath := "/usr/bin/sandbox-exec"
	if _, err := os.Stat(sbxPath); err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: sandbox-exec not found at %s -- cannot elevate\n", sbxPath)
		os.Exit(1)
	}

	// 2. Resolve home directory
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	// 3. Generate and write SBPL profile
	sbplContent := GenerateSBPL(cfg, home)
	sbplPath, err := WriteSBPL(cacheDir, sbplContent)
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] ERROR: %v\n", err)
		os.Exit(1)
	}

	// 4. Compute FLOX_CACHE path
	floxCachePath := filepath.Join(home, ".cache", "flox")

	// 5. Diagnostic
	fmt.Fprintf(stderr, "[sandflox] Elevating to sandboxed shell (sandbox-exec)\n")

	// 6. Build argv and exec
	argv := buildElevateArgv(sbplPath, projectDir, home, floxCachePath, entrypointPath)

	// Sanitize environment
	env := BuildSanitizedEnv(cfg)
	env = append(env, "SANDFLOX_SANDBOX=1")

	// syscall.Exec replaces the process; does not return on success
	execErr := syscall.Exec(sbxPath, argv, env)
	fmt.Fprintf(stderr, "[sandflox] ERROR: elevate failed: %v\n", execErr)
	os.Exit(1)
}
