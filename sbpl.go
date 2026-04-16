// sbpl.go — Apple Sandbox Profile Language (SBPL) generator.
//
// Mirrors the bash _sfx_generate_sbpl() function (sandflox.bash:191-291).
// Strategy: (allow default) baseline + selective denials. deny-default
// breaks flox/Nix because of unpredictable read paths (KERN-02, D-02).
//
// Output is a pure string produced from *ResolvedConfig + home dir. The
// bash implementation is the canonical spec (D-01): rules emit in the
// same order, with the same comments, so the generated .sb is
// byte-compatible with the existing .flox/cache/sandflox/sandflox.sb
// for the same inputs.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Public API ──────────────────────────────────────────

// GenerateSBPL returns the complete SBPL profile content for the given
// resolved configuration. home is the absolute user home directory
// (typically os.UserHomeDir()). The function is pure — no I/O.
//
// Rule ordering (matches bash sandflox.bash:191-291):
//  1. Header: (version 1), comments, (allow default)
//  2. Denied paths block (only if cfg.Denied is non-empty), followed by
//     Flox-required overrides
//  3. Filesystem write rules, switched on cfg.FsMode
//  4. Network rules, switched on cfg.NetMode
func GenerateSBPL(cfg *ResolvedConfig, home string) string {
	var sb strings.Builder
	writeSBPLHeader(&sb)
	writeSBPLDenied(&sb, cfg.Denied, home)
	writeSBPLFilesystem(&sb, cfg.FsMode, cfg.ReadOnly, home)
	writeSBPLNetwork(&sb, cfg.NetMode, cfg.AllowLocalhost)
	return sb.String()
}

// WriteSBPL writes the generated SBPL content to {cacheDir}/sandflox.sb
// with mode 0644. Creates cacheDir if it does not exist. Returns the
// absolute path to the written file, or a wrapped error.
func WriteSBPL(cacheDir string, content string) (string, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("[sandflox] ERROR: cannot write SBPL profile: %w", err)
	}
	sbplPath := filepath.Join(cacheDir, "sandflox.sb")
	if err := os.WriteFile(sbplPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("[sandflox] ERROR: cannot write SBPL profile: %w", err)
	}
	return sbplPath, nil
}

// ── Header ──────────────────────────────────────────────

// writeSBPLHeader emits the fixed SBPL preamble. Always present.
// Matches bash sandflox.bash:200-206.
func writeSBPLHeader(sb *strings.Builder) {
	sb.WriteString("(version 1)\n")
	sb.WriteString("\n")
	sb.WriteString(";; sandflox — generated SBPL profile\n")
	sb.WriteString(";; Baseline: allow everything, then restrict per policy.toml\n")
	sb.WriteString("(allow default)\n")
}

// ── Denied Paths ────────────────────────────────────────

// writeSBPLDenied emits the denied-paths block and the flox-required
// overrides block. Both are emitted ONLY if denied is non-empty
// (matches bash conditional at sandflox.bash:209).
//
// Trailing "/" is stripped from each denied path before emission
// (matches bash ${dpath%/} at sandflox.bash:214) — subpath semantics
// do not need the slash.
func writeSBPLDenied(sb *strings.Builder, denied []string, home string) {
	if len(denied) == 0 {
		return
	}

	// Denied paths
	sb.WriteString("\n")
	sb.WriteString(";; ── Denied paths (sensitive data) ──\n")
	for _, p := range denied {
		if p == "" {
			continue
		}
		p = strings.TrimSuffix(p, "/")
		fmt.Fprintf(sb, "(deny file-read* (subpath \"%s\"))\n", p)
		fmt.Fprintf(sb, "(deny file-write* (subpath \"%s\"))\n", p)
	}

	// Flox-required overrides — flox/nix need these paths even when
	// the home tree is denied. Matches bash sandflox.bash:219-224.
	sb.WriteString("\n")
	sb.WriteString(";; ── Flox-required overrides ──\n")
	sb.WriteString("(allow file-read* (subpath (param \"FLOX_CACHE\")))\n")
	fmt.Fprintf(sb, "(allow file-read* (subpath \"%s/.config/flox\"))\n", home)
	fmt.Fprintf(sb, "(allow file-write* (subpath \"%s/.config/flox\"))\n", home)
}

// ── Filesystem ──────────────────────────────────────────

// writeSBPLFilesystem emits filesystem write rules based on mode.
//   - "permissive": no rules at all
//   - "strict":    deny all writes, re-allow tmp/dev/flox only (no PROJECT)
//   - "workspace": deny all writes, re-allow PROJECT + tmp/dev/flox, then
//     emit read-only overrides within the project
//
// Unknown modes default to "workspace" (matches bash workspace|*) at
// sandflox.bash:245).
func writeSBPLFilesystem(sb *strings.Builder, fsMode string, readOnly []string, home string) {
	switch fsMode {
	case "permissive":
		// No write restrictions — emit nothing
		return

	case "strict":
		sb.WriteString("\n")
		sb.WriteString(";; ── Filesystem writes (strict — deny most writes) ──\n")
		sb.WriteString("(deny file-write*)\n")
		sb.WriteString("(allow file-write*\n")
		sb.WriteString("  (subpath \"/private/tmp\")\n")
		sb.WriteString("  (subpath \"/private/var/folders\")\n")
		sb.WriteString("  (subpath \"/dev\")\n")
		sb.WriteString("  (subpath (param \"FLOX_CACHE\"))\n")
		fmt.Fprintf(sb, "  (subpath \"%s/.config/flox\")\n", home)
		fmt.Fprintf(sb, "  (subpath \"%s/.local/share/flox\"))\n", home)

	default: // "workspace" and any unknown value (matches bash workspace|*))
		sb.WriteString("\n")
		sb.WriteString(";; ── Filesystem writes (workspace) ──\n")
		sb.WriteString("(deny file-write*)\n")
		sb.WriteString("(allow file-write*\n")
		sb.WriteString("  (subpath (param \"PROJECT\"))\n")
		sb.WriteString("  (subpath \"/private/tmp\")\n")
		sb.WriteString("  (subpath \"/private/var/folders\")\n")
		sb.WriteString("  (subpath \"/dev\")\n")
		sb.WriteString("  (subpath (param \"FLOX_CACHE\"))\n")
		fmt.Fprintf(sb, "  (subpath \"%s/.config/flox\")\n", home)
		fmt.Fprintf(sb, "  (subpath \"%s/.local/share/flox\"))\n", home)

		// Read-only overrides within project (D-04):
		//   trailing "/" -> subpath (directory)
		//   no trailing "/" -> literal (exact file)
		sb.WriteString("\n")
		sb.WriteString(";; Read-only overrides within project\n")
		for _, r := range readOnly {
			if r == "" {
				continue
			}
			if strings.HasSuffix(r, "/") {
				fmt.Fprintf(sb, "(deny file-write* (subpath \"%s\"))\n", strings.TrimSuffix(r, "/"))
			} else {
				fmt.Fprintf(sb, "(deny file-write* (literal \"%s\"))\n", r)
			}
		}
	}
}

// ── Network ─────────────────────────────────────────────

// writeSBPLNetwork emits network rules based on mode.
//   - "unrestricted": emit comment only, no rules (matches bash sandflox.bash:277)
//   - "blocked":      deny all, re-allow unix sockets (always), localhost (optional)
//
// Unknown modes default to "blocked" (matches bash blocked|*) at
// sandflox.bash:279).
//
// The leading blank line + header comment are always emitted, matching
// the bash unconditional `echo ""` at sandflox.bash:274.
func writeSBPLNetwork(sb *strings.Builder, netMode string, allowLocalhost bool) {
	sb.WriteString("\n")

	switch netMode {
	case "unrestricted":
		sb.WriteString(";; ── Network (unrestricted) ──\n")
		// No rules — comment only

	default: // "blocked" and any unknown value (matches bash blocked|*))
		sb.WriteString(";; ── Network (blocked) ──\n")
		sb.WriteString("(deny network*)\n")
		// Always allow unix sockets — nix daemon IPC (KERN-08)
		sb.WriteString("(allow network* (remote unix-socket))\n")
		if allowLocalhost {
			sb.WriteString("(allow network* (remote ip \"localhost:*\"))\n")
		}
	}
}
