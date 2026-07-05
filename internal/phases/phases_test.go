package phases

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDirStripsRuntimeContextFromDriverID(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "59eb6a64")
	stateDir := t.TempDir()

	require.Equal(t,
		filepath.Join(stateDir, "test-observe", "phases"),
		Dir(stateDir, "taxiway-dev-59eb6a64-test-observe"),
	)
}

func TestDoneAfterMark(t *testing.T) {
	tmp := t.TempDir()
	id := "test-lab"

	require.False(t, Done(tmp, id, PhaseBootstrap), "expected false before Mark")
	require.NoError(t, Mark(tmp, id, PhaseBootstrap))
	require.True(t, Done(tmp, id, PhaseBootstrap), "expected true after Mark")
}

func TestClearAll(t *testing.T) {
	tmp := t.TempDir()
	id := "test-lab"

	for _, p := range []Phase{PhaseCreate, PhaseBootstrap} {
		require.NoError(t, Mark(tmp, id, p))
		require.True(t, Done(tmp, id, p))
	}

	require.NoError(t, ClearAll(tmp, id))

	for _, p := range []Phase{PhaseCreate, PhaseBootstrap} {
		require.False(t, Done(tmp, id, p), "phase %s should be false after ClearAll", p)
	}
}

func TestParsePhase_Valid(t *testing.T) {
	for _, p := range Order {
		got, err := ParsePhase(string(p))
		require.NoError(t, err, "phase %s should parse without error", p)
		require.Equal(t, p, got)
	}
}

func TestOrderIncludesAuthBetweenWorkspaceAndStart(t *testing.T) {
	require.Equal(t, []Phase{
		PhaseCreate,
		PhaseBootstrap,
		PhaseInstall,
		PhaseVerify,
		PhaseGateway,
		PhaseWorkspace,
		PhaseAuth,
		PhaseStart,
	}, Order)
}

func TestParsePhase_Invalid(t *testing.T) {
	_, err := ParsePhase("foobar")
	require.Error(t, err)
	require.Contains(t, err.Error(), "foobar")
}

func TestMark_CreatesDir(t *testing.T) {
	tmp := t.TempDir()
	id := "new-lab"

	// Ensure the phases dir does NOT exist before Mark
	phasesDir := filepath.Join(tmp, id, "phases")
	_, err := os.Stat(phasesDir)
	require.True(t, os.IsNotExist(err), "phases dir should not exist before Mark")

	require.NoError(t, Mark(tmp, id, PhaseVerify))

	_, err = os.Stat(phasesDir)
	require.NoError(t, err, "phases dir should exist after Mark")
	require.True(t, Done(tmp, id, PhaseVerify))
}
