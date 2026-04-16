//go:build darwin

// exec_test.go -- Unit tests for buildSandboxExecArgv (argv shape only).
//
// Rationale: buildSandboxExecArgv is pure (no I/O, no syscalls), so it is
// cheap and safe to unit-test. execWithKernelEnforcement itself cannot be
// unit-tested because it ends in syscall.Exec / os.Exit -- those are
// covered by the subprocess integration tests in Plan 02-03.
//
// Required tests (per .planning/phases/02-.../02-VALIDATION.md):
//   - TestBuildSandboxExecArgs_Interactive (KERN-04, KERN-05)
//   - TestBuildSandboxExecArgs_WithUserCommand (KERN-06)
//   - TestBuildSandboxExecArgs_PreservesAbsolutePathForFlox (Pitfall 6)
//   - TestBuildSandboxExecArgs_NoUserArgsDoesNotEmitDoubleDash
//   - TestBuildSandboxExecArgs_HandlesUserArgsWithDashes
//
// Stdlib only (testing + reflect). No third-party assertion libraries.

package main

import (
	"reflect"
	"testing"
)

// ── Test 1: KERN-04, KERN-05 -- interactive mode (no userArgs) ──

func TestBuildSandboxExecArgs_Interactive(t *testing.T) {
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		nil,
	)
	want := []string{
		"sandbox-exec",
		"-f", "/cache/sandflox.sb",
		"-D", "PROJECT=/proj",
		"-D", "HOME=/home/x",
		"-D", "FLOX_CACHE=/home/x/.cache/flox",
		"/abs/flox", "activate",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv mismatch\n got:  %v\n want: %v", got, want)
	}
	if len(got) != 11 {
		t.Errorf("expected 11 elements, got %d: %v", len(got), got)
	}
}

// ── Test 2: KERN-06 -- wrap arbitrary command via -- boundary ──

func TestBuildSandboxExecArgs_WithUserCommand(t *testing.T) {
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		[]string{"echo", "hello"},
	)
	want := []string{
		"sandbox-exec",
		"-f", "/cache/sandflox.sb",
		"-D", "PROJECT=/proj",
		"-D", "HOME=/home/x",
		"-D", "FLOX_CACHE=/home/x/.cache/flox",
		"/abs/flox", "activate",
		"--", "echo", "hello",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv mismatch\n got:  %v\n want: %v", got, want)
	}
	if len(got) != 14 {
		t.Errorf("expected 14 elements, got %d: %v", len(got), got)
	}

	// Last 4 elements are [activate, --, echo, hello]
	tail := got[len(got)-4:]
	wantTail := []string{"activate", "--", "echo", "hello"}
	if !reflect.DeepEqual(tail, wantTail) {
		t.Errorf("argv tail mismatch\n got:  %v\n want: %v", tail, wantTail)
	}
}

// ── Test 3: Pitfall 6 -- flox path must be passed through verbatim ──

func TestBuildSandboxExecArgs_PreservesAbsolutePathForFlox(t *testing.T) {
	floxAbs := "/some/absolute/path/flox"
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		floxAbs,
		nil,
	)
	if got[9] != floxAbs {
		t.Errorf("argv[9] should be the absolute flox path %q, got %q (full argv: %v)",
			floxAbs, got[9], got)
	}
	// Ensure it's NOT the bare word "flox"
	if got[9] == "flox" {
		t.Errorf("argv[9] should NOT be the relative name \"flox\"; got: %v", got)
	}
}

// ── Test 4: -- boundary must not appear when userArgs is empty ──

func TestBuildSandboxExecArgs_NoUserArgsDoesNotEmitDoubleDash(t *testing.T) {
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		nil,
	)
	for _, a := range got {
		if a == "--" {
			t.Errorf("argv should not contain -- when userArgs is empty: %v", got)
		}
	}

	// Also verify when userArgs is an explicit empty slice (vs nil)
	gotEmpty := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		[]string{},
	)
	for _, a := range gotEmpty {
		if a == "--" {
			t.Errorf("argv should not contain -- for empty slice userArgs: %v", gotEmpty)
		}
	}
}

// ── Test 5: flags inside userArgs must land AFTER -- boundary ──

func TestBuildSandboxExecArgs_HandlesUserArgsWithDashes(t *testing.T) {
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		[]string{"bash", "-c", "echo hi"},
	)
	// Last 5 elements MUST be [activate, --, bash, -c, echo hi]
	tail := got[len(got)-5:]
	want := []string{"activate", "--", "bash", "-c", "echo hi"}
	if !reflect.DeepEqual(tail, want) {
		t.Errorf("argv tail with dash-containing userArgs mismatch\n got:  %v\n want: %v",
			tail, want)
	}

	// Additional safety: the "-c" inside userArgs must be AFTER the "--"
	// boundary, not confused with sandbox-exec's own flag parsing.
	dashDashIdx := -1
	for i, a := range got {
		if a == "--" {
			dashDashIdx = i
			break
		}
	}
	if dashDashIdx < 0 {
		t.Fatalf("expected -- boundary in argv: %v", got)
	}
	cFlagIdx := -1
	for i, a := range got {
		if a == "-c" {
			cFlagIdx = i
			break
		}
	}
	if cFlagIdx < 0 {
		t.Fatalf("expected -c in argv: %v", got)
	}
	if cFlagIdx < dashDashIdx {
		t.Errorf("-c (idx=%d) must be AFTER -- (idx=%d) in argv: %v",
			cFlagIdx, dashDashIdx, got)
	}
}
