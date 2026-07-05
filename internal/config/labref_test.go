package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteReadLabRef_RoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	id := "mon-lab"
	ref := LabRef{Lab: "mon-lab", Orch: "claude-code", Driver: DefaultDriver}

	require.NoError(t, WriteLabRef(stateDir, id, ref))

	got, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok, "ref.json should be found")
	require.Equal(t, ref, got)
}

func TestReadLabRef_Absent(t *testing.T) {
	stateDir := t.TempDir()
	_, ok, err := ReadLabRef(stateDir, "nonexistent")
	require.NoError(t, err)
	require.False(t, ok, "absent sidecar should return ok=false")
}

func TestReadLabRef_DirectoryWithoutSidecar(t *testing.T) {
	// A lab directory without ref.json should return ok=false, no error.
	stateDir := t.TempDir()
	id := "gastown"
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, id), 0o755))

	_, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.False(t, ok, "lab without sidecar should return ok=false")
}

func TestWriteLabRef_Atomic_CreatesParentDirs(t *testing.T) {
	stateDir := t.TempDir()
	id := "new-id"
	ref := LabRef{Lab: "new-id", Orch: "gastown", Driver: DefaultDriver}

	require.NoError(t, WriteLabRef(stateDir, id, ref))

	// The sidecar file must exist.
	_, err := os.Stat(filepath.Join(stateDir, id, "ref.json"))
	require.NoError(t, err, "ref.json should exist after WriteLabRef")

	// No .tmp file should linger.
	_, err = os.Stat(filepath.Join(stateDir, id, "ref.json.tmp"))
	require.True(t, os.IsNotExist(err), "no .tmp file should remain after atomic write")
}

func TestEnsureCreatedAt_DoesNotOverwriteExistingTimestamp(t *testing.T) {
	stateDir := t.TempDir()
	id := "taxiway-existing"
	path := CreatedAtPath(stateDir, id)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("2026-05-26T10:00:00Z\n"), 0o644))

	require.NoError(t, EnsureCreatedAt(stateDir, id))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "2026-05-26T10:00:00Z\n", string(raw))
	got, ok := ReadCreatedAt(stateDir, id)
	require.True(t, ok)
	require.Equal(t, "2026-05-26T10:00:00Z", got.UTC().Format("2006-01-02T15:04:05Z"))
}

func TestReadLabRef_CorruptJSON(t *testing.T) {
	stateDir := t.TempDir()
	id := "bad"
	dir := filepath.Join(stateDir, id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ref.json"), []byte("not-json{"), 0o644))

	_, ok, err := ReadLabRef(stateDir, id)
	require.Error(t, err, "corrupt JSON should return an error")
	require.False(t, ok)
}

func TestWriteReadLabRef_Multiplelabs(t *testing.T) {
	stateDir := t.TempDir()
	refs := []struct {
		id  string
		ref LabRef
	}{
		{"lab1", LabRef{Lab: "lab1", Orch: "claude-code", Driver: DefaultDriver}},
		{"lab2", LabRef{Lab: "lab2", Orch: "gastown", Driver: DefaultDriver}},
		{"lab3", LabRef{Lab: "lab3", Orch: "codex", Driver: DefaultDriver}},
	}
	for _, tc := range refs {
		require.NoError(t, WriteLabRef(stateDir, tc.id, tc.ref))
	}
	for _, tc := range refs {
		got, ok, err := ReadLabRef(stateDir, tc.id)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, tc.ref, got)
	}
}

func TestWriteReadLabRef_WithWorkspace(t *testing.T) {
	stateDir := t.TempDir()
	id := "bench"
	ref := LabRef{
		Lab:    "bench",
		Orch:   "claude-code",
		Driver: DefaultDriver,
		Workspace: &Workspace{
			Repo: "https://github.com/foo/bar",
			Ref:  "main",
			Path: "subdir",
		},
	}

	require.NoError(t, WriteLabRef(stateDir, id, ref))

	got, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, ref, got)
	require.NotNil(t, got.Workspace)
	require.Equal(t, "https://github.com/foo/bar", got.Workspace.Repo)
	require.Equal(t, "main", got.Workspace.Ref)
	require.Equal(t, "subdir", got.Workspace.Path)
}

func TestWriteReadLabRef_WithoutWorkspace(t *testing.T) {
	stateDir := t.TempDir()
	id := "nowork"
	ref := LabRef{Lab: "nowork", Orch: "codex", Driver: DefaultDriver}

	require.NoError(t, WriteLabRef(stateDir, id, ref))

	got, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Nil(t, got.Workspace, "Workspace should be nil when not configured")
	require.Equal(t, "nowork", got.Lab)
	require.Equal(t, "codex", got.Orch)
}

func TestWriteLabRef_WritesCurrentVersion(t *testing.T) {
	stateDir := t.TempDir()
	id := "vcheck"
	ref := LabRef{Lab: "vcheck", Orch: "claude-code", Driver: "docker"}
	require.NoError(t, WriteLabRef(stateDir, id, ref))

	data, err := os.ReadFile(filepath.Join(stateDir, id, "ref.json"))
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	ver, ok := raw["version"].(float64)
	require.True(t, ok, "version field must be present")
	require.Equal(t, float64(5), ver, "written sidecar must be version 5")
	require.Equal(t, "docker", raw["driver"], "driver field must be recorded")
}

func TestWriteReadLabRef_WithOrchestratorProfile(t *testing.T) {
	stateDir := t.TempDir()
	id := "profiled"
	ref := LabRef{
		Lab:    "profiled",
		Orch:   "gastown",
		Driver: "mock",
		OrchestratorProfile: &OrchestratorProfile{
			Name: "budget",
		},
	}
	require.NoError(t, WriteLabRef(stateDir, id, ref))

	got, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, got.OrchestratorProfile)
	require.Equal(t, "budget", got.OrchestratorProfile.Name)

	data, err := os.ReadFile(filepath.Join(stateDir, id, "ref.json"))
	require.NoError(t, err)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Equal(t, float64(5), raw["version"], "profile sidecar must be version 5")
	require.Equal(t, map[string]interface{}{"name": "budget"}, raw["orchestrator_profile"])
	require.Contains(t, string(data), "\n  \"orchestrator_profile\": {\n    \"name\": \"budget\"\n  }\n")
}

func TestWriteReadLabRef_WithSettings(t *testing.T) {
	stateDir := t.TempDir()
	id := "versioned"
	ref := LabRef{
		Lab:      "versioned",
		Orch:     "gastown",
		Driver:   "mock",
		Settings: map[string]string{"version": "1.1.0"},
	}
	require.NoError(t, WriteLabRef(stateDir, id, ref))

	got, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{"version": "1.1.0"}, got.Settings)

	data, err := os.ReadFile(filepath.Join(stateDir, id, "ref.json"))
	require.NoError(t, err)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Equal(t, float64(5), raw["version"], "settings sidecar must be version 5")
	require.Equal(t, map[string]interface{}{"version": "1.1.0"}, raw["settings"])
}

func TestWriteLabRef_RequiresDriver(t *testing.T) {
	stateDir := t.TempDir()
	id := "default-driver"
	ref := LabRef{Lab: "default-driver", Orch: "gastown"}

	require.EqualError(t, WriteLabRef(stateDir, id, ref), `labref: driver is required for default-driver`)
}

func TestWorkspace_WorkspaceOnlyRepo(t *testing.T) {
	stateDir := t.TempDir()
	id := "onlyrepo"
	ref := LabRef{
		Lab:    "onlyrepo",
		Orch:   "claude-code",
		Driver: DefaultDriver,
		Workspace: &Workspace{
			Repo: "https://github.com/acme/project",
		},
	}
	require.NoError(t, WriteLabRef(stateDir, id, ref))

	got, ok, err := ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, got.Workspace)
	require.Equal(t, "https://github.com/acme/project", got.Workspace.Repo)
	require.Empty(t, got.Workspace.Ref)
	require.Empty(t, got.Workspace.Path)
}
