package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

// ── taxiway up <lab> --type <orch> creates lab taxiway-<lab> ──────────────────────────

func TestLabUp_CreatesCorrectID(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	id := "taxiway-mon-lab"
	exists, err := state.Driver.Exists(testCtx(t), id)
	require.NoError(t, err)
	require.True(t, exists, "lab %s should exist", id)

	// The old-style lab "taxiway-gastown" must NOT have been created.
	existsOld, _ := state.Driver.Exists(testCtx(t), "taxiway-gastown")
	require.False(t, existsOld, "taxiway-gastown should not exist; lab is taxiway-mon-lab")
}

func TestLabUp_CreatesContextualRuntimeID(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	id := "taxiway-dev-a1b2c3d4-mon-lab"
	exists, err := state.Driver.Exists(testCtx(t), id)
	require.NoError(t, err)
	require.True(t, exists, "lab %s should exist", id)

	ref, ok, err := config.ReadLabRef(stateDir, id)
	require.NoError(t, err)
	require.True(t, ok, "ref.json should be readable through the contextual runtime id")
	require.Equal(t, "mon-lab", ref.Lab)

	_, err = os.Stat(filepath.Join(stateDir, "mon-lab", "ref.json"))
	require.NoError(t, err, "lab state should stay keyed by the plain lab name")
}

// ── Sidecar written after Create ───────────────────────────────────────────

func TestLabUp_WritesSidecar(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	ref, ok, err := config.ReadLabRef(stateDir, "taxiway-mon-lab")
	require.NoError(t, err)
	require.True(t, ok, "ref.json should have been written")
	require.Equal(t, "mon-lab", ref.Lab)
	require.Equal(t, "gastown", ref.Orch)
}

// ── Phase markers go under taxiway-<lab> ─────────────────────────────────────────

func TestLabUp_PhaseMarkersUnderLabID(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	id := "taxiway-mon-lab"
	require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap marker under %s", id)
	require.False(t, phases.Done(stateDir, "taxiway-gastown", phases.PhaseBootstrap), "no marker under taxiway-gastown")
}

// ── Two labs, independent phase isolation ────────────────────────────────────

func TestLabUp_TwoLabs_PhaseIsolation(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	id1, id2 := "taxiway-lab1", "taxiway-lab2"

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "lab1", "--type", "gastown")
	require.NoError(t, err)

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "lab2", "--type", "codex")
	require.NoError(t, err)

	for _, id := range []string{id1, id2} {
		exists, err := state.Driver.Exists(testCtx(t), id)
		require.NoError(t, err)
		require.True(t, exists, "lab %s should exist", id)
		require.True(t, phases.Done(stateDir, id, phases.PhaseBootstrap), "bootstrap marker under %s", id)
	}

	// Deleting lab1 must not affect lab2
	_, _, err = execUpRoot(t, root, stdout, stderr, "rm", "--yes", "lab1")
	require.NoError(t, err)

	exists1, _ := state.Driver.Exists(testCtx(t), id1)
	require.False(t, exists1, "id1 should be deleted")

	exists2, _ := state.Driver.Exists(testCtx(t), id2)
	require.True(t, exists2, "id2 should still exist")

	require.True(t, phases.Done(stateDir, id2, phases.PhaseBootstrap),
		"id2 phase markers should not be cleared by rm lab1")
}

// ── taxiway rm purges the sidecar ─────────────────────────────────────────────────

func TestRmPurgesSidecar(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	ref, ok, err := config.ReadLabRef(stateDir, "taxiway-mon-lab")
	require.NoError(t, err)
	require.True(t, ok, "ref.json should exist before rm")

	require.NoError(t, ensureLabGatewayEnv(state, ref))
	values, err := readLabGatewayEnv(stateDir, ref)
	require.NoError(t, err)
	values[labLangfuseProjectPublicKeyEnv] = "pk-lf-lab"
	values[labLangfuseProjectSecretKeyEnv] = "sk-lf-lab"
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, values))
	observabilityDir := observabilityStateDir(state)
	files, err := prepareLabLiteLLMSidecarFiles(state, stateDir, observabilityDir, ref)
	require.NoError(t, err)
	proxyDir := state.proxyRuntime().StateDir
	_, err = ensureProxyConfig(stateDir, proxyDir)
	require.NoError(t, err)

	require.FileExists(t, files.ComposePath)
	require.FileExists(t, labLiteLLMRoutePath(stateDir, ref))
	caddyPath := proxyConfigStatePath(proxyDir)
	require.FileExists(t, caddyPath)
	caddyBefore, err := os.ReadFile(caddyPath)
	require.NoError(t, err)
	require.Contains(t, string(caddyBefore), "mon-lab.litellm.localhost")

	origLangfuse := removeLabLangfuseProjectForRm
	removeLabLangfuseProjectForRm = func(_ *RootState, stateDir string, ref config.LabRef) error {
		values, err := readLabGatewayEnv(stateDir, ref)
		require.NoError(t, err)
		require.Equal(t, "pk-lf-lab", values[labLangfuseProjectPublicKeyEnv])
		require.Equal(t, "sk-lf-lab", values[labLangfuseProjectSecretKeyEnv])
		return nil
	}
	t.Cleanup(func() {
		removeLabLangfuseProjectForRm = origLangfuse
	})

	// rm the lab.
	_, _, err = execUpRoot(t, root, stdout, stderr, "rm", "--yes", "mon-lab")
	require.NoError(t, err)

	// lab should be gone.
	exists, _ := state.Driver.Exists(testCtx(t), "taxiway-mon-lab")
	require.False(t, exists)

	// Sidecar should be gone (Delete → os.RemoveAll on lab dir).
	_, ok, err = config.ReadLabRef(stateDir, "taxiway-mon-lab")
	require.NoError(t, err)
	require.False(t, ok, "ref.json should be removed after rm")
	require.NoFileExists(t, files.ComposePath)
	require.NoFileExists(t, labLiteLLMRoutePath(stateDir, ref))
	caddyAfter, err := os.ReadFile(caddyPath)
	require.NoError(t, err)
	require.NotContains(t, string(caddyAfter), "mon-lab.litellm.localhost")
}

func TestRmStopsSidecarBeforeRemovingLangfuseProjectAndSidecarState(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	ref, ok, err := config.ReadLabRef(stateDir, "taxiway-mon-lab")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, ensureLabGatewayEnv(state, ref))
	values, err := readLabGatewayEnv(stateDir, ref)
	require.NoError(t, err)
	values[labLangfuseProjectIDEnv] = "project-mon-lab"
	values[labLangfuseProjectPublicKeyEnv] = "pk-lf-lab"
	values[labLangfuseProjectSecretKeyEnv] = "sk-lf-lab"
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, values))

	var calls []string
	origStop := stopLabLiteLLMSidecarForRm
	origLangfuse := removeLabLangfuseProjectForRm
	origSidecar := removeLabLiteLLMSidecarForRm
	stopLabLiteLLMSidecarForRm = func(_ context.Context, _ *RootState, _ config.LabRef) error {
		values, err := readLabGatewayEnv(stateDir, ref)
		require.NoError(t, err)
		require.Equal(t, "project-mon-lab", values[labLangfuseProjectIDEnv])
		calls = append(calls, "stop-sidecar")
		return nil
	}
	removeLabLangfuseProjectForRm = func(_ *RootState, stateDir string, ref config.LabRef) error {
		values, err := readLabGatewayEnv(stateDir, ref)
		require.NoError(t, err)
		require.Equal(t, "project-mon-lab", values[labLangfuseProjectIDEnv])
		calls = append(calls, "langfuse")
		return nil
	}
	removeLabLiteLLMSidecarForRm = func(_ context.Context, _ *RootState, _ config.LabRef) error {
		calls = append(calls, "remove-sidecar-state")
		return nil
	}
	t.Cleanup(func() {
		stopLabLiteLLMSidecarForRm = origStop
		removeLabLangfuseProjectForRm = origLangfuse
		removeLabLiteLLMSidecarForRm = origSidecar
	})

	_, _, err = execUpRoot(t, root, stdout, stderr, "rm", "--yes", "mon-lab")
	require.NoError(t, err)
	require.Equal(t, []string{"stop-sidecar", "langfuse", "remove-sidecar-state"}, calls)
}

// ── default --type behavior: no --type flag → defaults to claude-code ───────────────────

func TestLabUp_DefaultType_ClaudeCode(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	// "up mon-lab" without --type → uses claude-code (default).
	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab")
	require.NoError(t, err)

	require.Equal(t, "claude-code", mock.LastCreateOptions.Orch,
		"default --type should be claude-code")

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	ref, ok, _ := config.ReadLabRef(stateDir, "taxiway-mon-lab")
	require.True(t, ok)
	require.Equal(t, "claude-code", ref.Orch)
}

// ── Anti-collision: same lab, different --type ────────────────────────────────

func TestLabUp_AntiCollision_DifferentType(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	// Second up with different type should fail with a clear error.
	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "codex")
	require.Error(t, err, "up with different --type on existing lab should fail")
	require.Contains(t, err.Error(), "mon-lab")
}

// ── Anti-collision: same lab, same --type → idempotent ───────────────────────

func TestLabUp_SameType_Idempotent(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	// Same up again → should succeed (already running).
	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)
}

// ── DryRun: no lab created, no sidecar written ─────────────────────────────────

func TestLabUp_DryRun_NoSidecar(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown", "--dry-run")
	require.NoError(t, err)

	exists, _ := state.Driver.Exists(testCtx(t), "taxiway-mon-lab")
	require.False(t, exists, "dry-run must not create lab")

	_, ok, err := config.ReadLabRef(stateDir, "taxiway-mon-lab")
	require.NoError(t, err)
	require.False(t, ok, "dry-run must not write sidecar")
}

// ── env vars passed to start.sh ───────────────────────────────────────────────

func TestLabUp_StartEnvVars(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	// Find the env passed to start.sh
	var startEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "start.sh" {
			startEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, startEnv, "start.sh should have been executed")
	require.Equal(t, "gastown", startEnv["TAXIWAY_ORCH"])
	require.Equal(t, "mon-lab", startEnv["TAXIWAY_LAB"])
	require.Equal(t, "taxiway-mon-lab", startEnv["TAXIWAY_ID"])
	require.NotEmpty(t, startEnv["TAXIWAY_DASHBOARD_HOST_PORT"])
	require.NotEqual(t, "8080", startEnv["TAXIWAY_DASHBOARD_HOST_PORT"])
	require.NotContains(t, startEnv, "TAXIWAY_HQ_DIR",
		"TAXIWAY_HQ_DIR defaults must be resolved inside the lab")
	_ = state
}

func TestGastownDashboardHostPortIsPerLabAndPersistent(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "alpha-lab", "--type", "gastown")
	require.NoError(t, err)
	alphaPort := latestStartEnv(t, mock)["TAXIWAY_DASHBOARD_HOST_PORT"]
	require.NotEmpty(t, alphaPort)
	require.NotEqual(t, "8080", alphaPort)

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "beta-lab", "--type", "gastown")
	require.NoError(t, err)
	betaPort := latestStartEnv(t, mock)["TAXIWAY_DASHBOARD_HOST_PORT"]
	require.NotEmpty(t, betaPort)
	require.NotEqual(t, "8080", betaPort)
	require.NotEqual(t, alphaPort, betaPort)

	_, _, err = execUpRoot(t, root, stdout, stderr, "start", "alpha-lab")
	require.NoError(t, err)
	require.Equal(t, alphaPort, latestStartEnv(t, mock)["TAXIWAY_DASHBOARD_HOST_PORT"])
}

func latestStartEnv(t *testing.T, mock *driver.MockDriver) map[string]string {
	t.Helper()
	for i := len(mock.ExecLog) - 1; i >= 0; i-- {
		if mock.ExecLog[i] == "start.sh" {
			return mock.ExecEnvLog[i]
		}
	}
	require.Fail(t, "start.sh should have been executed")
	return nil
}

// ── I6: No partial state quand WriteLabRef échoue ───────────────────────────

// TestLabUp_NoPartialStateOnSidecarFailure: si WriteLabRef échoue avant
// Create, l'état initialisé doit être nettoyé et une erreur retournée.
func TestLabUp_NoPartialStateOnSidecarFailure(t *testing.T) {
	tmp := t.TempDir()
	for _, orch := range []string{"gastown"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\n"), 0755))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra"), 0755))

	stateDir := filepath.Join(tmp, ".lab-state")
	mock := driver.NewMockDriver(stateDir)
	mock.RepoDir = tmp

	// Inject a WriteLabRef failure.
	mock.FailWriteLabRef = true

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  mock,
	}

	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	err := labUp(context.Background(), state, ref, nil)
	require.Error(t, err, "labUp should fail when WriteLabRef fails")
	require.Contains(t, err.Error(), "write lab ref", "error should mention sidecar write failure")

	// No fake lab should remain from the pre-create state directory.
	exists, _ := state.Driver.Exists(context.Background(), "taxiway-gastown")
	require.False(t, exists, "lab should not appear after pre-create sidecar failure")
}

// TestLabUp_NoRollbackWhenSidecarAlreadyPresent: si le sidecar existe déjà
// (ex: re-création crash-restart), ne pas rollback.
func TestLabUp_NoRollbackWhenSidecarAlreadyPresent(t *testing.T) {
	tmp := t.TempDir()
	for _, orch := range []string{"gastown"} {
		dir := filepath.Join(tmp, "orchestrators", orch)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\n"), 0755))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra"), 0755))

	stateDir := filepath.Join(tmp, ".lab-state")
	mock := driver.NewMockDriver(stateDir)
	mock.RepoDir = tmp

	// Pre-write a sidecar (simulates crash-restart where sidecar was written by Create).
	require.NoError(t, config.WriteLabRef(stateDir, "taxiway-gastown", config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))

	// Inject a WriteLabRef failure (2nd write attempt).
	mock.FailWriteLabRef = true

	// Also pre-create the lab state dir so Exists returns true... but we want
	// to test the new-lab path. Directly call labUp with a lab that does NOT yet
	// exist in the mock driver (only the sidecar dir exists).
	// Actually: the lab directory IS the sidecar directory in MockDriver.
	// So we need to also pre-create the lab via mock.
	require.NoError(t, mock.Create(context.Background(), "taxiway-gastown", driver.CreateOptions{}))

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  mock,
	}

	// This second labUp hits the "exists && already running" path → no WriteLabRef at all.
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	err := labUp(context.Background(), state, ref, nil)
	// Should succeed (lab already running).
	require.NoError(t, err, "labUp on an already-running lab should succeed without touching sidecar")
}

func TestLabUp_ExistingLabWithoutSidecarErrors(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".lab-state")
	mock := driver.NewMockDriver(stateDir)
	mock.RepoDir = tmp

	id := "taxiway-partial-lab"
	require.NoError(t, mock.Create(context.Background(), id, driver.CreateOptions{}))

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  mock,
	}
	ref := config.LabRef{
		Lab:  "partial-lab",
		Orch: "gastown",
	}

	err := labUp(context.Background(), state, ref, nil)
	require.EqualError(t, err, `lab "partial-lab" is missing ref.json`)
}

type inspectCreateDriver struct {
	*driver.MockDriver
	stateDir string
	sawRef   bool
	sawStamp bool
}

func (d *inspectCreateDriver) Create(_ context.Context, id string, _ driver.CreateOptions) error {
	lab := config.LabDirOf(id)
	_, d.sawRef, _ = config.ReadLabRef(d.stateDir, lab)
	_, stampErr := os.Stat(filepath.Join(d.stateDir, lab, "created_at"))
	d.sawStamp = stampErr == nil
	return fmt.Errorf("injected create failure")
}

func TestLabUp_InitializesLabStateBeforeDriverCreate(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".lab-state")
	inner := driver.NewMockDriver(stateDir)
	inner.RepoDir = tmp
	inspector := &inspectCreateDriver{MockDriver: inner, stateDir: stateDir}

	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{DryRun: false, StateDir: stateDir},
		Driver:  inspector,
	}
	ref := config.LabRef{Lab: "pending-lab", Orch: "gastown"}

	err := labUp(context.Background(), state, ref, nil)
	require.Error(t, err)
	require.True(t, inspector.sawRef, "ref.json should exist before driver create starts")
	require.True(t, inspector.sawStamp, "created_at should exist before driver create starts")
}

// ── B-R2: env vars in bootstrap/install/verify ───────────────────────────────

// TestUp_BootstrapReceivesTaxiwayEnvVars: bootstrap.sh reçoit TAXIWAY_ORCH, TAXIWAY_LAB,
// and TAXIWAY_ID. TAXIWAY_HQ_DIR is intentionally absent unless explicitly overridden.
func TestUp_BootstrapReceivesTaxiwayEnvVars(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	// Find env for bootstrap.sh
	var bootstrapEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "bootstrap.sh" {
			bootstrapEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, bootstrapEnv, "bootstrap.sh should have been executed")
	require.Equal(t, "gastown", bootstrapEnv["TAXIWAY_ORCH"])
	require.Equal(t, "mon-lab", bootstrapEnv["TAXIWAY_LAB"])
	require.Equal(t, "taxiway-mon-lab", bootstrapEnv["TAXIWAY_ID"])
	require.NotContains(t, bootstrapEnv, "TAXIWAY_HQ_DIR")
	_ = state
}

// TestUp_InstallReceivesTaxiwayEnvVars: install.sh reçoit TAXIWAY_ORCH, TAXIWAY_LAB, etc.
func TestUp_InstallReceivesTaxiwayEnvVars(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	var installEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "install.sh" {
			installEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, installEnv, "install.sh should have been executed")
	require.Equal(t, "gastown", installEnv["TAXIWAY_ORCH"])
	require.Equal(t, "mon-lab", installEnv["TAXIWAY_LAB"])
	require.Equal(t, "taxiway-mon-lab", installEnv["TAXIWAY_ID"])
	require.NotContains(t, installEnv, "TAXIWAY_HQ_DIR")
	_ = state
}

// TestUp_VerifyReceivesTaxiwayEnvVars: verify.sh reçoit TAXIWAY_ORCH, TAXIWAY_LAB, etc.
func TestUp_VerifyReceivesTaxiwayEnvVars(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "mon-lab", "--type", "gastown")
	require.NoError(t, err)

	var verifyEnv map[string]string
	for i, script := range mock.ExecLog {
		if script == "verify.sh" {
			verifyEnv = mock.ExecEnvLog[i]
			break
		}
	}
	require.NotNil(t, verifyEnv, "verify.sh should have been executed")
	require.Equal(t, "gastown", verifyEnv["TAXIWAY_ORCH"])
	require.Equal(t, "mon-lab", verifyEnv["TAXIWAY_LAB"])
	require.Equal(t, "taxiway-mon-lab", verifyEnv["TAXIWAY_ID"])
	require.NotContains(t, verifyEnv, "TAXIWAY_HQ_DIR")
	_ = state
}
