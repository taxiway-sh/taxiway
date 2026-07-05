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
