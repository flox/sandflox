package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Cache Artifact Writer ───────────────────────────────

// WriteCache writes all resolved configuration artifacts to the cache directory.
// Creates the directory if it does not exist. Writes 10 files matching the
// existing bash+python implementation's cache layout.
func WriteCache(cacheDir string, config *ResolvedConfig, projectDir string) error {
	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot create cache dir: %w", err)
	}

	// Individual text files (matching existing cache layout)
	textFiles := map[string]string{
		"net-mode.txt":       config.NetMode + "\n",
		"fs-mode.txt":        config.FsMode + "\n",
		"active-profile.txt": config.Profile + "\n",
	}
	for name, content := range textFiles {
		path := filepath.Join(cacheDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("[sandflox] ERROR: cannot write %s: %w", name, err)
		}
	}

	// Net-blocked flag (presence = network is blocked)
	flagPath := filepath.Join(cacheDir, "net-blocked.flag")
	if config.NetMode == "blocked" {
		if err := os.WriteFile(flagPath, []byte("1\n"), 0644); err != nil {
			return fmt.Errorf("[sandflox] ERROR: cannot write net-blocked.flag: %w", err)
		}
	} else {
		os.Remove(flagPath) // clean up if mode changed
	}

	// Path lists (one resolved path per line)
	if err := writePathList(filepath.Join(cacheDir, "writable-paths.txt"), config.Writable); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write writable-paths.txt: %w", err)
	}
	if err := writePathList(filepath.Join(cacheDir, "read-only-paths.txt"), config.ReadOnly); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write read-only-paths.txt: %w", err)
	}
	if err := writePathList(filepath.Join(cacheDir, "denied-paths.txt"), config.Denied); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write denied-paths.txt: %w", err)
	}

	// Copy requisites file to cache
	reqSrc := filepath.Join(projectDir, config.Requisites)
	reqData, err := os.ReadFile(reqSrc)
	if err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot read requisites file %s: %w", config.Requisites, err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "requisites.txt"), reqData, 0644); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write requisites.txt: %w", err)
	}

	// config.json (full resolved config as JSON)
	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot marshal config: %w", err)
	}
	jsonData = append(jsonData, '\n')
	if err := os.WriteFile(filepath.Join(cacheDir, "config.json"), jsonData, 0644); err != nil {
		return fmt.Errorf("[sandflox] ERROR: cannot write config.json: %w", err)
	}

	return nil
}

// writePathList writes a slice of paths to a file, one per line.
func writePathList(path string, paths []string) error {
	var buf strings.Builder
	for _, p := range paths {
		buf.WriteString(p)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(buf.String()), 0644)
}
