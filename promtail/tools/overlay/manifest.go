package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

// Manifest operations.
const (
	opDelete  = "delete"
	opReplace = "replace"
	opPatch   = "patch"
	opKeep    = "keep"
)

var blobIDRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// entry is a single manifest operation applied to an upstream path.
type entry struct {
	line int
	op   string
	path string
	keep []string
	blob string
}

// parseManifest parses manifest lines of the form "<op> <path> [args...]".
//
// Returns the entries in manifest order.
func parseManifest(r io.Reader) ([]entry, error) {
	var (
		entries []entry
		errs    []error
		seen    = map[string]int{}
	)

	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		e, err := parseEntry(n, fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("manifest line %d: %w", n, err))
			continue
		}
		if prev, ok := seen[e.path]; ok {
			errs = append(errs, fmt.Errorf("manifest line %d: %s is already handled on line %d", n, e.path, prev))
			continue
		}
		seen[e.path] = n
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return entries, errors.Join(errs...)
}

// parseEntry builds an entry from the whitespace separated fields of manifest line n.
func parseEntry(n int, fields []string) (entry, error) {
	if len(fields) < 2 {
		return entry{}, fmt.Errorf("expected \"<op> <path>\", got %q", strings.Join(fields, " "))
	}

	e := entry{line: n, op: fields[0], path: fields[1]}
	if err := validatePath(e.path); err != nil {
		return entry{}, err
	}
	args := fields[2:]

	switch e.op {
	case opDelete, opPatch:
		if len(args) != 0 {
			return entry{}, fmt.Errorf("%s takes no arguments", e.op)
		}
	case opReplace:
		for _, arg := range args {
			blob, ok := strings.CutPrefix(arg, "blob=")
			if !ok || !blobIDRe.MatchString(blob) || e.blob != "" {
				return entry{}, fmt.Errorf("replace accepts a single blob=<40 hex digits> argument, got %q", arg)
			}
			e.blob = blob
		}
	case opKeep:
		if len(args) == 0 {
			return entry{}, errors.New("keep requires at least one file name")
		}
		for _, name := range args {
			if strings.ContainsRune(name, '/') || !strings.HasSuffix(name, ".go") {
				return entry{}, fmt.Errorf("keep expects Go file names inside %s, got %q", e.path, name)
			}
		}
		e.keep = args
	default:
		return entry{}, fmt.Errorf("unknown operation %q", e.op)
	}
	return e, nil
}

// validatePath reports whether p is a clean slash-separated path inside the repository.
func validatePath(p string) error {
	if path.IsAbs(p) || p != path.Clean(p) || p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.ContainsRune(p, '\\') {
		return fmt.Errorf("path %q must be clean, relative and slash-separated", p)
	}
	return nil
}
