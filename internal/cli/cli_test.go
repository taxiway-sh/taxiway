package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

// buildTestRoot creates a root cobra command backed by a MockDriver in a temp dir.
func buildTestRoot(t *testing.T) (*cobra.Command, *RootState, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()

	// Create required script dirs and orchestrator stubs
	for _, p := range []string{"infra/commands"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, p), 0755))
	}
	for _, script := range []string{"infra/commands/bootstrap.sh",
		"infra/commands/doctor.sh", "infra/commands/reset.sh"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, script), []byte("#!/bin/bash\necho ok\n"), 0755))
	}
	// Orchestrator stub so that `up gastown` can find install.sh.
	for _, orch := range []string{"gastown", "codex", "claude-code"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\necho install\n"), 0755))
	}

	stateDir := filepath.Join(tmp, ".lab-state")
	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  driver.NewMockDriver(stateDir),
	}

	var stdout, stderr bytes.Buffer

	root := &cobra.Command{
		Use:          "taxiway",
		SilenceUsage: true,
	}
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	root.AddCommand(
		newVersionCmd(state),
		newUpCmd(state),
		newDownCmd(state),
		newRmCmd(state),
		newListCmd(state),
		newCreateCmd(state),
	)

	return root, state, &stdout, &stderr
}

func execRoot(t *testing.T, root *cobra.Command, stdout, stderr *bytes.Buffer, args ...string) (string, string, error) {
	t.Helper()
	stdout.Reset()
	stderr.Reset()
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func createCLITestLab(t *testing.T, state *RootState, lab string) string {
	t.Helper()
	return createCLITestLabWithOrch(t, state, lab, lab)
}

func createCLITestLabWithOrch(t *testing.T, state *RootState, lab, orch string) string {
	t.Helper()
	id := config.IDOf(lab)
	require.NoError(t, state.Driver.Create(context.Background(), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), id, config.LabRef{
		Lab:    lab,
		Orch:   orch,
		Driver: "mock",
	}))
	return id
}

func TestRootHelpGroupsCommands(t *testing.T) {
	_, state, _, _ := buildTestRoot(t)
	var stdout, stderr bytes.Buffer
	root := &cobra.Command{Use: "taxiway", Short: "Taxiway lab operations CLI", SilenceUsage: true}
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.AddCommand(
		newUpCmd(state),
		newPrepareCmd(state),
		newRunCmd(state),
		newCreateCmd(state),
		newBootstrapCmd(state),
		newInstallCmd(state),
		newVerifyCmd(state),
		newGatewayCmd(state),
		newWorkspaceCmd(state),
		newLabAuthCmd(state),
		newStartCmd(state),
		newListCmd(state),
		newShellCmd(state),
		newDoctorCmd(state),
		newDownCmd(state),
		newRmCmd(state),
		newResetCmd(state),
		newInitCmd(state),
		newStatusCmd(state),
		newAccessCmd(state),
		newRepairCmd(state),
		newDestroyCmd(state),
		newCredentialsCmd(state),
		newObserveCmd(state),
		newVersionCmd(state),
	)
	configureCommandGroups(root)

	out, _, err := execRoot(t, root, &stdout, &stderr, "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Lifecycle:")
	require.Contains(t, out, "Lifecycle Phases:")
	require.Contains(t, out, "Labs:")
	require.Contains(t, out, "Runtime:")
	require.Contains(t, out, "Utility:")
	require.Contains(t, out, "gateway")
	require.Contains(t, out, "workspace")
	require.Less(t, strings.Index(out, "Lifecycle:"), strings.Index(out, "Lifecycle Phases:"))
	require.Less(t, strings.Index(out, "Lifecycle Phases:"), strings.Index(out, "Labs:"))
	require.Less(t, strings.Index(out, "Labs:"), strings.Index(out, "Runtime:"))
	require.Less(t, strings.Index(out, "Runtime:"), strings.Index(out, "Utility:"))
	require.Less(t, strings.Index(out, "  up "), strings.Index(out, "  prepare "))
	require.Less(t, strings.Index(out, "  prepare "), strings.Index(out, "  run "))
	require.Less(t, strings.Index(out, "  create "), strings.Index(out, "  bootstrap "))
	require.Less(t, strings.Index(out, "  bootstrap "), strings.Index(out, "  install "))
	require.Less(t, strings.Index(out, "  install "), strings.Index(out, "  verify "))
	require.Less(t, strings.Index(out, "  gateway "), strings.Index(out, "  workspace "))
	require.Less(t, strings.Index(out, "  workspace "), strings.Index(out, "  auth "))
	require.Less(t, strings.Index(out, "  auth "), strings.Index(out, "  start "))
	require.Less(t, strings.Index(out, "  shell "), strings.Index(out, "  doctor "))
	require.Less(t, strings.Index(out, "  doctor "), strings.Index(out, "  down "))
	require.Less(t, strings.Index(out, "  down "), strings.Index(out, "  rm "))
	require.Less(t, strings.Index(out, "  rm "), strings.Index(out, "  reset "))
	require.Less(t, strings.Index(out, "  reset "), strings.Index(out, "Runtime:"))
	require.Less(t, strings.Index(out, "  init "), strings.Index(out, "  status "))
	require.Less(t, strings.Index(out, "  status "), strings.Index(out, "  access "))
	require.Less(t, strings.Index(out, "  access "), strings.Index(out, "  repair "))
	require.Less(t, strings.Index(out, "  repair "), strings.Index(out, "  destroy "))
	require.Less(t, strings.Index(out, "  destroy "), strings.Index(out, "  credentials "))
	require.Less(t, strings.Index(out, "  credentials "), strings.Index(out, "  observe "))
	require.Less(t, strings.Index(out, "  observe "), strings.Index(out, "Utility:"))
	require.NotContains(t, out, "  open ")
	require.Regexp(t, `(?m)^\s+up\s+Run prepare then runtime phases$`, out)
	require.Regexp(t, `(?m)^\s+prepare\s+Run prepare phases: create, bootstrap, install, verify$`, out)
	require.Regexp(t, `(?m)^\s+run\s+Run runtime phases: gateway, workspace, auth, start$`, out)
	require.Regexp(t, `(?m)^\s+init\s+Initialize Taxiway runtime$`, out)
	require.Regexp(t, `(?m)^\s+destroy\s+Destroy Taxiway runtime$`, out)
	require.Empty(t, stderr.String())
}

// ---- id lifecycle (via flat verbs) ----

func TestLabUp_Create(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	_, errOut, err := execRoot(t, root, stdout, stderr, "create", "gastown")
	require.NoError(t, err)
	_ = errOut

	ctx := context.Background()
	running, err := state.Driver.Running(ctx, "taxiway-gastown")
	require.NoError(t, err)
	require.True(t, running)
}

func TestLabUp_AlreadyRunning(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	ctx := context.Background()

	// Pre-create
	createCLITestLabWithOrch(t, state, "gastown", "claude-code")

	_, _, err := execRoot(t, root, stdout, stderr, "create", "gastown")
	require.NoError(t, err, "create on already-running lab should succeed")

	// lab should still be running
	running, err := state.Driver.Running(ctx, "taxiway-gastown")
	require.NoError(t, err)
	require.True(t, running)
}

func TestStop(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	createCLITestLab(t, state, "gastown")

	_, _, err := execRoot(t, root, stdout, stderr, "down", "gastown")
	require.NoError(t, err)

	st, err := state.Driver.Status(context.Background(), "taxiway-gastown")
	require.NoError(t, err)
	require.Equal(t, "stopped", st.State)
}

func TestDelete(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	createCLITestLab(t, state, "gastown")

	_, _, err := execRoot(t, root, stdout, stderr, "rm", "--yes", "gastown")
	require.NoError(t, err)

	exists, err := state.Driver.Exists(context.Background(), "taxiway-gastown")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestList_Single(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	createCLITestLab(t, state, "gastown")

	out, _, err := execRoot(t, root, stdout, stderr, "list", "gastown")
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

func TestList_SingleShowsPhaseLabel(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createCLITestLab(t, state, "gastown")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseCreate))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseStart))

	out, _, err := execRoot(t, root, stdout, stderr, "list", "gastown")
	require.NoError(t, err)
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "TYPE")
	require.Contains(t, out, "STATUS")
	require.Contains(t, out, "PHASE")
	require.Contains(t, out, "degraded")
	require.Contains(t, out, "started")
}

func TestList_IncludesUnifiedRunningStatusWithoutGatewayDetails(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	proxyDir := filepath.Join(t.TempDir(), ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	id := createCLITestLabWithOrch(t, state, "test-observe", "codex")
	ref := config.LabRef{Lab: "test-observe", Orch: "codex", Driver: "mock"}
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseGateway))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseStart))
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{labLiteLLMAPIKeyEnv: "sk-test"}))
	require.NoError(t, os.WriteFile(filepath.Join(labGatewayDir(stateDir, ref), "litellm.compose.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, writeLabLiteLLMRoute(stateDir, ref, labLiteLLMRoute{
		Lab:     "test-observe",
		Service: "taxiway-dev-a1b2c3d4-test-observe-gateway-litellm-1",
		Host:    "test-observe.litellm.localhost",
		Project: "taxiway-dev-a1b2c3d4-test-observe-gateway",
	}))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "ps" ]; then
  printf 'taxiway-dev-a1b2c3d4-test-observe-gateway-litellm-1\trunning\tUp 1 minute\n'
  printf 'taxiway-dev-a1b2c3d4-test-observe-gateway-postgres-1\trunning\tUp 1 minute\n'
  exit 0
fi
exit 0
`)

	out, _, err := execRoot(t, root, stdout, stderr, "list")

	require.NoError(t, err)
	require.Contains(t, out, "STATUS")
	require.NotContains(t, out, "GATEWAY")
	require.NotContains(t, out, "ACCESS")
	require.Contains(t, out, "test-observe")
	require.Contains(t, out, "running")
	require.NotContains(t, out, "http://test-observe.litellm.localhost:55124")
	require.NotContains(t, out, "SHELL")
	require.NotContains(t, out, "taxiway shell test-observe")
}

func TestList_MarksLabDegradedUntilStartEvenWhenGatewayIsRunning(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	proxyDir := filepath.Join(t.TempDir(), ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	id := createCLITestLabWithOrch(t, state, "test-observe", "codex")
	ref := config.LabRef{Lab: "test-observe", Orch: "codex", Driver: "mock"}
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseGateway))
	configureRunningTestGateway(t, stateDir, proxyDir, ref)

	out, _, err := execRoot(t, root, stdout, stderr, "list")

	require.NoError(t, err)
	require.Contains(t, out, "test-observe")
	require.Contains(t, out, "degraded")
	require.Contains(t, out, "gateway ready")
	require.NotContains(t, out, "http://test-observe.litellm.localhost:55124")
}

func TestList_MarksLabDegradedWhenGatewayIsMissing(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	createCLITestLab(t, state, "gastown")

	out, _, err := execRoot(t, root, stdout, stderr, "list")

	require.NoError(t, err)
	require.Contains(t, out, "STATUS")
	require.NotContains(t, out, "GATEWAY")
	require.Contains(t, out, "gastown")
	require.Contains(t, out, "degraded")
	require.NotContains(t, out, "not configured")
}

func configureRunningTestGateway(t *testing.T, stateDir, proxyDir string, ref config.LabRef) {
	t.Helper()
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{labLiteLLMAPIKeyEnv: "sk-test"}))
	require.NoError(t, os.WriteFile(filepath.Join(labGatewayDir(stateDir, ref), "litellm.compose.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, writeLabLiteLLMRoute(stateDir, ref, labLiteLLMRoute{
		Lab:     ref.Lab,
		Service: "taxiway-dev-a1b2c3d4-" + ref.Lab + "-gateway-litellm-1",
		Host:    ref.Lab + ".litellm.localhost",
		Project: "taxiway-dev-a1b2c3d4-" + ref.Lab + "-gateway",
	}))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "ps" ]; then
  printf 'taxiway-dev-a1b2c3d4-test-observe-gateway-litellm-1\trunning\tUp 1 minute\n'
  printf 'taxiway-dev-a1b2c3d4-test-observe-gateway-postgres-1\trunning\tUp 1 minute\n'
  exit 0
fi
exit 0
`)
}

func TestList_ListAll(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	createCLITestLab(t, state, "gastown")
	createCLITestLab(t, state, "codex")

	out, _, err := execRoot(t, root, stdout, stderr, "list")
	require.NoError(t, err)
	// taxiway- prefix stripped; header row present on list
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "TYPE")
	require.Contains(t, out, "STATUS")
	require.Contains(t, out, "PHASE")
	require.Contains(t, out, "gastown")
	require.Contains(t, out, "codex")
	require.NotContains(t, out, "taxiway-gastown")
	require.NotContains(t, out, "taxiway-codex")
}

func TestListOneFindsRuntimeScopedLabName(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "e2e")
	t.Setenv("TAXIWAY_CONTEXT_ID", "abcd1234")
	root, state, stdout, stderr := buildTestRoot(t)
	lab := "e2e-abcd1234-codex-phase-by-phase"
	createCLITestLabWithOrch(t, state, lab, "codex")

	out, _, err := execRoot(t, root, stdout, stderr, "list", lab)

	require.NoError(t, err)
	require.Contains(t, out, lab)
	require.Contains(t, out, "codex")
	require.Contains(t, out, "degraded")
}

func TestList_ListAllAlignsLongLabNames(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	ctx := context.Background()
	id := "taxiway-test-witness-with-sonnet"
	require.NoError(t, state.Driver.Create(ctx, id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(ctx, id, config.LabRef{
		Lab:    "test-witness-with-sonnet",
		Orch:   "gastown",
		Driver: "mock",
	}))

	out, _, err := execRoot(t, root, stdout, stderr, "list")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	headerTypeIdx := strings.Index(lines[0], "TYPE")
	rowTypeIdx := strings.Index(lines[1], "gastown")
	require.NotEqual(t, -1, headerTypeIdx)
	require.Equal(t, headerTypeIdx, rowTypeIdx, out)

	require.NotContains(t, lines[0], "SHELL")
	require.NotContains(t, lines[0], "GATEWAY")
	require.NotContains(t, lines[0], "ACCESS")
	for _, column := range []string{"STATUS", "PHASE", "DRIVER", "CREATED"} {
		headerIdx := strings.Index(lines[0], column)
		require.NotEqual(t, -1, headerIdx)
		rowIdx := firstNonSpaceAtOrAfter(lines[1], headerIdx)
		require.Equal(t, headerIdx, rowIdx, out)
	}
}

func TestList_ShowsDriverStateAndInitializedPhaseBeforeCreateDone(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	lab := "pending-lab"
	labDir := filepath.Join(stateDir, lab)
	require.NoError(t, os.MkdirAll(labDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(labDir, "state"), []byte("running"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(labDir, "created_at"), []byte("2026-05-26T10:00:00Z\n"), 0644))
	require.NoError(t, config.WriteLabRef(stateDir, lab, config.LabRef{
		Lab:    lab,
		Orch:   "gastown",
		Driver: "mock",
	}))

	out, _, err := execRoot(t, root, stdout, stderr, "list")
	require.NoError(t, err)
	require.Contains(t, out, "pending-lab")
	require.Contains(t, out, "gastown")
	require.Contains(t, out, "degraded")
	require.Contains(t, out, "initialized")
	require.Contains(t, out, "2026-05-26T10:00")
}

func TestList_ErrorsWhenLabSidecarIsMissing(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	lab := "broken-lab"
	labDir := filepath.Join(stateDir, lab)
	require.NoError(t, os.MkdirAll(labDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(labDir, "created_at"), []byte("2026-05-26T10:00:00Z\n"), 0644))

	_, _, err := execRoot(t, root, stdout, stderr, "list")

	require.EqualError(t, err, `lab "broken-lab" is missing ref.json`)
}

func TestList_ShowsLatestCompletedPhaseLabel(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := createCLITestLab(t, state, "gastown")
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseCreate))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseWorkspace))

	out, _, err := execRoot(t, root, stdout, stderr, "list")
	require.NoError(t, err)
	require.Contains(t, out, "PHASE")
	require.Contains(t, out, "workspace created")
}

func TestList(t *testing.T) {
	root, state, stdout, stderr := buildTestRoot(t)
	createCLITestLab(t, state, "gastown")

	out, _, err := execRoot(t, root, stdout, stderr, "list")
	require.NoError(t, err)
	// taxiway- prefix stripped; header row present on list
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "PHASE")
	require.Contains(t, out, "gastown")
	require.NotContains(t, out, "taxiway-gastown")
}

// ---- validation ----

func TestValidation_InvalidOrchName(t *testing.T) {
	root, _, stdout, stderr := buildTestRoot(t)
	_, _, err := execRoot(t, root, stdout, stderr, "up", "../evil")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "invalid")
}

func TestValidation_EmptyOrchName(t *testing.T) {
	root, _, stdout, stderr := buildTestRoot(t)
	_, _, err := execRoot(t, root, stdout, stderr, "up", "")
	require.Error(t, err)
}

// ---- dry-run ----

func TestDryRun_LabUp(t *testing.T) {
	tmp := t.TempDir()
	for _, orch := range []string{"gastown", "claude-code"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\n"), 0755))
	}
	stateDir := filepath.Join(tmp, ".lab-state")
	innerDriver := driver.NewMockDriver(stateDir)
	dryDriver := driver.NewDryRun(innerDriver)

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: true, StateDir: stateDir},
		Driver:  dryDriver,
	}
	root := &cobra.Command{Use: "taxiway", SilenceUsage: true}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.AddCommand(newUpCmd(state))

	root.SetArgs([]string{"up", "gastown"})
	require.NoError(t, root.Execute())

	// dry-run: lab should NOT exist in state
	ctx := context.Background()
	exists, err := innerDriver.Exists(ctx, "taxiway-gastown")
	require.NoError(t, err)
	require.False(t, exists, "dry-run must not create lab")
}

// ---- driver --driver flag ----

func TestDriverFlag_UnknownDriver(t *testing.T) {
	state := &RootState{
		RepoDir: t.TempDir(),
		Flags:   GlobalFlags{DriverName: "nonexistent"},
	}
	err := initDriver(state)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown driver")
}

func TestDriverAutoSelectsDockerWhenLimaIsUnavailable(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	require.NoError(t, os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0755))
	t.Setenv("PATH", binDir)
	t.Setenv("LAB_DRIVER", "")

	name, err := selectDriverName("")
	require.NoError(t, err)
	require.Equal(t, "docker", name)
}

func TestDriverAutoErrorsWhenNoUserDriverIsAvailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LAB_DRIVER", "")

	_, err := selectDriverName("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "install Lima or Docker")
}

func TestDriverFlag_LimaWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	state := &RootState{
		RepoDir: t.TempDir(),
		Flags:   GlobalFlags{DriverName: "lima"},
	}
	err := initDriver(state)
	require.Error(t, err)
	require.Contains(t, err.Error(), "limactl")
}

func TestDriverFlag_DockerWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	state := &RootState{
		RepoDir: t.TempDir(),
		Flags:   GlobalFlags{DriverName: "docker"},
	}
	err := initDriver(state)
	require.Error(t, err)
	require.Contains(t, err.Error(), "docker")
}

func firstNonSpaceAtOrAfter(line string, start int) int {
	for i := start; i < len(line); i++ {
		if line[i] != ' ' {
			return i
		}
	}
	return -1
}
