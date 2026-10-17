// Command depcheck fails when a denylisted package or a module missing from the
// module allowlist is linked into alloy-promtail.
//
// It reads `go list -deps -f '{{.ImportPath}}{{with .Module}} @{{.Path}}{{end}}{{range .Imports}} {{.}}{{end}}'`
// output from stdin and prints the shortest import chain of every violation.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() {
	var (
		denylist = flag.String("denylist", "promtail/denylist.txt", "Denylist path.")
		modules  = flag.String("modules", "promtail/modules.txt", "Module allowlist path.")
		groups   = flag.String("groups", "", "Space separated component groups enabled in the build.")
		root     = flag.String("root", "", "Import path of the main package.")
	)
	flag.Parse()

	opts := options{denylist: *denylist, modules: *modules, groups: strings.Fields(*groups), root: *root}
	if err := run(os.Stdin, os.Stdout, opts); err != nil {
		fmt.Fprintf(os.Stderr, "depcheck: %v\n", err)
		os.Exit(1)
	}
}

// options configures a dependency check.
type options struct {
	denylist string
	modules  string
	groups   []string
	root     string
}

// run checks the dependency graph read from in against the denylist and the
// module allowlist and reports violations to out.
func run(in io.Reader, out io.Writer, opts options) error {
	rules, err := parseFile(opts.denylist, parseRules)
	if err != nil {
		return err
	}
	allowed, err := parseFile(opts.modules, parseAllowlist)
	if err != nil {
		return err
	}

	graph, modules, err := parseGraph(in)
	if err != nil {
		return err
	}
	if _, ok := graph[opts.root]; !ok {
		return fmt.Errorf("main package %q is not in the dependency graph", opts.root)
	}

	chain := func(pkg string) string {
		return strings.Join(importChain(graph, opts.root, pkg), "\n  -> ")
	}
	violations := check(graph, rules, opts.groups)
	for _, v := range violations {
		fmt.Fprintf(out, "denylist line %d %q matches %d linked packages, %s is linked via:\n  %s\n",
			v.rule.line, v.rule.pattern, len(v.pkgs), v.pkgs[0], chain(v.pkgs[0]))
	}
	unlisted := checkModules(modules, allowed, opts.groups)
	for _, m := range unlisted {
		fmt.Fprintf(out, "module %s is not in %s, %s is linked via:\n  %s\n", m.module, opts.modules, m.pkgs[0], chain(m.pkgs[0]))
	}
	if len(violations) > 0 || len(unlisted) > 0 {
		return fmt.Errorf("%d denylist rules are violated, %d linked modules are not allowlisted", len(violations), len(unlisted))
	}
	return nil
}

// parseFile opens path and parses it with parse.
func parseFile[T any](path string, parse func(io.Reader) ([]T, error)) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parse(f)
}

// rule denies packages matching pattern unless one of the groups is enabled.
type rule struct {
	line    int
	pattern string
	unless  []string
}

// parseRules parses denylist lines of the form "<pattern> [unless <group>...]".
func parseRules(r io.Reader) ([]rule, error) {
	var (
		rules []rule
		errs  []error
	)
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) == 0:
			continue
		case len(fields) == 1:
			rules = append(rules, rule{line: n, pattern: fields[0]})
		case len(fields) > 2 && fields[1] == "unless":
			rules = append(rules, rule{line: n, pattern: fields[0], unless: fields[2:]})
		default:
			errs = append(errs, fmt.Errorf("denylist line %d: expected \"<pattern> [unless <group>...]\"", n))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return rules, errors.Join(errs...)
}

// allowed permits linking module when it has no groups or one of its groups is enabled.
type allowed struct {
	line   int
	module string
	groups []string
}

// parseAllowlist parses module allowlist lines of the form "<module> [if <group>...]".
func parseAllowlist(r io.Reader) ([]allowed, error) {
	var (
		list []allowed
		errs []error
	)
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) == 0:
			continue
		case len(fields) == 1:
			list = append(list, allowed{line: n, module: fields[0]})
		case len(fields) > 2 && fields[1] == "if":
			list = append(list, allowed{line: n, module: fields[0], groups: fields[2:]})
		default:
			errs = append(errs, fmt.Errorf("module allowlist line %d: expected \"<module> [if <group>...]\"", n))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return list, errors.Join(errs...)
}

// parseGraph parses "<package> [@<module>] <import>..." lines.
//
// Returns the import graph and the module of every package that belongs to one.
func parseGraph(r io.Reader) (map[string][]string, map[string]string, error) {
	var (
		graph   = map[string][]string{}
		modules = map[string]string{}
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		pkg, imports := fields[0], fields[1:]
		if len(imports) > 0 && strings.HasPrefix(imports[0], "@") {
			modules[pkg] = imports[0][1:]
			imports = imports[1:]
		}
		graph[pkg] = imports
	}
	return graph, modules, sc.Err()
}

// violation is a denylist rule with the linked packages it matches.
type violation struct {
	rule rule
	pkgs []string
}

// check returns the rules violated by graph for the enabled groups in denylist order.
//
// Matched packages of each violation are sorted.
func check(graph map[string][]string, rules []rule, groups []string) []violation {
	enabled := map[string]bool{}
	for _, g := range groups {
		enabled[g] = true
	}

	var violations []violation
	for _, r := range rules {
		if anyEnabled(r.unless, enabled) {
			continue
		}
		var pkgs []string
		for pkg := range graph {
			if matches(pkg, r.pattern) {
				pkgs = append(pkgs, pkg)
			}
		}
		if len(pkgs) > 0 {
			sort.Strings(pkgs)
			violations = append(violations, violation{rule: r, pkgs: pkgs})
		}
	}
	return violations
}

// unlistedModule is a linked module missing from the allowlist with its linked packages.
type unlistedModule struct {
	module string
	pkgs   []string
}

// checkModules returns the linked modules not allowed for the enabled groups, sorted by module.
//
// Packages of each module are sorted.
func checkModules(modules map[string]string, list []allowed, groups []string) []unlistedModule {
	enabled := map[string]bool{}
	for _, g := range groups {
		enabled[g] = true
	}
	permitted := map[string]bool{}
	for _, a := range list {
		if len(a.groups) == 0 || anyEnabled(a.groups, enabled) {
			permitted[a.module] = true
		}
	}

	byModule := map[string][]string{}
	for pkg, module := range modules {
		if !permitted[module] {
			byModule[module] = append(byModule[module], pkg)
		}
	}
	unlisted := make([]unlistedModule, 0, len(byModule))
	for module, pkgs := range byModule {
		sort.Strings(pkgs)
		unlisted = append(unlisted, unlistedModule{module: module, pkgs: pkgs})
	}
	sort.Slice(unlisted, func(i, j int) bool { return unlisted[i].module < unlisted[j].module })
	return unlisted
}

// matches reports whether pkg matches the denylist pattern.
//
// "=path" matches path only, "path/" matches packages below path,
// "path" matches path and packages below it.
func matches(pkg, pattern string) bool {
	if exact, ok := strings.CutPrefix(pattern, "="); ok {
		return pkg == exact
	}
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(pkg, pattern)
	}
	return pkg == pattern || strings.HasPrefix(pkg, pattern+"/")
}

func anyEnabled(groups []string, enabled map[string]bool) bool {
	for _, g := range groups {
		if enabled[g] {
			return true
		}
	}
	return false
}

// importChain returns the shortest import path from root to pkg.
func importChain(graph map[string][]string, root, pkg string) []string {
	prev := map[string]string{root: ""}
	queue := []string{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == pkg {
			break
		}
		for _, next := range graph[cur] {
			if _, seen := prev[next]; !seen {
				prev[next] = cur
				queue = append(queue, next)
			}
		}
	}

	if _, ok := prev[pkg]; !ok {
		return []string{pkg}
	}
	var chain []string
	for p := pkg; p != ""; p = prev[p] {
		chain = append(chain, p)
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
