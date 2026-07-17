package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

// buildAliasTestRoot creates a fully-wired root (all flat verbs included),
// backed by a MockDriver in a temp dir. Returns root, state, mock, stdout, stderr.
func buildAliasTestRoot(t *testing.T) (*cobra.Command, *RootState, *driver.MockDriver, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()

	for _, orch := range []string{"codex", "gastown"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\nif [[ \"$TAXIWAY_EXECUTION_MODE\" == plan ]]; then echo '["+orch+"-install] Installing "+orch+"'; exit 0; fi\necho install\n"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "verify.sh"), []byte("#!/bin/bash\nif [[ \"$TAXIWAY_EXECUTION_MODE\" == plan ]]; then echo '["+orch+"-verify] Verifying "+orch+"'; exit 0; fi\necho verify\n"), 0755))
	}
	// gastown has start.sh; codex does not in this test fixture (for TestStart_MissingScript).
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, "orchestrators", "gastown", "start.sh"),
		[]byte("#!/bin/bash\nif [[ \"$TAXIWAY_EXECUTION_MODE\" == plan ]]; then echo \"[gastown-start] Starting tmux session 'gastown'\"; exit 0; fi\necho start\n"), 0755,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, "orchestrators", "gastown", "workspace.sh"),
		[]byte("#!/bin/bash\nif [[ \"$TAXIWAY_EXECUTION_MODE\" == plan ]]; then echo '[gastown-workspace] Cloning workspace repository'; exit 0; fi\necho workspace\n"), 0755,
	))
	for _, p := range []string{"infra/commands"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, p), 0755))
	}
	for _, script := range []string{"infra/commands/bootstrap.sh", "infra/commands/doctor.sh"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, script), []byte("#!/bin/bash\necho ok\n"), 0755))
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, "infra/commands/reset.sh"),
		[]byte("#!/bin/bash\nif [[ \"$TAXIWAY_EXECUTION_MODE\" == plan ]]; then echo '[reset] Stopping workspace services'; echo '[reset] Clearing /lab/work contents'; exit 0; fi\necho ok\n"), 0755,
	))

	stateDir := filepath.Join(tmp, ".lab-state")
	mock := driver.NewMockDriver(stateDir)
	mock.RepoDir = tmp // allow Exec to translate /lab/ lab paths to real paths

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  mock,
	}

	var stdout, stderr bytes.Buffer
	root := &cobra.Command{Use: "taxiway", SilenceUsage: true}
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	wrapTestDriverForDryRun(root, state)

	// Wire all commands exactly as Execute() does (including flat verbs).
	root.AddCommand(
		newVersionCmd(state),
		newUpCmd(state),
		newCreateCmd(state),
		newDownCmd(state),
		newShellCmd(state),
		newListCmd(state),
		newRmCmd(state),
		newCredentialsCmd(state),
		newLabAuthCmd(state),
		newBootstrapCmd(state),
		newStartCmd(state),
		newInstallCmd(state),
		newVerifyCmd(state),
		newGatewayCmd(state),
		newWorkspaceCmd(state),
		newDoctorCmd(state),
		newStatusCmd(state),
		newAccessCmd(state),
		newRepairCmd(state),
		newResetCmd(state),
	)

	return root, state, mock, &stdout, &stderr
}

func execAlias(t *testing.T, root *cobra.Command, stdout, stderr *bytes.Buffer, args ...string) (string, string, error) {
	t.Helper()
	stdout.Reset()
	stderr.Reset()
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// ---- flat verb tests ----

func TestFlatVerbs_List(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	out, _, err := execAlias(t, root, stdout, stderr, "list", "gastown")
	require.NoError(t, err)
	// taxiway- prefix stripped; lab name and state must appear with the table header.
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "TYPE")
	require.Contains(t, out, "STATUS")
	require.Contains(t, out, "PHASE")
	require.NotContains(t, out, "GATEWAY")
	require.Contains(t, out, "gastown")
	require.NotContains(t, out, "taxiway-gastown")
	require.Contains(t, out, "degraded")
}

func TestFlatVerbs_List_NoArg(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	// Create two labs so printList has something to show.
	createAliasLab(t, state, "gastown")
	createAliasLab(t, state, "codex")

	out, _, err := execAlias(t, root, stdout, stderr, "list")
	require.NoError(t, err)
	// Both labs must appear (without taxiway- prefix); header row present on list.
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "gastown")
	require.Contains(t, out, "codex")
	require.NotContains(t, out, "taxiway-gastown")
	require.NotContains(t, out, "taxiway-codex")
}

func TestFlatVerbs_LsAlias(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	out, _, err := execAlias(t, root, stdout, stderr, "ls")
	require.NoError(t, err)
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "gastown")
}

func TestFlatVerbs_StatusShowsTaxiwayRuntime(t *testing.T) {
	root, _, _, stdout, stderr := buildAliasTestRoot(t)

	out, _, err := execAlias(t, root, stdout, stderr, "status")
	require.NoError(t, err)
	require.Contains(t, out, "Langfuse stack:")
	require.Contains(t, out, "Proxy:")
	require.Empty(t, stderr.String())
}

func TestFlatVerbs_Rm_ClearsPhases(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createAliasLab(t, state, "gastown")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap))

	_, _, err := execAlias(t, root, stdout, stderr, "rm", "--yes", "gastown")
	require.NoError(t, err)

	// lab should be deleted
	exists, err := state.Driver.Exists(context.Background(), id)
	require.NoError(t, err)
	require.False(t, exists)

	// Phase markers should be cleared
	require.False(t, phases.Done(stateDir, id, phases.PhaseBootstrap))
}

func TestFlatVerbs_Rm_RemindsManualForkDeletion(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	id := idName("gastown")
	ref := config.LabRef{
		Lab:    "gastown",
		Orch:   "gastown",
		Driver: "mock",
		Workspace: &config.Workspace{
			Repo: "https://github.com/source/project.git",
			Fork: "https://github.com/jlrigau/project-lab-gastown.git",
		},
	}
	require.NoError(t, state.Driver.Create(context.Background(), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), id, ref))

	out, errOut, err := execAlias(t, root, stdout, stderr, "rm", "--yes", "gastown")
	require.NoError(t, err)
	require.Contains(t, out, "workspace fork that must be deleted manually")
	require.Contains(t, out, "https://github.com/jlrigau/project-lab-gastown.git")
	require.Contains(t, errOut, "Manual cleanup required")
	require.Contains(t, errOut, "https://github.com/jlrigau/project-lab-gastown")
	require.NotContains(t, errOut, "could not delete fork")
}

func TestFlatVerbs_Rm_MissingLabErrors(t *testing.T) {
	root, _, _, stdout, stderr := buildAliasTestRoot(t)

	_, errOut, err := execAlias(t, root, stdout, stderr, "rm", "missing-lab")
	require.Error(t, err)
	require.Contains(t, err.Error(), `lab "missing-lab" does not exist`)
	require.Contains(t, errOut, `lab "missing-lab" does not exist`)
	require.NotContains(t, errOut, `Lab "missing-lab" does not exist`)
	require.NotContains(t, errOut, `Deleting lab "missing-lab"`)
}

func TestFlatVerbs_Rm_MissingSidecarErrors(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	id := idName("broken-lab")
	require.NoError(t, state.Driver.Create(context.Background(), id, driver.CreateOptions{}))

	_, errOut, err := execAlias(t, root, stdout, stderr, "rm", "--yes", "broken-lab")

	require.EqualError(t, err, `lab "broken-lab" is missing ref.json`)
	require.Contains(t, errOut, `lab "broken-lab" is missing ref.json`)
	require.NotContains(t, errOut, `Deleting lab "broken-lab"`)
}

func TestFlatVerbs_Rm_LabelsLiteLLMSidecarRemoval(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createAliasLab(t, state, "codex")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseGateway))

	_, errOut, err := execAlias(t, root, stdout, stderr, "rm", "--yes", "codex")
	require.NoError(t, err)
	require.Contains(t, errOut, `Deleting lab "codex"`)
	require.Contains(t, errOut, `Stopping LiteLLM sidecar for lab "codex"`)
	require.Contains(t, errOut, `Removing LiteLLM sidecar state for lab "codex"`)
}

func TestFlatVerbs_Rm_PreparedLabSkipsGatewayCleanup(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("test-unknown")

	require.NoError(t, state.Driver.Create(context.Background(), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), id, config.LabRef{Lab: "test-unknown", Orch: "codex", Driver: "mock"}))
	markPrepareCompleted(t, stateDir, id)

	_, errOut, err := execUpRoot(t, root, stdout, stderr, "rm", "--yes", "test-unknown")
	require.NoError(t, err)
	require.Contains(t, errOut, `Deleting lab "test-unknown"`)

	exists, err := state.Driver.Exists(context.Background(), id)
	require.NoError(t, err)
	require.False(t, exists)
	require.False(t, phases.Done(stateDir, id, phases.PhaseVerify))
}

func createAliasLab(t *testing.T, state *RootState, lab string) string {
	t.Helper()
	id := idName(lab)
	require.NoError(t, state.Driver.Create(context.Background(), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), id, config.LabRef{Lab: lab, Orch: lab, Driver: "mock"}))
	return id
}

func TestFlatVerbs_Install(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "install", "gastown")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "install.sh"), "expected install.sh in ExecLog, got %v", mock.ExecLog)
}

func TestFlatVerbs_Verify(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "verify", "gastown")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "verify.sh"), "expected verify.sh in ExecLog, got %v", mock.ExecLog)
}

// TestVerify_MissingScript: taxiway verify standalone must error (with "verify.sh" in message)
// when verify.sh is absent — unlike taxiway up which skips silently.
func TestVerify_MissingScript(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "codex")

	// Remove verify.sh so it doesn't exist
	require.NoError(t, os.Remove(filepath.Join(state.RepoDir, "orchestrators", "codex", "verify.sh")))

	_, _, err := execAlias(t, root, stdout, stderr, "verify", "codex")
	require.Error(t, err, "taxiway verify must error when verify.sh is absent")
	require.Contains(t, err.Error(), "verify.sh", "error message should mention verify.sh")
}

func TestFlatVerbs_Doctor(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "doctor", "gastown")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "doctor.sh"), "expected doctor.sh in ExecLog, got %v", mock.ExecLog)
	require.NotEmpty(t, mock.ExecEnvLog)
	require.Equal(t, "gastown", mock.ExecEnvLog[len(mock.ExecEnvLog)-1]["TAXIWAY_ORCH"])
	require.NotContains(t, mock.ExecEnvLog[len(mock.ExecEnvLog)-1], "TAXIWAY_DOCTOR_FIX")
	require.NotContains(t, mock.ExecEnvLog[len(mock.ExecEnvLog)-1], "ORCH")
}

func TestFlatVerbs_DoctorFixPropagatesToDoctorScripts(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "gastown", "doctor.sh"),
		[]byte("#!/bin/bash\necho orch-doctor\n"), 0755,
	))

	_, _, err := execAlias(t, root, stdout, stderr, "doctor", "gastown", "--fix")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "doctor.sh"))
	for _, env := range mock.ExecEnvLog {
		require.Equal(t, "true", env["TAXIWAY_DOCTOR_FIX"])
	}
}

func TestFlatVerbs_DoctorRunsOrchestratorAndAgentDoctors(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	setAgents(t, state, "gastown", "claude-code", "codex")
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "gastown", "doctor.sh"),
		[]byte("#!/bin/bash\necho orch-doctor\n"), 0755,
	))
	addAgentScript(t, state, "claude-code", "doctor.sh")
	addAgentScript(t, state, "codex", "doctor.sh")

	_, _, err := execAlias(t, root, stdout, stderr, "doctor", "gastown")
	require.NoError(t, err)
	require.Equal(t, 4, countStr(mock.ExecLog, "doctor.sh"))
	require.ElementsMatch(t, []string{"claude-code", "codex"}, agentEnvValues(mock.ExecEnvLog))
}

func TestFlatVerbs_Reset_ClearsPhases(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createAliasLab(t, state, "gastown")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap))

	_, _, err := execAlias(t, root, stdout, stderr, "reset", "gastown")
	require.NoError(t, err)

	require.False(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap marker should be cleared")
}

func TestFlatVerbs_ResetYes_PassesNonInteractiveEnv(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "reset", "--yes", "gastown")
	require.NoError(t, err)

	require.NotEmpty(t, mock.ExecEnvLog)
	require.Equal(t, "1", mock.ExecEnvLog[len(mock.ExecEnvLog)-1]["LAB_RESET_YES"])
}

func TestFlatVerbs_DryRunDoesNotMarkPhases(t *testing.T) {
	tests := []struct {
		name  string
		phase phases.Phase
		args  []string
	}{
		{name: "install", phase: phases.PhaseInstall, args: []string{"install", "gastown", "--dry-run"}},
		{name: "verify", phase: phases.PhaseVerify, args: []string{"verify", "gastown", "--dry-run"}},
		{name: "gateway", phase: phases.PhaseGateway, args: []string{"gateway", "gastown", "--dry-run"}},
		{name: "workspace", phase: phases.PhaseWorkspace, args: []string{"workspace", "gastown", "--repo", "https://github.com/acme/project", "--dry-run"}},
		{name: "start", phase: phases.PhaseStart, args: []string{"start", "gastown", "--dry-run"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, state, _, stdout, stderr := buildAliasTestRoot(t)
			id := createAliasLab(t, state, "gastown")
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

			_, _, err := execAlias(t, root, stdout, stderr, tt.args...)
			require.NoError(t, err)
			require.False(t, phases.Done(stateDir, id, tt.phase), "dry-run must not mark phase %s", tt.phase)
		})
	}
}

func TestFlatVerbs_DryRunPlansInstallAndVerify(t *testing.T) {
	tests := []struct {
		command string
		label   string
		phase   phases.Phase
	}{
		{command: "install", label: "[gastown-install] Installing gastown", phase: phases.PhaseInstall},
		{command: "verify", label: "[gastown-verify] Verifying gastown", phase: phases.PhaseVerify},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			root, state, _, stdout, stderr := buildAliasTestRoot(t)
			id := createAliasLab(t, state, "gastown")
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

			out, _, err := execAlias(t, root, stdout, stderr, tt.command, "gastown", "--dry-run")

			require.NoError(t, err)
			require.Contains(t, out, fmt.Sprintf("Dry-run for phase %q on lab %q", tt.phase, "gastown"))
			require.Contains(t, out, tt.label)
			require.Contains(t, out, "No changes were made.")
			require.False(t, phases.Done(stateDir, id, tt.phase))
		})
	}
}

func TestFlatVerbs_DryRunPlansWorkspaceAndStart(t *testing.T) {
	tests := []struct {
		command string
		args    []string
		label   string
		phase   phases.Phase
	}{
		{command: "workspace", args: []string{"workspace", "gastown", "--repo", "https://github.com/acme/project", "--dry-run"}, label: "[gastown-workspace] Cloning workspace repository", phase: phases.PhaseWorkspace},
		{command: "start", args: []string{"start", "gastown", "--dry-run"}, label: "[gastown-start] Starting tmux session 'gastown'", phase: phases.PhaseStart},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			root, state, _, stdout, stderr := buildAliasTestRoot(t)
			id := createAliasLab(t, state, "gastown")
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

			out, _, err := execAlias(t, root, stdout, stderr, tt.args...)

			require.NoError(t, err)
			require.Contains(t, out, tt.label)
			require.Contains(t, out, "No changes were made.")
			require.False(t, phases.Done(stateDir, id, tt.phase))
		})
	}
}

func TestFlatVerbs_DryRunPlansGoLifecycleOperations(t *testing.T) {
	tests := []struct {
		command string
		args    []string
		labels  []string
		create  bool
	}{
		{command: "create", args: []string{"create", "demo", "--type", "gastown", "--dry-run"}, labels: []string{"Creating Mock lab runtime", "Preparing Taxiway lab state"}},
		{command: "gateway", args: []string{"gateway", "gastown", "--dry-run"}, labels: []string{"Configuring lab gateway environment", "Reconciling LiteLLM sidecar"}, create: true},
		{command: "down", args: []string{"down", "gastown", "--dry-run"}, labels: []string{"Stopping lab runtime", "Stopping LiteLLM sidecar"}, create: true},
		{command: "reset", args: []string{"reset", "gastown", "--dry-run"}, labels: []string{"Stopping workspace services", "Clearing /lab/work contents", "Clearing lifecycle phase markers"}, create: true},
		{command: "rm", args: []string{"rm", "gastown", "--dry-run"}, labels: []string{"Deleting lab runtime and storage", "Clearing lifecycle phase markers"}, create: true},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			root, state, _, stdout, stderr := buildAliasTestRoot(t)
			if tt.create {
				createAliasLab(t, state, "gastown")
			}

			out, errOut, err := execAlias(t, root, stdout, stderr, tt.args...)

			require.NoError(t, err)
			combined := out + errOut
			for _, label := range tt.labels {
				require.Contains(t, combined, label)
			}
			require.Contains(t, combined, "No changes were made.")
		})
	}
}

func TestBootstrapDryRunPrintsSemanticSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := t.TempDir()
	for _, name := range []string{"bash", "dirname", "env", "head"} {
		resolved, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(resolved, filepath.Join(binDir, name)))
	}
	t.Setenv("PATH", binDir)
	profilePath := filepath.Join(home, ".profile")
	tmuxPath := filepath.Join(home, ".tmux.conf")
	require.NoError(t, os.WriteFile(profilePath, []byte("existing profile\n"), 0o644))
	require.NoError(t, os.WriteFile(tmuxPath, []byte("existing tmux config\n"), 0o644))

	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	id := createAliasLab(t, state, "gastown")

	for _, name := range []string{"bootstrap.sh", "steps.sh"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "infra", "commands", name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(state.RepoDir, "infra", "commands", name), data, 0o755))
	}

	out, _, err := execAlias(t, root, stdout, stderr, "bootstrap", "gastown", "--dry-run")
	require.NoError(t, err)
	plainOut := strings.NewReplacer("\x1b[1;34m", "", "\x1b[0m", "").Replace(out)
	require.Contains(t, plainOut, `Dry-run for phase "bootstrap" on lab "gastown"`)
	var bootstrapSteps []string
	for _, line := range strings.Split(plainOut, "\n") {
		if step, found := strings.CutPrefix(line, "[bootstrap] "); found {
			bootstrapSteps = append(bootstrapSteps, step)
		}
	}
	require.Equal(t, []string{
		"Updating apt cache",
		"Installing base packages",
		"Installing Docker",
		"Installing Node.js 22",
		"Enabling Corepack",
		"Toolchain summary",
		"Enabling tmux mouse support in " + tmuxPath,
		"Configuring login shells to load the Taxiway environment",
	}, bootstrapSteps)
	require.Contains(t, plainOut, "java       : missing")
	require.Contains(t, plainOut, "No changes were made.")
	require.NotContains(t, plainOut, "docker: Exec")
	require.False(t, phases.Done(config.StateDir(state.Flags.StateDir, state.RepoDir), id, phases.PhaseBootstrap))
	profile, err := os.ReadFile(profilePath)
	require.NoError(t, err)
	require.Equal(t, "existing profile\n", string(profile))
	tmuxConfig, err := os.ReadFile(tmuxPath)
	require.NoError(t, err)
	require.Equal(t, "existing tmux config\n", string(tmuxConfig))
}

func TestFlatVerbs_GatewayDryRunDoesNotCreateHostState(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}

	out, _, err := execAlias(t, root, stdout, stderr, "gateway", "gastown", "--dry-run")
	require.NoError(t, err)
	require.Contains(t, out, "Configuring lab gateway environment")
	require.Contains(t, out, "Reconciling LiteLLM sidecar")
	require.Contains(t, out, "No changes were made.")
	_, err = os.Stat(labGatewayDir(stateDir, ref))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestFlatVerbs_CreateDryRunDoesNotCreateOrMark(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("preview")

	_, _, err := execAlias(t, root, stdout, stderr, "create", "preview", "--type", "gastown", "--dry-run")
	require.NoError(t, err)
	exists, err := mock.Exists(context.Background(), id)
	require.NoError(t, err)
	require.False(t, exists)
	require.False(t, phases.Done(stateDir, id, phases.PhaseCreate))
}

func TestFlatVerbs_ResetDryRunKeepsMarkersAndShowsPlan(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createAliasLab(t, state, "gastown")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))

	out, _, err := execAlias(t, root, stdout, stderr, "reset", "gastown", "--yes", "--dry-run")
	require.NoError(t, err)
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap))
	require.Contains(t, out, "Stopping workspace services")
	require.Contains(t, out, "Clearing lifecycle phase markers")
}

func TestFlatVerbs_RmDryRunKeepsLabAndMarkers(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createAliasLab(t, state, "gastown")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))

	_, _, err := execAlias(t, root, stdout, stderr, "rm", "gastown", "--yes", "--dry-run")
	require.NoError(t, err)
	exists, err := mock.Exists(context.Background(), id)
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap))
}

func TestFlatVerbs_RmDryRunDoesNotPrompt(t *testing.T) {
	root, state, _, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	out, _, err := execAlias(t, root, stdout, stderr, "rm", "gastown", "--dry-run")
	require.NoError(t, err)
	require.NotContains(t, out, "Delete lab")
	require.NotContains(t, out, "Aborted")
}

func TestHiddenNouns_HiddenInHelp(t *testing.T) {
	root, _, _, stdout, stderr := buildAliasTestRoot(t)

	out, _, err := execAlias(t, root, stdout, stderr, "--help")
	require.NoError(t, err)

	require.NotContains(t, out, "  id ", "id noun should be hidden from --help")
	require.NotContains(t, out, "  env ", "env noun should be hidden from --help")
	require.NotContains(t, out, "  orchestrator ", "orchestrator noun should be hidden from --help")
}

func TestBootstrap_CallsScript(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "bootstrap", "gastown")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "bootstrap.sh"), "expected bootstrap.sh in ExecLog, got %v", mock.ExecLog)
}

func TestStart_CallsScript(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "start", "gastown")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "start.sh"), "expected start.sh in ExecLog, got %v", mock.ExecLog)
}

func TestStart_Force(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")

	_, _, err := execAlias(t, root, stdout, stderr, "start", "gastown", "--force")
	require.NoError(t, err)
	require.True(t, containsStr(mock.ExecLog, "start.sh"), "expected start.sh in ExecLog, got %v", mock.ExecLog)

	// Find the env map recorded for the start.sh call
	var startEnv map[string]string
	for i, name := range mock.ExecLog {
		if name == "start.sh" {
			startEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, startEnv, "ExecEnvLog entry for start.sh should not be nil")
	require.Equal(t, "true", startEnv["TAXIWAY_FORCE"], "expected TAXIWAY_FORCE=true with --force")
	require.NotEmpty(t, startEnv["TAXIWAY_DASHBOARD_HOST_PORT"])
	require.NotEqual(t, "8080", startEnv["TAXIWAY_DASHBOARD_HOST_PORT"])
}

func TestStart_MissingScript(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "codex")
	_ = mock

	_, _, err := execAlias(t, root, stdout, stderr, "start", "codex")
	require.Error(t, err)
	require.Contains(t, err.Error(), "start.sh")
}
