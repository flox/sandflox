//go:build darwin

// exec_test.go -- Unit tests for buildSandboxExecArgv (argv shape only).
//
// Rationale: buildSandboxExecArgv is pure (no I/O, no syscalls), so it is
// cheap and safe to unit-test. execWithKernelEnforcement itself cannot be
// unit-tested because it ends in syscall.Exec / os.Exit -- those are
// covered by the subprocess integration tests in Plan 02-03.
//
// Tests verify the D-01/D-02 argv shape:
//   - Interactive (D-01): ... flox activate -- bash --rcfile <entrypoint> -i
//   - Non-interactive (D-02): ... flox activate -- bash -c 'source <ep> && exec "$@"' -- CMD...
//
// Required tests (per .planning/phases/02-.../02-VALIDATION.md, updated for Phase 3):
//   - TestBuildSandboxExecArgs_Interactive (D-01)
//   - TestBuildSandboxExecArgs_WithUserCommand (D-02)
//   - TestBuildSandboxExecArgs_PreservesAbsolutePathForFlox (Pitfall 6)
//   - TestBuildSandboxExecArgs_InteractiveUsesRcfileNotDashC (renamed from NoUserArgsDoesNotEmitDoubleDash)
//   - TestBuildSandboxExecArgs_HandlesUserArgsWithDashes
//
// Stdlib only (testing + reflect). No third-party assertion libraries.

package main

import (
	"reflect"
	"strings"
	"testing"
)

// ── Test 1: D-01 -- interactive mode (no userArgs) ──────

func TestBuildSandboxExecArgs_Interactive(t *testing.T) {
	entrypoint := "/cache/entrypoint.sh"
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		entrypoint,
		nil,
	)
	want := []string{
		"sandbox-exec",
		"-f", "/cache/sandflox.sb",
		"-D", "PROJECT=/proj",
		"-D", "HOME=/home/x",
		"-D", "FLOX_CACHE=/home/x/.cache/flox",
		"/abs/flox", "activate",
		"--",
		"bash", "--rcfile", entrypoint, "-i",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv mismatch\n got:  %v\n want: %v", got, want)
	}
	if len(got) != 16 {
		t.Errorf("expected 16 elements, got %d: %v", len(got), got)
	}
}

// ── Test 2: D-02 -- wrap arbitrary command via bash -c ──

func TestBuildSandboxExecArgs_WithUserCommand(t *testing.T) {
	entrypoint := "/cache/entrypoint.sh"
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		entrypoint,
		[]string{"echo", "hello"},
	)

	// Non-interactive: 16 + len(userArgs) = 18 elements
	if len(got) != 18 {
		t.Errorf("expected 18 elements, got %d: %v", len(got), got)
	}

	// The -c payload must be exactly this string
	wantPayload := "source '/cache/entrypoint.sh' && exec \"$@\""

	// Assert elements from activate onward (index 10..)
	wantTail := []string{
		"activate", "--", "bash", "-c", wantPayload, "--", "echo", "hello",
	}
	gotTail := got[10:]
	if !reflect.DeepEqual(gotTail, wantTail) {
		t.Errorf("argv tail mismatch\n got:  %v\n want: %v", gotTail, wantTail)
	}

	// Verify the -c payload at element[14]
	if got[14] != wantPayload {
		t.Errorf("element[14] (bash -c payload) mismatch\n got:  %q\n want: %q", got[14], wantPayload)
	}
}

// ── Test 3: Pitfall 6 -- flox path must be passed through verbatim ──

func TestBuildSandboxExecArgs_PreservesAbsolutePathForFlox(t *testing.T) {
	entrypoint := "/cache/entrypoint.sh"
	floxAbs := "/some/absolute/path/flox"
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		floxAbs,
		entrypoint,
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

// ── Test 4: D-01 uses --rcfile and -i, NOT -c or "$@" ──

func TestBuildSandboxExecArgs_InteractiveUsesRcfileNotDashC(t *testing.T) {
	entrypoint := "/cache/entrypoint.sh"
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		entrypoint,
		nil,
	)

	// Must contain --rcfile and -i
	hasRcfile := false
	hasDashI := false
	for _, a := range got {
		if a == "--rcfile" {
			hasRcfile = true
		}
		if a == "-i" {
			hasDashI = true
		}
	}
	if !hasRcfile {
		t.Errorf("interactive argv must contain --rcfile: %v", got)
	}
	if !hasDashI {
		t.Errorf("interactive argv must contain -i: %v", got)
	}

	// Must NOT contain -c or "$@" (those are non-interactive D-02 only)
	for _, a := range got {
		if a == "-c" {
			t.Errorf("interactive argv must NOT contain -c: %v", got)
		}
		if strings.Contains(a, `"$@"`) {
			t.Errorf("interactive argv must NOT contain \"$@\": %v", got)
		}
	}

	// Must contain exactly one "--" token (the one before "bash")
	ddCount := 0
	for _, a := range got {
		if a == "--" {
			ddCount++
		}
	}
	if ddCount != 1 {
		t.Errorf("expected exactly 1 '--' token in interactive argv, got %d: %v", ddCount, got)
	}

	// Also verify when userArgs is an explicit empty slice (vs nil)
	gotEmpty := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		entrypoint,
		[]string{},
	)
	if !reflect.DeepEqual(got, gotEmpty) {
		t.Errorf("nil vs empty slice should produce identical argv\n nil:   %v\n empty: %v", got, gotEmpty)
	}
}

// ── Test 5: flags inside userArgs must land AFTER second -- boundary ──

func TestBuildSandboxExecArgs_HandlesUserArgsWithDashes(t *testing.T) {
	entrypoint := "/cache/entrypoint.sh"
	got := buildSandboxExecArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/abs/flox",
		entrypoint,
		[]string{"bash", "-c", "echo hi"},
	)

	wantPayload := "source '/cache/entrypoint.sh' && exec \"$@\""

	// Verify the -c payload at element[14]
	if got[14] != wantPayload {
		t.Errorf("element[14] (bash -c payload) mismatch\n got:  %q\n want: %q", got[14], wantPayload)
	}

	// User args start at element[16]
	if got[16] != "bash" || got[17] != "-c" || got[18] != "echo hi" {
		t.Errorf("user args mismatch at elements 16-18: got %v", got[16:])
	}

	// Count -- occurrences: should be exactly 2 (one after activate, one before user args)
	ddCount := 0
	for _, a := range got {
		if a == "--" {
			ddCount++
		}
	}
	if ddCount != 2 {
		t.Errorf("expected exactly 2 '--' tokens, got %d: %v", ddCount, got)
	}

	// Count -c occurrences: should be exactly 2 (our bash -c and the one in userArgs)
	cCount := 0
	for _, a := range got {
		if a == "-c" {
			cCount++
		}
	}
	if cCount != 2 {
		t.Errorf("expected exactly 2 '-c' tokens, got %d: %v", cCount, got)
	}
}

// ── buildElevateArgv Tests ──────────────────────────────
//
// buildElevateArgv produces a 12-element argv for sandbox-exec wrapping
// the current shell WITHOUT flox activate. Used by `sandflox elevate`
// to re-exec an existing flox session under kernel enforcement.

func TestBuildElevateArgv_Interactive(t *testing.T) {
	got := buildElevateArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/cache/entrypoint.sh",
	)
	want := []string{
		"sandbox-exec",
		"-f", "/cache/sandflox.sb",
		"-D", "PROJECT=/proj",
		"-D", "HOME=/home/x",
		"-D", "FLOX_CACHE=/home/x/.cache/flox",
		"bash", "--rcfile", "/cache/entrypoint.sh", "-i",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv mismatch\n got:  %v\n want: %v", got, want)
	}
	if len(got) != 12 {
		t.Errorf("expected 12 elements, got %d: %v", len(got), got)
	}
}

func TestBuildElevateArgv_NoFloxActivate(t *testing.T) {
	got := buildElevateArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/cache/entrypoint.sh",
	)
	for i, elem := range got {
		lower := strings.ToLower(elem)
		if strings.Contains(lower, "flox") {
			t.Errorf("argv[%d] = %q contains 'flox' -- elevate argv must NOT include flox", i, elem)
		}
		if strings.Contains(lower, "activate") {
			t.Errorf("argv[%d] = %q contains 'activate' -- elevate argv must NOT include activate", i, elem)
		}
	}
}

func TestBuildElevateArgv_HasEntrypoint(t *testing.T) {
	entrypoint := "/cache/entrypoint.sh"
	got := buildElevateArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		entrypoint,
	)
	foundRcfile := false
	for i, elem := range got {
		if elem == "--rcfile" {
			foundRcfile = true
			if i+1 >= len(got) {
				t.Fatalf("--rcfile at index %d has no following element", i)
			}
			if got[i+1] != entrypoint {
				t.Errorf("--rcfile followed by %q, want %q", got[i+1], entrypoint)
			}
			break
		}
	}
	if !foundRcfile {
		t.Errorf("argv does not contain --rcfile: %v", got)
	}
}

func TestBuildElevateArgv_SandboxExecParams(t *testing.T) {
	got := buildElevateArgv(
		"/cache/sandflox.sb",
		"/proj",
		"/home/x",
		"/home/x/.cache/flox",
		"/cache/entrypoint.sh",
	)
	if got[0] != "sandbox-exec" {
		t.Errorf("argv[0] = %q, want \"sandbox-exec\"", got[0])
	}
	if got[1] != "-f" {
		t.Errorf("argv[1] = %q, want \"-f\"", got[1])
	}
	if got[3] != "-D" {
		t.Errorf("argv[3] = %q, want \"-D\"", got[3])
	}
	if !strings.HasPrefix(got[4], "PROJECT=") {
		t.Errorf("argv[4] = %q, want prefix \"PROJECT=\"", got[4])
	}
}
