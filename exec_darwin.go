//go:build darwin

// exec_darwin.go -- sandbox-exec wrapping for macOS kernel enforcement (KERN-04, KERN-05, KERN-06).
// Mirrors sandflox.bash:465-481 invocation pattern.
//
// execWithKernelEnforcement wraps `flox activate` under Apple sandbox-exec using
// syscall.Exec. PID is preserved (no child process), so the process tree shows
// sandbox-exec in place of sandflox (success criterion #5 for Phase 2).
//
// On missing sandbox-exec: warn and fall through to plain execFlox (D-05).
// On missing flox: hard error -- cannot continue without flox.
// On syscall.Exec failure: hard error with [sandflox] ERROR prefix.
//
// The argv shape mirrors sandflox.bash:476-480:
//   sandbox-exec -f <sbpl> -D PROJECT=... -D HOME=... -D FLOX_CACHE=... <flox-abs> activate [-- CMD...]

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
// Argv shape (11 elements for empty userArgs):
//
//	[0] "sandbox-exec"                          -- argv0 (basename, not path)
//	[1] "-f"                                    -- profile-file flag
//	[2] sbplPath                                -- generated .sb path
//	[3] "-D"                                    -- first param
//	[4] "PROJECT=" + projectDir
//	[5] "-D"                                    -- second param
//	[6] "HOME=" + home
//	[7] "-D"                                    -- third param
//	[8] "FLOX_CACHE=" + floxCachePath
//	[9] floxAbsPath                             -- ABSOLUTE path (Pitfall 6)
//	[10] "activate"                             -- flox subcommand
//
// If userArgs is non-empty, append "--" then userArgs (KERN-06).
func buildSandboxExecArgv(sbplPath, projectDir, home, floxCachePath, floxAbsPath string, userArgs []string) []string {
	argv := []string{
		"sandbox-exec",
		"-f", sbplPath,
		"-D", "PROJECT=" + projectDir,
		"-D", "HOME=" + home,
		"-D", "FLOX_CACHE=" + floxCachePath,
		floxAbsPath, "activate",
	}
	if len(userArgs) > 0 {
		argv = append(argv, "--")
		argv = append(argv, userArgs...)
	}
	return argv
}

// ── Main entry point ────────────────────────────────────

// execWithKernelEnforcement wraps flox activate under sandbox-exec on darwin.
// Replaces the current process via syscall.Exec (PID preserved).
//
// Flow (mirrors sandflox.bash:465-481):
//  1. LookPath("sandbox-exec"); on miss, warn + fall through to execFlox (D-05)
//  2. UserHomeDir(); on error, hard error
//  3. GenerateSBPL -> WriteSBPL into .flox/cache/sandflox/sandflox.sb
//  4. LookPath("flox") -- must be absolute in argv (Pitfall 6)
//  5. Build argv via buildSandboxExecArgv
//  6. syscall.Exec -- does not return on success; error path is hard error (Pitfall 1)
func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, userArgs []string) {
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
	argv := buildSandboxExecArgv(sbplPath, projectDir, home, floxCachePath, floxAbsPath, userArgs)

	// syscall.Exec replaces the current process; does not return on success.
	// If we get here, exec failed (Pitfall 1 -- never fall through after exec).
	execErr := syscall.Exec(sbxPath, argv, os.Environ())
	fmt.Fprintf(stderr, "[sandflox] ERROR: sandbox-exec failed: %v\n", execErr)
	os.Exit(1)
}
