package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

func TestDryRunPlanWritesOneHeaderAndFooter(t *testing.T) {
	var out bytes.Buffer
	plan := newDryRunPlan(&out, "phase", "install", "demo")
	plan.Step("codex-agent-install", "Installing @openai/codex@latest")
	plan.Detail("bubblewrap prerequisite")
	plan.Finish()
	plan.Finish()

	require.Equal(t, `Dry-run for phase "install" on lab "demo"

[codex-agent-install] Installing @openai/codex@latest
  bubblewrap prerequisite

No changes were made.
`, out.String())
}

func TestExecOfflinePlanScriptUsesUnavailableInspectionContext(t *testing.T) {
	script := filepath.Join(t.TempDir(), "plan.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/usr/bin/env bash
printf '%s|%s|%s|%s\n' "$TAXIWAY_EXECUTION_MODE" "$TAXIWAY_PLAN_INSPECTION" "$HOME" "$CUSTOM_VALUE"
`), 0o755))

	var stdout, stderr bytes.Buffer
	err := execOfflinePlanScript(context.Background(), script, &stdout, &stderr, map[string]string{
		"CUSTOM_VALUE": "forwarded",
	})

	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Equal(t, "plan|unavailable|/nonexistent|forwarded\n", stdout.String())
}

func TestExecPlannableScriptMarksLabInspectionAvailable(t *testing.T) {
	_, state, mock, _, _ := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	state.Flags.DryRun = true

	var captured driver.ExecRequest
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		captured = req
		return driver.MockExecResponse{}
	}

	var stdout, stderr bytes.Buffer
	err := execPlannableScriptToWithRef(
		context.Background(),
		state,
		ref,
		filepath.Join(state.RepoDir, "infra", "commands", "bootstrap.sh"),
		&stdout,
		&stderr,
		nil,
	)

	require.NoError(t, err)
	require.True(t, captured.Inspect)
	require.Equal(t, "plan", captured.Env["TAXIWAY_EXECUTION_MODE"])
	require.Equal(t, "available", captured.Env["TAXIWAY_PLAN_INSPECTION"])
}

func TestDryRunInstallAndVerifyPlansKeepOrchestratorBeforeAgents(t *testing.T) {
	repoDir, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	tests := []struct {
		orch          string
		phase         phases.Phase
		orchestrator  string
		agent         string
		semanticLabel string
	}{
		{orch: "codex", phase: phases.PhaseInstall, orchestrator: "[codex-orchestrator-install]", agent: "[codex-agent-install]", semanticLabel: "Installing @openai/codex@latest"},
		{orch: "codex", phase: phases.PhaseVerify, orchestrator: "[codex-orchestrator-verify]", agent: "[codex-agent-verify]", semanticLabel: "Verifying codex version and help"},
		{orch: "claude-code", phase: phases.PhaseInstall, orchestrator: "[claude-code-orchestrator-install]", agent: "[claude-code-agent-install]", semanticLabel: "Installing @anthropic-ai/claude-code@latest"},
		{orch: "claude-code", phase: phases.PhaseVerify, orchestrator: "[claude-code-orchestrator-verify]", agent: "[claude-code-agent-verify]", semanticLabel: "Verifying claude version, help, and auth status"},
		{orch: "gastown", phase: phases.PhaseInstall, orchestrator: "[gastown-install]", agent: "[claude-code-agent-install]", semanticLabel: "Installing Gas Town"},
		{orch: "gastown", phase: phases.PhaseVerify, orchestrator: "[gastown-verify]", agent: "[claude-code-agent-verify]", semanticLabel: "Verifying tool versions"},
	}

	for _, tt := range tests {
		t.Run(tt.orch+"/"+string(tt.phase), func(t *testing.T) {
			state := &RootState{
				RepoDir: repoDir,
				Flags:   GlobalFlags{DryRun: true, StateDir: t.TempDir()},
				Driver:  driver.NewDryRun(driver.NewMockDriver(t.TempDir())),
			}
			ref := config.LabRef{Lab: "demo", Orch: tt.orch, Driver: "mock"}
			var stdout, stderr bytes.Buffer

			err := planPhase(context.Background(), state, ref, tt.phase, &stdout, &stderr, phasePlanOptions{
				InspectionAvailable: false,
			})

			require.NoError(t, err)
			require.Empty(t, stderr.String())
			output := stdout.String()
			require.Contains(t, output, tt.semanticLabel)
			orchestratorIndex := strings.Index(output, tt.orchestrator)
			agentIndex := strings.Index(output, tt.agent)
			require.NotEqual(t, -1, orchestratorIndex)
			require.NotEqual(t, -1, agentIndex)
			require.Less(t, orchestratorIndex, agentIndex)
		})
	}
}
