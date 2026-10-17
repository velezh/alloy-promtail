package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// readOverlay returns the parsed overlay.json of outDir.
func readOverlay(t *testing.T, outDir string) map[string]map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(outDir, "overlay.json"))
	require.NoError(t, err)
	var overlay map[string]map[string]string
	require.NoError(t, json.Unmarshal(data, &overlay))
	return overlay
}

func TestRun(t *testing.T) {
	cfg := testConfig(t)
	cfg.outDir = filepath.Join(cfg.outDir, "nested", "overlay")
	writeFiles(t, cfg.root, map[string]string{
		"a/a.go":    testUpstream,
		"b/b.go":    "package b\n",
		"c/c.go":    "package c\n",
		"d/keep.go": "package d\n",
		"d/drop.go": "package d\n",
	})
	writeFiles(t, cfg.dataDir, map[string]string{
		"manifest.txt": `# Test manifest.
patch a/a.go
replace b/b.go blob=` + gitBlobID([]byte("package b\n")) + `
delete c/c.go
keep d keep.go
`,
		"_files/b/b.go":        "package b // replaced\n",
		"patches/a/a.go.patch": testPatch,
	})
	writeFiles(t, cfg.outDir, map[string]string{"files/stale/stale.go": "package stale\n"})

	require.NoError(t, run(cfg.root, cfg.dataDir, cfg.outDir))

	require.Equal(t, map[string]map[string]string{"Replace": {
		repoPath(cfg.root, "a/a.go"):    repoPath(cfg.outDir, "files/a/a.go"),
		repoPath(cfg.root, "b/b.go"):    repoPath(cfg.dataDir, "_files/b/b.go"),
		repoPath(cfg.root, "c/c.go"):    "",
		repoPath(cfg.root, "d/drop.go"): "",
	}}, readOverlay(t, cfg.outDir))

	patched, err := os.ReadFile(repoPath(cfg.outDir, "files/a/a.go"))
	require.NoError(t, err)
	require.Equal(t, testPatched, string(patched))
	require.NoDirExists(t, repoPath(cfg.outDir, "files/stale"))
}

func TestRunRelativePaths(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFiles(t, ".", map[string]string{
		"repo/a/a.go":       testUpstream,
		"data/manifest.txt": "delete a/a.go\n",
	})

	require.NoError(t, run("repo", "data", "out"))

	root, err := filepath.Abs("repo")
	require.NoError(t, err)
	require.Equal(t, map[string]map[string]string{"Replace": {
		filepath.Join(root, "a", "a.go"): "",
	}}, readOverlay(t, "out"))
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		err      string
	}{
		{"invalid manifest", "drop a/a.go\n", `manifest line 1: unknown operation "drop"`},
		{"failed entry", "delete a/missing.go\n", "manifest line 1: delete a/missing.go: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			writeFiles(t, cfg.root, map[string]string{"a/a.go": testUpstream})
			writeFiles(t, cfg.dataDir, map[string]string{"manifest.txt": tt.manifest})

			err := run(cfg.root, cfg.dataDir, cfg.outDir)
			require.ErrorContains(t, err, tt.err)
			require.NoFileExists(t, filepath.Join(cfg.outDir, "overlay.json"))
		})
	}

	t.Run("missing manifest", func(t *testing.T) {
		cfg := testConfig(t)
		err := run(cfg.root, cfg.dataDir, cfg.outDir)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

// TestRunRealManifest fails once an upstream change breaks a patch, a blob pin
// or a kept file of the alloy-promtail overlay.
func TestRunRealManifest(t *testing.T) {
	const (
		root    = "../../.."
		dataDir = "../../overlay"
	)
	outDir := t.TempDir()
	require.NoError(t, run(root, dataDir, outDir))

	overlay := readOverlay(t, outDir)
	require.NotEmpty(t, overlay["Replace"])
	for upstream, src := range overlay["Replace"] {
		require.FileExists(t, upstream)
		if src != "" {
			require.FileExists(t, src)
		}
	}
}
