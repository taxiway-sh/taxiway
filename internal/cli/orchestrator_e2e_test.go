//go:build e2e

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
	"github.com/taxiway-sh/taxiway/internal/recording"
)

const e2eFixtureRepoURL = "https://github.com/manufacture-dev/agreement-hub.git"
const e2eFakeModelResponse = "taxiway e2e fake upstream"

func TestE2E_OrchestratorCodex_Up(t *testing.T) {
	testE2EOrchestratorUp(t, "codex")
}

func TestE2E_OrchestratorClaudeCode_Up(t *testing.T) {
	testE2EOrchestratorUp(t, "claude-code")
}

func TestE2E_OrchestratorGastown_Up(t *testing.T) {
	testE2EOrchestratorUp(t, "gastown")
}

func TestE2E_OrchestratorCodex_PrepareRun(t *testing.T) {
	testE2EOrchestratorPrepareRun(t, "codex")
}

func TestE2E_OrchestratorClaudeCode_PrepareRun(t *testing.T) {
	testE2EOrchestratorPrepareRun(t, "claude-code")
}

func TestE2E_OrchestratorGastown_PrepareRun(t *testing.T) {
	testE2EOrchestratorPrepareRun(t, "gastown")
}

func TestE2E_OrchestratorCodex_PhaseByPhase(t *testing.T) {
	testE2EOrchestratorPhaseByPhase(t, "codex")
}

func TestE2E_OrchestratorClaudeCode_PhaseByPhase(t *testing.T) {
	testE2EOrchestratorPhaseByPhase(t, "claude-code")
}

func TestE2E_OrchestratorGastown_PhaseByPhase(t *testing.T) {
	testE2EOrchestratorPhaseByPhase(t, "gastown")
}

func testE2EOrchestratorPrepareRun(t *testing.T, orch string) {
	t.Helper()
	requireDockerOrSkip(t)

	scope := e2eOrchestratorScope(orch, "prepare-run")
	configureE2EScenarioEnvironment(t, scope)
	id := uniqueDockerID(t, orch, "prepare-run")
	lab := labNameFromID(id)
	fakeUpstream := startE2EFakeOpenAIUpstream(t)
	root, state, tb := buildRealOrchestratorDockerRoot(t, orch, scope)
	cleanupE2EOrchestratorLab(t, state, id, lab, orch)
	configureE2ELiteLLMModelCatalog(t, state, fakeUpstream)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	runE2EStep(t, "taxiway:init", func(t *testing.T) {
		ensureE2ERuntimeInitialized(t, root, tb)
		runE2EAssert(t, "assert:runtime-initialized", func(t *testing.T) {
			assertE2EObservabilityRunning(t, state)
		})
	})

	runE2EStep(t, fmt.Sprintf("taxiway:prepare[--type=%s]", orch), func(t *testing.T) {
		runE2ECommand(t, root, tb, "prepare", lab, "--type", orch)
		configureE2EFixtureWorkspace(t, state, id)
		runE2EAssert(t, "assert:phase-verified", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseVerify)
		})
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "degraded", "verified")
		})
	})

	runE2EStep(t, "taxiway:run[--skip-auth-check]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "run", lab, "--skip-auth-check")
		runE2EAssert(t, "assert:phase-started", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseStart)
		})
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "running", "started")
		})
		runE2EAssert(t, e2eWorkspaceAssertName(orch), func(t *testing.T) {
			assertE2EFixtureWorkspace(t, state, id, orch)
		})
		runE2EAssert(t, "assert:gateway-started", func(t *testing.T) {
			assertE2EGatewayRuntimeRunning(t, state, lab, orch)
		})
		runE2EAssert(t, "assert:gateway-routed", func(t *testing.T) {
			assertE2EGatewayRequestRouted(t, state, lab, orch, fakeUpstream)
		})
		runE2EAssert(t, "assert:trace-ingested", func(t *testing.T) {
			assertE2EObservabilityTraceIngested(t, state, lab, orch)
		})
	})

	runE2EStep(t, e2eCommandStepAt("after-start", "shell", "--check"), func(t *testing.T) {
		assertE2EShellCheck(t, root, tb, lab, orch)
	})

	runE2EStep(t, e2eCommandStepAt("after-start", "doctor"), func(t *testing.T) {
		runE2ECommand(t, root, tb, "doctor", lab)
	})

	runE2EStep(t, "taxiway:rm[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "rm", "--yes", lab)
		runE2EAssert(t, "assert:lab-removed", func(t *testing.T) {
			exists, err := state.Driver.Exists(context.Background(), id)
			require.NoError(t, err)
			require.False(t, exists)
		})
	})

	runE2EStep(t, "taxiway:destroy[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "destroy", "--yes")
		runE2EAssert(t, "assert:runtime-destroyed", func(t *testing.T) {
			assertE2ERuntimeDestroyed(t, state)
		})
	})
}

func testE2EOrchestratorPhaseByPhase(t *testing.T, orch string) {
	t.Helper()
	requireDockerOrSkip(t)

	scope := e2eOrchestratorScope(orch, "phase-by-phase")
	configureE2EScenarioEnvironment(t, scope)
	id := uniqueDockerID(t, orch, "phase-by-phase")
	lab := labNameFromID(id)
	fakeUpstream := startE2EFakeOpenAIUpstream(t)
	root, state, tb := buildRealOrchestratorDockerRoot(t, orch, scope)
	cleanupE2EOrchestratorLab(t, state, id, lab, orch)
	configureE2ELiteLLMModelCatalog(t, state, fakeUpstream)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	runE2EStep(t, "taxiway:init", func(t *testing.T) {
		ensureE2ERuntimeInitialized(t, root, tb)
		runE2EAssert(t, "assert:runtime-initialized", func(t *testing.T) {
			assertE2EObservabilityRunning(t, state)
		})
	})

	runE2EStep(t, fmt.Sprintf("taxiway:create[--type=%s]", orch), func(t *testing.T) {
		runE2ECommand(t, root, tb, "create", lab, "--type", orch)
		configureE2EFixtureWorkspace(t, state, id)
		runE2EAssert(t, "assert:phase-created", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseCreate)
		})
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "degraded", "created")
		})
	})

	runE2EStep(t, "taxiway:bootstrap", func(t *testing.T) {
		runE2ECommand(t, root, tb, "bootstrap", lab)
		runE2EAssert(t, "assert:phase-bootstrapped", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseBootstrap)
		})
	})

	runE2EStep(t, "taxiway:install", func(t *testing.T) {
		runE2ECommand(t, root, tb, "install", lab)
		runE2EAssert(t, "assert:phase-installed", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseInstall)
		})
	})

	runE2EStep(t, "taxiway:verify", func(t *testing.T) {
		runE2ECommand(t, root, tb, "verify", lab)
		runE2EAssert(t, "assert:phase-verified", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseVerify)
		})
	})

	runE2EStep(t, "taxiway:gateway", func(t *testing.T) {
		runE2ECommand(t, root, tb, "gateway", lab)
		runE2EAssert(t, "assert:phase-gateway-completed", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseGateway)
		})
		runE2EAssert(t, "assert:gateway-started", func(t *testing.T) {
			assertE2EGatewayRuntimeRunning(t, state, lab, orch)
		})
	})

	runE2EStep(t, "taxiway:workspace", func(t *testing.T) {
		runE2ECommand(t, root, tb, "workspace", lab)
		runE2EAssert(t, "assert:phase-workspace-created", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseWorkspace)
		})
		runE2EAssert(t, e2eWorkspaceAssertName(orch), func(t *testing.T) {
			assertE2EFixtureWorkspace(t, state, id, orch)
		})
	})

	runE2EStep(t, "taxiway:start", func(t *testing.T) {
		runE2ECommand(t, root, tb, "start", lab)
		runE2EAssert(t, "assert:phase-started", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseStart)
		})
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "running", "started")
		})
		runE2EAssert(t, "assert:gateway-routed", func(t *testing.T) {
			assertE2EGatewayRequestRouted(t, state, lab, orch, fakeUpstream)
		})
		runE2EAssert(t, "assert:trace-ingested", func(t *testing.T) {
			assertE2EObservabilityTraceIngested(t, state, lab, orch)
		})
	})

	runE2EStep(t, "taxiway:observe[down]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "observe", "down")
		runE2EAssert(t, "assert:observability-stopped", func(t *testing.T) {
			assertE2EObservabilityStopped(t, state)
			assertE2EGatewayRuntimeRunning(t, state, lab, orch)
		})
	})

	runE2EStep(t, "taxiway:observe[up]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "observe", "up")
		runE2EAssert(t, "assert:observability-started", func(t *testing.T) {
			assertE2EObservabilityRunning(t, state)
			assertE2EGatewayRuntimeRunning(t, state, lab, orch)
		})
	})

	runE2EStep(t, e2eCommandStepAt("after-start", "shell", "--check"), func(t *testing.T) {
		assertE2EShellCheck(t, root, tb, lab, orch)
	})

	runE2EStep(t, e2eCommandStepAt("after-start", "doctor"), func(t *testing.T) {
		runE2ECommand(t, root, tb, "doctor", lab)
	})

	runE2EStep(t, "taxiway:down", func(t *testing.T) {
		runE2ECommand(t, root, tb, "down", lab)
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "stopped", "started")
		})
	})

	runE2EStep(t, e2eCommandStepAt("after-down", "up", "--type="+orch, "--skip-auth-check"), func(t *testing.T) {
		runE2ECommand(t, root, tb, "up", lab, "--type", orch, "--skip-auth-check")
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "running", "started")
		})
		runE2EAssert(t, "assert:gateway-routed", func(t *testing.T) {
			assertE2EGatewayRequestRouted(t, state, lab, orch, fakeUpstream)
		})
		runE2EAssert(t, "assert:trace-ingested", func(t *testing.T) {
			assertE2EObservabilityTraceIngested(t, state, lab, orch)
		})
	})

	runE2EStep(t, e2eCommandStepAt("after-up", "shell", "--check"), func(t *testing.T) {
		assertE2EShellCheck(t, root, tb, lab, orch)
	})

	runE2EStep(t, e2eCommandStepAt("after-up", "doctor"), func(t *testing.T) {
		runE2ECommand(t, root, tb, "doctor", lab)
	})

	runE2ERecordScenario(t, root, tb, state, lab, orch)

	runE2EStep(t, "taxiway:reset[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "reset", "--yes", lab)
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "degraded", "-")
		})
	})

	runE2EStep(t, "taxiway:rm[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "rm", "--yes", lab)
		runE2EAssert(t, "assert:lab-removed", func(t *testing.T) {
			exists, err := state.Driver.Exists(context.Background(), id)
			require.NoError(t, err)
			require.False(t, exists)
		})
	})

	runE2EStep(t, "taxiway:destroy[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "destroy", "--yes")
		runE2EAssert(t, "assert:runtime-destroyed", func(t *testing.T) {
			assertE2ERuntimeDestroyed(t, state)
		})
	})
}

func testE2EOrchestratorUp(t *testing.T, orch string) {
	t.Helper()
	requireDockerOrSkip(t)

	scope := e2eOrchestratorScope(orch, "up")
	configureE2EScenarioEnvironment(t, scope)
	id := uniqueDockerID(t, orch, "up")
	lab := labNameFromID(id)
	fakeUpstream := startE2EFakeOpenAIUpstream(t)
	root, state, tb := buildRealOrchestratorDockerRoot(t, orch, scope)
	cleanupE2EOrchestratorLab(t, state, id, lab, orch)
	configureE2ELiteLLMModelCatalog(t, state, fakeUpstream)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	runE2EStep(t, "taxiway:init", func(t *testing.T) {
		ensureE2ERuntimeInitialized(t, root, tb)
		runE2EAssert(t, "assert:runtime-initialized", func(t *testing.T) {
			assertE2EObservabilityRunning(t, state)
		})
	})

	runE2EStep(t, fmt.Sprintf("taxiway:up[--type=%s,--repo=<fixture>,--skip-auth-check]", orch), func(t *testing.T) {
		runE2ECommand(t, root, tb, "up", lab, "--type", orch, "--repo", e2eFixtureRepoURL, "--skip-auth-check")
		runE2EAssert(t, "assert:phase-started", func(t *testing.T) {
			assertE2EPhase(t, stateDir, id, phases.PhaseStart)
		})
		runE2EAssert(t, "assert:lab-listed", func(t *testing.T) {
			assertE2EList(t, root, tb, lab, orch, "running", "started")
		})
		runE2EAssert(t, e2eWorkspaceAssertName(orch), func(t *testing.T) {
			assertE2EFixtureWorkspace(t, state, id, orch)
		})
		runE2EAssert(t, "assert:gateway-started", func(t *testing.T) {
			assertE2EGatewayRuntimeRunning(t, state, lab, orch)
		})
		runE2EAssert(t, "assert:gateway-routed", func(t *testing.T) {
			assertE2EGatewayRequestRouted(t, state, lab, orch, fakeUpstream)
		})
		runE2EAssert(t, "assert:trace-ingested", func(t *testing.T) {
			assertE2EObservabilityTraceIngested(t, state, lab, orch)
		})
	})

	runE2EStep(t, "taxiway:shell[--check]", func(t *testing.T) {
		assertE2EShellCheck(t, root, tb, lab, orch)
	})

	runE2EStep(t, "taxiway:doctor", func(t *testing.T) {
		runE2ECommand(t, root, tb, "doctor", lab)
	})

	runE2EStep(t, "taxiway:rm[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "rm", "--yes", lab)
		runE2EAssert(t, "assert:lab-removed", func(t *testing.T) {
			exists, err := state.Driver.Exists(context.Background(), id)
			require.NoError(t, err)
			require.False(t, exists)
		})
	})

	runE2EStep(t, "taxiway:destroy[--yes]", func(t *testing.T) {
		runE2ECommand(t, root, tb, "destroy", "--yes")
		runE2EAssert(t, "assert:runtime-destroyed", func(t *testing.T) {
			assertE2ERuntimeDestroyed(t, state)
		})
	})
}

func buildRealOrchestratorDockerRoot(t *testing.T, orch, scope string) (*cobra.Command, *RootState, *dockerTestBuf) {
	t.Helper()

	tmp := filepath.Join(e2eScenarioRoot(t, scope), "repo")
	stateDir := e2eLabStateDir()
	require.NoError(t, os.MkdirAll(stateDir, 0o700))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra"), 0o755))
	copyRuntimeTree(t, filepath.Join("infra", "commands"), filepath.Join(tmp, "infra", "commands"))
	copyRuntimeTree(t, filepath.Join("infra", "gateway"), filepath.Join(tmp, "infra", "gateway"))
	copyRuntimeTree(t, filepath.Join("infra", "observability"), filepath.Join(tmp, "infra", "observability"))
	copyRuntimeTree(t, filepath.Join("infra", "trace"), filepath.Join(tmp, "infra", "trace"))
	copyRuntimeTree(t, filepath.Join("infra", "workspace"), filepath.Join(tmp, "infra", "workspace"))

	copyRuntimeTree(t, filepath.Join("orchestrators", orch), filepath.Join(tmp, "orchestrators", orch))
	for _, agent := range e2eAgents(t, tmp, orch) {
		copyRuntimeTree(t, filepath.Join("agents", agent), filepath.Join(tmp, "agents", agent))
	}

	d := driver.NewDockerDriver(stateDir)
	state := &RootState{
		RepoDir: tmp,
		Flags: GlobalFlags{
			DryRun:   false,
			StateDir: stateDir,
		},
		Driver: d,
	}
	state.Observability = e2eObservabilityRuntime(t)
	requireE2ERuntimeIsolated(t, state)

	tb := &dockerTestBuf{}
	root := &cobra.Command{Use: "taxiway", SilenceUsage: true}
	root.SetOut(&tb.out)
	root.SetErr(&tb.err)
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
		newCredentialsCmd(state),
		newLabAuthCmd(state),
		newStartCmd(state),
		newShellCmd(state),
		newListCmd(state),
		newRecordCmd(state),
		newDoctorCmd(state),
		newDownCmd(state),
		newRmCmd(state),
		newResetCmd(state),
		newInitCmd(state),
		newStatusCmd(state),
		newAccessCmd(state),
		newRepairCmd(state),
		newDestroyCmd(state),
		newObserveCmd(state),
	)
	return root, state, tb
}

func cleanupE2EOrchestratorLab(t *testing.T, state *RootState, id, lab, orch string) {
	t.Helper()
	t.Cleanup(func() {
		ref := config.LabRef{Lab: lab, Orch: orch, Driver: state.Driver.Name()}
		if err := removeLabLiteLLMSidecar(context.Background(), state, ref); err != nil {
			t.Logf("cleanup LiteLLM sidecar for %s: %v", lab, err)
		}
		if err := state.Driver.Delete(context.Background(), id); err != nil {
			t.Logf("cleanup lab %s: %v", lab, err)
		}
	})
}

func e2eOrchestratorScope(orch, action string) string {
	return labLiteLLMSlug(orch) + "-" + action
}

func e2eAgents(t *testing.T, repoDir, orch string) []string {
	t.Helper()
	manifest, err := config.LoadOrchManifest(repoDir, orch)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	return manifest.Agents
}

func runE2ECommand(t *testing.T, root *cobra.Command, tb *dockerTestBuf, args ...string) string {
	t.Helper()
	t.Logf("running: taxiway %s", strings.Join(args, " "))
	out, errOut, err := execDockerRoot(t, root, tb, args...)
	require.NoError(t, err, "taxiway %v failed\nstdout:\n%s\nstderr:\n%s", args, out, errOut)
	return out
}

func e2eCommandStepAt(contextName, command string, args ...string) string {
	name := "taxiway:" + command
	if len(args) > 0 {
		name += "[" + strings.Join(args, ",") + "]"
	}
	if contextName != "" {
		name += "@" + contextName
	}
	return name
}

func runE2EStep(t *testing.T, name string, fn func(t *testing.T)) {
	t.Helper()
	runE2ESubtest(t, name, fn)
}

func runE2EAssert(t *testing.T, name string, fn func(t *testing.T)) {
	t.Helper()
	runE2ESubtest(t, name, fn)
}

func runE2ESubtest(t *testing.T, name string, fn func(t *testing.T)) {
	t.Helper()
	if !t.Run(name, fn) {
		t.FailNow()
	}
}

func configureE2EFixtureWorkspace(t *testing.T, state *RootState, id string) {
	t.Helper()
	ref, ok, err := state.Driver.ReadLabRef(context.Background(), id)
	require.NoError(t, err)
	require.True(t, ok, "lab ref must exist for %s", id)
	ref.Workspace = &config.Workspace{
		Repo: e2eFixtureRepoURL,
	}
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), id, ref))
}

func assertE2EFixtureWorkspace(t *testing.T, state *RootState, id, orch string) {
	t.Helper()
	checkCommand, expected := e2eFixtureWorkspaceCheck(t, orch)
	var stdout, stderr bytes.Buffer
	res, err := state.Driver.Exec(context.Background(), id, driver.ExecRequest{
		Workdir: "/lab",
		Argv: []string{
			"bash",
			"-lc",
			checkCommand,
		},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode, "fixture workspace must be provisioned at %s\nstdout:\n%s\nstderr:\n%s", expected, stdout.String(), stderr.String())
}

func assertE2EShellCheck(t *testing.T, root *cobra.Command, tb *dockerTestBuf, lab, orch string) {
	t.Helper()
	out := runE2ECommand(t, root, tb, "shell", lab, "--check")
	require.Contains(t, out, "Shell target ready:")
	require.Contains(t, out, orch)
}

func runE2ERecordScenario(t *testing.T, root *cobra.Command, tb *dockerTestBuf, state *RootState, lab, orch string) {
	t.Helper()
	const recordName = "e2e-status"

	var castPath string
	recordingPresent := false
	t.Cleanup(func() {
		if !recordingPresent {
			return
		}
		_, _, _ = execDockerRoot(t, root, tb, "record", "rm", lab, "--name", recordName, "--force")
	})

	runE2EStep(t, "taxiway:record[start,--name=e2e-status]", func(t *testing.T) {
		out := runE2ECommand(t, root, tb, "record", "start", lab, "--name", recordName)
		recordingPresent = true
		runE2EAssert(t, "assert:recording-started", func(t *testing.T) {
			require.Contains(t, out, "Recording started: "+recordName)
			require.Contains(t, out, ".cast")
		})
	})

	statusInput := e2eRecordStatusInput(t, orch)
	runE2EStep(t, fmt.Sprintf("taxiway:shell[--input=%s]", statusInput), func(t *testing.T) {
		out := runE2ECommand(t, root, tb, "shell", lab, "--input", statusInput)
		runE2EAssert(t, "assert:shell-input-sent", func(t *testing.T) {
			require.Contains(t, out, "Sent to shell target:")
		})
	})

	runE2EStep(t, "taxiway:record[stop]", func(t *testing.T) {
		out := runE2ECommand(t, root, tb, "record", "stop", lab)
		runE2EAssert(t, "assert:recording-stopped", func(t *testing.T) {
			require.Contains(t, out, "Recording stopped: "+recordName)
			require.Contains(t, out, ".cast")
		})
	})

	runE2EStep(t, e2eCommandStepAt("after-stop", "record", "list"), func(t *testing.T) {
		out := runE2ECommand(t, root, tb, "record", "list", lab)
		runE2EAssert(t, "assert:recording-listed", func(t *testing.T) {
			require.Contains(t, out, recordName)
			require.Contains(t, out, string(recording.StateStopped))
			require.Contains(t, out, ".cast")
		})
		runE2EAssert(t, "assert:recording-cast-written", func(t *testing.T) {
			session := requireE2ERecordingSession(t, state, lab, recordName)
			require.Equal(t, recording.StateStopped, session.State)
			require.NotEmpty(t, session.CastPathHost)
			castPath = session.CastPathHost
			assertE2EAsciicastFile(t, castPath)
		})
	})

	runE2EStep(t, "taxiway:record[rm,--name=e2e-status]", func(t *testing.T) {
		out := runE2ECommand(t, root, tb, "record", "rm", lab, "--name", recordName)
		recordingPresent = false
		runE2EAssert(t, "assert:recording-removed", func(t *testing.T) {
			require.Contains(t, out, "Recording removed: "+recordName)
			if castPath != "" {
				require.NoFileExists(t, castPath)
			}
		})
	})

	runE2EStep(t, e2eCommandStepAt("after-rm", "record", "list"), func(t *testing.T) {
		out := runE2ECommand(t, root, tb, "record", "list", lab)
		runE2EAssert(t, "assert:recording-absent", func(t *testing.T) {
			require.NotContains(t, out, recordName)
		})
	})
}

func e2eRecordStatusInput(t *testing.T, orch string) string {
	t.Helper()
	switch orch {
	case "codex":
		return "/status"
	case "claude-code":
		return "/status"
	case "gastown":
		return "gt status"
	default:
		t.Fatalf("unsupported e2e record orchestrator %q", orch)
		return ""
	}
}

func requireE2ERecordingSession(t *testing.T, state *RootState, lab, name string) recording.Session {
	t.Helper()
	store := recording.NewStore(config.StateDir(state.Flags.StateDir, state.RepoDir), lab)
	idx, err := store.Load()
	require.NoError(t, err)
	for _, session := range idx.Sessions {
		if session.Name == name {
			return session
		}
	}
	require.FailNowf(t, "recording not found", "recording %q not found for lab %q", name, lab)
	return recording.Session{}
}

func assertE2EAsciicastFile(t *testing.T, path string) {
	t.Helper()
	var data []byte
	var err error
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(path)
		if err == nil && len(bytes.Split(bytes.TrimSpace(data), []byte("\n"))) >= 2 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err)
	require.NotEmpty(t, data)

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2, "asciicast must contain a header and at least one event")
	var header struct {
		Version int `json:"version"`
	}
	require.NoError(t, json.Unmarshal(lines[0], &header))
	require.Equal(t, 2, header.Version)
}

func e2eWorkspaceAssertName(orch string) string {
	if orch == "gastown" {
		return "assert:workspace-provisioned"
	}
	return "assert:workspace-cloned"
}

func e2eFixtureWorkspaceCheck(t *testing.T, orch string) (string, string) {
	t.Helper()
	if orch == "gastown" {
		rawCrew, err := userLookup()
		require.NoError(t, err)
		crewName := sanitizeIdentifier(rawCrew)
		hqMarker := "/lab/work/gt/.taxiway-hq-initialized"
		rigDir := "/lab/work/gt/agreement_hub"
		crewDir := rigDir + "/crew/" + crewName
		return strings.Join([]string{
			"test -f '" + hqMarker + "'",
			"test -d '" + rigDir + "'",
			"test -d '" + crewDir + "'",
			"git -C '" + crewDir + "' rev-parse --is-inside-work-tree >/dev/null",
		}, " && "), hqMarker + ", " + rigDir + ", and git workspace " + crewDir
	}
	workspaceDir := e2eFixtureWorkspaceDir()
	return "test -d '" + workspaceDir + "/.git'", workspaceDir + "/.git"
}

func e2eFixtureWorkspaceDir() string {
	return "/lab/work/agreement-hub"
}

func ensureE2ERuntimeInitialized(t *testing.T, root *cobra.Command, tb *dockerTestBuf) {
	t.Helper()
	e2eObservabilityStartMutex.Lock()
	defer e2eObservabilityStartMutex.Unlock()
	runE2ECommand(t, root, tb, "init")
	runE2ECommand(t, root, tb, "status")
}

func assertE2EObservabilityRunning(t *testing.T, state *RootState) {
	t.Helper()
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()

	require.Equal(t, "running", e2eDockerContainerState(t, proxy.Container), "proxy must be running")
	for _, service := range observabilityComposeServices() {
		name := runtime.ComposeProject + "-" + service + "-1"
		require.Equal(t, "running", e2eDockerContainerState(t, name), "%s must be running", name)
	}
	require.True(t, e2eDockerNetworkExists(t, runtime.DockerNetwork()), "observability network must exist")
	require.NotEmpty(t, e2eDockerNames("volume", runtime.ComposeProject+"_"), "observability volumes must exist")
}

func assertE2EObservabilityStopped(t *testing.T, state *RootState) {
	t.Helper()
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()

	require.Equal(t, "running", e2eDockerContainerState(t, proxy.Container), "proxy must keep running")
	for _, service := range observabilityComposeServices() {
		name := runtime.ComposeProject + "-" + service + "-1"
		require.NotEqual(t, "running", e2eDockerContainerState(t, name), "%s must be stopped", name)
	}
	require.True(t, e2eDockerNetworkExists(t, runtime.DockerNetwork()), "observability network must be preserved")
	require.NotEmpty(t, e2eDockerNames("volume", runtime.ComposeProject+"_"), "observability volumes must be preserved")
}

func assertE2ERuntimeDestroyed(t *testing.T, state *RootState) {
	t.Helper()
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()

	require.Equal(t, "missing", e2eDockerContainerState(t, proxy.Container), "proxy must be removed")
	require.Empty(t, e2eDockerNames("container", runtime.ComposeProject+"-"), "observability containers must be removed")
	require.Empty(t, e2eDockerNames("volume", runtime.ComposeProject+"_"), "observability volumes must be removed")
	require.False(t, e2eDockerNetworkExists(t, runtime.DockerNetwork()), "observability network must be removed")
	require.NoDirExists(t, proxy.StateDir, "proxy state dir must be removed")
	require.NoDirExists(t, runtime.StateDir, "observability state dir must be removed")
	require.NoDirExists(t, config.StateDir(state.Flags.StateDir, state.RepoDir), "lab state dir must be removed")
}

func e2eDockerContainerState(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", name).CombinedOutput()
	if err != nil {
		return "missing"
	}
	state := strings.TrimSpace(string(out))
	if state == "" {
		return "unknown"
	}
	return state
}

func e2eDockerNetworkExists(t *testing.T, name string) bool {
	t.Helper()
	err := exec.Command("docker", "network", "inspect", name).Run()
	return err == nil
}

func startE2EFakeOpenAIUpstream(t *testing.T) string {
	t.Helper()
	var mu sync.Mutex
	var modelCalls int
	var requests []string
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/v1/models" {
			writeE2EJSON(t, w, map[string]any{
				"object": "list",
				"data": []map[string]any{{
					"id":       "e2e-smoke",
					"object":   "model",
					"created":  0,
					"owned_by": "taxiway-e2e",
				}},
			})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" {
			mu.Lock()
			modelCalls++
			mu.Unlock()
			writeE2EJSON(t, w, map[string]any{
				"id":      "chatcmpl-taxiway-e2e",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   "e2e-smoke",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": e2eFakeModelResponse,
					},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 1,
					"total_tokens":      2,
				},
			})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/responses" {
			mu.Lock()
			modelCalls++
			mu.Unlock()
			writeE2EJSON(t, w, map[string]any{
				"id":         "resp-taxiway-e2e",
				"object":     "response",
				"created_at": time.Now().Unix(),
				"status":     "completed",
				"model":      "e2e-smoke",
				"output": []map[string]any{{
					"id":     "msg-taxiway-e2e",
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []map[string]any{{
						"type": "output_text",
						"text": e2eFakeModelResponse,
					}},
				}},
				"usage": map[string]any{
					"input_tokens":  1,
					"output_tokens": 1,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if t.Failed() {
			t.Logf("fake upstream model requests: %d; all requests: %s", modelCalls, strings.Join(requests, ", "))
			return
		}
		require.Greater(t, modelCalls, 0, "fake upstream must receive at least one model request; all requests: %s", strings.Join(requests, ", "))
	})
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	return "http://host.docker.internal:" + port
}

func writeE2EJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func configureE2ELiteLLMModelCatalog(t *testing.T, state *RootState, fakeUpstreamBaseURL string) {
	t.Helper()
	modelsPath := filepath.Join(state.RepoDir, "infra", "gateway", "litellm", "models.yaml")
	content := fmt.Sprintf(`models:
  - name: gpt-5.5
    provider: openai
    upstream: e2e-smoke
    api_base: %[1]s/v1
    api_key: sk-e2e-upstream
  - name: claude-opus-4-8
    provider: openai
    upstream: e2e-smoke
    api_base: %[1]s/v1
    api_key: sk-e2e-upstream
`, fakeUpstreamBaseURL)
	require.NoError(t, os.WriteFile(modelsPath, []byte(content), 0o644))
}

func assertE2EGatewayRuntimeRunning(t *testing.T, state *RootState, lab, orch string) {
	t.Helper()
	ref := config.LabRef{Lab: lab, Orch: orch, Driver: state.Driver.Name()}
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	route, err := readLabLiteLLMRoute(stateDir, ref)
	require.NoError(t, err)
	require.Equal(t, lab, route.Lab)
	require.Equal(t, labLiteLLMHost(lab), route.Host)
	require.NotEmpty(t, route.Project)

	composePath := filepath.Join(labGatewayDir(stateDir, ref), "litellm.compose.yml")
	out, err := exec.Command("docker", "compose", "-p", route.Project, "-f", composePath, "ps", "--status", "running", "--services").CombinedOutput()
	require.NoError(t, err, string(out))
	services := strings.Fields(string(out))
	require.Contains(t, services, "litellm")
	require.Contains(t, services, "postgres")
}

func assertE2EGatewayRequestRouted(t *testing.T, state *RootState, lab, orch string, fakeUpstreamBaseURL string) {
	t.Helper()
	ref := config.LabRef{Lab: lab, Orch: orch, Driver: state.Driver.Name()}
	values, err := readLabGatewayEnv(config.StateDir(state.Flags.StateDir, state.RepoDir), ref)
	require.NoError(t, err)
	apiKey := values[labLiteLLMAPIKeyEnv]
	require.NotEmpty(t, apiKey)
	model := e2ELiteLLMSmokeModel(orch)

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		content, err := callE2ELiteLLMChatCompletion(state, lab, apiKey, model)
		if err == nil {
			require.Equal(t, e2eFakeModelResponse, content)
			return
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, lastErr, "LiteLLM smoke via %s should reach fake upstream %s\n%s", labLiteLLMBaseURL(state, ref), fakeUpstreamBaseURL, collectE2ELiteLLMDiagnostics(t, state, ref, fakeUpstreamBaseURL))
}

func callE2ELiteLLMChatCompletion(state *RootState, lab, apiKey, model string) (string, error) {
	body := strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"smoke"}],"max_tokens":8}`, model))
	req, err := http.NewRequest(http.MethodPost, state.proxyRuntime().BaseURL()+"/v1/chat/completions", body)
	if err != nil {
		return "", err
	}
	req.Host = labLiteLLMHost(lab)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-litellm-api-key", "Bearer "+apiKey)
	req.Header.Set("x-litellm-agent-id", "taxiway-e2e")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LiteLLM returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("LiteLLM response has no choices: %s", string(data))
	}
	return parsed.Choices[0].Message.Content, nil
}

func collectE2ELiteLLMDiagnostics(t *testing.T, state *RootState, ref config.LabRef, fakeUpstreamBaseURL string) string {
	t.Helper()
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	route, routeErr := readLabLiteLLMRoute(stateDir, ref)

	var b strings.Builder
	fmt.Fprintln(&b, "LiteLLM diagnostics:")
	if routeErr != nil {
		fmt.Fprintf(&b, "  route: %v\n", routeErr)
		return b.String()
	}
	fmt.Fprintf(&b, "  route service: %s\n", route.Service)
	fmt.Fprintf(&b, "  route host: %s\n", route.Host)
	proxy := state.proxyRuntime()
	fmt.Fprintf(&b, "  proxy container: %s\n", proxy.Container)
	fmt.Fprintf(&b, "  proxy resolved port: %d\n", proxy.Port)
	fmt.Fprintf(&b, "  proxy published port (docker port): %s\n", e2eCommandOutput("docker", "port", proxy.Container, "4000/tcp"))
	fmt.Fprintf(&b, "  proxy reachable from host (%s): %s\n", proxy.BaseURL(), e2eHostProxyProbe(proxy))
	fmt.Fprintf(&b, "  sidecar health from proxy: %s\n", e2eCommandOutput("docker", "exec", proxy.Container, "wget", "-qO-", "http://"+route.Service+":4000/health/liveliness"))
	sidecarContainer := labLiteLLMComposeProject(proxy.Context, proxy.ContextID, ref.Lab) + "-litellm-1"
	fmt.Fprintf(&b, "  sidecar container: %s\n", sidecarContainer)
	fmt.Fprintf(&b, "  fake upstream models from sidecar: %s\n", e2eCommandOutput("docker", "exec", sidecarContainer, "python", "-c", fmt.Sprintf("import urllib.request; print(urllib.request.urlopen(%q, timeout=5).read().decode())", fakeUpstreamBaseURL+"/v1/models")))
	fmt.Fprintf(&b, "  sidecar network: %s\n", e2eCommandOutput("docker", "inspect", sidecarContainer, "--format", "{{json .NetworkSettings.Networks}}"))
	fmt.Fprintf(&b, "  proxy network: %s\n", e2eCommandOutput("docker", "inspect", proxy.Container, "--format", "{{json .NetworkSettings.Networks}}"))
	fmt.Fprintf(&b, "  sidecar logs:\n%s\n", indentE2EDiagnostic(e2eCommandOutput("docker", "logs", "--tail", "120", sidecarContainer)))
	fmt.Fprintf(&b, "  proxy logs:\n%s\n", indentE2EDiagnostic(e2eCommandOutput("docker", "logs", "--tail", "120", proxy.Container)))
	return redactE2EDiagnosticSecrets(b.String())
}

func e2eHostProxyProbe(proxy proxyRuntime) string {
	if proxy.Port == 0 {
		return "no resolved port"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(proxy.BaseURL() + "/health/liveliness")
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	return resp.Status
}

func e2eCommandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return err.Error()
		}
		return text + " (" + err.Error() + ")"
	}
	if text == "" {
		return "<empty>"
	}
	return text
}

func indentE2EDiagnostic(text string) string {
	if text == "" {
		return "    <empty>"
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "    " + line
	}
	return strings.Join(lines, "\n")
}

func redactE2EDiagnosticSecrets(text string) string {
	re := regexp.MustCompile(`sk-[A-Za-z0-9._-]+`)
	return re.ReplaceAllString(text, "sk-[redacted]")
}

func assertE2EObservabilityTraceIngested(t *testing.T, state *RootState, lab, orch string) {
	t.Helper()
	ref := config.LabRef{Lab: lab, Orch: orch, Driver: state.Driver.Name()}
	values, err := readLabGatewayEnv(config.StateDir(state.Flags.StateDir, state.RepoDir), ref)
	require.NoError(t, err)
	projectID := values[labLangfuseProjectIDEnv]
	require.NotEmpty(t, projectID)

	query := fmt.Sprintf("SELECT count() FROM traces WHERE project_id = '%s'", strings.ReplaceAll(projectID, "'", "''"))
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "exec", state.observabilityRuntime().ClickHouseContainer(), "clickhouse-client", "--query", query).CombinedOutput()
		last = strings.TrimSpace(string(out))
		if err == nil && last != "" && last != "0" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	require.Failf(t, "Langfuse trace not ingested", "project_id=%s last_count=%q", projectID, last)
}

func e2ELiteLLMSmokeModel(orch string) string {
	if orch == "codex" {
		return "gpt-5.5"
	}
	return "claude-opus-4-8"
}

func assertE2EPhase(t *testing.T, stateDir, id string, phase phases.Phase) {
	t.Helper()
	require.True(t, phases.Done(stateDir, id, phase), "phase %s must be marked", phase)
}

func assertE2EList(t *testing.T, root *cobra.Command, tb *dockerTestBuf, lab, orch, state, phase string) {
	t.Helper()
	out := runE2ECommand(t, root, tb, "ls", lab)
	require.Contains(t, out, lab)
	require.Contains(t, out, orch)
	require.Contains(t, out, state)
	require.Contains(t, out, phase)
}
