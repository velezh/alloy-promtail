package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testUpstream = "package a\n\nfunc A() int {\n\treturn 1\n}\n"
	testPatched  = "package a\n\nfunc A() int {\n\treturn 2\n}\n"
	testPatch    = `diff --git a/a/a.go b/a/a.go
index 1111111..2222222 100644
--- a/a/a.go
+++ b/a/a.go
@@ -3,3 +3,3 @@ package a
 func A() int {
-	return 1
+	return 2
 }
`
)

// testConfig returns a config with repository, data and output directories in a temporary directory.
func testConfig(t *testing.T) config {
	dir := t.TempDir()
	return config{
		root:    filepath.Join(dir, "repo"),
		dataDir: filepath.Join(dir, "data"),
		outDir:  filepath.Join(dir, "out"),
	}
}

// writeFiles writes files keyed by slash-separated paths below dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

// mustParseManifest parses manifest and fails the test on error.
func mustParseManifest(t *testing.T, manifest string) []entry {
	t.Helper()
	entries, err := parseManifest(strings.NewReader(manifest))
	require.NoError(t, err)
	return entries
}

// repoPath returns the absolute path of the slash-separated path p below dir.
func repoPath(dir, p string) string {
	return filepath.Join(dir, filepath.FromSlash(p))
}

func TestGenerateDelete(t *testing.T) {
	cfg := testConfig(t)
	writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream, "a/b.go": "package a\n"})

	replace, err := generate(cfg, mustParseManifest(t, "delete a/a.go\n"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{repoPath(cfg.root, "a/a.go"): ""}, replace)
}

func TestGenerateReplace(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
	}{
		{"without pin", "replace a/a.go\n"},
		{"with matching pin", "replace a/a.go blob=" + gitBlobID([]byte(testUpstream)) + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream})
			writeFiles(t, cfg.dataDir, map[string]string{"_files/a/a.go": testPatched})

			replace, err := generate(cfg, mustParseManifest(t, tt.manifest))
			require.NoError(t, err)
			require.Equal(t, map[string]string{repoPath(cfg.root, "a/a.go"): repoPath(cfg.dataDir, "_files/a/a.go")}, replace)
		})
	}
}

func TestGenerateReplacePinMismatch(t *testing.T) {
	cfg := testConfig(t)
	writeFiles(t, cfg.root, map[string]string{"a/a.go": testPatched})
	writeFiles(t, cfg.dataDir, map[string]string{"_files/a/a.go": "package a\n"})
	pinned, current := gitBlobID([]byte(testUpstream)), gitBlobID([]byte(testPatched))

	replace, err := generate(cfg, mustParseManifest(t, "replace a/a.go blob="+pinned+"\n"))
	require.EqualError(t, err, "manifest line 1: replace a/a.go: upstream file changed since the replacement was derived from it: "+
		"review `git diff "+pinned+" "+current+"`, update "+repoPath(cfg.dataDir, "_files/a/a.go")+" and pin blob="+current)
	require.Empty(t, replace)
}

func TestGeneratePatch(t *testing.T) {
	cfg := testConfig(t)
	writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream})
	writeFiles(t, cfg.dataDir, map[string]string{"patches/a/a.go.patch": testPatch})

	replace, err := generate(cfg, mustParseManifest(t, "patch a/a.go\n"))
	require.NoError(t, err)
	dst := repoPath(cfg.outDir, "files/a/a.go")
	require.Equal(t, map[string]string{repoPath(cfg.root, "a/a.go"): dst}, replace)

	patched, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, testPatched, string(patched))

	upstream, err := os.ReadFile(repoPath(cfg.root, "a/a.go"))
	require.NoError(t, err)
	require.Equal(t, testUpstream, string(upstream))
}

func TestGenerateKeep(t *testing.T) {
	cfg := testConfig(t)
	writeFiles(t, cfg.root, map[string]string{
		"a/a.go":         "package a\n",
		"a/b.go":         "package a\n",
		"a/b_test.go":    "package a\n",
		"a/c_linux.go":   "package a\n",
		"a/d.go":         "package a\n",
		"a/README.md":    "a\n",
		"a/sub/sub.go":   "package sub\n",
		"a/dir.go/x.txt": "x\n",
	})

	replace, err := generate(cfg, mustParseManifest(t, "keep a a.go d.go\n"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		repoPath(cfg.root, "a/b.go"):       "",
		repoPath(cfg.root, "a/c_linux.go"): "",
	}, replace)
}

func TestGenerateKeepWithFileOperations(t *testing.T) {
	cfg := testConfig(t)
	writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream, "a/b.go": "package a\n", "a/c.go": "package a\n"})
	writeFiles(t, cfg.dataDir, map[string]string{"patches/a/a.go.patch": testPatch})

	replace, err := generate(cfg, mustParseManifest(t, "keep a a.go\npatch a/a.go\ndelete a/b.go\n"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		repoPath(cfg.root, "a/a.go"): repoPath(cfg.outDir, "files/a/a.go"),
		repoPath(cfg.root, "a/b.go"): "",
		repoPath(cfg.root, "a/c.go"): "",
	}, replace)
}

func TestGenerateDuplicateTarget(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		target   string
	}{
		{"replace of a dropped file", "keep a a.go\nreplace a/b.go\n", "a/b.go"},
		{"drop of a replaced file", "replace a/b.go\nkeep a a.go\n", "a/b.go"},
		{"patch of a dropped file", "keep a b.go\npatch a/a.go\n", "a/a.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream, "a/b.go": "package a\n"})
			writeFiles(t, cfg.dataDir, map[string]string{"_files/a/b.go": "package a\n", "patches/a/a.go.patch": testPatch})

			_, err := generate(cfg, mustParseManifest(t, tt.manifest))
			require.EqualError(t, err, "manifest line 2: "+repoPath(cfg.root, tt.target)+" is replaced twice")
		})
	}
}

func TestGenerateErrors(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		data     map[string]string
		err      func(cfg config) string
		notExist bool
	}{
		{
			name:     "delete of a missing file",
			manifest: "delete a/missing.go",
			err:      func(config) string { return "manifest line 1: delete a/missing.go: " },
			notExist: true,
		},
		{
			name:     "delete of a directory",
			manifest: "delete a",
			err: func(cfg config) string {
				return "manifest line 1: delete a: " + repoPath(cfg.root, "a") + " is not a regular file"
			},
		},
		{
			name:     "replace of a missing file",
			manifest: "replace a/missing.go",
			data:     map[string]string{"_files/a/missing.go": "package a\n"},
			err:      func(config) string { return "manifest line 1: replace a/missing.go: " },
			notExist: true,
		},
		{
			name:     "replace without replacement",
			manifest: "replace a/a.go",
			err: func(cfg config) string {
				return "manifest line 1: replace a/a.go: stat " + repoPath(cfg.dataDir, "_files/a/a.go")
			},
			notExist: true,
		},
		{
			name:     "replace with a directory",
			manifest: "replace a/a.go",
			data:     map[string]string{"_files/a/a.go/x.go": "package x\n"},
			err: func(cfg config) string {
				return "manifest line 1: replace a/a.go: " + repoPath(cfg.dataDir, "_files/a/a.go") + " is not a regular file"
			},
		},
		{
			name:     "patch of a missing file",
			manifest: "patch a/missing.go",
			data:     map[string]string{"patches/a/missing.go.patch": testPatch},
			err:      func(config) string { return "manifest line 1: patch a/missing.go: " },
			notExist: true,
		},
		{
			name:     "patch without patch file",
			manifest: "patch a/a.go",
			err: func(cfg config) string {
				return "manifest line 1: patch a/a.go: open " + repoPath(cfg.dataDir, "patches/a/a.go.patch")
			},
			notExist: true,
		},
		{
			name:     "invalid patch",
			manifest: "patch a/a.go",
			data:     map[string]string{"patches/a/a.go.patch": "--- a/a/a.go\n+++ b/a/a.go\n"},
			err: func(cfg config) string {
				return "manifest line 1: patch a/a.go: " + repoPath(cfg.dataDir, "patches/a/a.go.patch") + ": patch has no hunks"
			},
		},
		{
			name:     "patch of another file",
			manifest: "patch a/a.go",
			data:     map[string]string{"patches/a/a.go.patch": strings.ReplaceAll(testPatch, "a/a.go", "a/b.go")},
			err: func(cfg config) string {
				return "manifest line 1: patch a/a.go: " + repoPath(cfg.dataDir, "patches/a/a.go.patch") + " changes a/b.go"
			},
		},
		{
			name:     "outdated patch",
			manifest: "patch a/a.go",
			data:     map[string]string{"patches/a/a.go.patch": strings.Replace(testPatch, "-\treturn 1", "-\treturn 0", 1)},
			err: func(cfg config) string {
				return "manifest line 1: patch a/a.go: " + repoPath(cfg.dataDir, "patches/a/a.go.patch") +
					": hunk 1 (@@ -3) does not match the upstream file; refresh it with `make -C promtail patch-start FILE=a/a.go`"
			},
		},
		{
			name:     "keep of a missing directory",
			manifest: "keep missing a.go",
			err:      func(config) string { return "manifest line 1: keep missing: " },
			notExist: true,
		},
		{
			name:     "keep of a file",
			manifest: "keep a/a.go a.go",
			err:      func(config) string { return "manifest line 1: keep a/a.go: " },
		},
		{
			name:     "keep of missing files",
			manifest: "keep a z.go a.go b_test.go y.go",
			err: func(config) string {
				return "manifest line 1: keep a: kept files do not exist: a/b_test.go, a/y.go, a/z.go"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream, "a/b_test.go": "package a\n"})
			writeFiles(t, cfg.dataDir, tt.data)

			replace, err := generate(cfg, mustParseManifest(t, tt.manifest))
			require.ErrorContains(t, err, tt.err(cfg))
			if tt.notExist {
				require.ErrorIs(t, err, fs.ErrNotExist)
			}
			require.Empty(t, replace)
		})
	}
}

func TestGenerateAggregatesErrors(t *testing.T) {
	cfg := testConfig(t)
	writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream, "a/b.go": "package a\n"})

	replace, err := generate(cfg, mustParseManifest(t, "delete a/x.go\ndelete a/b.go\nreplace a/a.go\nkeep a y.go\n"))
	require.Error(t, err)
	errs := strings.Split(err.Error(), "\n")
	require.Len(t, errs, 3)
	for i, prefix := range []string{"manifest line 1: delete a/x.go: ", "manifest line 3: replace a/a.go: ", "manifest line 4: keep a: "} {
		require.True(t, strings.HasPrefix(errs[i], prefix), errs[i])
	}
	require.Equal(t, map[string]string{repoPath(cfg.root, "a/b.go"): ""}, replace)
}

func TestGitBlobID(t *testing.T) {
	require.Equal(t, "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391", gitBlobID(nil))
	require.Equal(t, "ce013625030ba8dba906f756967f9e9ca394464a", gitBlobID([]byte("hello\n")))
}

func TestGitBlobIDMatchesGit(t *testing.T) {
	requireGit(t)
	for _, data := range []string{
		"",
		"hello\n",
		"without trailing newline",
		"crlf\r\nline\r\n",
		"\x00\x01\xfe\xff binary",
		strings.Repeat("package a // long line\n", 50000),
	} {
		cmd := exec.Command("git", "hash-object", "--stdin")
		cmd.Stdin = strings.NewReader(data)
		out, err := cmd.Output()
		require.NoError(t, err)
		require.Equal(t, strings.TrimSpace(string(out)), gitBlobID([]byte(data)), "%.40q", data)
	}
}
