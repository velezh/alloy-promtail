package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// config holds the directories used to build an overlay.
type config struct {
	root    string // repository root
	dataDir string // directory holding manifest.txt, _files/ and patches/
	outDir  string // directory receiving patched files
}

// generate resolves manifest entries into go build overlay replacements.
//
// Patched files are written below cfg.outDir. Returns a map from absolute
// upstream paths to absolute replacement paths, an empty replacement deletes
// the upstream file.
func generate(cfg config, entries []entry) (map[string]string, error) {
	var (
		replace = map[string]string{}
		errs    []error
	)
	add := func(e entry, upstream, src string) {
		if prev, ok := replace[upstream]; ok && prev != src {
			errs = append(errs, fmt.Errorf("manifest line %d: %s is replaced twice", e.line, upstream))
			return
		}
		replace[upstream] = src
	}

	for _, e := range entries {
		upstream := filepath.Join(cfg.root, filepath.FromSlash(e.path))
		var err error
		switch e.op {
		case opDelete:
			if err = requireFile(upstream); err == nil {
				add(e, upstream, "")
			}
		case opReplace:
			var src string
			if src, err = resolveReplace(cfg, e, upstream); err == nil {
				add(e, upstream, src)
			}
		case opPatch:
			var dst string
			if dst, err = resolvePatch(cfg, e, upstream); err == nil {
				add(e, upstream, dst)
			}
		case opKeep:
			var drop []string
			if drop, err = resolveKeep(e, upstream); err == nil {
				for _, f := range drop {
					add(e, f, "")
				}
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("manifest line %d: %s %s: %w", e.line, e.op, e.path, err))
		}
	}
	return replace, errors.Join(errs...)
}

// resolveReplace validates a replace entry and returns its replacement file.
func resolveReplace(cfg config, e entry, upstream string) (string, error) {
	data, err := os.ReadFile(upstream)
	if err != nil {
		return "", err
	}
	if e.blob != "" {
		if got := gitBlobID(data); got != e.blob {
			return "", fmt.Errorf("upstream file changed since the replacement was derived from it: review `git diff %s %s`, update %s and pin blob=%s",
				e.blob, got, filepath.Join(cfg.dataDir, "_files", filepath.FromSlash(e.path)), got)
		}
	}
	src := filepath.Join(cfg.dataDir, "_files", filepath.FromSlash(e.path))
	if err := requireFile(src); err != nil {
		return "", err
	}
	return src, nil
}

// resolvePatch applies the patch of e to the upstream file and returns the patched copy.
func resolvePatch(cfg config, e entry, upstream string) (string, error) {
	src, err := os.ReadFile(upstream)
	if err != nil {
		return "", err
	}
	patchFile := filepath.Join(cfg.dataDir, "patches", filepath.FromSlash(e.path)+".patch")
	patch, err := os.ReadFile(patchFile)
	if err != nil {
		return "", err
	}

	target, hunks, err := parsePatch(string(patch))
	if err != nil {
		return "", fmt.Errorf("%s: %w", patchFile, err)
	}
	if target != e.path {
		return "", fmt.Errorf("%s changes %s", patchFile, target)
	}
	patched, err := applyHunks(string(src), hunks)
	if err != nil {
		return "", fmt.Errorf("%s: %w; refresh it with `make -C promtail patch-start FILE=%s`", patchFile, err, e.path)
	}

	dst := filepath.Join(cfg.outDir, "files", filepath.FromSlash(e.path))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return dst, os.WriteFile(dst, []byte(patched), 0o644)
}

// resolveKeep returns the non-test Go files of the upstream package directory not listed in e.keep.
func resolveKeep(e entry, dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	kept := map[string]bool{}
	for _, name := range e.keep {
		kept[name] = false
	}

	var drop []string
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := kept[name]; ok {
			kept[name] = true
			continue
		}
		drop = append(drop, filepath.Join(dir, name))
	}

	var missing []string
	for name, found := range kept {
		if !found {
			missing = append(missing, path.Join(e.path, name))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("kept files do not exist: %s", strings.Join(missing, ", "))
	}
	return drop, nil
}

// requireFile reports an error unless p is an existing regular file.
func requireFile(p string) error {
	fi, err := os.Stat(p)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", p)
	}
	return nil
}

// gitBlobID returns the git object id of data stored as a blob.
func gitBlobID(data []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(data)) + "\x00"))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
