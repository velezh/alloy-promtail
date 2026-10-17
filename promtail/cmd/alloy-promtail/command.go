package main

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/grafana/alloy/internal/alloycli"
	"github.com/grafana/alloy/internal/build"
)

const programName = "alloy-promtail"

// removedCommands are upstream subcommands whose functionality is excluded by the overlay.
var removedCommands = []string{"convert", "gql", "tools"}

// commandChanges adjusts the flags of an upstream subcommand.
//
// Unsupported flags are hidden and rejected when set to a non-default value.
type commandChanges struct {
	name        string
	hidden      []string
	unsupported []string
	defaults    map[string]string
}

var changes = []commandChanges{
	{
		name: "run",
		hidden: []string{
			"cluster.discover-peers",
			"config.bypass-conversion-errors",
			"config.extra-args",
			"config.format",
			"feature.community-components.enabled",
			"feature.prometheus.direct-fanout.enabled",
			"server.http.ui-path-prefix",
		},
		unsupported: []string{
			"server.http.enable-graphql",
			"server.http.enable-graphql-playground",
		},
		defaults: map[string]string{
			"disable-reporting":                  "true",
			"server.http.disable-support-bundle": "true",
			"server.http.enable-pprof":           "false",
		},
	},
	{
		name: "validate",
		hidden: []string{
			"config.bypass-conversion-errors",
			"config.extra-args",
			"config.format",
			"feature.community-components.enabled",
		},
	},
}

// newCommand returns the upstream Alloy CLI adjusted for alloy-promtail.
//
// Returns an error when the upstream CLI no longer provides an adjusted command or flag.
func newCommand() (*cobra.Command, error) {
	root := alloycli.Command()
	root.Short = "Grafana Alloy distribution for log collection"
	root.Version = build.Print(programName)

	for _, name := range removedCommands {
		cmd, err := subcommand(root, name)
		if err != nil {
			return nil, err
		}
		root.RemoveCommand(cmd)
	}

	for _, ch := range changes {
		cmd, err := subcommand(root, ch.name)
		if err != nil {
			return nil, err
		}
		for _, name := range slices.Concat(ch.hidden, ch.unsupported) {
			if err := cmd.Flags().MarkHidden(name); err != nil {
				return nil, fmt.Errorf("%s: %w", ch.name, err)
			}
		}
		if len(ch.unsupported) > 0 {
			rejectUnsupported(cmd, ch.unsupported)
		}
		for name, value := range ch.defaults {
			if err := setDefault(cmd, name, value); err != nil {
				return nil, fmt.Errorf("%s: %w", ch.name, err)
			}
		}
	}
	return root, nil
}

// subcommand returns the direct subcommand of root with the given name.
func subcommand(root *cobra.Command, name string) (*cobra.Command, error) {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd, nil
		}
	}
	return nil, fmt.Errorf("upstream CLI has no %q command", name)
}

// rejectUnsupported makes cmd fail before running when one of the named flags is set to a non-default value.
func rejectUnsupported(cmd *cobra.Command, names []string) {
	preRun := cmd.PreRunE
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		for _, name := range names {
			if f := cmd.Flags().Lookup(name); f.Changed && f.Value.String() != f.DefValue {
				return fmt.Errorf("--%s is not supported by %s", name, programName)
			}
		}
		if preRun != nil {
			return preRun(cmd, args)
		}
		return nil
	}
}

// setDefault changes the default value of the named flag of cmd.
func setDefault(cmd *cobra.Command, name, value string) error {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return fmt.Errorf("flag %q does not exist", name)
	}
	if err := f.Value.Set(value); err != nil {
		return fmt.Errorf("flag %q: %w", name, err)
	}
	f.DefValue = f.Value.String()
	return nil
}
