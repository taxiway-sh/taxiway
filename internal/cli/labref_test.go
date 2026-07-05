package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestMakeLabRef_Valid(t *testing.T) {
	ref, err := makeLabRef("mon-lab", "claude-code")
	require.NoError(t, err)
	require.Equal(t, config.LabRef{Lab: "mon-lab", Orch: "claude-code"}, ref)
	require.Equal(t, "taxiway-mon-lab", ref.ID())
}

func TestMakeLabRef_InvalidLabName(t *testing.T) {
	tests := []string{"", "bad name!", "bad/slash"}

	for _, lab := range tests {
		t.Run(lab, func(t *testing.T) {
			_, err := makeLabRef(lab, "claude-code")
			require.Error(t, err)
		})
	}
}

func TestMakeLabRef_InvalidOrchType(t *testing.T) {
	tests := []string{"", "bad type!"}

	for _, orch := range tests {
		t.Run(orch, func(t *testing.T) {
			_, err := makeLabRef("mon-lab", orch)
			require.Error(t, err)
		})
	}
}

func TestLoadLabRef_MissingLabErrorsClearly(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".lab-state")
	mock := driver.NewMockDriver(stateDir)

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  mock,
	}

	_, err := loadLabRef(context.Background(), state, "taxiway-typo-lab")
	require.Error(t, err)
	require.EqualError(t, err, `lab "typo-lab" does not exist`)
}
