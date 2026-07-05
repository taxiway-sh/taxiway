package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
)

const labOrchestratorProfileDir = "/lab/orchestrator-profile"

type profileSelection struct {
	name  string
	set   bool
	clear bool
}

func addProfileFlags(cmd *cobra.Command, profileName *string, noProfile *bool) {
	cmd.Flags().StringVar(profileName, "profile", "", "orchestrator profile name")
	cmd.Flags().BoolVar(noProfile, "no-profile", false, "clear the orchestrator profile and use orchestrator defaults")
}

func profileSelectionFromFlags(cmd *cobra.Command, profileName string, noProfile bool) (profileSelection, error) {
	set := cmd.Flags().Changed("profile")
	if set && noProfile {
		return profileSelection{}, fmt.Errorf("--profile and --no-profile are mutually exclusive")
	}
	if set && profileName == "" {
		return profileSelection{}, fmt.Errorf("--profile must not be empty")
	}
	return profileSelection{name: profileName, set: set, clear: noProfile}, nil
}

func applyProfileSelection(ctx context.Context, state *RootState, id string, ref *config.LabRef, sel profileSelection) (bool, error) {
	if !sel.set && !sel.clear {
		return false, nil
	}

	if sel.clear {
		ref.OrchestratorProfile = nil
		return true, persistProfileIfExists(ctx, state, id, *ref)
	}

	if err := config.ValidateOrchName(sel.name); err != nil {
		return false, fmt.Errorf("invalid --profile value: %w", err)
	}
	src := profileSourceDir(state.RepoDir, ref.Orch, sel.name)
	if info, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("orchestrator profile %q not found: expected %s", sel.name, src)
		}
		return false, fmt.Errorf("orchestrator profile %q: %w", sel.name, err)
	} else if !info.IsDir() {
		return false, fmt.Errorf("orchestrator profile %q is not a directory: %s", sel.name, src)
	}

	ref.OrchestratorProfile = &config.OrchestratorProfile{Name: sel.name}
	return true, persistProfileIfExists(ctx, state, id, *ref)
}

func persistProfileIfExists(ctx context.Context, state *RootState, id string, ref config.LabRef) error {
	exists, err := state.Driver.Exists(ctx, id)
	if err != nil || !exists {
		return err
	}
	if err := state.Driver.WriteLabRef(ctx, id, ref); err != nil {
		return fmt.Errorf("persisting orchestrator profile: %w", err)
	}
	return nil
}

func prepareOrchestratorProfileRuntime(ctx context.Context, state *RootState, ref config.LabRef, env map[string]string, clear bool) error {
	if clear {
		env["TAXIWAY_ORCH_PROFILE_CLEAR"] = "true"
		return nil
	}
	if ref.OrchestratorProfile == nil {
		return nil
	}
	src := profileSourceDir(state.RepoDir, ref.Orch, ref.OrchestratorProfile.Name)
	if info, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("orchestrator profile %q not found: expected %s", ref.OrchestratorProfile.Name, src)
		}
		return fmt.Errorf("orchestrator profile %q: %w", ref.OrchestratorProfile.Name, err)
	} else if !info.IsDir() {
		return fmt.Errorf("orchestrator profile %q is not a directory: %s", ref.OrchestratorProfile.Name, src)
	}
	if err := state.Driver.Copy(ctx, idName(ref.Lab), src, labOrchestratorProfileDir); err != nil {
		return fmt.Errorf("copy orchestrator profile to lab: %w", err)
	}
	env["TAXIWAY_ORCH_PROFILE_NAME"] = ref.OrchestratorProfile.Name
	env["TAXIWAY_ORCH_PROFILE_DIR"] = labOrchestratorProfileDir
	return nil
}

func profileSourceDir(repoDir, orch, name string) string {
	return filepath.Join(repoDir, "orchestrators", orch, "profiles", name)
}
