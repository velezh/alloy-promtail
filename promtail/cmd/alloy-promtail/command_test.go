package main

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	root, err := newCommand()
	require.NoError(t, err)
	require.Contains(t, root.Version, programName)

	var names []string
	for _, cmd := range root.Commands() {
		names = append(names, cmd.Name())
	}
	require.ElementsMatch(t, []string{"fmt", "run", "validate"}, names)
}

func TestNewCommandFlags(t *testing.T) {
	root, err := newCommand()
	require.NoError(t, err)

	for _, ch := range changes {
		cmd, err := subcommand(root, ch.name)
		require.NoError(t, err)

		for _, name := range slices.Concat(ch.hidden, ch.unsupported) {
			require.True(t, cmd.Flags().Lookup(name).Hidden, "%s --%s", ch.name, name)
		}
		for name, value := range ch.defaults {
			f := cmd.Flags().Lookup(name)
			require.Equal(t, value, f.DefValue, "%s --%s", ch.name, name)
			require.Equal(t, value, f.Value.String(), "%s --%s", ch.name, name)
			require.False(t, f.Changed, "%s --%s", ch.name, name)
		}
	}
}

func TestSecureRunDefaults(t *testing.T) {
	root, err := newCommand()
	require.NoError(t, err)
	run, err := subcommand(root, "run")
	require.NoError(t, err)

	for name, want := range map[string]string{
		"disable-reporting":                  "true",
		"server.http.disable-support-bundle": "true",
		"server.http.enable-pprof":           "false",
		"server.http.listen-addr":            "127.0.0.1:12345",
	} {
		require.Equal(t, want, run.Flags().Lookup(name).DefValue, "--%s", name)
	}
}

func TestRejectUnsupportedFlags(t *testing.T) {
	root, err := newCommand()
	require.NoError(t, err)
	run, err := subcommand(root, "run")
	require.NoError(t, err)
	require.NoError(t, run.PreRunE(run, nil))

	require.NoError(t, run.Flags().Set("server.http.enable-graphql", "false"))
	require.NoError(t, run.PreRunE(run, nil))

	require.NoError(t, run.Flags().Set("server.http.enable-graphql-playground", "true"))
	require.EqualError(t, run.PreRunE(run, nil), "--server.http.enable-graphql-playground is not supported by alloy-promtail")
}

func TestRejectUnsupportedKeepsPreRun(t *testing.T) {
	var called bool
	cmd := &cobra.Command{Use: "run", PreRunE: func(*cobra.Command, []string) error {
		called = true
		return nil
	}}
	cmd.Flags().Bool("unsupported", false, "")

	rejectUnsupported(cmd, []string{"unsupported"})
	require.NoError(t, cmd.PreRunE(cmd, nil))
	require.True(t, called)
}

func TestSetDefaultOverriddenByArgs(t *testing.T) {
	var enabled bool
	cmd := &cobra.Command{Use: "run", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "")

	require.NoError(t, setDefault(cmd, "enabled", "true"))
	require.True(t, enabled)

	cmd.SetArgs([]string{"--enabled=false"})
	require.NoError(t, cmd.Execute())
	require.False(t, enabled)
}

func TestMissingUpstreamCommandOrFlag(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "run"})

	_, err := subcommand(root, "convert")
	require.ErrorContains(t, err, `no "convert" command`)

	run, err := subcommand(root, "run")
	require.NoError(t, err)
	require.ErrorContains(t, setDefault(run, "disable-reporting", "true"), `flag "disable-reporting" does not exist`)
}
