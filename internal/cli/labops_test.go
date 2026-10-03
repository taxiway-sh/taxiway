package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

func TestCompletedPhaseLabel(t *testing.T) {
	tests := []struct {
		phase phases.Phase
		want  string
	}{
		{phase: phases.PhaseCreate, want: "created"},
		{phase: phases.PhaseBootstrap, want: "bootstrapped"},
		{phase: phases.PhaseInstall, want: "installed"},
		{phase: phases.PhaseVerify, want: "verified"},
		{phase: phases.PhaseGateway, want: "gateway ready"},
		{phase: phases.PhaseWorkspace, want: "workspace created"},
		{phase: phases.PhaseAuth, want: "authenticated"},
		{phase: phases.PhaseStart, want: "started"},
		{phase: phases.Phase("unknown"), want: "-"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			require.Equal(t, tt.want, completedPhaseLabel(tt.phase))
		})
	}
}

func TestStatusRowFromStatusUsesLabRef(t *testing.T) {
	created := time.Date(2026, 5, 26, 10, 30, 0, 0, time.UTC)

	row := statusRowFromStatus(
		driver.Status{Name: "taxiway-driver-id", State: "running", Driver: "mock", Created: created},
		config.LabRef{Lab: "public-lab", Orch: "claude-code"},
		"started",
	)

	require.Equal(t, "public-lab", row.lab)
	require.Equal(t, "claude-code", row.orch)
	require.Equal(t, "running", row.state)
	require.Equal(t, "started", row.phase)
	require.Equal(t, "mock", row.driver)
	require.Equal(t, "2026-05-26T10:30", row.created)
}

func TestStatusRowFromStatusFallsBackToDriverID(t *testing.T) {
	row := statusRowFromStatus(
		driver.Status{Name: "taxiway-demo", State: "stopped", Driver: "mock"},
		config.LabRef{},
		"-",
	)

	require.Equal(t, "demo", row.lab)
	require.Equal(t, "-", row.orch)
	require.Equal(t, "-", row.created)
}

func TestLabRuntimeStatusRequiresStartedPhase(t *testing.T) {
	proxy := proxyRuntime{Port: 55124, Container: "missing"}
	docker := dockerRuntimeStatus{Available: true}

	require.Equal(t, "degraded", labRuntimeStatus("running", "gateway ready", "running", true, proxy, docker))
}

func TestAgentVersionEventsPersistActualExecutable(t *testing.T) {
	root, state, _, out, _ := buildUpTestRoot(t)
	root.SetArgs([]string{"create", "pinned", "--type", "codex"})
	require.NoError(t, root.Execute())
	ref, err := loadLabRef(context.Background(), state, idName("pinned"))
	require.NoError(t, err)
	ref.Settings = map[string]string{"codex-version": "1.0.0"}
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), idName("pinned"), ref))
	script := filepath.Join(state.RepoDir, "infra", "commands", "version.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/bash
printf '%s\n' 'LAB_AGENT_EVENT {"type":"agent-version","agent":"codex","requested":"1.0.0","actual":"1.0.0","executable":"/usr/bin/codex"}'
`), 0755))
	require.NoError(t, execScriptWithRef(context.Background(), state, ref, script, nil))
	raw, err := os.ReadFile(filepath.Join(state.Flags.StateDir, "pinned", "agent-versions.json"))
	require.NoError(t, err)
	var versions map[string]map[string]string
	require.NoError(t, json.Unmarshal(raw, &versions))
	require.Equal(t, "1.0.0", versions["codex"]["actual"])
	require.Equal(t, "/usr/bin/codex", versions["codex"]["executable"])
	root.SetArgs([]string{"list", "pinned"})
	out.Reset()
	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), "codex: requested 1.0.0, installed 1.0.0")
	ref.Settings["codex-version"] = "2.0.0"
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), idName("pinned"), ref))
	out.Reset()
	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), "codex: requested 2.0.0, installed 1.0.0")
	root.SetArgs([]string{"rm", "pinned", "--yes"})
	require.NoError(t, root.Execute())
	_, err = os.Stat(filepath.Join(state.Flags.StateDir, "pinned", "agent-versions.json"))
	require.True(t, os.IsNotExist(err), "removal must discard the executable observation")
}
