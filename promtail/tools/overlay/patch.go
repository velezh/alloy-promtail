package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// hunk is a single hunk of a unified diff.
type hunk struct {
	oldStart int
	oldLines []string
	newLines []string
}

// parsePatch parses a single-file unified diff as produced by "git diff".
//
// Returns the path of the patched file without the "b/" prefix and the hunks.
func parsePatch(data string) (string, []hunk, error) {
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")

	var (
		target  string
		hunks   []hunk
		cur     *hunk
		oldLeft int
		newLeft int
	)
	for i, line := range lines {
		if strings.HasPrefix(line, "\\") {
			return "", nil, fmt.Errorf("patch line %d: missing trailing newline markers are not supported", i+1)
		}
		if cur != nil && (oldLeft > 0 || newLeft > 0) {
			switch {
			case line == "" || line[0] == ' ':
				text := strings.TrimPrefix(line, " ")
				cur.oldLines = append(cur.oldLines, text)
				cur.newLines = append(cur.newLines, text)
				oldLeft--
				newLeft--
			case line[0] == '-':
				cur.oldLines = append(cur.oldLines, line[1:])
				oldLeft--
			case line[0] == '+':
				cur.newLines = append(cur.newLines, line[1:])
				newLeft--
			default:
				return "", nil, fmt.Errorf("patch line %d: unexpected line inside hunk: %q", i+1, line)
			}
			if oldLeft < 0 || newLeft < 0 {
				return "", nil, fmt.Errorf("patch line %d: hunk is longer than its header declares", i+1)
			}
			continue
		}

		switch {
		case target != "" && (strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "--- ")):
			return "", nil, fmt.Errorf("patch line %d: patches must change a single file", i+1)
		case strings.HasPrefix(line, "--- "):
			if strings.TrimSpace(line[4:]) == "/dev/null" {
				return "", nil, fmt.Errorf("patch line %d: new files must be added with the replace operation", i+1)
			}
		case strings.HasPrefix(line, "+++ "):
			name := strings.TrimSpace(line[4:])
			if name == "/dev/null" {
				return "", nil, fmt.Errorf("patch line %d: deleted files must be removed with the delete operation", i+1)
			}
			target = strings.TrimPrefix(name, "b/")
		case strings.HasPrefix(line, "@@ "):
			if target == "" {
				return "", nil, fmt.Errorf("patch line %d: hunk before file header", i+1)
			}
			h, oldCount, newCount, err := parseHunkHeader(line)
			if err != nil {
				return "", nil, fmt.Errorf("patch line %d: %w", i+1, err)
			}
			hunks = append(hunks, h)
			cur, oldLeft, newLeft = &hunks[len(hunks)-1], oldCount, newCount
		case cur != nil:
			return "", nil, fmt.Errorf("patch line %d: unexpected line after hunk: %q", i+1, line)
		}
	}

	switch {
	case cur != nil && (oldLeft > 0 || newLeft > 0):
		return "", nil, errors.New("patch ends in the middle of a hunk")
	case len(hunks) == 0:
		return "", nil, errors.New("patch has no hunks")
	}
	for i, h := range hunks {
		if len(h.oldLines) == 0 {
			return "", nil, fmt.Errorf("hunk %d has no context lines", i+1)
		}
	}
	return target, hunks, nil
}

// parseHunkHeader parses "@@ -l[,s] +l[,s] @@".
//
// Returns the hunk with its start line and the declared old and new line counts.
func parseHunkHeader(line string) (hunk, int, int, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[3] != "@@" || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return hunk{}, 0, 0, fmt.Errorf("malformed hunk header %q", line)
	}
	oldStart, oldCount, err := parseRange(fields[1][1:])
	if err != nil {
		return hunk{}, 0, 0, fmt.Errorf("malformed hunk header %q: %w", line, err)
	}
	_, newCount, err := parseRange(fields[2][1:])
	if err != nil {
		return hunk{}, 0, 0, fmt.Errorf("malformed hunk header %q: %w", line, err)
	}
	return hunk{oldStart: oldStart}, oldCount, newCount, nil
}

// parseRange parses "l[,s]" and returns the start line and the line count.
func parseRange(s string) (int, int, error) {
	start, count, found := strings.Cut(s, ",")
	l, err := strconv.Atoi(start)
	if err != nil {
		return 0, 0, err
	}
	if !found {
		return l, 1, nil
	}
	c, err := strconv.Atoi(count)
	if err != nil {
		return 0, 0, err
	}
	return l, c, nil
}

// applyHunks applies hunks to src in order.
//
// The old lines of every hunk must occur exactly once in the content after the
// previous hunk, so upstream line shifts are tolerated while ambiguous context fails.
// Returns the patched content.
func applyHunks(src string, hunks []hunk) (string, error) {
	trailingNewline := strings.HasSuffix(src, "\n")
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")

	floor := 0
	for i, h := range hunks {
		pos, matches := -1, 0
		for at := floor; at+len(h.oldLines) <= len(lines); at++ {
			if linesEqual(lines[at:at+len(h.oldLines)], h.oldLines) {
				pos = at
				matches++
			}
		}
		switch {
		case matches == 0:
			return "", fmt.Errorf("hunk %d (@@ -%d) does not match the upstream file", i+1, h.oldStart)
		case matches > 1:
			return "", fmt.Errorf("hunk %d (@@ -%d) matches %d locations of the upstream file, save the patch with a larger CONTEXT", i+1, h.oldStart, matches)
		}

		patched := make([]string, 0, len(lines)-len(h.oldLines)+len(h.newLines))
		patched = append(patched, lines[:pos]...)
		patched = append(patched, h.newLines...)
		patched = append(patched, lines[pos+len(h.oldLines):]...)
		lines = patched

		floor = pos + len(h.newLines)
	}

	out := strings.Join(lines, "\n")
	if trailingNewline {
		out += "\n"
	}
	return out, nil
}

func linesEqual(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
