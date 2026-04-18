//go:build !darwin

// exec_other.go -- Non-darwin stub for kernel enforcement.
// Kernel enforcement requires Apple sandbox-exec (SBPL), which is macOS-only.
// On other platforms, warn and fall through to plain flox activate (shell-only
// enforcement). Matches sandflox.bash platform-dispatch behavior.

package main

import (
	"fmt"
	"os"
)

// execWithKernelEnforcement is a non-darwin stub. Prints a WARNING diagnostic
// indicating that kernel enforcement is unavailable on this platform, then
// delegates to execFlox for the standard flox activate path.
func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, cacheDir string, entrypointPath string, userArgs []string) {
	_ = projectDir
	_ = cacheDir
	_ = entrypointPath
	fmt.Fprintf(stderr, "[sandflox] WARNING: kernel enforcement only available on darwin -- falling back to shell-only\n")
	execFlox(cfg, userArgs)
}

// elevateExec is a non-darwin stub. Elevate requires macOS sandbox-exec
// and cannot fall back to shell-only enforcement -- it exists specifically
// to add kernel enforcement to an existing flox session. Hard error.
func elevateExec(cfg *ResolvedConfig, projectDir, cacheDir, entrypointPath string) {
	_ = cfg
	_ = projectDir
	_ = cacheDir
	_ = entrypointPath
	fmt.Fprintf(stderr, "[sandflox] ERROR: elevate requires macOS sandbox-exec -- not available on this platform\n")
	os.Exit(1)
}
