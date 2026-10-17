package main

import (
	"bytes"
	"go/build/constraint"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatches(t *testing.T) {
	tests := []struct {
		pattern string
		pkg     string
		want    bool
	}{
		{"=foo/bar", "foo/bar", true},
		{"=foo/bar", "foo/bar/baz", false},
		{"=foo/bar", "foo/barbaz", false},
		{"=foo/bar", "foo", false},

		{"foo/bar/", "foo/bar", false},
		{"foo/bar/", "foo/bar/baz", true},
		{"foo/bar/", "foo/bar/baz/qux", true},
		{"foo/bar/", "foo/barbaz", false},

		{"foo/bar", "foo/bar", true},
		{"foo/bar", "foo/bar/baz", true},
		{"foo/bar", "foo/bar/baz/qux", true},
		{"foo/bar", "foo/barbaz", false},
		{"foo/bar", "foo/barbaz/qux", false},
		{"foo/bar", "foo", false},
		{"foo/bar", "x/foo/bar", false},

		{"github.com/Azure", "github.com/Azure/azure-sdk-for-go/sdk/azcore", true},
		{"github.com/Azure", "github.com/AzureAD/microsoft-authentication-library-for-go", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, matches(tt.pkg, tt.pattern), "pattern %q, package %q", tt.pattern, tt.pkg)
	}
}

func TestParseRules(t *testing.T) {
	rules, err := parseRules(strings.NewReader(`# Header comment.

foo/bar
=foo/exact   # trailing comment
	foo/sub/ unless k8s
foo/multi unless k8s cloud
`))
	require.NoError(t, err)
	require.Equal(t, []rule{
		{line: 3, pattern: "foo/bar"},
		{line: 4, pattern: "=foo/exact"},
		{line: 5, pattern: "foo/sub/", unless: []string{"k8s"}},
		{line: 6, pattern: "foo/multi", unless: []string{"k8s", "cloud"}},
	}, rules)
}

func TestParseRulesErrors(t *testing.T) {
	rules, err := parseRules(strings.NewReader(`foo/ok
foo/two fields
foo/unless unless
foo/three if k8s
foo/ok2 unless cloud
`))
	const format = `expected "<pattern> [unless <group>...]"`
	require.EqualError(t, err, strings.Join([]string{
		"denylist line 2: " + format,
		"denylist line 3: " + format,
		"denylist line 4: " + format,
	}, "\n"))
	require.Equal(t, []rule{
		{line: 1, pattern: "foo/ok"},
		{line: 5, pattern: "foo/ok2", unless: []string{"cloud"}},
	}, rules)
}

func TestParseGraph(t *testing.T) {
	graph, modules, err := parseGraph(strings.NewReader("main @example.com a b\na @example.com/a  c\n\nc\nd @example.com/d\n"))
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"main": {"a", "b"},
		"a":    {"c"},
		"c":    {},
		"d":    {},
	}, graph)
	require.Equal(t, map[string]string{
		"main": "example.com",
		"a":    "example.com/a",
		"d":    "example.com/d",
	}, modules)
}

func TestParseGraphLongLine(t *testing.T) {
	imports := make([]string, 20000)
	for i := range imports {
		imports[i] = "example.com/some/long/import/path"
	}
	graph, _, err := parseGraph(strings.NewReader("main " + strings.Join(imports, " ") + "\n"))
	require.NoError(t, err)
	require.Len(t, graph["main"], len(imports))
}

func TestCheck(t *testing.T) {
	graph := map[string][]string{
		"main":        {"x/a", "y/b", "y/a/c"},
		"x/a":         {},
		"x/ab":        {},
		"y":           {},
		"y/b":         {},
		"y/a/c":       {},
		"k8s.io/api":  {},
		"cloud/sdk/s": {},
	}
	rules := []rule{
		{line: 1, pattern: "unused"},
		{line: 2, pattern: "y/"},
		{line: 3, pattern: "=x/a"},
		{line: 4, pattern: "k8s.io/api", unless: []string{"k8s"}},
		{line: 5, pattern: "cloud/sdk", unless: []string{"k8s", "cloud"}},
	}

	tests := []struct {
		name   string
		groups []string
		want   []violation
	}{
		{
			name:   "no groups",
			groups: nil,
			want: []violation{
				{rule: rules[1], pkgs: []string{"y/a/c", "y/b"}},
				{rule: rules[2], pkgs: []string{"x/a"}},
				{rule: rules[3], pkgs: []string{"k8s.io/api"}},
				{rule: rules[4], pkgs: []string{"cloud/sdk/s"}},
			},
		},
		{
			name:   "k8s group",
			groups: []string{"k8s"},
			want: []violation{
				{rule: rules[1], pkgs: []string{"y/a/c", "y/b"}},
				{rule: rules[2], pkgs: []string{"x/a"}},
			},
		},
		{
			name:   "cloud group",
			groups: []string{"cloud"},
			want: []violation{
				{rule: rules[1], pkgs: []string{"y/a/c", "y/b"}},
				{rule: rules[2], pkgs: []string{"x/a"}},
				{rule: rules[3], pkgs: []string{"k8s.io/api"}},
			},
		},
		{
			name:   "unrelated group",
			groups: []string{"secretfilter"},
			want: []violation{
				{rule: rules[1], pkgs: []string{"y/a/c", "y/b"}},
				{rule: rules[2], pkgs: []string{"x/a"}},
				{rule: rules[3], pkgs: []string{"k8s.io/api"}},
				{rule: rules[4], pkgs: []string{"cloud/sdk/s"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, check(graph, rules, tt.groups))
		})
	}

	require.Empty(t, check(map[string][]string{"main": {}}, rules, nil))
}

func TestParseAllowlist(t *testing.T) {
	list, err := parseAllowlist(strings.NewReader(`# Test allowlist.
example.com/a
  example.com/b if k8s  # comment
example.com/c if k8s cloud
`))
	require.NoError(t, err)
	require.Equal(t, []allowed{
		{line: 2, module: "example.com/a"},
		{line: 3, module: "example.com/b", groups: []string{"k8s"}},
		{line: 4, module: "example.com/c", groups: []string{"k8s", "cloud"}},
	}, list)

	_, err = parseAllowlist(strings.NewReader("example.com/a if\nexample.com/b unless k8s\nexample.com/c\n"))
	require.EqualError(t, err, `module allowlist line 1: expected "<module> [if <group>...]"`+"\n"+
		`module allowlist line 2: expected "<module> [if <group>...]"`)
}

func TestCheckModules(t *testing.T) {
	modules := map[string]string{
		"main":     "example.com",
		"k/a":      "example.com/k",
		"k/b":      "example.com/k",
		"c/a":      "example.com/c",
		"extra/a":  "example.com/extra",
		"extra/b":  "example.com/extra",
		"other/a":  "example.com/other",
		"second/a": "example.com/second",
	}
	list := []allowed{
		{module: "example.com"},
		{module: "example.com/k", groups: []string{"k8s"}},
		{module: "example.com/c", groups: []string{"k8s", "cloud"}},
		{module: "example.com/other"},
	}

	require.Equal(t, []unlistedModule{
		{module: "example.com/c", pkgs: []string{"c/a"}},
		{module: "example.com/extra", pkgs: []string{"extra/a", "extra/b"}},
		{module: "example.com/k", pkgs: []string{"k/a", "k/b"}},
		{module: "example.com/second", pkgs: []string{"second/a"}},
	}, checkModules(modules, list, nil))

	require.Equal(t, []unlistedModule{
		{module: "example.com/extra", pkgs: []string{"extra/a", "extra/b"}},
		{module: "example.com/second", pkgs: []string{"second/a"}},
	}, checkModules(modules, list, []string{"k8s"}))

	require.Equal(t, []unlistedModule{
		{module: "example.com/extra", pkgs: []string{"extra/a", "extra/b"}},
		{module: "example.com/k", pkgs: []string{"k/a", "k/b"}},
		{module: "example.com/second", pkgs: []string{"second/a"}},
	}, checkModules(modules, list, []string{"cloud"}))

	require.Empty(t, checkModules(map[string]string{"main": "example.com"}, list, nil))
}

func TestImportChain(t *testing.T) {
	graph := map[string][]string{
		"main":   {"a", "b"},
		"a":      {"c"},
		"b":      {"d", "target"},
		"c":      {"a", "target"},
		"d":      {"main"},
		"target": {"leaf"},
		"leaf":   {},
		"orphan": {"target"},
	}
	tests := []struct {
		pkg  string
		want []string
	}{
		{"main", []string{"main"}},
		{"a", []string{"main", "a"}},
		{"target", []string{"main", "b", "target"}},
		{"leaf", []string{"main", "b", "target", "leaf"}},
		{"orphan", []string{"orphan"}},
		{"unknown", []string{"unknown"}},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, importChain(graph, "main", tt.pkg), tt.pkg)
	}

	require.Equal(t, []string{"main", "a", "c"}, importChain(map[string][]string{
		"main": {"a", "b"},
		"a":    {"c"},
		"b":    {"c"},
	}, "main", "c"), "ties resolve in import order")
}

// writeFile writes content to a new file in a temporary directory and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// testOptions returns options for a test graph rooted at example.com/main.
func testOptions(t *testing.T, denylist, modules string, groups ...string) options {
	t.Helper()
	return options{
		denylist: writeFile(t, "denylist.txt", denylist),
		modules:  writeFile(t, "modules.txt", modules),
		groups:   groups,
		root:     "example.com/main",
	}
}

const testGraph = `example.com/main @example.com example.com/a fmt
example.com/a @example.com example.com/bad/x fmt
example.com/bad/x @example.com example.com/bad/y
example.com/bad/y @example.com
example.com/kube @example.com k8s.io/api
k8s.io/api @k8s.io/api
fmt
`

const testModules = `example.com
k8s.io/api if k8s
`

func TestRun(t *testing.T) {
	const denylist = `# Test denylist.
example.com/bad
=fmt
example.com/kube unless k8s
example.com/unused
`

	var out bytes.Buffer
	opts := testOptions(t, denylist, testModules)
	err := run(strings.NewReader(testGraph), &out, opts)
	require.EqualError(t, err, "3 denylist rules are violated, 1 linked modules are not allowlisted")
	require.Equal(t, `denylist line 2 "example.com/bad" matches 2 linked packages, example.com/bad/x is linked via:
  example.com/main
  -> example.com/a
  -> example.com/bad/x
denylist line 3 "=fmt" matches 1 linked packages, fmt is linked via:
  example.com/main
  -> fmt
denylist line 4 "example.com/kube" matches 1 linked packages, example.com/kube is linked via:
  example.com/kube
module k8s.io/api is not in `+opts.modules+`, k8s.io/api is linked via:
  k8s.io/api
`, out.String())

	out.Reset()
	err = run(strings.NewReader(testGraph), &out, testOptions(t, denylist, testModules, "k8s"))
	require.EqualError(t, err, "2 denylist rules are violated, 0 linked modules are not allowlisted")
	require.NotContains(t, out.String(), "example.com/kube")
	require.NotContains(t, out.String(), "module k8s.io/api")
}

func TestRunUnlistedModule(t *testing.T) {
	graph := testGraph + "example.com/a/new @example.com/new\n"
	graph = strings.Replace(graph, "example.com/a @example.com example.com/bad/x fmt", "example.com/a @example.com example.com/bad/x fmt example.com/a/new", 1)

	var out bytes.Buffer
	opts := testOptions(t, "example.com/unused\n", testModules, "k8s")
	err := run(strings.NewReader(graph), &out, opts)
	require.EqualError(t, err, "0 denylist rules are violated, 1 linked modules are not allowlisted")
	require.Equal(t, `module example.com/new is not in `+opts.modules+`, example.com/a/new is linked via:
  example.com/main
  -> example.com/a
  -> example.com/a/new
`, out.String())
}

func TestRunWithoutViolations(t *testing.T) {
	var out bytes.Buffer
	opts := testOptions(t, "example.com/unused\nexample.com/kube unless k8s\n", testModules, "k8s", "cloud")
	require.NoError(t, run(strings.NewReader(testGraph), &out, opts))
	require.Empty(t, out.String())
}

func TestRunErrors(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		opts := testOptions(t, "example.com/bad\n", testModules)
		opts.root = "example.com/other"
		var out bytes.Buffer
		err := run(strings.NewReader(testGraph), &out, opts)
		require.EqualError(t, err, `main package "example.com/other" is not in the dependency graph`)
		require.Empty(t, out.String())
	})

	t.Run("empty graph", func(t *testing.T) {
		err := run(strings.NewReader(""), &bytes.Buffer{}, testOptions(t, "example.com/bad\n", testModules))
		require.EqualError(t, err, `main package "example.com/main" is not in the dependency graph`)
	})

	t.Run("malformed denylist", func(t *testing.T) {
		err := run(strings.NewReader(testGraph), &bytes.Buffer{}, testOptions(t, "example.com/bad if k8s\n", testModules))
		require.EqualError(t, err, `denylist line 1: expected "<pattern> [unless <group>...]"`)
	})

	t.Run("malformed module allowlist", func(t *testing.T) {
		err := run(strings.NewReader(testGraph), &bytes.Buffer{}, testOptions(t, "example.com/bad\n", "example.com unless k8s\n"))
		require.EqualError(t, err, `module allowlist line 1: expected "<module> [if <group>...]"`)
	})

	t.Run("missing denylist", func(t *testing.T) {
		opts := testOptions(t, "", testModules)
		opts.denylist = filepath.Join(t.TempDir(), "missing.txt")
		err := run(strings.NewReader(testGraph), &bytes.Buffer{}, opts)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("missing module allowlist", func(t *testing.T) {
		opts := testOptions(t, "", testModules)
		opts.modules = filepath.Join(t.TempDir(), "missing.txt")
		err := run(strings.NewReader(testGraph), &bytes.Buffer{}, opts)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestRealDenylist(t *testing.T) {
	f, err := os.Open("../../denylist.txt")
	require.NoError(t, err)
	defer f.Close()
	rules, err := parseRules(f)
	require.NoError(t, err)
	require.NotEmpty(t, rules)

	groups := componentGroups(t)
	patterns := map[string]int{}
	for _, r := range rules {
		prev, ok := patterns[r.pattern]
		require.False(t, ok, "denylist line %d repeats line %d", r.line, prev)
		patterns[r.pattern] = r.line
		for _, g := range r.unless {
			require.True(t, groups[g], "denylist line %d names unknown component group %q", r.line, g)
		}
	}
}

func TestRealModuleAllowlist(t *testing.T) {
	f, err := os.Open("../../modules.txt")
	require.NoError(t, err)
	defer f.Close()
	list, err := parseAllowlist(f)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	groups := componentGroups(t)
	modules := map[string]int{}
	for _, a := range list {
		prev, ok := modules[a.module]
		require.False(t, ok, "module allowlist line %d repeats line %d", a.line, prev)
		modules[a.module] = a.line
		for _, g := range a.groups {
			require.True(t, groups[g], "module allowlist line %d names unknown component group %q", a.line, g)
		}
	}
}

// componentGroups returns the groups of the promtail_<group> build tags in promtail/components.
func componentGroups(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../../components/*.go")
	require.NoError(t, err)

	groups := map[string]bool{}
	for _, name := range files {
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		for _, line := range strings.Split(string(data), "\n") {
			if !constraint.IsGoBuild(line) {
				continue
			}
			expr, err := constraint.Parse(line)
			require.NoError(t, err, name)
			// Eval visits every tag of the expression regardless of the result.
			expr.Eval(func(tag string) bool {
				if g, ok := strings.CutPrefix(tag, "promtail_"); ok {
					groups[g] = true
				}
				return true
			})
		}
	}
	require.NotEmpty(t, groups)
	return groups
}
