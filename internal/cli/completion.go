package cli

import (
	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/discover"
)

// completeOrchestrators completes from available orchestrators (orchestrators/*/install.sh).
func completeOrchestrators(state *RootState) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		names, err := discover.Orchestrators(state.RepoDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}
