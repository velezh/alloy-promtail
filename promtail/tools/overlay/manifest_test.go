package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testBlob = "4033f06c2ecdd23710396d6e92cae446a6d104f3"

func TestParseManifest(t *testing.T) {
	entries, err := parseManifest(strings.NewReader(`# Header comment.

delete  internal/a/a.go
replace internal/b/b.go    # trailing comment
replace internal/c/c.go blob=` + testBlob + `
	patch internal/d/d.go
keep internal/e e.go e_linux.go
   # indented comment
`))
	require.NoError(t, err)
	require.Equal(t, []entry{
		{line: 3, op: opDelete, path: "internal/a/a.go"},
		{line: 4, op: opReplace, path: "internal/b/b.go"},
		{line: 5, op: opReplace, path: "internal/c/c.go", blob: testBlob},
		{line: 6, op: opPatch, path: "internal/d/d.go"},
		{line: 7, op: opKeep, path: "internal/e", keep: []string{"e.go", "e_linux.go"}},
	}, entries)
}

func TestParseManifestEmpty(t *testing.T) {
	entries, err := parseManifest(strings.NewReader("# Only comments.\n\n"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestParseManifestErrors(t *testing.T) {
	tests := []struct {
		name string
		line string
		err  string
	}{
		{"missing path", "delete", `expected "<op> <path>", got "delete"`},
		{"unknown operation", "remove a.go", `unknown operation "remove"`},
		{"absolute path", "delete /a.go", `path "/a.go" must be clean, relative and slash-separated`},
		{"dot path", "keep . a.go", `path "." must be clean`},
		{"parent path", "delete ..", `path ".." must be clean`},
		{"escaping path", "delete ../a.go", `path "../a.go" must be clean`},
		{"unclean path", "delete a/../b.go", `path "a/../b.go" must be clean`},
		{"dot segment", "delete ./a.go", `path "./a.go" must be clean`},
		{"double slash", "delete a//b.go", `path "a//b.go" must be clean`},
		{"trailing slash", "keep a/ a.go", `path "a/" must be clean`},
		{"backslash", `delete a\b.go`, `path "a\\b.go" must be clean`},
		{"delete argument", "delete a.go b.go", "delete takes no arguments"},
		{"patch argument", "patch a.go blob=" + testBlob, "patch takes no arguments"},
		{"replace argument", "replace a.go b.go", `replace accepts a single blob=<40 hex digits> argument, got "b.go"`},
		{"short blob", "replace a.go blob=4033f06", `replace accepts a single blob=<40 hex digits> argument, got "blob=4033f06"`},
		{"uppercase blob", "replace a.go blob=" + strings.ToUpper(testBlob), `replace accepts a single blob=<40 hex digits> argument, got "blob=` + strings.ToUpper(testBlob) + `"`},
		{"empty blob", "replace a.go blob=", `replace accepts a single blob=<40 hex digits> argument, got "blob="`},
		{"two blobs", "replace a.go blob=" + testBlob + " blob=" + testBlob, `replace accepts a single blob=<40 hex digits> argument, got "blob=` + testBlob + `"`},
		{"keep without files", "keep a", "keep requires at least one file name"},
		{"keep nested file", "keep a b/c.go", `keep expects Go file names inside a, got "b/c.go"`},
		{"keep non-Go file", "keep a README.md", `keep expects Go file names inside a, got "README.md"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := parseManifest(strings.NewReader("delete ok.go\n" + tt.line + "\n"))
			require.ErrorContains(t, err, "manifest line 2: "+tt.err)
			require.Equal(t, []entry{{line: 1, op: opDelete, path: "ok.go"}}, entries)
		})
	}
}

func TestParseManifestDuplicatePath(t *testing.T) {
	entries, err := parseManifest(strings.NewReader(`delete a/a.go
patch a/b.go
replace a/a.go
keep a a.go
keep a b.go
`))
	require.EqualError(t, err, "manifest line 3: a/a.go is already handled on line 1\n"+
		"manifest line 5: a is already handled on line 4")
	require.Equal(t, []entry{
		{line: 1, op: opDelete, path: "a/a.go"},
		{line: 2, op: opPatch, path: "a/b.go"},
		{line: 4, op: opKeep, path: "a", keep: []string{"a.go"}},
	}, entries)
}

func TestParseManifestAggregatesErrors(t *testing.T) {
	entries, err := parseManifest(strings.NewReader(`move a.go
delete b.go
delete /c.go
keep d
delete b.go
replace e.go blob=x
`))
	require.EqualError(t, err, strings.Join([]string{
		`manifest line 1: unknown operation "move"`,
		`manifest line 3: path "/c.go" must be clean, relative and slash-separated`,
		`manifest line 4: keep requires at least one file name`,
		`manifest line 5: b.go is already handled on line 2`,
		`manifest line 6: replace accepts a single blob=<40 hex digits> argument, got "blob=x"`,
	}, "\n"))
	require.Equal(t, []entry{{line: 2, op: opDelete, path: "b.go"}}, entries)
}

func TestParseManifestInvalidEntryDoesNotReservePath(t *testing.T) {
	entries, err := parseManifest(strings.NewReader("delete a.go extra\ndelete a.go\n"))
	require.EqualError(t, err, "manifest line 1: delete takes no arguments")
	require.Equal(t, []entry{{line: 2, op: opDelete, path: "a.go"}}, entries)
}

func TestValidatePath(t *testing.T) {
	for _, p := range []string{"a", "a.go", "internal/a/b.go", "_files/a.go", "a..b/c.go", "..a/b.go"} {
		require.NoError(t, validatePath(p), p)
	}
	for _, p := range []string{"", ".", "..", "/", "/a", "./a", "a/.", "a/..", "../a", "a/../b", "a//b", "a/", `a\b`} {
		require.Error(t, validatePath(p), p)
	}
}
