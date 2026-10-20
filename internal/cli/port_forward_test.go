package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func TestPortForwardHostPortCanSkipHostAvailabilityCheck(t *testing.T) {
	ref := config.LabRef{Lab: "demo", Orch: "gastown"}
	spec := orchestratorPortForwards["gastown"]

	port, err := portForwardHostPort(t.TempDir(), ref, spec, false, false)
	require.NoError(t, err)
	require.NotZero(t, port)
	require.NotEqual(t, spec.GuestPort, port)
}

func TestPortForwardGascityDashboardUsesSupervisorPort(t *testing.T) {
	spec, ok := orchestratorPortForwards["gascity"]
	require.True(t, ok)
	require.Equal(t, 8372, spec.GuestPort)
	require.Equal(t, "TAXIWAY_DASHBOARD_HOST_PORT", spec.EnvKey)

	port, err := portForwardHostPort(t.TempDir(), config.LabRef{Lab: "demo", Orch: "gascity"}, spec, false, false)
	require.NoError(t, err)
	require.NotEqual(t, spec.GuestPort, port)
}

func TestPreparePortForwardExplainsStoppedLimaLab(t *testing.T) {
	_, state, mock, _, _ := buildUpTestRoot(t)
	state.Driver = &namedTestDriver{MockDriver: mock, name: "lima"}
	ref := config.LabRef{Lab: "demo", Orch: "gascity", Driver: "lima"}

	err := preparePortForward(testCtx(t), state, ref, map[string]string{})

	require.Error(t, err)
	require.Contains(t, err.Error(), `lab "demo" is not running`)
	require.Contains(t, err.Error(), "taxiway create demo")
	require.Contains(t, err.Error(), "taxiway run demo")
}
