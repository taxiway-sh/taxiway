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
