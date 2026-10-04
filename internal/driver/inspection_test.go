package driver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDockerInspectionFailureIsNotAbsence(t *testing.T) {
	for _, message := range []string{"Cannot connect to the Docker daemon", "permission denied", "Error: No such object: another-container"} {
		t.Run(message, func(t *testing.T) {
			installFakeDocker(t, "#!/bin/sh\necho '"+message+"' >&2\nexit 1\n")
			d := NewDockerDriver(t.TempDir())
			exists, err := d.Exists(context.Background(), "taxiway-demo")
			require.ErrorContains(t, err, message)
			require.False(t, exists)
			_, err = d.Running(context.Background(), "taxiway-demo")
			require.ErrorContains(t, err, message)
			status, err := d.Status(context.Background(), "taxiway-demo")
			require.ErrorContains(t, err, message)
			require.Empty(t, status.State)
			require.NoError(t, os.MkdirAll(filepath.Join(d.stateDir, "demo"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(d.stateDir, "demo", "created_at"), []byte("2026-05-12T10:00:00Z"), 0644))
			_, err = d.List(context.Background())
			require.ErrorContains(t, err, message)
		})
	}
}

func TestLimaInspectionFailureIsNotAbsence(t *testing.T) {
	installFakeLimactl(t, "#!/bin/sh\necho 'limactl permission denied' >&2\nexit 1\n")
	d := NewLimaDriver(t.TempDir())
	exists, err := d.Exists(context.Background(), "taxiway-demo")
	require.False(t, exists)
	require.ErrorContains(t, err, "limactl permission denied")
	status, err := d.Status(context.Background(), "taxiway-demo")
	require.Empty(t, status.State)
	require.ErrorContains(t, err, "limactl permission denied")
	_, err = d.List(context.Background())
	require.ErrorContains(t, err, "limactl permission denied")
}

func TestLimaOperationFailureIncludesNativeDiagnostic(t *testing.T) {
	installFakeLimactl(t, "#!/bin/sh\necho 'native failure: fixture disk full' >&2\nexit 1\n")
	d := NewLimaDriver(t.TempDir())
	for _, operation := range []func() error{
		func() error { return d.Stop(context.Background(), "taxiway-demo") },
		func() error { return d.Delete(context.Background(), "taxiway-demo") },
		func() error { return d.Copy(context.Background(), "taxiway-demo", "fixture", "/lab/work/fixture") },
	} {
		require.ErrorContains(t, operation(), "native failure: fixture disk full")
	}
}
