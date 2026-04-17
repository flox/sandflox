//go:build darwin && integration

// shell_integration_test.go -- real subprocess tests for shell-tier enforcement
// (Plan 03-03, SHELL-01 through SHELL-08).
//
// These tests build the sandflox binary, invoke it with probe commands via
// `./sandflox -- /bin/bash -c <probe>`, then assert that the shell enforcement
// layer (entrypoint.sh, fs-filter.sh, usercustomize.py) produces the correct
// observable outcomes: PATH wipe, symlink bin composition, armor function
// blocking, fs-filter write blocking, Python open/ensurepip blocking,
// breadcrumb removal, curl removal under net-blocked, and [sandflox] BLOCKED:
// message prefix convention.
//
// Per 03-RESEARCH.md Pitfall 7: the test process itself MUST NOT be sandboxed.
// Only spawned subprocesses get the sandbox. That means:
//   - we use exec.CommandContext, NEVER direct exec from a test
//   - timeouts are enforced via context.WithTimeout to avoid hung tests
//   - the binary is built once via sync.Once and cached across all tests
//
// Build tag: `darwin && integration`. Default `go test ./...` excludes these
// tests; run them explicitly with `go test -tags integration ./...`.
//
// Prerequisites: flox in PATH, .flox/env.json at cwd, policy.toml at cwd.
// Tests skip gracefully when prerequisites are missing (never hard-fail).
//
// Environment note: the dev flox environment may only install Go (for building),
// not the full requisites toolset. Tests that require specific tools (python3,
// coreutils) inside the sandbox skip gracefully when those tools are absent.
// In a production flox environment with the full package set, all tests will
// exercise the complete enforcement chain.

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── Package-level binary cache ──────────────────────────

var (
	sharedBinPath string
	sharedBinErr  error
	sharedBinOnce sync.Once
)

// getSandfloxBin builds the sandflox binary once and caches the result.
// Subsequent calls return the cached path. If the build failed, subsequent
// calls also fail with the cached error.
func getSandfloxBin(t *testing.T) string {
	t.Helper()
	sharedBinOnce.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			sharedBinErr = fmt.Errorf("getwd: %w", err)
			return
		}
		sharedBinPath = filepath.Join(os.TempDir(), fmt.Sprintf("sandflox-shell-integration-test-%d", os.Getpid()))
		build := exec.Command("go", "build", "-o", sharedBinPath, ".")
		build.Dir = cwd
		if out, err := build.CombinedOutput(); err != nil {
			sharedBinErr = fmt.Errorf("go build failed: %v\n%s", err, out)
			return
		}
	})
	if sharedBinErr != nil {
		t.Fatalf("getSandfloxBin: %v", sharedBinErr)
	}
	return sharedBinPath
}

// ── Shared prerequisite check ───────────────────────────

// checkShellPrereqs skips the current test if sandbox-exec, flox,
// .flox/env.json, or policy.toml are unavailable. This ensures tests
// skip cleanly on CI runners without flox rather than failing.
func checkShellPrereqs(t *testing.T) {
	t.Helper()
	skipIfNoSandboxExec(t)

	if _, err := exec.LookPath("flox"); err != nil {
		t.Skip("flox not available -- skipping shell integration test")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".flox", "env.json")); err != nil {
		t.Skipf("no valid .flox/env.json at %s -- skipping", cwd)
	}
	if _, err := os.Stat(filepath.Join(cwd, "policy.toml")); err != nil {
		t.Skipf("no policy.toml at %s -- skipping", cwd)
	}
}

// ── Shared invocation helper ────────────────────────────

// runSandfloxProbe invokes the cached sandflox binary with the given bash
// probe script. Returns stdout, stderr, and exit code. Uses a 60-second
// timeout to handle slow flox activate on cold caches.
func runSandfloxProbe(t *testing.T, probeScript string) (stdout, stderr string, exitCode int) {
	t.Helper()
	checkShellPrereqs(t)
	binPath := getSandfloxBin(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "--", "/bin/bash", "-c", probeScript)
	cmd.Dir = cwd

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()

	exitCode = 0
	if ee, ok := runErr.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if runErr != nil {
		// Context timeout or exec failure -- not a normal exit code
		t.Logf("runSandfloxProbe: non-ExitError: %v", runErr)
		exitCode = -1
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// ── Test 1: SHELL-01 -- PATH wipe ──────────────────────

func TestShellEnforces_PathWipe(t *testing.T) {
	// echo is a bash builtin -- works regardless of what's in the symlink bin
	stdout, _, exitCode := runSandfloxProbe(t, `echo "$PATH"`)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0", exitCode)
	}

	pathLine := strings.TrimSpace(stdout)
	// PATH should end in /.flox/cache/sandflox/bin
	if !strings.HasSuffix(pathLine, "/.flox/cache/sandflox/bin") {
		t.Errorf("PATH does not end in /.flox/cache/sandflox/bin: %q", pathLine)
	}

	// PATH must be exactly one directory (no colons)
	parts := strings.Split(pathLine, ":")
	if len(parts) != 1 {
		t.Errorf("expected PATH to have exactly 1 directory, got %d: %q", len(parts), pathLine)
	}

	// Must not contain system paths
	systemPaths := []string{":/usr/bin", ":/bin", ":/usr/local/bin", ":/sbin", ":/usr/sbin"}
	for _, sp := range systemPaths {
		if strings.Contains(pathLine, sp) {
			t.Errorf("PATH contains system path %q: %q", sp, pathLine)
		}
	}
}

// ── Test 2: SHELL-02 -- Symlink bin composition ─────────

func TestShellEnforces_SymlinkBin(t *testing.T) {
	// Use bash builtins and globbing to list the bin directory contents.
	// We avoid external commands like basename/readlink/ls since they may
	// not be available in the minimal dev flox environment. Instead we use
	// bash's built-in parameter expansion and test -L.
	//
	// The probe lists each file in $PATH/*, prints its name (via parameter
	// expansion) and whether it's a symlink. We use `printf` (bash builtin)
	// instead of external commands.
	probe := `
shopt -s nullglob
count=0
for f in $PATH/*; do
  name="${f##*/}"
  if [ -L "$f" ]; then
    printf "SYMLINK:%s\n" "$name"
    count=$((count + 1))
  else
    printf "OTHER:%s\n" "$name"
  fi
done
printf "TOTAL:%d\n" "$count"
`
	stdout, _, exitCode := runSandfloxProbe(t, probe)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0", exitCode)
	}

	// Parse TOTAL count
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	totalCount := 0
	observedNames := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SYMLINK:") {
			name := strings.TrimPrefix(line, "SYMLINK:")
			observedNames[name] = true
		}
		if strings.HasPrefix(line, "TOTAL:") {
			fmt.Sscanf(strings.TrimPrefix(line, "TOTAL:"), "%d", &totalCount)
		}
	}

	// In a minimal dev environment, the bin may be empty because
	// $FLOX_ENV/bin only has `go` (not in requisites.txt). Skip the
	// composition checks if the bin has fewer than 3 tools, but still
	// verify the structural property (PATH wipe happened, bin exists).
	if totalCount < 3 {
		t.Skipf("symlink bin has %d tools (minimal flox env) -- skipping composition checks", totalCount)
	}

	// All entries should be symlinks (no regular files)
	for _, line := range lines {
		if strings.HasPrefix(line, "OTHER:") {
			t.Errorf("non-symlink entry in bin: %q", strings.TrimPrefix(line, "OTHER:"))
		}
	}

	// Essential tools must be present (in a full environment)
	essentials := []string{"ls", "cat", "bash"}
	for _, name := range essentials {
		if !observedNames[name] {
			t.Errorf("essential tool %q missing from symlink bin", name)
		}
	}

	// Parse the repo's requisites.txt to build expected set
	cwd, _ := os.Getwd()
	reqPath := filepath.Join(cwd, "requisites.txt")
	reqData, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("cannot read requisites.txt: %v", err)
	}

	requisitesSet := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(reqData))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			requisitesSet[fields[0]] = true
		}
	}

	// Observed names must be a subset of requisites (no unexpected binaries)
	for name := range observedNames {
		if !requisitesSet[name] {
			t.Errorf("unexpected binary %q in symlink bin -- not in requisites.txt", name)
		}
	}
}

// ── Test 3: SHELL-03 -- Armor function blocks ───────────

func TestShellEnforces_ArmorBlocks(t *testing.T) {
	// Test a representative subset of armored commands.
	// Armor functions are defined as bash functions in entrypoint.sh and
	// exported via export -f, so they work regardless of what's in the
	// symlink bin.
	armorSubset := []string{"flox", "pip", "docker"}

	for _, name := range armorSubset {
		t.Run(name, func(t *testing.T) {
			// Defensive: skip if the name is not in ArmoredCommands
			found := false
			for _, ac := range ArmoredCommands {
				if ac == name {
					found = true
					break
				}
			}
			if !found {
				t.Skipf("%s is not in ArmoredCommands -- skipping", name)
			}

			probe := fmt.Sprintf(`%s --version 2>&1; echo "EXIT=$?"`, name)
			stdout, stderrOut, _ := runSandfloxProbe(t, probe)
			combined := stdout + stderrOut

			expectedMsg := fmt.Sprintf("[sandflox] BLOCKED: %s is not available. Environment is immutable.", name)
			if !strings.Contains(combined, expectedMsg) {
				t.Errorf("expected BLOCKED message %q in output, got stdout=%q stderr=%q", expectedMsg, stdout, stderrOut)
			}

			if !strings.Contains(stdout, "EXIT=126") {
				t.Errorf("expected EXIT=126 in stdout, got: %q", stdout)
			}
		})
	}
}

// ── Test 4: SHELL-04 -- fs-filter write block ───────────

func TestShellEnforces_FsFilterBlocks(t *testing.T) {
	// The cp wrapper is defined as a bash function by fs-filter.sh and
	// exported via export -f. It calls _sfx_check_write_target (also
	// exported) before invoking the real cp. Even if the real cp isn't
	// in PATH, the wrapper still runs the path check and emits the
	// BLOCKED message before failing.
	target := fmt.Sprintf("/etc/sandflox-fs-test-%d", os.Getpid())
	probe := fmt.Sprintf(`cp /bin/ls %s 2>&1; echo "EXIT=$?"`, target)

	stdout, stderrOut, _ := runSandfloxProbe(t, probe)
	combined := stdout + stderrOut

	if !strings.Contains(combined, "[sandflox] BLOCKED:") {
		t.Errorf("expected [sandflox] BLOCKED: in output, got stdout=%q stderr=%q", stdout, stderrOut)
	}
	if !strings.Contains(combined, "outside workspace policy") {
		t.Errorf("expected 'outside workspace policy' in output, got stdout=%q stderr=%q", stdout, stderrOut)
	}
	if !strings.Contains(stdout, "EXIT=126") {
		t.Errorf("expected EXIT=126 in stdout, got: %q", stdout)
	}
}

// ── Test 5: SHELL-05 (open) -- Python builtins.open block ──

func TestShellEnforces_PythonOpenBlocked(t *testing.T) {
	// First check if python3 is available in the sandbox. In a minimal
	// dev flox environment, python3 may not be in $FLOX_ENV/bin and
	// therefore not in the sandboxed PATH.
	checkProbe := `command -v python3 >/dev/null 2>&1; echo "PYAVAIL=$?"`
	checkStdout, _, _ := runSandfloxProbe(t, checkProbe)
	if strings.Contains(checkStdout, "PYAVAIL=1") {
		t.Skip("python3 not available in sandbox -- skipping Python open test")
	}

	target := fmt.Sprintf("/etc/sandflox-py-test-%d", os.Getpid())
	probe := fmt.Sprintf(`python3 -c 'open("%s", "w").close()' 2>&1; echo "EXIT=$?"`, target)

	stdout, stderrOut, _ := runSandfloxProbe(t, probe)
	combined := stdout + stderrOut

	if !strings.Contains(combined, "PermissionError") {
		t.Errorf("expected PermissionError in output, got stdout=%q stderr=%q", stdout, stderrOut)
	}
	if !strings.Contains(combined, "[sandflox] BLOCKED:") {
		t.Errorf("expected [sandflox] BLOCKED: in output, got stdout=%q stderr=%q", stdout, stderrOut)
	}
	// Check for the denial reason
	if !strings.Contains(combined, "outside workspace policy") && !strings.Contains(combined, "is denied by policy") {
		t.Errorf("expected denial reason in output, got stdout=%q stderr=%q", stdout, stderrOut)
	}
	if !strings.Contains(stdout, "EXIT=1") {
		t.Errorf("expected EXIT=1 in stdout (Python non-zero exit), got: %q", stdout)
	}
}

// ── Test 6: SHELL-05 (ensurepip) -- ensurepip block ─────

func TestShellEnforces_EnsurepipBlocked(t *testing.T) {
	// Check if python3 is available in the sandbox
	checkProbe := `command -v python3 >/dev/null 2>&1; echo "PYAVAIL=$?"`
	checkStdout, _, _ := runSandfloxProbe(t, checkProbe)
	if strings.Contains(checkStdout, "PYAVAIL=1") {
		t.Skip("python3 not available in sandbox -- skipping ensurepip test")
	}

	probe := `python3 -c 'import ensurepip; ensurepip.bootstrap()' 2>&1; echo "EXIT=$?"`

	stdout, stderrOut, _ := runSandfloxProbe(t, probe)
	combined := stdout + stderrOut

	if !strings.Contains(combined, "[sandflox] BLOCKED: ensurepip is disabled in this sandbox") {
		t.Errorf("expected ensurepip BLOCKED message in output, got stdout=%q stderr=%q", stdout, stderrOut)
	}
	if !strings.Contains(stdout, "EXIT=1") {
		t.Errorf("expected EXIT=1 in stdout (SystemExit exits 1), got: %q", stdout)
	}
}

// ── Test 7: SHELL-06 -- Breadcrumb env vars cleared ─────

func TestShellEnforces_BreadcrumbsCleared(t *testing.T) {
	// Use bash builtins to check for breadcrumb env vars. We avoid
	// external commands (env, grep) that might not be in the sandboxed
	// PATH. Bash's parameter expansion ${VAR+SET} evaluates to "SET" if
	// the variable is set (even if empty), or empty if unset.
	probe := `
found=0
[ -n "${FLOX_ENV_PROJECT+SET}" ] && found=1 && printf "LEAKED:FLOX_ENV_PROJECT\n"
[ -n "${FLOX_ENV_DIRS+SET}" ] && found=1 && printf "LEAKED:FLOX_ENV_DIRS\n"
[ -n "${FLOX_PATH_PATCHED+SET}" ] && found=1 && printf "LEAKED:FLOX_PATH_PATCHED\n"
[ "$found" -eq 0 ] && printf "NOT_FOUND\n"
`
	stdout, _, exitCode := runSandfloxProbe(t, probe)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0", exitCode)
	}

	if !strings.Contains(stdout, "NOT_FOUND") {
		t.Errorf("expected NOT_FOUND (no breadcrumb env vars), got: %q", stdout)
	}

	// Double-check: none of the breadcrumb vars should appear as leaked
	if strings.Contains(stdout, "LEAKED:") {
		t.Errorf("breadcrumb env vars still present in output: %q", stdout)
	}
}

// ── Test 8: SHELL-07 -- Curl removed when net blocked ───

func TestShellEnforces_CurlRemovedWhenNetBlocked(t *testing.T) {
	// Parse the repo's policy.toml to check net mode
	cwd, _ := os.Getwd()
	policyPath := filepath.Join(cwd, "policy.toml")
	policy, err := ParsePolicy(policyPath)
	if err != nil {
		t.Fatalf("cannot parse policy.toml: %v", err)
	}

	// Resolve config to get the active profile's net mode
	flags := &CLIFlags{} // defaults
	config := ResolveConfig(policy, flags, cwd)

	if config.NetMode != "blocked" {
		t.Skipf("repo policy net mode is %q, expected 'blocked' -- skipping curl-removal test", config.NetMode)
	}

	// command -v is a bash builtin, works regardless of symlink bin contents
	probe := `command -v curl; echo "EXIT=$?"`
	stdout, _, _ := runSandfloxProbe(t, probe)

	if !strings.Contains(stdout, "EXIT=1") {
		t.Errorf("expected EXIT=1 (curl not in PATH when net blocked), got: %q", stdout)
	}
	if strings.Contains(stdout, "/bin/curl") {
		t.Errorf("curl should not be in PATH when net is blocked, but found path in output: %q", stdout)
	}
}

// ── Test 9: SHELL-08 -- BLOCKED message prefix convention ──

func TestShellEnforces_BlockedMessagesPrefix(t *testing.T) {
	// Collect output from multiple blocked probes. We use probes that
	// rely only on bash builtins and exported functions (armor + fs-filter),
	// plus optionally python3 if available. This ensures the test works
	// in minimal environments.
	type probeSpec struct {
		name  string
		probe string
	}

	probes := []probeSpec{
		{"armor-flox", `flox --version 2>&1`},
		{"armor-pip", `pip --version 2>&1`},
		{"fs-filter-cp", fmt.Sprintf(`cp /bin/ls /etc/sfx-meta-test-%d 2>&1`, os.Getpid())},
	}

	// Optionally add python3 probe if available
	checkProbe := `command -v python3 >/dev/null 2>&1; echo "PYAVAIL=$?"`
	checkStdout, _, _ := runSandfloxProbe(t, checkProbe)
	if strings.Contains(checkStdout, "PYAVAIL=0") {
		probes = append(probes, probeSpec{
			"python-ensurepip",
			`python3 -c 'import ensurepip; ensurepip.bootstrap()' 2>&1`,
		})
	}

	var allOutput strings.Builder
	for _, p := range probes {
		stdout, stderrOut, _ := runSandfloxProbe(t, p.probe)
		allOutput.WriteString(stdout)
		allOutput.WriteString(stderrOut)
	}

	combined := allOutput.String()

	// Find all lines matching [sandflox] BLOCKED: prefix
	blockedLineRe := regexp.MustCompile(`(?m)^\[sandflox\] BLOCKED:.+$`)
	blockedMatches := blockedLineRe.FindAllString(combined, -1)

	// We expect at least 3 BLOCKED lines: 2 from armor (flox, pip) + 1 from fs-filter (cp)
	if len(blockedMatches) < 3 {
		t.Errorf("expected at least 3 BLOCKED lines from probes, got %d: %v", len(blockedMatches), blockedMatches)
	}

	// Verify NO line contains "BLOCKED:" without the "[sandflox] " prefix.
	// Lines from Python tracebacks may contain the message embedded after
	// "PermissionError:" -- those are fine since the message itself starts
	// with [sandflox].
	lines := strings.Split(combined, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "BLOCKED:") && !strings.HasPrefix(line, "[sandflox] ") {
			// Allow lines where BLOCKED appears embedded after another prefix
			// (e.g., Python traceback: "PermissionError: [sandflox] BLOCKED:")
			if strings.Contains(line, "[sandflox] BLOCKED:") {
				continue
			}
			t.Errorf("found BLOCKED: line without [sandflox] prefix: %q", line)
		}
	}
}

// ── SEC-01/02/03 env scrubbing helpers ──────────────────

// runSandfloxProbeWithEnv invokes the cached sandflox binary with the given
// bash probe script and extra environment variables. Identical to
// runSandfloxProbe except it appends extraEnv to the subprocess environment
// so the test can inject specific vars (e.g., AWS_SECRET_ACCESS_KEY) and
// verify they are scrubbed inside the sandbox.
func runSandfloxProbeWithEnv(t *testing.T, probeScript string, extraEnv ...string) (stdout, stderrOut string, exitCode int) {
	t.Helper()
	checkShellPrereqs(t)
	binPath := getSandfloxBin(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "--", "/bin/bash", "-c", probeScript)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), extraEnv...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()

	exitCode = 0
	if ee, ok := runErr.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if runErr != nil {
		t.Logf("runSandfloxProbeWithEnv: non-ExitError: %v", runErr)
		exitCode = -1
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// runSandfloxWithFlags invokes the cached sandflox binary with CLI flags
// before the -- separator. This allows testing --debug output and other
// flag-dependent behavior in a real subprocess.
func runSandfloxWithFlags(t *testing.T, flags []string, probeScript string, extraEnv ...string) (stdout, stderrOut string, exitCode int) {
	t.Helper()
	checkShellPrereqs(t)
	binPath := getSandfloxBin(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := make([]string, 0, len(flags)+4)
	args = append(args, flags...)
	args = append(args, "--", "/bin/bash", "-c", probeScript)
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = cwd
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()

	exitCode = 0
	if ee, ok := runErr.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if runErr != nil {
		t.Logf("runSandfloxWithFlags: non-ExitError: %v", runErr)
		exitCode = -1
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// ── Test 10: SEC-01 -- Essential env vars pass through ──

func TestEnvScrubbing_AllowlistPassesEssentials(t *testing.T) {
	// Verify that essential POSIX variables (HOME, TERM, USER) survive
	// the sanitization pipeline and are present inside the sandbox.
	// Uses only bash builtins (printf) -- no external commands needed.
	probe := `printf "HOME=%s\n" "$HOME"
printf "TERM=%s\n" "$TERM"
printf "USER=%s\n" "$USER"
printf "SHELL=%s\n" "$SHELL"
`
	stdout, _, exitCode := runSandfloxProbe(t, probe)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0", exitCode)
	}

	// HOME must be non-empty
	if !strings.Contains(stdout, "HOME=/") {
		t.Errorf("HOME should start with / (an absolute path); got stdout=%q", stdout)
	}
	// TERM must be non-empty (set in all terminal sessions)
	for _, line := range strings.Split(stdout, "\n") {
		if line == "TERM=" {
			t.Errorf("TERM is empty inside sandbox; expected a non-empty value")
		}
	}
	// USER must be non-empty
	for _, line := range strings.Split(stdout, "\n") {
		if line == "USER=" {
			t.Errorf("USER is empty inside sandbox; expected a non-empty value")
		}
	}
}

// ── Test 11: SEC-01/SEC-02 -- Sensitive vars blocked ────

func TestEnvScrubbing_SensitiveVarsBlocked(t *testing.T) {
	// Inject known-sensitive env vars into the subprocess environment and
	// verify they are absent inside the sandbox. Uses printf (bash builtin).
	probe := `printf "AWS=%s\n" "$AWS_SECRET_ACCESS_KEY"
printf "GH=%s\n" "$GITHUB_TOKEN"
printf "SSH=%s\n" "$SSH_AUTH_SOCK"
printf "OPENAI=%s\n" "$OPENAI_API_KEY"
printf "HOME=%s\n" "$HOME"
`
	stdout, _, exitCode := runSandfloxProbeWithEnv(t, probe,
		"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI",
		"GITHUB_TOKEN=ghp_testtoken",
		"SSH_AUTH_SOCK=/tmp/ssh-test",
		"OPENAI_API_KEY=sk-test123",
	)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0", exitCode)
	}

	// Each sensitive var should be empty (scrubbed by BuildSanitizedEnv).
	// printf "AWS=%s\n" with an unset/empty var produces "AWS=\n".
	sensitiveChecks := map[string]string{
		"AWS=\n":    "AWS_SECRET_ACCESS_KEY",
		"GH=\n":    "GITHUB_TOKEN",
		"SSH=\n":    "SSH_AUTH_SOCK",
		"OPENAI=\n": "OPENAI_API_KEY",
	}
	for expected, varName := range sensitiveChecks {
		if !strings.Contains(stdout, expected) {
			t.Errorf("%s should be empty inside sandbox (expected %q in stdout); got stdout=%q",
				varName, expected, stdout)
		}
	}

	// HOME must still pass through (sanity check)
	if !strings.Contains(stdout, "HOME=/") {
		t.Errorf("HOME should still be set inside sandbox; got stdout=%q", stdout)
	}
}

// ── Test 12: SEC-03 -- Python safety flags forced ───────

func TestEnvScrubbing_PythonSafetyFlags(t *testing.T) {
	// Verify that PYTHONDONTWRITEBYTECODE=1 and PYTHON_NOPIP=1 are
	// set inside the sandbox. These are forced by both BuildSanitizedEnv
	// (Go-level) and entrypoint.sh (shell-level) for defense-in-depth.
	probe := `printf "PYDWB=%s\n" "$PYTHONDONTWRITEBYTECODE"
printf "PYNP=%s\n" "$PYTHON_NOPIP"
`
	stdout, _, exitCode := runSandfloxProbe(t, probe)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0", exitCode)
	}

	if !strings.Contains(stdout, "PYDWB=1") {
		t.Errorf("PYTHONDONTWRITEBYTECODE should be '1' inside sandbox; got stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "PYNP=1") {
		t.Errorf("PYTHON_NOPIP should be '1' inside sandbox; got stdout=%q", stdout)
	}
}

// ── Test 13: SEC-02 -- Debug env diagnostic output ──────

func TestEnvScrubbing_DebugDiagnostic(t *testing.T) {
	// Invoke sandflox with --debug and verify that the env scrubbing
	// diagnostic line appears in stderr with the expected format:
	//   [sandflox] Env: N vars passed, M blocked, K forced
	stdout, stderrOut, exitCode := runSandfloxWithFlags(t,
		[]string{"--debug"}, `echo ok`)
	if exitCode != 0 {
		t.Fatalf("probe exited %d; expected 0; stdout=%q stderr=%q", exitCode, stdout, stderrOut)
	}

	if !strings.Contains(stderrOut, "[sandflox] Env:") {
		t.Errorf("expected '[sandflox] Env:' in debug stderr; got stderr=%q", stderrOut)
	}
	if !strings.Contains(stderrOut, "vars passed") {
		t.Errorf("expected 'vars passed' in debug stderr; got stderr=%q", stderrOut)
	}
	if !strings.Contains(stderrOut, "blocked") {
		t.Errorf("expected 'blocked' in debug stderr; got stderr=%q", stderrOut)
	}
	if !strings.Contains(stderrOut, "forced") {
		t.Errorf("expected 'forced' in debug stderr; got stderr=%q", stderrOut)
	}
}
