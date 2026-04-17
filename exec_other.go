//go:build !darwin

// exec_other.go -- Non-darwin stub for kernel enforcement.
// Kernel enforcement requires Apple sandbox-exec (SBPL), which is macOS-only.
// On other platforms, warn and fall through to plain flox activate (shell-only
// enforcement). Matches sandflox.bash platform-dispatch behavior.

package main

import "fmt"

// execWithKernelEnforcement is a non-darwin stub. Prints a WARNING diagnostic
// indicating that kernel enforcement is unavailable on this platform, then
// delegates to execFlox for the standard flox activate path.
func execWithKernelEnforcement(cfg *ResolvedConfig, projectDir string, entrypointPath string, userArgs []string) {
	_ = projectDir
	_ = entrypointPath
	fmt.Fprintf(stderr, "[sandflox] WARNING: kernel enforcement only available on darwin -- falling back to shell-only\n")
	execFlox(cfg, userArgs)
}
