package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestSettingEnvName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "version", want: "TAXIWAY_SET_VERSION"},
		{key: "foo-bar", want: "TAXIWAY_SET_FOO_BAR"},
		{key: "foo.bar", want: "TAXIWAY_SET_FOO_BAR"},
		{key: "model_1", want: "TAXIWAY_SET_MODEL_1"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			require.Equal(t, tt.want, settingEnvName(tt.key))
		})
	}
}

func TestSettingsSelectionFromFlagsRejectsNormalisedCollision(t *testing.T) {
	_, err := settingsSelectionFromFlags([]string{"foo-bar=1", "foo.bar=2"}, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "normalizes to TAXIWAY_SET_FOO_BAR")
}

func TestMergeSettingsAppliesSetAndClear(t *testing.T) {
	got, err := mergeSettings(
		map[string]string{"version": "1.0.0", "model": "sonnet"},
		settingsSelection{
			values: map[string]string{"version": "1.1.0"},
			clears: []string{"model"},
		},
	)

	require.NoError(t, err)
	require.Equal(t, map[string]string{"version": "1.1.0"}, got)
}

func TestApplySettingsSelectionPersistsExistingLab(t *testing.T) {
	stateDir := t.TempDir()
	mock := driver.NewMockDriver(stateDir)
	id := config.IDOf("gastown")
	require.NoError(t, mock.Create(context.Background(), id, driver.CreateOptions{}))
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	state := &RootState{RepoDir: t.TempDir(), Flags: GlobalFlags{StateDir: stateDir}, Driver: mock}

	changed, err := applySettingsSelection(context.Background(), state, id, &ref, settingsSelection{
		values: map[string]string{"version": "1.1.0"},
	})

	require.NoError(t, err)
	require.True(t, changed)
	got, ok, err := mock.ReadLabRef(context.Background(), id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{"version": "1.1.0"}, got.Settings)
}

func TestInjectSettingsEnv(t *testing.T) {
	env := map[string]string{}

	injectSettingsEnv(env, map[string]string{"version": "1.1.0", "foo-bar": "baz"})

	require.Equal(t, "1.1.0", env["TAXIWAY_SET_VERSION"])
	require.Equal(t, "baz", env["TAXIWAY_SET_FOO_BAR"])
}
