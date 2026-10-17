package alloycli

import "github.com/spf13/cobra"

// toolsCommand returns an empty tools command: component tools are not included in alloy-promtail.
func toolsCommand() *cobra.Command {
	return &cobra.Command{Use: "tools"}
}
