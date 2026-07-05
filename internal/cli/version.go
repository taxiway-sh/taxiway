package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			obsRuntime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s\n", versionTitle(obsRuntime))
			fmt.Fprintf(out, "  version: %s\n", Version)
			fmt.Fprintf(out, "  commit: %s\n", Commit)
			fmt.Fprintf(out, "  build_date: %s\n", BuildDate)
			return nil
		},
	}
}

func versionTitle(runtime observabilityRuntime) string {
	if runtime.Context == "dev" && runtime.ContextID != "" {
		return fmt.Sprintf("Taxiway dev environment (%s):", runtime.ContextID)
	}
	return "Taxiway:"
}
