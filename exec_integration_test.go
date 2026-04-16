//go:build darwin && integration

// exec_integration_test.go -- real sandbox-exec subprocess tests for kernel enforcement
// (Plan 02-03, KERN-01 through KERN-08).
//
// These tests spawn an actual `sandbox-exec` subprocess with a profile produced
// by sandflox's GenerateSBPL, then verify that real kernel-level enforcement
// happens: writes outside the workspace are blocked, network policy is enforced,
// denied paths are unreadable, and the built sandflox binary end-to-end wraps
// commands correctly.
//
// Per 02-RESEARCH.md Pitfall 7: the test process itself MUST NOT be sandboxed.
// Only spawned subprocesses get the sandbox. That means:
//   - we use exec.CommandContext, NEVER syscall.Exec from a test
//   - timeouts are enforced via context.WithTimeout to avoid hung tests
//   - t.TempDir() + cwd-relative paths; no hard-coded user paths
//
// Build tag: `darwin && integration`. Default `go test ./...` excludes these
// tests; run them explicitly with `go test -tags integration ./...`.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ── Shared helpers ──────────────────────────────────────

// skipIfNoSandboxExec skips the current test if we are not on darwin or if
// /usr/bin/sandbox-exec is unavailable (theoretical on modern macOS, but
// guards against weird CI runners and future-macOS regressions).
func skipIfNoSandboxExec(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("kernel enforcement only on darwin")
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not available")
	}
}

// writeProfile generates an SBPL profile via the Plan 02-01 generator and
// writes it to {dir}/profile.sb. Returns the absolute path to the file.
func writeProfile(t *testing.T, dir string, cfg *ResolvedConfig) string {
	t.Helper()
	home, _ := os.UserHomeDir()
	sbpl := GenerateSBPL(cfg, home)
	path := filepath.Join(dir, "profile.sb")
	if err := os.WriteFile(path, []byte(sbpl), 0644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}

// runSandboxed invokes `sandbox-exec -f <sbplPath> -D PROJECT=... -D HOME=... -D FLOX_CACHE=...`
// with the given command and returns the subprocess's stdout, stderr, and
// exit code. Uses a 10-second context timeout to avoid hung tests.
//
// Exit codes:
//
//	 0: command succeeded under the sandbox
//	>0: command failed (either blocked by sandbox or command itself errored)
//	-1: context expired or exec could not be started (distinct from a normal
//	    non-zero exit)
func runSandboxed(t *testing.T, sbplPath, projectDir, home string, command ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	argv := []string{
		"-f", sbplPath,
		"-D", "PROJECT=" + projectDir,
		"-D", "HOME=" + home,
		"-D", "FLOX_CACHE=" + filepath.Join(home, ".cache", "flox"),
	}
	argv = append(argv, command...)
	cmd := exec.CommandContext(ctx, "sandbox-exec", argv...)
	var so, se strings.Builder
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return so.String(), se.String(), exitErr.ExitCode()
	}
	if err != nil {
		return so.String(), se.String(), -1
	}
	return so.String(), se.String(), 0
}

// ── TestSandboxBlocksWrite ──────────────────────────────
// KERN-01..KERN-03: prove the kernel blocks writes outside the workspace
// and allows writes inside. FsMode="workspace" resolves PROJECT=tmpDir via
// -D parameter substitution.

func TestSandboxBlocksWrite(t *testing.T) {
	skipIfNoSandboxExec(t)

	tmpDir := t.TempDir()
	home, _ := os.UserHomeDir()

	cfg := &ResolvedConfig{
		Profile:        "default",
		FsMode:         "workspace",
		NetMode:        "blocked",
		AllowLocalhost: true,
		Writable:       []string{tmpDir, "/private/tmp"},
		ReadOnly:       nil,
		Denied:         nil,
	}
	sbplPath := writeProfile(t, tmpDir, cfg)

	// ── Negative case: write outside workspace must be blocked ──
	outside := "/etc/sandflox-test-" + fmt.Sprint(os.Getpid())
	_, stderr, exitCode := runSandboxed(t, sbplPath, tmpDir, home,
		"/bin/sh", "-c", "echo hi > "+outside)
	if exitCode == 0 {
		// Defensive cleanup if somehow it succeeded (should be impossible --
		// /etc needs root anyway, but this also catches the sandbox miss).
		_ = os.Remove(outside)
		t.Fatalf("expected non-zero exit when writing to /etc, got 0; stderr=%q", stderr)
	}

	// ── Positive case: write inside workspace must succeed ──
	insideFile := filepath.Join(tmpDir, "inside.txt")
	_, stderr2, exitCode2 := runSandboxed(t, sbplPath, tmpDir, home,
		"/bin/sh", "-c", "echo hi > "+insideFile)
	if exitCode2 != 0 {
		t.Fatalf("expected exit 0 when writing to %s, got %d; stderr=%q",
			insideFile, exitCode2, stderr2)
	}
	// Sanity check that the file actually exists.
	if _, err := os.Stat(insideFile); err != nil {
		t.Fatalf("expected %s to exist after sandboxed write, stat err=%v", insideFile, err)
	}
}

// ── TestSandboxAllowsLocalhost ──────────────────────────
// KERN-07: with NetMode="blocked" + AllowLocalhost=true, TCP to 127.0.0.1
// should NOT be blocked at the kernel layer. Connection refused is the
// expected error (no listener on port 1), and crucially stderr must NOT
// contain "Operation not permitted".
//
// With AllowLocalhost=false, the same probe must be kernel-blocked:
// stderr must contain "Operation not permitted".

func TestSandboxAllowsLocalhost(t *testing.T) {
	skipIfNoSandboxExec(t)

	home, _ := os.UserHomeDir()

	// Deterministic probe: connect to 127.0.0.1 on port 1 (always unused).
	// With localhost allowed → Python sees ConnectionRefusedError.
	// With localhost blocked by sandbox → Python sees PermissionError
	// ("Operation not permitted"), which is the kernel EPERM bubbling up.
	probe := `import socket
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(2)
try:
    s.connect(("127.0.0.1", 1))
except Exception as e:
    import sys
    print(type(e).__name__, str(e), file=sys.stderr)
    sys.exit(1)
`

	t.Run("with_localhost", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &ResolvedConfig{
			Profile:        "default",
			FsMode:         "workspace",
			NetMode:        "blocked",
			AllowLocalhost: true,
			Writable:       []string{tmpDir, "/private/tmp"},
		}
		sbplPath := writeProfile(t, tmpDir, cfg)

		_, stderr, _ := runSandboxed(t, sbplPath, tmpDir, home,
			"/usr/bin/python3", "-c", probe)
		if strings.Contains(stderr, "Operation not permitted") {
			t.Fatalf("expected localhost to be allowed, but stderr contains 'Operation not permitted'; stderr=%q", stderr)
		}
		// A "Connection refused" or "ConnectionRefusedError" in stderr is
		// the happy path -- we never have a listener on port 1. Empty
		// stderr (somehow succeeded) is also fine, though very unlikely.
	})

	t.Run("without_localhost", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &ResolvedConfig{
			Profile:        "default",
			FsMode:         "workspace",
			NetMode:        "blocked",
			AllowLocalhost: false,
			Writable:       []string{tmpDir, "/private/tmp"},
		}
		sbplPath := writeProfile(t, tmpDir, cfg)

		_, stderr, _ := runSandboxed(t, sbplPath, tmpDir, home,
			"/usr/bin/python3", "-c", probe)
		if !strings.Contains(stderr, "Operation not permitted") {
			t.Fatalf("expected 'Operation not permitted' when localhost is blocked; stderr=%q", stderr)
		}
	})
}

// ── TestSandboxAllowsUnixSocket ─────────────────────────
// KERN-08: Unix socket communication must be allowed regardless of NetMode
// or AllowLocalhost. The bash reference always emits
//
//	(allow network* (remote unix-socket))
//
// after the deny rule, specifically so the Nix daemon IPC keeps working
// inside the sandbox.
//
// macOS AF_UNIX sun_path is limited to ~104 bytes, and t.TempDir() paths
// under /var/folders/... routinely blow that budget. We use /private/tmp
// (always short, always writable under workspace mode) for the socket
// itself, while still using t.TempDir() for the profile file.

func TestSandboxAllowsUnixSocket(t *testing.T) {
	skipIfNoSandboxExec(t)

	tmpDir := t.TempDir()
	home, _ := os.UserHomeDir()

	cfg := &ResolvedConfig{
		Profile:        "default",
		FsMode:         "workspace",
		NetMode:        "blocked",
		AllowLocalhost: false, // even with localhost blocked, unix sockets must work
		Writable:       []string{tmpDir, "/private/tmp"},
	}
	sbplPath := writeProfile(t, tmpDir, cfg)

	// Use /private/tmp to keep sun_path well under the 104-byte limit.
	sockPath := filepath.Join("/private/tmp", fmt.Sprintf("sf-test-sock-%d", os.Getpid()))
	defer os.Remove(sockPath)

	probe := `import socket, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.bind("` + sockPath + `")
s.close()
print("ok")
`
	stdout, stderr, exitCode := runSandboxed(t, sbplPath, tmpDir, home,
		"/usr/bin/python3", "-c", probe)
	if exitCode != 0 {
		t.Fatalf("expected unix-socket bind to succeed under blocked+no-localhost sandbox; exit=%d stdout=%q stderr=%q",
			exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "ok") {
		t.Fatalf("expected stdout 'ok', got stdout=%q stderr=%q", stdout, stderr)
	}
}

// ── TestSandboxBlocksDeniedPath ─────────────────────────
// KERN-01..KERN-03: Denied paths from policy.toml must be unreadable under
// the sandbox. Workspace mode allows the project dir broadly, but a denied
// subpath inside the project must still be blocked.
//
// Implementation note: We use /private/tmp/sandflox-kern-<pid> as the
// project dir (NOT t.TempDir()). Rationale: t.TempDir() returns a path
// under /var/folders/... which macOS canonicalizes to /private/var/folders/...
// at the VFS layer (the same class of firmlink behavior that forces
// /tmp -> /private/tmp). SBPL path predicates see the canonicalized form,
// but our ResolvedConfig.Denied here is plain tmpDir -- so a `(deny (subpath
// "/var/folders/..."))` rule would NOT match the canonicalized "/private/
// var/folders/..." path the kernel actually sees. Using /private/tmp/...
// directly sidesteps the canonicalization gap and mirrors what real users'
// policy paths look like (~/.ssh/ resolves to /Users/..., not /var/...).

func TestSandboxBlocksDeniedPath(t *testing.T) {
	skipIfNoSandboxExec(t)

	home, _ := os.UserHomeDir()

	projectDir := filepath.Join("/private/tmp", fmt.Sprintf("sandflox-kern-%d", os.Getpid()))
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	defer os.RemoveAll(projectDir)

	// Create a secrets dir + file OUTSIDE the sandbox first (the test
	// process is unsandboxed per Pitfall 7).
	secretsDir := filepath.Join(projectDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	secretFile := filepath.Join(secretsDir, "file.txt")
	if err := os.WriteFile(secretFile, []byte("top-secret\n"), 0644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	cfg := &ResolvedConfig{
		Profile:        "default",
		FsMode:         "workspace",
		NetMode:        "blocked",
		AllowLocalhost: true,
		Writable:       []string{projectDir, "/private/tmp"},
		ReadOnly:       nil,
		// Trailing slash => (subpath ...) in the generated SBPL.
		Denied: []string{secretsDir + "/"},
	}
	// Profile file itself can live in t.TempDir() -- sandbox-exec reads it
	// with the test-process's unsandboxed credentials.
	sbplPath := writeProfile(t, t.TempDir(), cfg)

	// Reading under a denied subpath must fail with permission error.
	stdout, stderr, exitCode := runSandboxed(t, sbplPath, projectDir, home,
		"/bin/cat", secretFile)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit when reading denied path %s; stdout=%q stderr=%q",
			secretFile, stdout, stderr)
	}
	lower := strings.ToLower(stderr)
	if !(strings.Contains(stderr, "Operation not permitted") ||
		strings.Contains(lower, "permission denied")) {
		t.Fatalf("expected permission-denied-style error when reading denied path; stderr=%q", stderr)
	}
}

// ── TestBuiltBinaryWrapsCommand ─────────────────────────
// KERN-04, KERN-05, KERN-06: Build the real sandflox binary and invoke it
// with `-- /bin/echo hello`. This exercises the full pipeline:
//
//	ParseFlags → ParsePolicy → ResolveConfig → WriteCache
//	  → emitDiagnostics → execWithKernelEnforcement
//	  → sandbox-exec → flox activate → echo
//
// Requires flox in PATH and a valid flox environment rooted at the test's
// cwd (the sandflox repo itself). A throwaway t.TempDir() will NOT work
// because `flox activate` probes for .flox/env.json on the way up and
// errors out with a "corrupt environment" message when it finds sandflox's
// cache-writer-created .flox/ directory without env.json.
//
// Skipped gracefully if flox is absent or no valid .flox/env.json exists.

func TestBuiltBinaryWrapsCommand(t *testing.T) {
	skipIfNoSandboxExec(t)
	if _, err := exec.LookPath("flox"); err != nil {
		t.Skip("flox not available -- skipping built-binary integration test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	// Require a real flox env at cwd. Without this check the test fails
	// with a cryptic flox error; an explicit skip is clearer for anyone
	// running `go test -tags integration` from outside the repo.
	if _, err := os.Stat(filepath.Join(cwd, ".flox", "env.json")); err != nil {
		t.Skipf("no valid .flox/env.json at %s -- skipping built-binary test", cwd)
	}
	// Similarly, a policy.toml is required for the sandflox->sandbox-exec
	// path (without one, sandflox falls back to plain flox activate and
	// this test would no longer exercise kernel enforcement).
	if _, err := os.Stat(filepath.Join(cwd, "policy.toml")); err != nil {
		t.Skipf("no policy.toml at %s -- skipping built-binary test", cwd)
	}

	// 1. Build the sandflox binary into an isolated temp dir.
	binPath := filepath.Join(t.TempDir(), "sandflox")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = cwd
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// 2. Invoke the built binary with a 60-second timeout. `flox activate`
	//    can be slow the first time it populates caches, but should be
	//    comfortably under 60s on a warm system.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "--", "/bin/echo", "hello")
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandflox -- echo failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", out)
	}
}
