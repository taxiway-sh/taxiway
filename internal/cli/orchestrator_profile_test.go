package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestProfileSelectionFromFlagsRejectsMutuallyExclusiveFlags(t *testing.T) {
	cmd := &cobra.Command{}
	var profile string
	var noProfile bool
	addProfileFlags(cmd, &profile, &noProfile)
	require.NoError(t, cmd.Flags().Parse([]string{"--profile", "budget", "--no-profile"}))

	_, err := profileSelectionFromFlags(cmd, profile, noProfile)

	require.EqualError(t, err, "--profile and --no-profile are mutually exclusive")
}

func TestApplyProfileSelectionPersistsExistingLab(t *testing.T) {
	stateDir := t.TempDir()
	repoDir := t.TempDir()
	addOrchestratorProfileDir(t, repoDir, "gastown", "budget")
	mock := driver.NewMockDriver(stateDir)
	id := config.IDOf("gastown")
	require.NoError(t, mock.Create(context.Background(), id, driver.CreateOptions{}))
	state := &RootState{RepoDir: repoDir, Flags: GlobalFlags{StateDir: stateDir}, Driver: mock}
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}

	changed, err := applyProfileSelection(context.Background(), state, id, &ref, profileSelection{name: "budget", set: true})

	require.NoError(t, err)
	require.True(t, changed)
	got, ok, err := mock.ReadLabRef(context.Background(), id)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, got.OrchestratorProfile)
	require.Equal(t, "budget", got.OrchestratorProfile.Name)
}

func TestPrepareOrchestratorProfileRuntimeCopiesProfileAndInjectsEnv(t *testing.T) {
	stateDir := t.TempDir()
	repoDir := t.TempDir()
	profileDir := addOrchestratorProfileDir(t, repoDir, "gastown", "budget")
	mock := driver.NewMockDriver(stateDir)
	state := &RootState{RepoDir: repoDir, Flags: GlobalFlags{StateDir: stateDir}, Driver: mock}
	ref := config.LabRef{
		Lab:    "gastown",
		Orch:   "gastown",
		Driver: "mock",
		OrchestratorProfile: &config.OrchestratorProfile{
			Name: "budget",
		},
	}
	env := map[string]string{}

	err := prepareOrchestratorProfileRuntime(context.Background(), state, ref, env, false)

	require.NoError(t, err)
	require.Equal(t, "budget", env["TAXIWAY_ORCH_PROFILE_NAME"])
	require.Equal(t, "/lab/orchestrator-profile", env["TAXIWAY_ORCH_PROFILE_DIR"])
	require.Len(t, mock.CopyLog, 1)
	require.Equal(t, profileDir, mock.CopyLog[0].Src)
	require.Equal(t, "/lab/orchestrator-profile", mock.CopyLog[0].Dst)
}

func TestPrepareOrchestratorProfileRuntimeClearOnlyInjectsClearEnv(t *testing.T) {
	state := &RootState{RepoDir: t.TempDir(), Driver: driver.NewMockDriver(t.TempDir())}
	env := map[string]string{}

	err := prepareOrchestratorProfileRuntime(context.Background(), state, config.LabRef{Lab: "gastown"}, env, true)

	require.NoError(t, err)
	require.Equal(t, "true", env["TAXIWAY_ORCH_PROFILE_CLEAR"])
	require.NotContains(t, env, "TAXIWAY_ORCH_PROFILE_NAME")
	require.NotContains(t, env, "TAXIWAY_ORCH_PROFILE_DIR")
}

func addOrchestratorProfileDir(t *testing.T, repoDir, orch, name string) string {
	t.Helper()
	dir := filepath.Join(repoDir, "orchestrators", orch, "profiles", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}
