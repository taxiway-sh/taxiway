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
