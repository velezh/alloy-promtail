// Command overlay generates the go build -overlay file that turns the upstream
// Alloy tree into alloy-promtail without modifying upstream sources.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		root    = flag.String("root", ".", "Repository root.")
		dataDir = flag.String("dir", "promtail/overlay", "Directory holding manifest.txt, _files/ and patches/.")
		outDir  = flag.String("out", "build/promtail/overlay", "Output directory for overlay.json and patched files.")
	)
	flag.Parse()

	if err := run(*root, *dataDir, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "overlay: %v\n", err)
		os.Exit(1)
	}
}

// run generates <outDir>/overlay.json from <dataDir>/manifest.txt.
func run(root, dataDir, outDir string) error {
	cfg, err := absConfig(root, dataDir, outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		return err
	}

	f, err := os.Open(filepath.Join(cfg.dataDir, "manifest.txt"))
	if err != nil {
		return err
	}
	entries, err := parseManifest(f)
	f.Close()
	if err != nil {
		return err
	}

	overlayFile := filepath.Join(cfg.outDir, "overlay.json")
	if err := os.Remove(overlayFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(filepath.Join(cfg.outDir, "files")); err != nil {
		return err
	}
	replace, err := generate(cfg, entries)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(struct{ Replace map[string]string }{replace}, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(overlayFile, append(data, '\n'), 0o644)
}

// absConfig returns a config with absolute directories.
func absConfig(root, dataDir, outDir string) (config, error) {
	var cfg config
	for _, p := range []struct {
		dst *string
		src string
	}{{&cfg.root, root}, {&cfg.dataDir, dataDir}, {&cfg.outDir, outDir}} {
		abs, err := filepath.Abs(p.src)
		if err != nil {
			return config{}, err
		}
		*p.dst = abs
	}
	return cfg, nil
}
