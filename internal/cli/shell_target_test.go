package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestResolveShellTargetUsesManifestCommand(t *testing.T) {
	stateDir := t.TempDir()
	repoDir := t.TempDir()
	writeShellManifest(t, repoDir, "testorch", "name: testorch\nshell:\n  command: [\"echo\", \"test\"]\n")
	mock := driver.NewMockDriver(stateDir)
	state := &RootState{RepoDir: repoDir, Flags: GlobalFlags{StateDir: stateDir}, Driver: mock}
	ref := config.LabRef{Lab: "testorch", Orch: "testorch", Driver: "mock"}

	target, err := resolveShellTarget(context.Background(), state, ref)

	require.NoError(t, err)
	require.Equal(t, LabRepoRoot, target.Workdir)
	require.Equal(t, "exec 'echo' 'test'", target.Command)
	require.Equal(t, "echo test", target.AttachCommand)
	require.False(t, target.RequiresTmux)
	require.Empty(t, mock.ExecLog)
}

func TestResolveShellTargetRequiresExistingTmuxSessionForDefaultTarget(t *testing.T) {
	stateDir := t.TempDir()
	repoDir := t.TempDir()
	writeShellManifest(t, repoDir, "gastown", "name: gastown\n")
	mock := driver.NewMockDriver(stateDir)
	id := config.IDOf("gastown")
	require.NoError(t, mock.Create(context.Background(), id, driver.CreateOptions{}))
	require.NoError(t, mock.WriteLabRef(context.Background(), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	checkedTmux := false
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		if len(req.Argv) >= 2 && req.Argv[0] == "tmux" && req.Argv[1] == "has-session" {
			checkedTmux = true
			return driver.MockExecResponse{ExitCode: 0}
		}
		return driver.MockExecResponse{ExitCode: 1}
	}
	state := &RootState{RepoDir: repoDir, Flags: GlobalFlags{StateDir: stateDir}, Driver: mock}
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}

	target, err := resolveShellTarget(context.Background(), state, ref)

	require.NoError(t, err)
	require.True(t, target.RequiresTmux)
	require.Equal(t, "gastown", target.SessionName)
	require.Equal(t, "tmux attach-session -t gastown", target.AttachCommand)
	require.Contains(t, target.Command, "tmux attach-session -t gastown")
	require.True(t, checkedTmux)
}

func TestResolveShellTargetErrorsWhenDefaultTmuxSessionIsMissing(t *testing.T) {
	stateDir := t.TempDir()
	repoDir := t.TempDir()
	writeShellManifest(t, repoDir, "gastown", "name: gastown\n")
	mock := driver.NewMockDriver(stateDir)
	id := config.IDOf("gastown")
	require.NoError(t, mock.Create(context.Background(), id, driver.CreateOptions{}))
	require.NoError(t, mock.WriteLabRef(context.Background(), id, config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}))
	mock.ExecResponder = func(id string, req driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 1}
	}
	state := &RootState{RepoDir: repoDir, Flags: GlobalFlags{StateDir: stateDir}, Driver: mock}
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}

	_, err := resolveShellTarget(context.Background(), state, ref)

	require.Error(t, err)
	require.Contains(t, err.Error(), `shell session "gastown" is not running`)
	require.Contains(t, err.Error(), "tmux session may have been closed")
	require.Contains(t, err.Error(), "taxiway up gastown --from start --force")
	require.Contains(t, err.Error(), "taxiway doctor")
	require.NotContains(t, err.Error(), "taxiway shell gastown")
}

func writeShellManifest(t *testing.T, repoDir, orch, content string) {
	t.Helper()
	dir := filepath.Join(repoDir, "orchestrators", orch)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0o644))
}
