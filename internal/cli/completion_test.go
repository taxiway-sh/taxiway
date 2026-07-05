package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/taxiway-sh/taxiway/internal/config"
)

// buildCompletionState creates a minimal RootState with orchestrators and active labs
// in a temp dir.
func buildCompletionState(t *testing.T) (*RootState, string) {
	t.Helper()
	tmp := t.TempDir()

	// Two orchestrators with install.sh
	for _, orch := range []string{"codex", "gastown"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\n"), 0755))
	}

	// Two active lab directories (lab name, no taxiway- prefix) with created_at sentinel.
	stateDir := filepath.Join(tmp, ".lab-state")
	for _, lab := range []string{"codex", "gastown"} {
		dir := filepath.Join(stateDir, lab)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "created_at"), []byte("2026-01-01T00:00:00Z"), 0644))
		require.NoError(t, config.WriteLabRef(stateDir, lab, config.LabRef{Lab: lab, Orch: lab, Driver: "mock"}))
	}

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{StateDir: stateDir},
	}
	return state, tmp
}

func TestCompleteOrchestrators(t *testing.T) {
	state, _ := buildCompletionState(t)
	fn := completeOrchestrators(state)

	// First arg completion — should return orchestrators
	names, directive := fn(&cobra.Command{}, []string{}, "")
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	require.Equal(t, []string{"codex", "gastown"}, names)

	// Second arg — already have one arg, no more completions
	names2, directive2 := fn(&cobra.Command{}, []string{"codex"}, "")
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive2)
	require.Nil(t, names2)
}

func TestCreateTypeFlagCompletesOrchestrators(t *testing.T) {
	state, _ := buildCompletionState(t)
	cmd := newCreateCmd(state)

	fn, ok := cmd.GetFlagCompletionFunc("type")
	require.True(t, ok)

	names, directive := fn(cmd, []string{}, "")
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	require.Equal(t, []string{"codex", "gastown"}, names)
}

func TestCompleteActiveLabs(t *testing.T) {
	state, _ := buildCompletionState(t)
	fn := completeActiveLabs(state)

	// First arg completion — should return active labs (stripped of prefix)
	names, directive := fn(&cobra.Command{}, []string{}, "")
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	require.Equal(t, []string{"codex", "gastown"}, names)

	// Second arg — no more completions
	names2, directive2 := fn(&cobra.Command{}, []string{"gastown"}, "")
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive2)
	require.Nil(t, names2)
}
