package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/discover"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/envfile"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

// buildUpTestRoot builds a cobra root with all commands including up/down/shell,
// backed by a MockDriver in a temp directory. Returns root, state (with typed
// MockDriver accessible), and stdout/stderr buffers.
func buildUpTestRoot(t *testing.T) (*cobra.Command, *RootState, *driver.MockDriver, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	installFakeWorkspaceGit(t, nil)

	// Minimal orchestrators/ structure
	for _, orch := range []string{"claude-code", "codex", "gastown"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\necho install\n"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "verify.sh"), []byte("#!/bin/bash\necho verify\n"), 0755))
	}
	// gastown also has start.sh
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "orchestrators", "gastown", "start.sh"), []byte("#!/bin/bash\necho start\n"), 0755))

	// Required scripts
	for _, p := range []string{"infra/commands"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, p), 0755))
	}
	for _, script := range []string{
		"infra/commands/bootstrap.sh",
		"infra/commands/doctor.sh",
		"infra/commands/reset.sh",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, script), []byte("#!/bin/bash\necho ok\n"), 0755))
	}
	gatewayDir := filepath.Join(tmp, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(gatewayDir, "litellm"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(gatewayDir, "litellm", "models.yaml"), []byte(`models:
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
    forward_client_headers: true
`), 0644))

	stateDir := filepath.Join(tmp, ".ID-state")
	mock := driver.NewMockDriver(stateDir)
	mock.RepoDir = tmp // allow Exec to translate /lab/ lab paths to real paths

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  mock,
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
		newPrepareCmd(state),
		newDownCmd(state),
		newShellCmd(state),
		newCreateCmd(state),
		newRmCmd(state),
		newListCmd(state),
		newCredentialsCmd(state),
		newLabAuthCmd(state),
		newGatewayCmd(state),
		newWorkspaceCmd(state),
		newStartCmd(state),
		newInstallCmd(state),
		newVerifyCmd(state),
		newBootstrapCmd(state),
		newResetCmd(state),
		newRunCmd(state),
		newDescribeCmd(state),
	)

	return root, state, mock, &stdout, &stderr
}

type namedTestDriver struct {
	*driver.MockDriver
	name        string
	exists      *bool
	createCalls []string
	startCalls  []string
}

func (d *namedTestDriver) Name() string {
	return d.name
}

func (d *namedTestDriver) Exists(ctx context.Context, id string) (bool, error) {
	if d.exists != nil {
		return *d.exists, nil
	}
	return d.MockDriver.Exists(ctx, id)
}

func (d *namedTestDriver) Create(ctx context.Context, id string, opts driver.CreateOptions) error {
	d.createCalls = append(d.createCalls, id)
	return d.MockDriver.Create(ctx, id, opts)
}

func (d *namedTestDriver) Start(ctx context.Context, id string) error {
	d.startCalls = append(d.startCalls, id)
	return d.MockDriver.Start(ctx, id)
}

func installFakeWorkspaceGit(t *testing.T, commands *[]string) {
	t.Helper()
	origRunWorkspaceGit := runWorkspaceGit
	runWorkspaceGit = func(_ context.Context, dir string, args ...string) error {
		if commands != nil {
			*commands = append(*commands, strings.TrimSpace(dir+" git "+strings.Join(args, " ")))
		}
		if len(args) >= 4 && args[0] == "clone" && args[1] == "--mirror" {
			require.NoError(t, os.MkdirAll(filepath.Join(args[3], "objects"), 0o755))
		}
		if len(args) >= 5 && args[0] == "-c" && args[2] == "clone" {
			require.NoError(t, os.MkdirAll(filepath.Join(args[4], ".git"), 0o755))
		}
		return nil
	}
	t.Cleanup(func() { runWorkspaceGit = origRunWorkspaceGit })
}

func installFakeDockerForStoppedLab(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
case "$1" in
  inspect)
    case "$*" in
      *'.State.Running'*)
        printf 'false\n'
        ;;
      *)
        printf 'taxiway-gastown\n'
        ;;
    esac
    exit 0
    ;;
  start)
    exit 0
    ;;
esac
printf 'unexpected docker invocation: %%s\n' "$*" >&2
exit 1
`, strconv.Quote(logPath))
	path := filepath.Join(binDir, "docker")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func addAgentScript(t *testing.T, state *RootState, agent, script string) {
	t.Helper()
	dir := filepath.Join(state.RepoDir, "agents", agent)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, script),
		[]byte("#!/bin/bash\necho agent-"+script+"\n"), 0755,
	))
}

func addAuthScript(t *testing.T, state *RootState, agent string) {
	t.Helper()
	addAgentScript(t, state, agent, "auth.sh")
}

func writeAgentManifest(t *testing.T, state *RootState, agent, content string) {
	t.Helper()
	dir := filepath.Join(state.RepoDir, "agents", agent)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0644))
}

func addAgentLifecycleScripts(t *testing.T, state *RootState, agent string) {
	t.Helper()
	addAgentScript(t, state, agent, "install.sh")
	addAgentScript(t, state, agent, "verify.sh")
	addAgentScript(t, state, agent, "auth.sh")
}

func addOrchestratorProfile(t *testing.T, state *RootState, orch, profile string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(state.RepoDir, "orchestrators", orch, "profiles", profile, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}
}

func setAgents(t *testing.T, state *RootState, orch string, agents ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("name: " + orch + "\n")
	b.WriteString("agents:\n")
	for _, agent := range agents {
		b.WriteString("  - " + agent + "\n")
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", orch, "manifest.yaml"),
		[]byte(b.String()), 0644,
	))
}

func setManifest(t *testing.T, state *RootState, orch, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", orch, "manifest.yaml"),
		[]byte(content), 0644,
	))
}

func execUpRoot(t *testing.T, root *cobra.Command, stdout, stderr *bytes.Buffer, args ...string) (string, string, error) {
	t.Helper()
	stdout.Reset()
	stderr.Reset()
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func markPrepareCompleted(t *testing.T, stateDir, id string) {
	t.Helper()
	for _, phase := range []phases.Phase{
		phases.PhaseCreate,
		phases.PhaseBootstrap,
		phases.PhaseInstall,
		phases.PhaseVerify,
	} {
		require.NoError(t, phases.Mark(stateDir, id, phase))
	}
}

func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

func agentEnvValues(logs []map[string]string) []string {
	var values []string
	for _, env := range logs {
		if agent := env["TAXIWAY_AGENT"]; agent != "" {
			values = append(values, agent)
		}
	}
	return values
}

// ---- taxiway up tests ----

// TestUp_FreshRun: all 5 pipeline phases run and markers are written.
// doctor is NOT a pipeline phase and must never be invoked by up.
func TestUp_FreshRun(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	out, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err, "expected no error\nstdout: %s", out)

	// Workspace is skipped without a configured repo; all other pipeline phase markers should be present.
	for _, p := range phases.Order {
		if p == phases.PhaseWorkspace {
			require.False(t, phases.Done(stateDir, id, p), "expected workspace to stay unmarked without a repo")
			continue
		}
		require.True(t, phases.Done(stateDir, id, p), "expected phase %s to be marked", p)
	}

	// Exec should have been called for bootstrap, install, verify, start
	// lab-doctor.sh must NOT be called by the pipeline (doctor is standalone)
	expectedScripts := []string{"bootstrap.sh", "install.sh", "verify.sh", "start.sh"}
	for _, script := range expectedScripts {
		require.True(t, containsStr(mock.ExecLog, script), "expected Exec called with %s, got %v", script, mock.ExecLog)
	}
	require.False(t, containsStr(mock.ExecLog, "lab-doctor.sh"), "lab-doctor.sh must not be invoked by up")
	require.Empty(t, mock.InteractiveExecLog, "auth must not run by default")
}

func TestUp_NoProfileLeavesOrchestratorDefaults(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Nil(t, ref.OrchestratorProfile)
	for _, cp := range mock.CopyLog {
		require.NotEqual(t, "/lab/orchestrator-profile", cp.Dst)
	}
	for _, env := range mock.ExecEnvLog {
		require.NotContains(t, env, "TAXIWAY_ORCH_PROFILE_NAME")
		require.NotContains(t, env, "TAXIWAY_ORCH_PROFILE_DIR")
		require.NotContains(t, env, "TAXIWAY_ORCH_PROFILE_CLEAR")
	}
}

func TestUp_ProfilePersistsAndInjectsRuntimeEnv(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	addOrchestratorProfile(t, state, "gastown", "budget", map[string]string{
		"settings/config.json": `{"type":"town-settings","default_agent":"claude-sonnet"}`,
	})

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--profile", "budget",
	)
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, ref.OrchestratorProfile)
	require.Equal(t, "budget", ref.OrchestratorProfile.Name)

	_, err = os.Stat(filepath.Join(state.Flags.StateDir, "gastown", "orchestrator-profile"))
	require.True(t, os.IsNotExist(err), "profile content must not be snapshotted into .ID-state")

	var profileCopy *driver.MockCopyCall
	for i := range mock.CopyLog {
		if mock.CopyLog[i].Dst == "/lab/orchestrator-profile" {
			profileCopy = &mock.CopyLog[i]
			break
		}
	}
	require.NotNil(t, profileCopy)
	require.Equal(t, idName("gastown"), profileCopy.ID)
	require.Equal(t, filepath.Join(state.RepoDir, "orchestrators", "gastown", "profiles", "budget"), profileCopy.Src)

	var startEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "start.sh" {
			startEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, startEnv)
	require.Equal(t, "budget", startEnv["TAXIWAY_ORCH_PROFILE_NAME"])
	require.Equal(t, "/lab/orchestrator-profile", startEnv["TAXIWAY_ORCH_PROFILE_DIR"])
	require.NotContains(t, startEnv, "TAXIWAY_ORCH_PROFILE_CLEAR")
}

func TestUp_SetPersistsAndInjectsEnv(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--set", "version=1.1.0",
	)
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{"version": "1.1.0"}, ref.Settings)

	var installEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "install.sh" {
			installEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, installEnv)
	require.Equal(t, "1.1.0", installEnv["TAXIWAY_SET_VERSION"])
}

func TestInstall_ReusesPersistedSet(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)
	ref := config.LabRef{
		Lab:      "gastown",
		Orch:     "gastown",
		Driver:   "mock",
		Settings: map[string]string{"version": "1.1.0"},
	}
	require.NoError(t, mock.Create(context.Background(), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, mock.WriteLabRef(context.Background(), idName("gastown"), ref))

	_, _, err := execUpRoot(t, root, stdout, stderr, "install", "gastown")
	require.NoError(t, err)

	var installEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "install.sh" {
			installEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, installEnv)
	require.Equal(t, "1.1.0", installEnv["TAXIWAY_SET_VERSION"])
}

func TestUp_SetChangeRerunsCachedInstall(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	require.NoError(t, mock.Create(context.Background(), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, mock.WriteLabRef(context.Background(), idName("gastown"), ref))
	require.NoError(t, phases.Mark(stateDir, idName("gastown"), phases.PhaseCreate))
	require.NoError(t, phases.Mark(stateDir, idName("gastown"), phases.PhaseBootstrap))
	require.NoError(t, phases.Mark(stateDir, idName("gastown"), phases.PhaseInstall))

	out, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--set", "version=1.1.0", "--prepare-only",
	)
	require.NoError(t, err)
	require.Contains(t, mock.ExecLog, "install.sh")
	require.NotContains(t, out, "install              (cached)")
}

func TestSetRejectsNormalisedCollision(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--set", "foo-bar=1", "--set", "foo.bar=2",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "normalizes to TAXIWAY_SET_FOO_BAR")
}

func TestUp_ProfileMissingErrorsClearly(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--profile", "missing",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "orchestrator profile \"missing\"")
	require.Contains(t, err.Error(), "orchestrators/gastown/profiles/missing")
}

func TestRunNoProfileClearsPersistedProfile(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	addOrchestratorProfile(t, state, "gastown", "budget", map[string]string{
		"settings/config.json": `{"type":"town-settings","default_agent":"claude-sonnet"}`,
	})
	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--profile", "budget",
	)
	require.NoError(t, err)

	_, _, err = execUpRoot(t, root, stdout, stderr,
		"run", "gastown", "--force", "--no-profile",
	)
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Nil(t, ref.OrchestratorProfile)

	var startEnv map[string]string
	for i := len(mock.ExecLog) - 1; i >= 0; i-- {
		if mock.ExecLog[i] == "start.sh" {
			startEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, startEnv)
	require.Equal(t, "true", startEnv["TAXIWAY_ORCH_PROFILE_CLEAR"])
	require.NotContains(t, startEnv, "TAXIWAY_ORCH_PROFILE_NAME")
	require.NotContains(t, startEnv, "TAXIWAY_ORCH_PROFILE_DIR")
}

func TestPrepareProfilePreservesExistingWorkspace(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	addOrchestratorProfile(t, state, "gastown", "budget", map[string]string{
		"settings/config.json": `{"type":"town-settings","default_agent":"claude-sonnet"}`,
	})

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown",
		"--repo", "https://github.com/org/repo.git",
		"--prepare-only",
	)
	require.NoError(t, err)

	_, _, err = execUpRoot(t, root, stdout, stderr,
		"prepare", "gastown", "--type", "gastown", "--profile", "budget",
	)
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, ref.Workspace)
	require.Equal(t, "https://github.com/org/repo.git", ref.Workspace.Repo)
	require.NotNil(t, ref.OrchestratorProfile)
	require.Equal(t, "budget", ref.OrchestratorProfile.Name)
}

func TestUp_AuthRunsBeforeStartByDefault(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")
	setAgents(t, state, "gastown", "claude-code")
	addAgentLifecycleScripts(t, state, "claude-code")

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown",
	)
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 1)
	require.Contains(t, strings.Join(mock.InteractiveExecLog[0].Argv, " "), "auth.sh")
	require.Contains(t, strings.Join(mock.InteractiveExecLog[0].Argv, " "), "TAXIWAY_AGENT=claude-code")
	require.Equal(t, LabWorkRoot, mock.InteractiveExecLog[0].Workdir)

	authOrder := indexOf(mock.CallLog, "interactive:auth.sh")
	startOrder := indexOf(mock.CallLog, "exec:start.sh")
	require.NotEqual(t, -1, authOrder)
	require.NotEqual(t, -1, startOrder)
	require.Less(t, authOrder, startOrder)
	require.True(t, phases.Done(stateDir, id, phases.PhaseAuth), "auth phase should be marked")
}

func TestUp_AuthRunsMultipleAgentsBeforeStartByDefault(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	setAgents(t, state, "gastown", "claude-code", "codex")
	addAgentLifecycleScripts(t, state, "claude-code")
	addAgentLifecycleScripts(t, state, "codex")

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown",
	)
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 2)
	require.Contains(t, strings.Join(mock.InteractiveExecLog[0].Argv, " "), "TAXIWAY_AGENT=claude-code")
	require.Contains(t, strings.Join(mock.InteractiveExecLog[1].Argv, " "), "TAXIWAY_AGENT=codex")

	firstAuthOrder := indexOf(mock.CallLog, "interactive:auth.sh")
	startOrder := indexOf(mock.CallLog, "exec:start.sh")
	require.NotEqual(t, -1, firstAuthOrder)
	require.NotEqual(t, -1, startOrder)
	require.Less(t, firstAuthOrder, startOrder)
}

func TestUp_AuthRunsEvenWhenAuthPhaseCached(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	setAgents(t, state, "gastown", "claude-code")
	addAgentLifecycleScripts(t, state, "claude-code")

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown",
	)
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 1)
	require.True(t, phases.Done(config.StateDir(state.Flags.StateDir, state.RepoDir), idName("gastown"), phases.PhaseAuth))

	_, _, err = execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown",
	)
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 2)
	require.Contains(t, strings.Join(mock.InteractiveExecLog[1].Argv, " "), "auth.sh")
}

func TestUp_SkipAuthCheckSkipsAuthPhase(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")
	setAgents(t, state, "gastown", "claude-code")
	addAgentLifecycleScripts(t, state, "claude-code")

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--skip-auth-check",
	)
	require.NoError(t, err)
	require.Empty(t, mock.InteractiveExecLog)
	require.False(t, phases.Done(stateDir, id, phases.PhaseAuth), "auth phase should not be marked when skipped")
	require.True(t, phases.Done(stateDir, id, phases.PhaseStart), "start phase should still run")
}

func TestUp_InteractiveAuthFlagRemoved(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--interactive-auth",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown flag")
}

func TestUp_SkipStartFlagRemoved(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--skip-start",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown flag")
}

func TestUp_HelpDescribesAuthAndAvoidsDuplicateTypeDefault(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	out, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "test-claude-auth", "--type", "claude-code", "--help",
	)
	require.NoError(t, err)
	require.Contains(t, out, "Usage:\n  taxiway up <lab> [--type <orch>] [flags]")
	require.Contains(t, out, "prepare: create -> bootstrap -> install -> verify")
	require.Contains(t, out, "run:     gateway -> workspace -> auth -> start")
	require.Contains(t, out, "--skip-gateway")
	require.Contains(t, out, "By default, taxiway up runs prepare then run.")
	require.Contains(t, out, "--type <orch>   required when creating a new lab (defaults to claude-code)")
	require.Contains(t, out, "ignored when resuming — type is read from ref.json instead")
	require.Contains(t, out, "--prepare-only")
	require.Contains(t, out, "--skip-auth-check")
	require.Contains(t, out, "skip declared agent authentication checks before start")
	require.Contains(t, out, "--profile <name>")
	require.Contains(t, out, "orchestrators/<orch>/profiles/<name>")
	require.Contains(t, out, "--no-profile")
	require.NotContains(t, out, "--skip-start")
	require.NotContains(t, out, "(default: claude-code) (default \"claude-code\")")
	require.Empty(t, stderr.String())
}

func TestLabCreatingCommandsHelpDescribeLabNameLimit(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)
	expected := "Lab names must be 48 characters or fewer and contain only letters, numbers, dashes, or underscores."

	for _, args := range [][]string{
		{"up", "--help"},
		{"prepare", "--help"},
		{"create", "--help"},
	} {
		out, _, err := execUpRoot(t, root, stdout, stderr, args...)
		require.NoError(t, err)
		require.Contains(t, out, expected, args)
	}
}

func TestUp_InstallAndVerifyRunDeclaredAgentLifecycle(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	setAgents(t, state, "gastown", "claude-code", "codex")
	addAgentLifecycleScripts(t, state, "claude-code")
	addAgentLifecycleScripts(t, state, "codex")

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown", "--prepare-only",
	)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"claude-code", "codex", "claude-code", "codex"}, agentEnvValues(mock.ExecEnvLog))
}

// TestUp_DoctorNotInPipeline: up never calls lab-doctor.sh regardless of state.
func TestUp_DoctorNotInPipeline(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	require.False(t, containsStr(mock.ExecLog, "lab-doctor.sh"),
		"lab-doctor.sh must never be invoked by the up pipeline, got ExecLog: %v", mock.ExecLog)
}

// TestUp_Cached: second run should not re-execute Exec.
func TestUp_Cached(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)

	// First run
	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	firstExecCount := len(mock.ExecLog)
	require.Greater(t, firstExecCount, 0, "first run should have executed scripts")

	// Second run
	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	// No additional Exec calls
	require.Equal(t, firstExecCount, len(mock.ExecLog), "second run must not re-execute scripts")

	// Output should mention cached
	out := stdout.String()
	require.Contains(t, out, "cached", "second run output should mention cached phases")
}

func TestUp_RestartsStoppedCachedLab(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	id := idName("gastown")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)
	firstStartCount := countStr(mock.ExecLog, "start.sh")
	require.Equal(t, 1, firstStartCount)

	_, _, err = execUpRoot(t, root, stdout, stderr, "down", "gastown")
	require.NoError(t, err)
	st, err := state.Driver.Status(testCtx(t), id)
	require.NoError(t, err)
	require.Equal(t, "stopped", st.State)

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	st, err = state.Driver.Status(testCtx(t), id)
	require.NoError(t, err)
	require.Equal(t, "running", st.State)
	require.Equal(t, firstStartCount+1, countStr(mock.ExecLog, "start.sh"))
}

func TestUp_RestartsSidecarWhenResumingStoppedCachedLab(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	var ensured []config.LabRef
	orig := ensureLabLiteLLMSidecarForUp
	ensureLabLiteLLMSidecarForUp = func(_ context.Context, _ *RootState, ref config.LabRef) error {
		ensured = append(ensured, ref)
		return nil
	}
	t.Cleanup(func() { ensureLabLiteLLMSidecarForUp = orig })

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)
	require.Len(t, ensured, 1)

	_, _, err = execUpRoot(t, root, stdout, stderr, "down", "gastown")
	require.NoError(t, err)

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	require.Len(t, ensured, 2)
	require.Equal(t, "gastown", ensured[1].Lab)
}

func TestUp_ReconcilesSidecarWhenGatewayCached(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	var ensured []config.LabRef
	orig := ensureLabLiteLLMSidecarForUp
	ensureLabLiteLLMSidecarForUp = func(_ context.Context, _ *RootState, ref config.LabRef) error {
		ensured = append(ensured, ref)
		return nil
	}
	t.Cleanup(func() { ensureLabLiteLLMSidecarForUp = orig })

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)
	require.Len(t, ensured, 1)

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	require.Len(t, ensured, 2)
	require.Equal(t, "gastown", ensured[1].Lab)
}

// TestUp_Force: --force re-runs all phases on the second run.
func TestUp_Force(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)

	// First run (normal)
	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	firstExecCount := len(mock.ExecLog)

	// Second run with --force
	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown", "--force")
	require.NoError(t, err)

	// Should have executed scripts again (at least as many as the first run minus create phase which uses Create/Start)
	require.Greater(t, len(mock.ExecLog), firstExecCount, "--force should re-execute scripts")
}

// TestUp_From: --from install skips create phase and bootstrap; runs install, verify, start.
func TestUp_From(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	// Pre-mark create phase and bootstrap so the lab exists and the phase is done
	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseCreate))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))

	// Run from install
	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown", "--from", "install")
	require.NoError(t, err)

	// install and verify should have been executed; lab-doctor.sh must not
	require.True(t, containsStr(mock.ExecLog, "install.sh"), "install should run")
	require.True(t, containsStr(mock.ExecLog, "verify.sh"), "verify should run")
	require.False(t, containsStr(mock.ExecLog, "lab-doctor.sh"), "lab-doctor.sh must not run")

	// Markers for install, verify, and start should be written
	for _, p := range []phases.Phase{phases.PhaseInstall, phases.PhaseVerify, phases.PhaseStart} {
		require.True(t, phases.Done(stateDir, id, p), "phase %s should be marked", p)
	}
}

func TestUp_FromStartReusesExistingTypeWhenTypeOmitted(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{
		Lab:    "gastown",
		Orch:   "gastown",
		Driver: "mock",
	}))

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--from", "start", "--force")
	require.NoError(t, err)
	require.Equal(t, 1, countStr(mock.ExecLog, "start.sh"))
	require.NotContains(t, stderr.String(), "requested \"claude-code\"")

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "codex", "--from", "start", "--force")
	require.Error(t, err)
	require.Contains(t, err.Error(), "requested \"codex\"")
	require.Contains(t, stderr.String(), "Omit --type to re-use the existing lab")
}

func TestUp_FromStartFailsBeforeDriverWhenCreateMissing(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown", "--from", "start", "--force")

	require.Error(t, err)
	require.Contains(t, err.Error(), `cannot resume from phase "start"`)
	require.Contains(t, err.Error(), `missing required earlier phase "create"`)
	require.Contains(t, err.Error(), `taxiway up gastown --type gastown --force`)
	require.Empty(t, mock.ExecLog)
}

func TestResumePreflightUsesRecordedDriver(t *testing.T) {
	_, state, mock, _, _ := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "docker"}

	selectedExists := false
	state.Driver = &namedTestDriver{MockDriver: mock, name: "lima", exists: &selectedExists}
	_ = installFakeDockerForStoppedLab(t)

	err := preflightResumePrerequisites(testCtx(t), state, id, stateDir, ref, runUpOpts{from: phases.PhaseGateway}, 4)
	require.NoError(t, err)
}

// TestUp_FailureMidway: if install fails, its marker is absent; bootstrap marker is present.
func TestUp_FailureMidway(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	// Inject failure for install.sh
	mock.FailExec["install.sh"] = errors.New("injected install failure")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.Error(t, err, "expected error when install fails")
	require.True(t, strings.Contains(err.Error(), "install") || strings.Contains(err.Error(), "injected"),
		"error should mention install or injected failure, got: %v", err)

	// bootstrap should be marked (ran successfully before install)
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap should be marked")

	// install should NOT be marked (it failed)
	require.False(t, phases.Done(stateDir, id, phases.PhaseInstall), "install marker should not exist after failure")

	// verify should NOT be marked (not reached)
	require.False(t, phases.Done(stateDir, id, phases.PhaseVerify), "verify should not be marked")
}

func TestUp_SkipGateway(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown", "--skip-gateway")
	require.NoError(t, err)

	require.False(t, phases.Done(stateDir, id, phases.PhaseGateway), "gateway should not be marked with --skip-gateway")

	// start should still run
	require.True(t, phases.Done(stateDir, id, phases.PhaseStart), "start should still run without --skip-gateway affecting it")
	require.True(t, containsStr(mock.ExecLog, "start.sh"), "start.sh should be called")

	require.Empty(t, mock.CopyLog, "Copy should not be called when gateway is skipped")

	// Output should mention skip reason
	out := stdout.String()
	require.Contains(t, out, "--skip-gateway")
}

// TestUp_PrepareOnlyStopsBeforeRunSegment: --prepare-only runs prepare and skips run.
func TestUp_PrepareOnlyStopsBeforeRunSegment(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")
	proxyDir := filepath.Join(state.RepoDir, ".proxy")
	observabilityDir := filepath.Join(state.RepoDir, ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown", "--prepare-only")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseCreate), "create should be marked with --prepare-only")
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap should be marked with --prepare-only")
	require.True(t, phases.Done(stateDir, id, phases.PhaseInstall), "install should be marked with --prepare-only")
	require.True(t, phases.Done(stateDir, id, phases.PhaseVerify), "verify should be marked with --prepare-only")
	require.False(t, phases.Done(stateDir, id, phases.PhaseGateway), "gateway should not be marked with --prepare-only")
	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace), "workspace should not be marked with --prepare-only")
	require.False(t, phases.Done(stateDir, id, phases.PhaseAuth), "auth should not be marked with --prepare-only")
	require.False(t, phases.Done(stateDir, id, phases.PhaseStart), "start should not be marked with --prepare-only")

	// No Copy should have been attempted
	require.Empty(t, mock.CopyLog, "Copy should not be called in prepare-only mode")

	require.NoFileExists(t, proxyConfigStatePath(proxyDir))
}

func TestPrepareCommandRunsPrepareSegment(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	_, _, err := execUpRoot(t, root, stdout, stderr, "prepare", "gastown", "--type", "gastown")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseCreate), "create should be marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap should be marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseInstall), "install should be marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseVerify), "verify should be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseGateway), "gateway should not be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace), "workspace should not be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseAuth), "auth should not be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseStart), "start should not be marked")
	require.False(t, containsStr(mock.ExecLog, "start.sh"), "start should not run during prepare")
}

func TestRunCommandRunsRunSegment(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	markPrepareCompleted(t, stateDir, id)

	_, _, err := execUpRoot(t, root, stdout, stderr, "run", "gastown")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseCreate), "create should remain marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap should remain marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseInstall), "install should remain marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseVerify), "verify should remain marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseGateway), "gateway should be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace), "workspace should not be marked without a repo")
	require.True(t, phases.Done(stateDir, id, phases.PhaseAuth), "auth should be marked")
	require.True(t, phases.Done(stateDir, id, phases.PhaseStart), "start should be marked")
	require.True(t, containsStr(mock.ExecLog, "start.sh"), "start should run during run")
}

func TestRunCommandRequiresPrepareCompleted(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))

	_, _, err := execUpRoot(t, root, stdout, stderr, "run", "gastown")
	require.Error(t, err)
	require.Contains(t, err.Error(), `cannot run lab "gastown"`)
	require.Contains(t, err.Error(), `missing required prepare phase "verify"`)
	require.Contains(t, err.Error(), `taxiway prepare gastown --type gastown`)
	require.False(t, phases.Done(stateDir, id, phases.PhaseGateway), "gateway should not be marked")
	require.Empty(t, mock.ExecLog, "run should stop before runtime phase scripts")
}

func TestRunCommandShowsGatewayProgressWhenReconcilingCachedGateway(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	markPrepareCompleted(t, stateDir, id)
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseGateway))

	var ensured []string
	orig := ensureLabLiteLLMSidecarForUp
	ensureLabLiteLLMSidecarForUp = func(_ context.Context, _ *RootState, ref config.LabRef) error {
		ensured = append(ensured, ref.Lab)
		return nil
	}
	t.Cleanup(func() { ensureLabLiteLLMSidecarForUp = orig })

	out, _, err := execUpRoot(t, root, stdout, stderr, "run", "gastown", "--skip-workspace", "--skip-auth-check")
	require.NoError(t, err)

	require.Equal(t, []string{"gastown"}, ensured)
	require.Contains(t, out, "  ⏵  gateway")
	require.Contains(t, out, "  ✓  gateway              (ready)")
	require.Less(t, strings.Index(out, "  ⏵  gateway"), strings.Index(out, "  ✓  gateway              (ready)"))
	require.Empty(t, mock.CopyLog)
}

func TestGatewayCommandMarksPhaseWithoutRefreshingProxyPage(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")
	proxyDir := filepath.Join(state.RepoDir, ".proxy")
	observabilityDir := filepath.Join(state.RepoDir, ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_RUNTIME_DIR", state.RepoDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	for _, phase := range []phases.Phase{phases.PhaseCreate, phases.PhaseBootstrap, phases.PhaseInstall, phases.PhaseVerify} {
		require.NoError(t, phases.Mark(stateDir, id, phase))
	}
	_, err := ensureProxyConfigForState(state, stateDir, proxyDir)
	require.NoError(t, err)
	before, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)

	_, _, err = execUpRoot(t, root, stdout, stderr, "gateway", "gastown")
	require.NoError(t, err)
	require.True(t, phases.Done(stateDir, id, phases.PhaseGateway))

	after, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}

func TestRunCommandProvisionsLabGatewayRuntimeEnv(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	markPrepareCompleted(t, stateDir, id)

	var copiedEnv string
	mock.CopyResponder = func(_ string, srcHost string, _ string) error {
		data, err := os.ReadFile(srcHost)
		require.NoError(t, err)
		copiedEnv = string(data)
		return nil
	}

	_, _, err := execUpRoot(t, root, stdout, stderr, "run", "gastown")
	require.NoError(t, err)

	values, err := envfile.Load(labGatewayEnvPath(stateDir, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	require.NoError(t, err)
	require.NotEmpty(t, values[labLiteLLMAPIKeyEnv])
	require.Equal(t, "http://gastown.litellm.localhost:4000", values[labLiteLLMBaseURLEnv])
	require.Contains(t, copiedEnv, "TAXIWAY_LITELLM_API_KEY='")
	require.Contains(t, copiedEnv, "TAXIWAY_LITELLM_BASE_URL='http://gastown.litellm.localhost:4000'")
}

func TestRunCommandWithMockDriverDoesNotCreateObservabilityRuntime(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	id := idName("gastown")
	proxyDir := filepath.Join(state.RepoDir, ".proxy")
	observabilityDir := filepath.Join(state.RepoDir, ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_RUNTIME_DIR", state.RepoDir)

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	markPrepareCompleted(t, stateDir, id)

	_, _, err := execUpRoot(t, root, stdout, stderr, "run", "gastown")
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(proxyDir, "runtime.json"))
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
}

func TestRunCommandProvisionsLabLangfuseProjectBeforeSidecarWhenObservabilityIsRunning(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, ref))
	markPrepareCompleted(t, stateDir, id)

	var calls []string
	origShouldProvision := shouldProvisionLabLangfuseProject
	origEnsureLangfuse := ensureLabLangfuseProjectForGateway
	origEnsureSidecar := ensureLabLiteLLMSidecarForUp
	shouldProvisionLabLangfuseProject = func(_ *RootState) bool {
		return true
	}
	ensureLabLangfuseProjectForGateway = func(_ *RootState, stateDir, _ string, ref config.LabRef) error {
		values, err := readLabGatewayEnv(stateDir, ref)
		require.NoError(t, err)
		require.NotEmpty(t, values[labLiteLLMAPIKeyEnv])
		values[labLangfuseProjectIDEnv] = "project-gastown"
		values[labLangfuseProjectNameEnv] = ref.Lab
		values[labLangfuseProjectPublicKeyEnv] = "pk-lf-gastown"
		values[labLangfuseProjectSecretKeyEnv] = "sk-lf-gastown"
		calls = append(calls, "langfuse")
		return writeLabGatewayEnv(stateDir, ref, values)
	}
	ensureLabLiteLLMSidecarForUp = func(_ context.Context, _ *RootState, ref config.LabRef) error {
		values, err := readLabGatewayEnv(stateDir, ref)
		require.NoError(t, err)
		require.Equal(t, "project-gastown", values[labLangfuseProjectIDEnv])
		require.Equal(t, "pk-lf-gastown", values[labLangfuseProjectPublicKeyEnv])
		require.Equal(t, "sk-lf-gastown", values[labLangfuseProjectSecretKeyEnv])
		calls = append(calls, "sidecar")
		return nil
	}
	t.Cleanup(func() {
		shouldProvisionLabLangfuseProject = origShouldProvision
		ensureLabLangfuseProjectForGateway = origEnsureLangfuse
		ensureLabLiteLLMSidecarForUp = origEnsureSidecar
	})

	_, _, err := execUpRoot(t, root, stdout, stderr, "run", "gastown")
	require.NoError(t, err)

	values, err := envfile.Load(labGatewayEnvPath(stateDir, ref))
	require.NoError(t, err)
	require.Equal(t, "project-gastown", values[labLangfuseProjectIDEnv])
	require.Equal(t, "gastown", values[labLangfuseProjectNameEnv])
	require.Equal(t, []string{"langfuse", "sidecar"}, calls)
}

func TestGatewayCommandRunsPhase(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))

	_, _, err := execUpRoot(t, root, stdout, stderr, "gateway", "gastown")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseGateway), "gateway should be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace), "workspace should not be marked")
}

func TestGatewayPreservesExistingLabEnvBlocks(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	id := idName("gastown")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))

	existing := "EXAMPLE_API_KEY='secret'\n"
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		argv := strings.Join(req.Argv, " ")
		if strings.Contains(argv, `cat "$HOME/.config/taxiway/env"`) {
			return driver.MockExecResponse{ExitCode: 0, Stdout: existing}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}
	var copiedEnv string
	mock.CopyResponder = func(_ string, srcHost string, _ string) error {
		data, err := os.ReadFile(srcHost)
		require.NoError(t, err)
		copiedEnv = string(data)
		return nil
	}

	_, _, err := execUpRoot(t, root, stdout, stderr, "gateway", "gastown")
	require.NoError(t, err)
	require.Contains(t, copiedEnv, "EXAMPLE_API_KEY='secret'")
	require.Contains(t, copiedEnv, "# >>> taxiway gateway scope=gateway")
	require.Contains(t, copiedEnv, "TAXIWAY_LITELLM_API_KEY='")
}

func TestWorkspaceCommandRunsPhase(t *testing.T) {
	root, state, mock, stdout, stderr := buildWorkspaceTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("claude-code")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "claude-code", Orch: "claude-code", Driver: "mock"}))

	_, _, err := execUpRoot(t, root, stdout, stderr, "workspace", "claude-code", "--repo", "https://github.com/acme/proj")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseWorkspace), "workspace should be marked")
	require.False(t, phases.Done(stateDir, id, phases.PhaseAuth), "auth should not be marked")
	require.True(t, containsStr(mock.ExecLog, "workspace.sh"), "workspace.sh should run")
}

func TestWorkspaceCommandSkipsWithoutRepo(t *testing.T) {
	root, state, mock, stdout, stderr := buildWorkspaceTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("claude-code")

	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "claude-code", Orch: "claude-code", Driver: "mock"}))

	out, _, err := execUpRoot(t, root, stdout, stderr, "workspace", "claude-code")
	require.NoError(t, err)

	require.Contains(t, out, "No repo configured")
	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace), "workspace should not be marked without a repo")
	require.False(t, containsStr(mock.ExecLog, "workspace.sh"), "workspace.sh should not run without a repo")
}

func TestUp_GatewayPhaseMarked(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseGateway),
		"gateway phase should be marked after successful run")
}

// TestUp_StartNoScript: if an orchestrator has no start.sh, the start phase is skipped silently.
func TestUp_StartNoScript(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("codex")

	// codex has no start.sh in this fixture — up should complete without error
	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "codex", "--type", "codex")
	require.NoError(t, err)

	// The start phase marker should still be written (phase returned nil = success)
	require.True(t, phases.Done(stateDir, id, phases.PhaseStart), "start phase should be marked even when no start.sh")
}

// TestUp_VerifyNoScript: if an orchestrator has no verify.sh, the verify phase is skipped silently
// and the marker is still written.
func TestUp_VerifyNoScript(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("codex")

	// Remove verify.sh from codex so it has no verify script
	require.NoError(t, os.Remove(filepath.Join(state.RepoDir, "orchestrators", "codex", "verify.sh")))

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "codex", "--type", "codex")
	require.NoError(t, err, "up should succeed even without verify.sh")

	// verify.sh should not have been executed
	require.False(t, containsStr(mock.ExecLog, "verify.sh"), "verify.sh should not be called when absent")

	// The verify phase marker should still be written (silent skip = success)
	require.True(t, phases.Done(stateDir, id, phases.PhaseVerify), "verify phase should be marked even when no verify.sh")
}

// ---- taxiway down / taxiway shell ----

func TestDown_StopsLab(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)

	// Ensure lab exists
	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"), config.LabRef{
		Lab:    "gastown",
		Orch:   "gastown",
		Driver: "mock",
	}))

	_, _, err := execUpRoot(t, root, stdout, stderr, "down", "gastown")
	require.NoError(t, err)

	st, err := state.Driver.Status(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.Equal(t, "stopped", st.State)
}

func TestDown_StopsLabLiteLLMSidecar(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)

	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"), config.LabRef{
		Lab:    "gastown",
		Orch:   "gastown",
		Driver: "mock",
	}))

	var stopped []config.LabRef
	orig := stopLabLiteLLMSidecarForDown
	stopLabLiteLLMSidecarForDown = func(_ context.Context, _ *RootState, ref config.LabRef) error {
		stopped = append(stopped, ref)
		return nil
	}
	t.Cleanup(func() { stopLabLiteLLMSidecarForDown = orig })

	_, _, err := execUpRoot(t, root, stdout, stderr, "down", "gastown")
	require.NoError(t, err)

	require.Len(t, stopped, 1)
	require.Equal(t, "gastown", stopped[0].Lab)
}

func TestLabUp_ResumesStoppedLabWithRecordedDriver(t *testing.T) {
	root, state, mock, _, _ := buildUpTestRoot(t)
	ctx := testCtx(t)
	id := idName("gastown")
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "docker"}

	require.NoError(t, mock.Create(ctx, id, driver.CreateOptions{}))
	require.NoError(t, mock.Stop(ctx, id))
	require.NoError(t, mock.WriteLabRef(ctx, id, ref))
	dockerLog := installFakeDockerForStoppedLab(t)

	selectedExists := false
	selected := &namedTestDriver{MockDriver: mock, name: "lima", exists: &selectedExists}
	state.Driver = selected

	err := labUp(ctx, state, ref, root.ErrOrStderr())
	require.NoError(t, err)

	got, ok, err := config.ReadLabRef(config.StateDir(state.Flags.StateDir, state.RepoDir), id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "docker", got.Driver)
	require.Empty(t, selected.createCalls)

	log, err := os.ReadFile(dockerLog)
	require.NoError(t, err)
	require.Contains(t, string(log), "start taxiway-gastown")
}

// TestCreate_Createslab: taxiway create gastown calls Create + Start.
func TestCreate_CreatesLab(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "create", "gastown")
	require.NoError(t, err)

	// lab should exist and be running after create
	st, err := state.Driver.Status(testCtx(t), idName("gastown"))
	require.NoError(t, err)
	require.Equal(t, "running", st.State)
}

func TestShell_InvalidOrch(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)
	_, _, err := execUpRoot(t, root, stdout, stderr, "shell", "../bad")
	require.Error(t, err)
}

// TestShell_TmuxSessionExists: when tmux has-session succeeds, ShellExec is called
// with the correct attach command.
func TestShell_TmuxSessionExists(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Ensure lab is running and has a sidecar.
	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"),
		config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"},
	))

	// Responder: tmux has-session succeeds (exit 0).
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		if len(req.Argv) >= 2 && req.Argv[0] == "tmux" && req.Argv[1] == "has-session" {
			return driver.MockExecResponse{ExitCode: 0}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execUpRoot(t, root, stdout, stderr, "shell", "gastown")
	require.NoError(t, err)

	// ShellExec must have been called exactly once with the tmux attach command.
	require.Len(t, mock.ShellExecLog, 1, "expected exactly one ShellExec call")
	call := mock.ShellExecLog[0]
	require.Equal(t, idName("gastown"), call.ID)
	resizeCommand := "tmux resize-window -t gastown"
	recordResizeCommand := "tmux resize-window -t \"$record_session\""
	attachCommand := "exec tmux attach-session -t gastown"
	require.Contains(t, call.Cmd, resizeCommand)
	require.Contains(t, call.Cmd, recordResizeCommand)
	require.Contains(t, call.Cmd, attachCommand)
	require.Less(t, strings.Index(call.Cmd, resizeCommand), strings.Index(call.Cmd, attachCommand), "tmux resize must happen before attach")
	require.Less(t, strings.Index(call.Cmd, recordResizeCommand), strings.Index(call.Cmd, attachCommand), "recorder tmux resize must happen before attach")
	require.Contains(t, call.Cmd, "stty size")
	require.Contains(t, call.Cmd, "tmux list-sessions -F '#{session_name}'")
	require.Contains(t, call.Cmd, "taxiway-record-*")
	require.Contains(t, call.Cmd, "tmux attach-session")
	require.Contains(t, call.Cmd, "gastown")
}

func TestShellCheck_TmuxSessionExistsDoesNotOpenShell(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"),
		config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"},
	))
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		if len(req.Argv) >= 2 && req.Argv[0] == "tmux" && req.Argv[1] == "has-session" {
			return driver.MockExecResponse{ExitCode: 0}
		}
		return driver.MockExecResponse{ExitCode: 1}
	}

	out, _, err := execUpRoot(t, root, stdout, stderr, "shell", "gastown", "--check")

	require.NoError(t, err)
	require.Contains(t, out, "Shell target ready:")
	require.Contains(t, out, "tmux attach-session -t gastown")
	require.Empty(t, mock.ShellExecLog, "shell --check must not open the interactive shell")
}

func TestShellInputWritesLiteralInputToTmuxSession(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"),
		config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"},
	))

	var commands [][]string
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		commands = append(commands, append([]string(nil), req.Argv...))
		if len(req.Argv) >= 2 && req.Argv[0] == "tmux" && req.Argv[1] == "has-session" {
			return driver.MockExecResponse{ExitCode: 0}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	out, _, err := execUpRoot(t, root, stdout, stderr, "shell", "gastown", "--input", "gt status")

	require.NoError(t, err)
	require.Contains(t, out, "Sent to shell target:")
	require.Empty(t, mock.ShellExecLog, "shell --input must not open the interactive shell")
	joined := joinedCommands(commands)
	require.Contains(t, joined, "tmux has-session -t gastown")
	require.Contains(t, joined, "tmux send-keys -l -t gastown gt status")
	require.Contains(t, joined, "tmux send-keys -t gastown Enter")
}

// TestShell_TmuxSessionMissing: when tmux has-session fails, an error is returned
// with hints for taxiway up and taxiway doctor.
func TestShell_TmuxSessionMissing(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Ensure lab is running and has a sidecar.
	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"),
		config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"},
	))

	// Responder: tmux has-session fails (non-zero exit, no error from transport).
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		if len(req.Argv) >= 2 && req.Argv[0] == "tmux" && req.Argv[1] == "has-session" {
			return driver.MockExecResponse{ExitCode: 1}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execUpRoot(t, root, stdout, stderr, "shell", "gastown")
	require.Error(t, err)
	require.Contains(t, err.Error(), `shell session "gastown" is not running`)
	require.Contains(t, err.Error(), "gastown")
	require.Contains(t, err.Error(), "taxiway up gastown --from start --force")
	require.Contains(t, err.Error(), "taxiway doctor")
	require.NotContains(t, err.Error(), "taxiway shell gastown")

	// ShellExec must NOT have been called.
	require.Empty(t, mock.ShellExecLog, "ShellExec must not be called when tmux session is absent")
}

// ---- taxiway rm clears phases ----

func TestDelete_ClearsPhases(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	// Create lab and mark a phase
	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{
		Lab:    "gastown",
		Orch:   "gastown",
		Driver: "mock",
	}))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap))

	_, _, err := execUpRoot(t, root, stdout, stderr, "rm", "--yes", "gastown")
	require.NoError(t, err)

	// Phase marker should be cleared
	require.False(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "phase marker should be cleared after rm")
}

// ---- taxiway reset clears phases ----

func TestEnvReset_ClearsPhases(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	// Create lab and mark phases
	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	require.NoError(t, phases.Mark(stateDir, id, phases.PhaseBootstrap))

	_, _, err := execUpRoot(t, root, stdout, stderr, "reset", "gastown")
	require.NoError(t, err)

	// Phase markers should be cleared
	require.False(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap marker should be cleared after reset")
}

// ---- dry-run for taxiway up ----

func TestUp_DryRun(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("gastown")

	out, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown", "--dry-run")
	require.NoError(t, err)

	// No Exec calls in dry-run
	require.Empty(t, mock.ExecLog, "dry-run should not execute scripts")

	// No phase markers written
	for _, p := range phases.Order {
		require.False(t, phases.Done(stateDir, id, p), "dry-run should not write phase markers for %s", p)
	}

	// Output should mention all pipeline phases; "doctor" must not appear
	for _, p := range phases.Order {
		require.Contains(t, out, string(p), "dry-run output should mention phase %s", p)
	}
	require.NotContains(t, out, "doctor", "dry-run output must not mention doctor (not a pipeline phase)")
}

// TestUp_NoRuntimeEnvCopyDuringInstallVerify: with --prepare-only, the install
// and verify phases must run without any runtime env Copy calls.
func TestUp_NoRuntimeEnvCopyDuringInstallVerify(t *testing.T) {
	// The invariant under test is that --prepare-only does not run runtime
	// phases and produces zero Copy calls.

	// Use the shared test root which creates codex and gastown orchestrators.
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Discover all orchestrators in the test repo dir.
	orchestrators, err := discoverOrchestrators(state.RepoDir)
	require.NoError(t, err)
	require.NotEmpty(t, orchestrators, "expected at least one orchestrator in test repo")

	for _, orch := range orchestrators {
		t.Run(orch, func(t *testing.T) {
			// Reset CopyLog between sub-tests.
			mock.CopyLog = nil

			_, _, runErr := execUpRoot(t, root, stdout, stderr,
				"up", orch, "--type", orch,
				"--prepare-only",
			)
			require.NoError(t, runErr, "up %s --prepare-only should succeed", orch)

			// No Copy must have been called during install/verify phases.
			require.Empty(t, mock.CopyLog,
				"Copy must not be called during install/verify for %s, got: %v", orch, mock.CopyLog)
		})
	}
}

// discoverOrchestrators wraps discover.Orchestrators for use in tests.
func discoverOrchestrators(repoDir string) ([]string, error) {
	return discover.Orchestrators(repoDir)
}

// ── workspace tests ──────────────────────────────────────────────────────────

// buildWorkspaceTestRoot creates a test root with claude-code that has workspace.sh.
func buildWorkspaceTestRoot(t *testing.T) (*cobra.Command, *RootState, *driver.MockDriver, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Add workspace.sh for claude-code (echo only, idempotent)
	workspaceScript := "#!/bin/bash\necho workspace done\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "claude-code", "workspace.sh"),
		[]byte(workspaceScript), 0755,
	))

	// claude-code also needs start.sh for full pipeline
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "claude-code", "start.sh"),
		[]byte("#!/bin/bash\necho start\n"), 0755,
	))

	return root, state, mock, stdout, stderr
}

// TestUp_WorkspacePhaseMarked: with workspace.sh present, phase marker is written.
func TestUp_WorkspacePhaseMarked(t *testing.T) {
	root, state, _, stdout, stderr := buildWorkspaceTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("claude-code")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "claude-code", "--type", "claude-code", "--repo", "https://github.com/acme/proj")
	require.NoError(t, err)

	require.True(t, phases.Done(stateDir, id, phases.PhaseWorkspace),
		"workspace phase should be marked after successful run")
}

// TestUp_WorkspacePhaseNotMarkedWithoutRepo: without a repo, workspace is skipped and not marked.
func TestUp_WorkspacePhaseNotMarkedWithoutScript(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	// gastown has no workspace.sh in buildUpTestRoot
	id := idName("gastown")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)

	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace),
		"workspace marker should not be written without a repo")
}

// TestUp_SkipWorkspace: --skip-workspace skips the workspace phase.
func TestUp_SkipWorkspace(t *testing.T) {
	root, state, mock, stdout, stderr := buildWorkspaceTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("claude-code")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "claude-code", "--type", "claude-code", "--skip-workspace")
	require.NoError(t, err)

	// workspace phase should NOT be marked
	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace),
		"workspace phase should not be marked with --skip-workspace")

	// workspace.sh should not have been executed
	require.False(t, containsStr(mock.ExecLog, "workspace.sh"),
		"workspace.sh must not be called with --skip-workspace")

	// start should still run
	require.True(t, phases.Done(stateDir, id, phases.PhaseStart),
		"start phase should still be marked")
}

// TestUp_PrepareOnlySkipsWorkspace: --prepare-only skips workspace.
func TestUp_PrepareOnlySkipsWorkspace(t *testing.T) {
	root, state, mock, stdout, stderr := buildWorkspaceTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := idName("claude-code")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "claude-code", "--type", "claude-code", "--prepare-only")
	require.NoError(t, err)

	require.False(t, phases.Done(stateDir, id, phases.PhaseWorkspace),
		"workspace phase should not be marked with --prepare-only")
	require.False(t, phases.Done(stateDir, id, phases.PhaseAuth),
		"auth phase should not be marked with --prepare-only")
	require.False(t, phases.Done(stateDir, id, phases.PhaseStart),
		"start phase should not be marked with --prepare-only")
	require.False(t, containsStr(mock.ExecLog, "workspace.sh"),
		"workspace.sh must not be called with --prepare-only")
}

// TestUp_RepoEnvPropagated: TAXIWAY_REPO_URL and TAXIWAY_WORKSPACE_DIR are in the env passed to workspace.sh.
func TestUp_RepoEnvPropagated(t *testing.T) {
	root, _, mock, stdout, stderr := buildWorkspaceTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "claude-code", "--type", "claude-code",
		"--repo", "https://github.com/acme/proj",
		"--repo-ref", "main",
	)
	require.NoError(t, err)

	// Find the env for workspace.sh invocation.
	var wsEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "workspace.sh" {
			wsEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, wsEnv, "workspace.sh should have been called")
	require.Equal(t, "https://github.com/acme/proj", wsEnv["TAXIWAY_REPO_URL"])
	require.Equal(t, "main", wsEnv["TAXIWAY_REPO_REF"])
	// TAXIWAY_WORKSPACE_DIR should contain "proj" (repoBasename)
	require.Contains(t, wsEnv["TAXIWAY_WORKSPACE_DIR"], "proj")
}

func TestUp_PreparesLocalWorkspaceRepositoryBeforeWorkspaceScript(t *testing.T) {
	root, state, mock, stdout, stderr := buildWorkspaceTestRoot(t)
	id := idName("claude-code")
	var gitCommands []string
	installFakeWorkspaceGit(t, &gitCommands)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "claude-code", "--type", "claude-code",
		"--repo", "https://github.com/acme/proj",
		"--repo-ref", "main",
	)
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), id)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, ref.Workspace)
	require.Equal(t, "file:///lab/git/proj.git", ref.Workspace.Fork)

	var wsEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "workspace.sh" {
			wsEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, wsEnv, "workspace.sh should have been called")
	require.Equal(t, "https://github.com/acme/proj", wsEnv["TAXIWAY_REPO_URL"])
	require.Equal(t, "file:///lab/git/proj.git", wsEnv["TAXIWAY_REPO_FORK_URL"])
	require.Equal(t, "/lab/work/proj", wsEnv["TAXIWAY_WORKSPACE_DIR"])

	require.True(t, containsSubstring(gitCommands, "clone --mirror https://github.com/acme/proj"), "workspace preparation must mirror the source repo: %v", gitCommands)
	require.True(t, containsSubstring(gitCommands, "proj.git"), "workspace preparation must use the lab-local bare repo: %v", gitCommands)
	require.False(t, containsSubstring(gitCommands, "work/proj"), "workspace preparation must not clone the working tree: %v", gitCommands)
}

// TestUp_GastownWorkspaceEnv: gastown gets TAXIWAY_RIG_NAME and TAXIWAY_CREW_NAME == host username.
// TAXIWAY_WORKSPACE_DIR is NOT set by Go for gastown — it is exported by workspace.sh
// itself using $HOME, so we verify only TAXIWAY_RIG_NAME and TAXIWAY_CREW_NAME here.
func TestUp_GastownWorkspaceEnv(t *testing.T) {
	// Inject a deterministic host username so the test is hermetic.
	orig := userLookup
	userLookup = func() (string, error) { return "testuser", nil }
	t.Cleanup(func() { userLookup = orig })

	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Give gastown a workspace.sh
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "gastown", "workspace.sh"),
		[]byte("#!/bin/bash\necho workspace\n"), 0755,
	))

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "gastown", "--type", "gastown",
		"--repo", "https://github.com/acme/myrepo",
	)
	require.NoError(t, err)

	// Find the workspace.sh env
	var wsEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "workspace.sh" {
			wsEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, wsEnv, "workspace.sh should have been called")
	require.Equal(t, "myrepo", wsEnv["TAXIWAY_RIG_NAME"])
	// TAXIWAY_CREW_NAME == host username
	require.Equal(t, "testuser", wsEnv["TAXIWAY_CREW_NAME"])
	require.Equal(t, "file:///lab/git/myrepo.git", wsEnv["TAXIWAY_RIG_SOURCE_URL"])
	// TAXIWAY_WORKSPACE_DIR is NOT set by Go for gastown; workspace.sh exports it
	// using $HOME after gt crew start. Verify it is absent from the Go-built env.
	_, hasWD := wsEnv["TAXIWAY_WORKSPACE_DIR"]
	require.False(t, hasWD, "TAXIWAY_WORKSPACE_DIR must not be set by Go for gastown (workspace.sh exports it via $HOME)")
}

// TestUp_GastownWorkspaceEnv_HyphenatedNames: hyphens in repo basename and lab name
// are sanitised to underscores before being placed in TAXIWAY_RIG_NAME / TAXIWAY_CREW_NAME.
func TestUp_GastownWorkspaceEnv_HyphenatedNames(t *testing.T) {
	// Inject a host username that itself contains a hyphen to exercise crew sanitisation.
	orig := userLookup
	userLookup = func() (string, error) { return "test-user", nil }
	t.Cleanup(func() { userLookup = orig })

	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Give gastown a workspace.sh.
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "gastown", "workspace.sh"),
		[]byte("#!/bin/bash\necho workspace\n"), 0755,
	))

	// Use a hyphenated lab name via the ValidateLabName-allowed characters (hyphens ok).
	// "my-lab" is a valid lab name; the repo basename also contains hyphens.
	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "my-lab", "--type", "gastown",
		"--repo", "https://github.com/acme/agentic-clm-demo",
	)
	require.NoError(t, err)

	var wsEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "workspace.sh" {
			wsEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, wsEnv, "workspace.sh should have been called")
	// agentic-clm-demo → agentic_clm_demo
	require.Equal(t, "agentic_clm_demo", wsEnv["TAXIWAY_RIG_NAME"],
		"hyphens in repo basename must be sanitised to underscores")
	// test-user → test_user
	require.Equal(t, "test_user", wsEnv["TAXIWAY_CREW_NAME"],
		"hyphens in host username must be sanitised to underscores")
	_, _ = stdout, stderr
}

// TestUp_RepoSwitchRefused: changing --repo on an existing lab is refused.
func TestUp_RepoSwitchRefused(t *testing.T) {
	root, state, _, stdout, stderr := buildWorkspaceTestRoot(t)
	id := idName("claude-code")

	// First run: set a repo
	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "claude-code", "--type", "claude-code",
		"--repo", "https://github.com/org/repo1",
	)
	require.NoError(t, err)

	// Verify sidecar has the first repo
	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), id)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, ref.Workspace)
	require.Equal(t, "https://github.com/org/repo1", ref.Workspace.Repo)

	// Second run: try to switch to a different repo — must fail
	_, _, err = execUpRoot(t, root, stdout, stderr,
		"up", "claude-code", "--type", "claude-code",
		"--repo", "https://github.com/org/repo2",
	)
	require.Error(t, err, "switching --repo should be refused")
	require.Contains(t, err.Error(), "refusing to switch")
}

func TestUp_ExistingLabRepoUpdatePreservesDriver(t *testing.T) {
	root, state, _, stdout, stderr := buildWorkspaceTestRoot(t)
	id := idName("claude-code")
	require.NoError(t, state.Driver.Create(testCtx(t), id, driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), id, config.LabRef{
		Lab:    "claude-code",
		Orch:   "claude-code",
		Driver: "docker",
	}))

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "claude-code", "--type", "claude-code",
		"--repo", "https://github.com/org/repo1",
		"--dry-run",
	)
	require.NoError(t, err)

	ref, ok, err := state.Driver.ReadLabRef(testCtx(t), id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "docker", ref.Driver)
	require.NotNil(t, ref.Workspace)
	require.Equal(t, "https://github.com/org/repo1", ref.Workspace.Repo)
}

// TestUp_InvalidRepoURL: non-git URL is rejected early.
func TestUp_InvalidRepoURL(t *testing.T) {
	root, _, _, stdout, stderr := buildWorkspaceTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr,
		"up", "claude-code", "--type", "claude-code",
		"--repo", "file:///home/user/private-repo",
	)
	require.Error(t, err, "file:// URL should be rejected")
	require.Contains(t, err.Error(), "file://")
}

// TestShell_ManifestCommand: an orchestrator with manifest.yaml shell.command → ShellExec with that command.
func TestShell_ManifestCommand(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Create a stub orchestrator 'testorch' with install.sh and a manifest with shell.command.
	testOrchDir := filepath.Join(state.RepoDir, "orchestrators", "testorch")
	require.NoError(t, os.MkdirAll(testOrchDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(testOrchDir, "install.sh"), []byte("#!/bin/bash\necho install\n"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(testOrchDir, "verify.sh"), []byte("#!/bin/bash\necho verify\n"), 0755))
	manifestContent := "name: testorch\nshell:\n  command: [\"echo\", \"test\"]\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(testOrchDir, "manifest.yaml"),
		[]byte(manifestContent), 0644,
	))

	// Ensure lab is running and has a sidecar (for loadLabRef).
	require.NoError(t, state.Driver.Create(testCtx(t), idName("testorch"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("testorch"),
		config.LabRef{Lab: "testorch", Orch: "testorch", Driver: "mock"},
	))

	_, _, err := execUpRoot(t, root, stdout, stderr, "shell", "testorch")
	require.NoError(t, err)

	// ShellExec should have been called with a command containing 'echo' and 'test'.
	require.Len(t, mock.ShellExecLog, 1)
	call := mock.ShellExecLog[0]
	require.Equal(t, idName("testorch"), call.ID)
	require.Equal(t, "exec 'echo' 'test'", call.Cmd)
}

// TestShell_GastownDefaultBranch: gastown manifest.yaml without shell: → tmux attach-session -t gastown.
func TestShell_GastownDefaultBranch(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// Write the final gastown manifest.yaml (no shell: block).
	manifestContent := "name: gastown\ndescription: Gas Town HQ + workspace shell\ndocs_url: https://gastownhall.ai\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(state.RepoDir, "orchestrators", "gastown", "manifest.yaml"),
		[]byte(manifestContent), 0644,
	))

	// Ensure lab is running and has a sidecar (for loadLabRef).
	require.NoError(t, state.Driver.Create(testCtx(t), idName("gastown"), driver.CreateOptions{}))
	require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("gastown"),
		config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"},
	))

	// Responder: tmux has-session succeeds (session exists).
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		if len(req.Argv) >= 2 && req.Argv[0] == "tmux" && req.Argv[1] == "has-session" {
			return driver.MockExecResponse{ExitCode: 0}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execUpRoot(t, root, stdout, stderr, "shell", "gastown")
	require.NoError(t, err)

	// ShellExec must have been called with tmux attach-session -t gastown.
	require.Len(t, mock.ShellExecLog, 1, "expected exactly one ShellExec call")
	call := mock.ShellExecLog[0]
	require.Equal(t, idName("gastown"), call.ID)
	require.Contains(t, call.Cmd, "tmux attach-session")
	require.Contains(t, call.Cmd, "gastown")
}

// ---- helpers ----

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func containsSubstring(slice []string, s string) bool {
	for _, v := range slice {
		if strings.Contains(v, s) {
			return true
		}
	}
	return false
}

func countStr(slice []string, s string) int {
	count := 0
	for _, v := range slice {
		if v == s {
			count++
		}
	}
	return count
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

// TestBuildBaseEnv_CrewName_FailFast: if userLookup returns an error, buildBaseEnv
// propagates it and runPhase (and therefore the up command) fails fast.
func TestBuildBaseEnv_CrewName_FailFast(t *testing.T) {
	// Inject a failing userLookup.
	origUserLookup := userLookup
	userLookup = func() (string, error) {
		return "", fmt.Errorf("injected: cannot resolve username")
	}
	t.Cleanup(func() { userLookup = origUserLookup })

	// Call buildBaseEnv directly with a gastown ref that has a Workspace,
	// which triggers the userLookup() call.
	ref := config.LabRef{
		Lab:  "mylab",
		Orch: "gastown",
		Workspace: &config.Workspace{
			Repo: "https://github.com/acme/myrepo",
		},
	}
	_, err := buildBaseEnv(ref)
	require.Error(t, err)
	require.Contains(t, err.Error(), "injected: cannot resolve username")
}
