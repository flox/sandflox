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
func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, entrypointPath string, userArgs []string) {
	// 1. Preflight: sandbox-exec availability
	sbxPath, err := exec.LookPath("sandbox-exec")
	if err != nil {
		fmt.Fprintf(stderr, "[sandflox] WARNING: sandbox-exec not found -- falling back to shell-only\n")
		execFlox(userArgs)
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
	cacheDir := filepath.Join(projectDir, ".flox", "cache", "sandflox")
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

	// syscall.Exec replaces the current process; does not return on success.
	// If we get here, exec failed (Pitfall 1 -- never fall through after exec).
	execErr := syscall.Exec(sbxPath, argv, os.Environ())
	fmt.Fprintf(stderr, "[sandflox] ERROR: sandbox-exec failed: %v\n", execErr)
	os.Exit(1)
}
