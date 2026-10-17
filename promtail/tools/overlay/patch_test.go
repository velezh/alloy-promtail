package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const gitHeader = `diff --git a/internal/x/x.go b/internal/x/x.go
index 1111111..2222222 100644
--- a/internal/x/x.go
+++ b/internal/x/x.go
`

func TestParsePatch(t *testing.T) {
	tests := []struct {
		name   string
		patch  string
		target string
		hunks  []hunk
	}{
		{
			name: "single hunk",
			patch: gitHeader + `@@ -2,3 +2,4 @@ func f() {
 a
-b
+B
+C
 c
`,
			target: "internal/x/x.go",
			hunks:  []hunk{{oldStart: 2, oldLines: []string{"a", "b", "c"}, newLines: []string{"a", "B", "C", "c"}}},
		},
		{
			name: "multiple hunks",
			patch: gitHeader + `@@ -1,3 +1,2 @@
 a
-b
 c
@@ -10,2 +9,3 @@ func g() {
 j
+k
 l
`,
			target: "internal/x/x.go",
			hunks: []hunk{
				{oldStart: 1, oldLines: []string{"a", "b", "c"}, newLines: []string{"a", "c"}},
				{oldStart: 10, oldLines: []string{"j", "l"}, newLines: []string{"j", "k", "l"}},
			},
		},
		{
			name: "ranges without counts",
			patch: gitHeader + `@@ -3 +3 @@
-x
+y
`,
			target: "internal/x/x.go",
			hunks:  []hunk{{oldStart: 3, oldLines: []string{"x"}, newLines: []string{"y"}}},
		},
		{
			name: "removal only",
			patch: gitHeader + `@@ -4,2 +3,0 @@
-x
-y
`,
			target: "internal/x/x.go",
			hunks:  []hunk{{oldStart: 4, oldLines: []string{"x", "y"}}},
		},
		{
			name:   "empty context lines",
			patch:  gitHeader + "@@ -1,5 +1,5 @@\n a\n \n\n-b\n+c\n \n",
			target: "internal/x/x.go",
			hunks: []hunk{{
				oldStart: 1,
				oldLines: []string{"a", "", "", "b", ""},
				newLines: []string{"a", "", "", "c", ""},
			}},
		},
		{
			name:   "stripped empty context line at the end",
			patch:  gitHeader + "@@ -1,2 +1,2 @@\n-a\n+b\n\n",
			target: "internal/x/x.go",
			hunks:  []hunk{{oldStart: 1, oldLines: []string{"a", ""}, newLines: []string{"b", ""}}},
		},
		{
			name:   "content resembling headers",
			patch:  gitHeader + "@@ -1,3 +1,3 @@\n --- a\n---- b\n+++++ c\n @@ d\n",
			target: "internal/x/x.go",
			hunks:  []hunk{{oldStart: 1, oldLines: []string{"--- a", "--- b", "@@ d"}, newLines: []string{"--- a", "++++ c", "@@ d"}}},
		},
		{
			name:   "plain unified diff without trailing newline",
			patch:  "--- x.go\n+++ x.go\n@@ -1 +1 @@\n-a\n+b",
			target: "x.go",
			hunks:  []hunk{{oldStart: 1, oldLines: []string{"a"}, newLines: []string{"b"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, hunks, err := parsePatch(tt.patch)
			require.NoError(t, err)
			require.Equal(t, tt.target, target)
			require.Equal(t, tt.hunks, hunks)
		})
	}
}

func TestParsePatchErrors(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		err   string
	}{
		{"empty", "", "patch has no hunks"},
		{"headers only", gitHeader, "patch has no hunks"},
		{"new file", "--- /dev/null\n+++ b/x.go\n@@ -0,0 +1 @@\n+a\n", "patch line 1: new files must be added with the replace operation"},
		{"deleted file", "--- a/x.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n", "patch line 2: deleted files must be removed with the delete operation"},
		{"hunk before file header", "@@ -1 +1 @@\n-a\n+b\n", "patch line 1: hunk before file header"},
		{"missing newline marker inside hunk", gitHeader + "@@ -1,2 +1,2 @@\n-a\n\\ No newline at end of file\n+b\n c\n", "patch line 7: missing trailing newline markers are not supported"},
		{"missing newline marker after hunk", gitHeader + "@@ -1,2 +1,2 @@\n-a\n+b\n c\n\\ No newline at end of file\n", "patch line 9: missing trailing newline markers are not supported"},
		{"second git file", gitHeader + "@@ -1 +1 @@\n-a\n+b\ndiff --git a/y.go b/y.go\n--- a/y.go\n+++ b/y.go\n@@ -1 +1 @@\n-a\n+b\n", "patch line 8: patches must change a single file"},
		{"second plain file", "--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n--- a/y.go\n+++ b/y.go\n@@ -1 +1 @@\n-a\n+b\n", "patch line 6: patches must change a single file"},
		{"hunk without context", gitHeader + "@@ -1,0 +2,2 @@\n+a\n+b\n", "hunk 1 has no context lines"},
		{"second hunk without context", gitHeader + "@@ -1 +1 @@\n-a\n+b\n@@ -5,0 +6 @@\n+c\n", "hunk 2 has no context lines"},
		{"malformed old range", gitHeader + "@@ -a,1 +1,1 @@\n a\n", `patch line 5: malformed hunk header "@@ -a,1 +1,1 @@": strconv.Atoi: parsing "a": invalid syntax`},
		{"malformed old count", gitHeader + "@@ -1,b +1,1 @@\n a\n", `patch line 5: malformed hunk header "@@ -1,b +1,1 @@": strconv.Atoi: parsing "b": invalid syntax`},
		{"malformed new range", gitHeader + "@@ -1,1 +c @@\n a\n", `patch line 5: malformed hunk header "@@ -1,1 +c @@": strconv.Atoi: parsing "c": invalid syntax`},
		{"unterminated header", gitHeader + "@@ -1 +1\n a\n", `patch line 5: malformed hunk header "@@ -1 +1"`},
		{"swapped ranges", gitHeader + "@@ +1 -1 @@\n a\n", `patch line 5: malformed hunk header "@@ +1 -1 @@"`},
		{"hunk longer than declared", gitHeader + "@@ -1,1 +1,2 @@\n a\n b\n", "patch line 7: hunk is longer than its header declares"},
		{"removal beyond declared", gitHeader + "@@ -1,1 +1,2 @@\n-a\n-b\n", "patch line 7: hunk is longer than its header declares"},
		{"line after hunk", gitHeader + "@@ -1 +1 @@\n-a\n+b\n c\n", `patch line 8: unexpected line after hunk: " c"`},
		{"line inside hunk", gitHeader + "@@ -1,3 +1,3 @@\n a\n@@ -5 +5 @@\n", `patch line 7: unexpected line inside hunk: "@@ -5 +5 @@"`},
		{"truncated hunk", gitHeader + "@@ -1,3 +1,3 @@\n a\n-b\n", "patch ends in the middle of a hunk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parsePatch(tt.patch)
			require.EqualError(t, err, tt.err)
		})
	}
}

func TestApplyHunks(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		hunks []hunk
		want  string
	}{
		{
			name:  "exact position",
			src:   "a\nb\nc\nd\n",
			hunks: []hunk{{oldStart: 2, oldLines: []string{"b", "c"}, newLines: []string{"b", "C", "c2"}}},
			want:  "a\nb\nC\nc2\nd\n",
		},
		{
			name:  "shifted position",
			src:   "new1\nnew2\nnew3\na\nb\nc\n",
			hunks: []hunk{{oldStart: 1, oldLines: []string{"a", "b"}, newLines: []string{"a", "B"}}},
			want:  "new1\nnew2\nnew3\na\nB\nc\n",
		},
		{
			name:  "position moved up",
			src:   "a\nb\nc\n",
			hunks: []hunk{{oldStart: 40, oldLines: []string{"c"}, newLines: []string{"C"}}},
			want:  "a\nb\nC\n",
		},
		{
			name: "matches before the previous hunk are ignored",
			src:  "x\na\nx\n",
			hunks: []hunk{
				{oldStart: 1, oldLines: []string{"x", "a"}, newLines: []string{"x", "A"}},
				{oldStart: 3, oldLines: []string{"x"}, newLines: []string{"Z"}},
			},
			want: "x\nA\nZ\n",
		},
		{
			name: "hunks do not match inserted lines",
			src:  "a\nb\nc\n",
			hunks: []hunk{
				{oldStart: 1, oldLines: []string{"a"}, newLines: []string{"b", "a"}},
				{oldStart: 1, oldLines: []string{"b"}, newLines: []string{"B"}},
			},
			want: "b\na\nB\nc\n",
		},
		{
			name: "multiple hunks",
			src:  "1\n2\n3\n4\n5\n6\n7\n8\n9\n",
			hunks: []hunk{
				{oldStart: 1, oldLines: []string{"1", "2"}, newLines: []string{"1"}},
				{oldStart: 5, oldLines: []string{"5"}, newLines: []string{"5", "5a", "5b"}},
				{oldStart: 8, oldLines: []string{"8", "9"}, newLines: []string{"8", "9", "10"}},
			},
			want: "1\n3\n4\n5\n5a\n5b\n6\n7\n8\n9\n10\n",
		},
		{
			name:  "removes lines",
			src:   "a\nb\nc\n",
			hunks: []hunk{{oldStart: 1, oldLines: []string{"a", "b"}, newLines: []string{"a"}}},
			want:  "a\nc\n",
		},
		{
			name:  "empty lines",
			src:   "a\n\nb\n\n",
			hunks: []hunk{{oldStart: 2, oldLines: []string{"", "b", ""}, newLines: []string{"", "B", ""}}},
			want:  "a\n\nB\n\n",
		},
		{
			name:  "without trailing newline",
			src:   "a\nb",
			hunks: []hunk{{oldStart: 2, oldLines: []string{"b"}, newLines: []string{"B", "c"}}},
			want:  "a\nB\nc",
		},
		{
			name:  "no hunks",
			src:   "a\nb\n",
			hunks: nil,
			want:  "a\nb\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyHunks(tt.src, tt.hunks)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestApplyHunksErrors(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		hunks []hunk
		err   string
	}{
		{
			name:  "mismatch",
			src:   "a\nb\nc\n",
			hunks: []hunk{{oldStart: 2, oldLines: []string{"b", "x"}, newLines: []string{"b"}}},
			err:   "hunk 1 (@@ -2) does not match the upstream file",
		},
		{
			name:  "whitespace mismatch",
			src:   "\ta\n",
			hunks: []hunk{{oldStart: 1, oldLines: []string{"    a"}, newLines: []string{"b"}}},
			err:   "hunk 1 (@@ -1) does not match the upstream file",
		},
		{
			name:  "longer than source",
			src:   "a\n",
			hunks: []hunk{{oldStart: 1, oldLines: []string{"a", ""}, newLines: []string{"b"}}},
			err:   "hunk 1 (@@ -1) does not match the upstream file",
		},
		{
			name: "match only before the previous hunk",
			src:  "x\na\nb\n",
			hunks: []hunk{
				{oldStart: 2, oldLines: []string{"a"}, newLines: []string{"A"}},
				{oldStart: 1, oldLines: []string{"x"}, newLines: []string{"X"}},
			},
			err: "hunk 2 (@@ -1) does not match the upstream file",
		},
		{
			name:  "ambiguous context",
			src:   "x\na\nx\n",
			hunks: []hunk{{oldStart: 3, oldLines: []string{"x"}, newLines: []string{"X"}}},
			err:   "hunk 1 (@@ -3) matches 2 locations of the upstream file, save the patch with a larger CONTEXT",
		},
		{
			name: "ambiguous context after upstream drift",
			src:  "i\ni\ni\ni\na\np\nq\nr\nx\ns\nx\n",
			hunks: []hunk{
				{oldStart: 1, oldLines: []string{"a"}, newLines: []string{"A"}},
				{oldStart: 6, oldLines: []string{"x"}, newLines: []string{"X"}},
			},
			err: "hunk 2 (@@ -6) matches 2 locations of the upstream file, save the patch with a larger CONTEXT",
		},
		{
			name: "second hunk mismatch",
			src:  "a\nb\nc\n",
			hunks: []hunk{
				{oldStart: 1, oldLines: []string{"a"}, newLines: []string{"A"}},
				{oldStart: 3, oldLines: []string{"d"}, newLines: []string{"D"}},
			},
			err: "hunk 2 (@@ -3) does not match the upstream file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyHunks(tt.src, tt.hunks)
			require.EqualError(t, err, tt.err)
		})
	}
}

// goSource returns a Go file with n numbered functions.
func goSource(n int) string {
	var sb strings.Builder
	sb.WriteString("package x\n\nimport (\n\t\"fmt\"\n\n\t\"strings\"\n)\n")
	for i := range n {
		fmt.Fprintf(&sb, "\n// F%d prints its number.\nfunc F%d() {\n\tfmt.Println(%d)\n\n\treturn\n}\n", i, i, i)
	}
	return sb.String()
}

func TestPatchGitRoundTrip(t *testing.T) {
	upstream := goSource(20)
	patched := strings.NewReplacer(
		"\n\t\"strings\"\n", "\n",
		"fmt.Println(3)\n", "fmt.Println(3)\n\tfmt.Println(\"three\")\n",
		"// F10 prints its number.\nfunc F10() {\n\tfmt.Println(10)\n\n\treturn\n}\n\n", "",
		"fmt.Println(19)", "fmt.Println(strings.Repeat(\"9\", 2))",
	).Replace(upstream)
	require.NotEqual(t, upstream, patched)

	for _, context := range []int{1, 3, 12} {
		t.Run(fmt.Sprintf("context %d", context), func(t *testing.T) {
			diff := gitDiff(t, map[string]string{"internal/x/x.go": upstream}, map[string]string{"internal/x/x.go": patched}, context)
			target, hunks, err := parsePatch(diff)
			require.NoError(t, err)
			require.Equal(t, "internal/x/x.go", target)

			got, err := applyHunks(upstream, hunks)
			require.NoError(t, err)
			require.Equal(t, patched, got)

			// Upstream inserted lines above the hunks after the patch was made.
			drift := strings.NewReplacer(
				"package x\n", "// Package x.\n//\n// Drifted.\npackage x\n",
				"func F7() {\n", "func F7() {\n\tfmt.Println(\"drift\")\n",
			).Replace
			got, err = applyHunks(drift(upstream), hunks)
			require.NoError(t, err)
			require.Equal(t, drift(patched), got)
		})
	}
}

func TestParsePatchGitRejects(t *testing.T) {
	tests := []struct {
		name    string
		before  map[string]string
		after   map[string]string
		context int
		err     string
	}{
		{
			name:    "missing newline at end of unchanged last line",
			before:  map[string]string{"x.go": "a\nb\nc"},
			after:   map[string]string{"x.go": "A\nb\nc"},
			context: 3,
			err:     "missing trailing newline markers are not supported",
		},
		{
			name:    "missing newline at end of added line",
			before:  map[string]string{"x.go": "a\nb\n"},
			after:   map[string]string{"x.go": "a\nb\nc"},
			context: 3,
			err:     "missing trailing newline markers are not supported",
		},
		{
			name:    "multiple files",
			before:  map[string]string{"x.go": "a\n", "y.go": "a\n"},
			after:   map[string]string{"x.go": "b\n", "y.go": "b\n"},
			context: 3,
			err:     "patches must change a single file",
		},
		{
			name:    "insertion without context",
			before:  map[string]string{"x.go": "a\nb\n"},
			after:   map[string]string{"x.go": "a\nnew\nb\n"},
			context: 0,
			err:     "hunk 1 has no context lines",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parsePatch(gitDiff(t, tt.before, tt.after, tt.context))
			require.ErrorContains(t, err, tt.err)
		})
	}
}

// gitDiff stages the before files in a new repository, writes the after files
// and returns "git diff" of the working tree with the given number of context lines.
func gitDiff(t *testing.T, before, after map[string]string, context int) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.Output()
		require.NoError(t, err, "git %s", strings.Join(args, " "))
		return string(out)
	}

	git("init", "-q")
	writeFiles(t, dir, before)
	git("add", ".")
	writeFiles(t, dir, after)
	return git("diff", "--no-color", fmt.Sprintf("-U%d", context))
}

// requireGit skips the test when git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}
