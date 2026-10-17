package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLI(t *testing.T) {
	skipWithoutBinary(t)
	t.Parallel()

	t.Run("version", func(t *testing.T) {
		t.Parallel()
		out, code := runCommand(t, "--version")
		require.Zero(t, code, out)
		require.Contains(t, out, "alloy-promtail")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		out, code := runCommand(t, "--help")
		require.Zero(t, code, out)

		commands := helpCommands(out)
		require.Subset(t, commands, []string{"fmt", "run", "validate"})
		for _, name := range []string{"convert", "gql", "tools"} {
			require.NotContains(t, commands, name)
		}
	})

	t.Run("validate pipelines", func(t *testing.T) {
		t.Parallel()
		const pushURL = "http://127.0.0.1/loki/api/v1/push"
		for _, config := range []string{
			filePipelineConfig(filepath.Join(t.TempDir(), "*.log"), pushURL),
			apiPipelineConfig(freePort(t), pushURL),
		} {
			out, code := runCommand(t, "validate", writeConfig(t, config))
			require.Zero(t, code, out)
		}
	})

	t.Run("validate unknown component", func(t *testing.T) {
		t.Parallel()
		out, code := runCommand(t, "validate", writeConfig(t, `
prometheus.scrape "default" {
	targets    = []
	forward_to = []
}
`))
		require.NotZero(t, code)
		require.Contains(t, out, `"prometheus.scrape"`)
	})

	t.Run("import.git", func(t *testing.T) {
		t.Parallel()
		const config = `
import.git "modules" {
	repository = "https://github.com/grafana/alloy-modules.git"
	revision   = "main"
	path       = "modules"
}
`
		for _, tc := range []struct {
			name string
			args []string
		}{
			{name: "validate", args: []string{"validate", writeConfig(t, config)}},
			{name: "run", args: runArgs(t, freeAddr(t), config)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, code := runCommand(t, tc.args...)
				require.NotZero(t, code)
				require.Contains(t, out, "import.git is not supported by alloy-promtail")
			})
		}
	})

	t.Run("cluster peer discovery", func(t *testing.T) {
		t.Parallel()
		out, code := runCommand(t, runArgs(t, freeAddr(t), "", "--cluster.enabled", "--cluster.discover-peers=provider=aws")...)
		require.NotZero(t, code)
		require.Contains(t, out, "--cluster.discover-peers is not supported by alloy-promtail")
	})

	t.Run("graphql", func(t *testing.T) {
		t.Parallel()
		out, code := runCommand(t, runArgs(t, freeAddr(t), "", "--stability.level=experimental", "--server.http.enable-graphql")...)
		require.NotZero(t, code)
		require.Contains(t, out, "--server.http.enable-graphql is not supported by alloy-promtail")
	})
}

// helpCommands returns the subcommands listed by the help output of the root command.
func helpCommands(help string) []string {
	_, section, _ := strings.Cut(help, "Available Commands:\n")
	var names []string
	for line := range strings.Lines(section) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			break
		}
		names = append(names, fields[0])
	}
	return names
}
