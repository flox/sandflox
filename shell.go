// shell.go -- Shell-tier enforcement generators (SHELL-01 .. SHELL-08).
//
// Mirrors the bash hook + profile.common shell enforcement in
// manifest.toml.v2-bash (lines 49-412) and the entrypoint.sh generation
// in sandflox.bash (lines 353-438). Strategy: pure string generators
// rendered from text/template; zero external Go deps; artifacts written
// to .flox/cache/sandflox/ and .flox/cache/sandflox-python/ for the bash
// entrypoint (interactive: flox activate -- bash --rcfile entrypoint.sh;
// non-interactive: flox activate -- bash -c 'source entrypoint.sh && exec "$@"').

package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// ── Embedded Templates ──────────────────────────────────

//go:embed templates/entrypoint.sh.tmpl templates/fs-filter.sh.tmpl templates/usercustomize.py.tmpl
var shellTemplates embed.FS

// ── Exported Constants ──────────────────────────────────

// ArmoredCommands lists the 26 package-manager command names that are
// shadowed by shell functions returning exit 126. Order matches
// manifest.toml.v2-bash:369-394 verbatim.
var ArmoredCommands = []string{
	"flox", "nix", "nix-env", "nix-store", "nix-shell", "nix-build",
	"apt", "apt-get", "yum", "dnf",
	"brew", "snap", "flatpak",
	"pip", "pip3", "npm", "npx", "yarn", "pnpm",
	"cargo", "go", "gem", "composer", "uv",
	"docker", "podman",
}

// WriteCmds lists the 8 write commands wrapped by fs-filter.sh with
// path-checking functions. Order matches manifest.toml.v2-bash:208.
var WriteCmds = []string{"cp", "mv", "mkdir", "rm", "rmdir", "ln", "chmod", "tee"}

// ── Template Data ───────────────────────────────────────

// shellTemplateData holds the values passed to each .tmpl file during
// rendering. Fields map 1:1 to ResolvedConfig plus the two constant
// slices (ArmoredCommands, WriteCmds).
type shellTemplateData struct {
	FsMode          string
	NetMode         string
	Writable        []string
	ReadOnly        []string
	Denied          []string
	ArmoredCommands []string
	WriteCmds       []string
}

// ── Shell Quoting ───────────────────────────────────────

// shellquote returns a string safe for inclusion in bash single-quoted
// contexts. Embedded single quotes are escaped via the '\'' idiom.
// Empty strings become ''.
func shellquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// trimslash strips trailing slashes from a path string. Policy paths
// use trailing "/" as a directory indicator, but bash case patterns
// need clean paths to avoid double-slash mismatches (e.g. "dir//*"
// won't match "dir/file").
func trimslash(s string) string {
	return strings.TrimRight(s, "/")
}

// ── Template Rendering ──────────────────────────────────

// renderTemplate parses and executes the named template from the
// embedded FS, returning the rendered string.
func renderTemplate(name string, data shellTemplateData) (string, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{
		"shellquote": shellquote,
		"trimslash":  trimslash,
	}).ParseFS(shellTemplates, "templates/"+name)
	if err != nil {
		return "", fmt.Errorf("[sandflox] ERROR: parse template %s: %w", name, err)
	}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("[sandflox] ERROR: render template %s: %w", name, err)
	}
	return buf.String(), nil
}

// buildTemplateData constructs the template data struct from a
// ResolvedConfig, filling in the constant slices.
func buildTemplateData(cfg *ResolvedConfig) shellTemplateData {
	return shellTemplateData{
		FsMode:          cfg.FsMode,
		NetMode:         cfg.NetMode,
		Writable:        cfg.Writable,
		ReadOnly:        cfg.ReadOnly,
		Denied:          cfg.Denied,
		ArmoredCommands: ArmoredCommands,
		WriteCmds:       WriteCmds,
	}
}

// ── Public Generators ───────────────────────────────────

// GenerateEntrypoint renders the entrypoint.sh template for the given
// config. The output is a complete bash script that applies PATH wipe,
// requisites symlink bin, armor functions, fs-filter sourcing, Python
// env exports, and breadcrumb cleanup.
func GenerateEntrypoint(cfg *ResolvedConfig) (string, error) {
	return renderTemplate("entrypoint.sh.tmpl", buildTemplateData(cfg))
}

// GenerateFsFilter renders the fs-filter.sh template for the given
// config. In permissive mode the output is a no-op script. In workspace
// or strict mode it contains _sfx_check_write_target and 8 wrapper
// functions for cp/mv/mkdir/rm/rmdir/ln/chmod/tee.
func GenerateFsFilter(cfg *ResolvedConfig) (string, error) {
	return renderTemplate("fs-filter.sh.tmpl", buildTemplateData(cfg))
}

// GenerateUsercustomize renders the usercustomize.py template. This is
// a static template (no Go-side substitutions) — Python reads cached
// policy state files at runtime.
func GenerateUsercustomize(cfg *ResolvedConfig) (string, error) {
	return renderTemplate("usercustomize.py.tmpl", buildTemplateData(cfg))
}

// ── Artifact Writer ─────────────────────────────────────

// WriteShellArtifacts generates and writes all three shell-tier
// enforcement artifacts to the cache directory structure:
//
//   - {cacheDir}/entrypoint.sh  (0755 — executable)
//   - {cacheDir}/fs-filter.sh   (0644)
//   - {cacheDir}/../sandflox-python/usercustomize.py (0644)
//
// cacheDir is typically .flox/cache/sandflox. The Python artifact
// lives in a sibling directory (sandflox-python) to match the bash
// reference layout.
func WriteShellArtifacts(cacheDir string, cfg *ResolvedConfig) error {
	pythonCacheDir := filepath.Join(filepath.Dir(cacheDir), "sandflox-python")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot create cache dir %s: %w", cacheDir, err)
	}
	if err := os.MkdirAll(pythonCacheDir, 0755); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot create python cache dir %s: %w", pythonCacheDir, err)
	}

	// Generate entrypoint.sh
	entrypoint, err := GenerateEntrypoint(cfg)
	if err != nil {
		return fmt.Errorf("[sandflox] ERROR: generate entrypoint.sh: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "entrypoint.sh"), []byte(entrypoint), 0755); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write entrypoint.sh: %w", err)
	}

	// Generate fs-filter.sh
	fsFilter, err := GenerateFsFilter(cfg)
	if err != nil {
		return fmt.Errorf("[sandflox] ERROR: generate fs-filter.sh: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "fs-filter.sh"), []byte(fsFilter), 0644); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write fs-filter.sh: %w", err)
	}

	// Generate usercustomize.py
	usercustomize, err := GenerateUsercustomize(cfg)
	if err != nil {
		return fmt.Errorf("[sandflox] ERROR: generate usercustomize.py: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pythonCacheDir, "usercustomize.py"), []byte(usercustomize), 0644); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write usercustomize.py: %w", err)
	}

	return nil
}
