package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TAXIWAY_RUNTIME_DIR", "")
	t.Setenv("TAXIWAY_LAB_STATE_DIR", "")
	root, state, stdout, stderr := buildTestRoot(t)
	state.RepoDir = ""
	state.Flags.StateDir = ""
	out, _, err := execRoot(t, root, stdout, stderr, "version")
	require.NoError(t, err)
	require.Equal(t, "Taxiway:\n  version: 0.1.0\n  commit: dev\n  build_date: unknown\n", out)
	require.Empty(t, stderr.String())
}

func TestVersionShowsDevObservabilityContext(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	t.Setenv("TAXIWAY_RUNTIME_DIR", tmp)
	t.Setenv("TAXIWAY_LAB_STATE_DIR", filepath.Join(tmp, ".lab-state"))
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	root, state, stdout, stderr := buildTestRoot(t)
	state.RepoDir = ""
	state.Flags.StateDir = ""

	out, _, err := execRoot(t, root, stdout, stderr, "version")

	require.NoError(t, err)
	require.Equal(t, "Taxiway dev environment (a1b2c3d4):\n  version: 0.1.0\n  commit: dev\n  build_date: unknown\n", out)
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	require.Empty(t, stderr.String())
}
