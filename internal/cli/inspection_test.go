package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestInspectionFailureSurfacesWithoutRecreatingLab(t *testing.T) {
	_, state, _, _, _ := buildUpTestRoot(t)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n"), 0755))
	t.Setenv("PATH", bin)
	state.Driver = driver.NewDockerDriver(state.Flags.StateDir)
	ref := config.LabRef{Lab: "existing", Orch: "claude-code", Driver: "docker"}
	require.NoError(t, config.WriteLabRef(state.Flags.StateDir, idName(ref.Lab), ref))
	require.NoError(t, config.EnsureCreatedAt(state.Flags.StateDir, idName(ref.Lab)))
	refPath := filepath.Join(state.Flags.StateDir, ref.Lab, "ref.json")
	createdPath := filepath.Join(state.Flags.StateDir, ref.Lab, "created_at")
	beforeRef, err := os.ReadFile(refPath)
	require.NoError(t, err)
	beforeCreated, err := os.ReadFile(createdPath)
	require.NoError(t, err)
	_, err = collectLabStatusRows(context.Background(), state, state.Flags.StateDir)
	require.ErrorContains(t, err, "Cannot connect to the Docker daemon")
	var logs bytes.Buffer
	err = labUp(context.Background(), state, ref, &logs)
	require.ErrorContains(t, err, "Cannot connect to the Docker daemon")
	require.NotContains(t, logs.String(), "Creating lab")
	afterRef, err := os.ReadFile(refPath)
	require.NoError(t, err)
	require.Equal(t, beforeRef, afterRef)
	afterCreated, err := os.ReadFile(createdPath)
	require.NoError(t, err)
	require.Equal(t, beforeCreated, afterCreated)
}
