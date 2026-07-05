package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/taxiway-sh/taxiway/internal/config"
)

func TestActiveLabs(t *testing.T) {
	tmp := t.TempDir()

	// Create two lab directories (no taxiway- prefix) with a created_at sentinel.
	for _, name := range []string{"gastown", "codex"} {
		dir := filepath.Join(tmp, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "created_at"), []byte("2026-01-01T00:00:00Z"), 0644))
		require.NoError(t, config.WriteLabRef(tmp, name, config.LabRef{Lab: name, Orch: name, Driver: "mock"}))
	}

	labs, err := ActiveLabs(tmp)
	require.NoError(t, err)
	names := make([]string, len(labs))
	for i, l := range labs {
		names[i] = l.Lab
	}
	require.Equal(t, []string{"codex", "gastown"}, names)
}

func TestActiveLabsIgnoresNonCanonicalRuntimeContextDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "59eb6a64")

	dir := filepath.Join(tmp, "test-observe")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "created_at"), []byte("2026-01-01T00:00:00Z"), 0o644))
	require.NoError(t, config.WriteLabRef(tmp, "test-observe", config.LabRef{Lab: "test-observe", Orch: "codex", Driver: "mock"}))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "dev-59eb6a64-test-observe", "phases"), 0o755))

	labs, err := ActiveLabs(tmp)

	require.NoError(t, err)
	require.Len(t, labs, 1)
	require.Equal(t, "test-observe", labs[0].Lab)
}

func TestOrchestrators(t *testing.T) {
	tmp := t.TempDir()
	orchDir := filepath.Join(tmp, "orchestrators")

	// alpha and beta have install.sh
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(orchDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\n"), 0755))
	}

	// noinstall has no install.sh
	noDir := filepath.Join(orchDir, "noinstall")
	require.NoError(t, os.MkdirAll(noDir, 0755))

	// plain file (not dir) — should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "notadir"), []byte{}, 0644))

	names, err := Orchestrators(tmp)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, names)
}

func TestOrchestratorsRealRepo(t *testing.T) {
	// Find the repo root relative to this test file.
	// Go tests run with cwd = the package directory.
	// Walk up to find go.mod.
	dir, err := os.Getwd()
	require.NoError(t, err)

	repoRoot := ""
	for d := dir; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			repoRoot = d
			break
		}
	}
	if repoRoot == "" {
		t.Skip("could not find repo root (no go.mod)")
	}

	names, err := Orchestrators(repoRoot)
	require.NoError(t, err)
	require.Equal(t, []string{"claude-code", "codex", "gastown"}, names)
}
