package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/discover"
)

const defaultOrchType = "claude-code"

// addTypeFlag registers --type / -t on cmd.
// It reads available orchestrators from the filesystem to build the usage string.
// If the list cannot be determined, it uses static help text.
func addTypeFlag(cmd *cobra.Command, state *RootState, p *string) {
	usage := buildTypeFlagUsage(state.RepoDir)
	cmd.Flags().StringVarP(p, "type", "t", defaultOrchType, usage)
	_ = cmd.RegisterFlagCompletionFunc("type", completeOrchestrators(state))
}

// buildTypeFlagUsage constructs the --type flag usage string.
// It discovers orchestrators from repoDir; on error, returns static help text.
func buildTypeFlagUsage(repoDir string) string {
	if repoDir == "" {
		return "orchestrator type (e.g. claude-code, gastown, codex)"
	}
	names, err := discover.Orchestrators(repoDir)
	if err != nil || len(names) == 0 {
		return "orchestrator type (e.g. claude-code, gastown, codex)"
	}
	return fmt.Sprintf("orchestrator type; available: %s", strings.Join(names, ", "))
}

// makeLabRef validates lab name and orch type, then returns a LabRef.
func makeLabRef(lab, orchType string) (config.LabRef, error) {
	if err := config.ValidateLabName(lab); err != nil {
		return config.LabRef{}, err
	}
	if err := config.ValidateOrchName(orchType); err != nil {
		return config.LabRef{}, fmt.Errorf("invalid --type value: %w", err)
	}
	return config.LabRef{Lab: lab, Orch: orchType}, nil
}

// loadLabRef reads the required LabRef sidecar for id.
func loadLabRef(ctx context.Context, state *RootState, id string) (config.LabRef, error) {
	ref, ok, err := state.Driver.ReadLabRef(ctx, id)
	if err != nil {
		return config.LabRef{}, fmt.Errorf("read lab ref for %s: %w", id, err)
	}
	if ok {
		if ref.Driver == "" {
			return config.LabRef{}, fmt.Errorf("lab %q is missing driver in ref.json", ref.Lab)
		}
		return ref, nil
	}
	lab, nameErr := config.LabNameFromID(id)
	if nameErr != nil {
		return config.LabRef{}, nameErr
	}
	exists, err := state.Driver.Exists(ctx, id)
	if err != nil {
		return config.LabRef{}, fmt.Errorf("check lab %s: %w", lab, err)
	}
	if !exists {
		return config.LabRef{}, fmt.Errorf("lab %q does not exist", lab)
	}
	return config.LabRef{}, fmt.Errorf("lab %q is missing ref.json", lab)
}

// validateLabArg validates a lab name (format only, no file check).
func validateLabArg(lab string) error {
	return config.ValidateLabName(lab)
}

func labNameHelpText() string {
	return fmt.Sprintf("Lab names must be %d characters or fewer and contain only letters, numbers, dashes, or underscores.", config.MaxLabNameLength)
}

// completeActiveLabs returns a completion function for active lab names.
func completeActiveLabs(state *RootState) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
		labs, err := discover.ActiveLabs(stateDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		names := make([]string, len(labs))
		for i, l := range labs {
			names[i] = l.Lab
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}
