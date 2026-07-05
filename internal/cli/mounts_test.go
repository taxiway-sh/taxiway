package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// TestCreate_PassesStateMountDirs verifies that labUp calls Create with
// mutable directories rooted under <state-dir>/<lab>/.
func TestCreate_PassesStateMountDirs(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	// Run taxiway up gastown --type gastown (creates the lab)
	out, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err, "up failed: %s", out)

	require.NoDirExists(t, filepath.Join(stateDir, "gastown", "work"),
		"host-side work directory should not be created")

	expectedGitDir := filepath.Join(stateDir, "gastown", "git")
	require.Equal(t, expectedGitDir, mock.LastCreateOptions.GitDir,
		"GitDir must point to per-lab git remote directory")
	require.DirExists(t, expectedGitDir,
		"GitDir must exist before Create so drivers can mount it at /lab/git")

	expectedRecordingsDir := filepath.Join(stateDir, "gastown", "recordings")
	require.Equal(t, expectedRecordingsDir, mock.LastCreateOptions.RecordingsDir,
		"RecordingsDir must point to per-lab recordings directory")
	require.DirExists(t, expectedRecordingsDir,
		"RecordingsDir must exist before Create so drivers can mount it at /lab/recordings")

	// Orch should be passed through
	require.Equal(t, "gastown", mock.LastCreateOptions.Orch)

	// Lab should be passed through
	require.Equal(t, "gastown", mock.LastCreateOptions.Lab)

	// RepoDir should match state.RepoDir
	require.Equal(t, state.RepoDir, mock.LastCreateOptions.RepoDir)
}

func TestStart_RecreatesMissingStateMountDirs(t *testing.T) {
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	id := config.IDOf("gastown")
	gitDir := filepath.Join(stateDir, "gastown", "git")
	recordingsDir := filepath.Join(stateDir, "gastown", "recordings")

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)
	require.NoError(t, mock.Stop(context.Background(), id))
	require.NoError(t, os.RemoveAll(gitDir))
	require.NoError(t, os.RemoveAll(recordingsDir))

	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "gastown", "--type", "gastown")
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(stateDir, "gastown", "work"),
		"host-side work directory should not be recreated")
	require.DirExists(t, gitDir,
		"GitDir must be recreated before starting an existing lab so /lab/git remains mountable")
	require.DirExists(t, recordingsDir,
		"RecordingsDir must be recreated before starting an existing lab so /lab/recordings remains mountable")
}

// TestExecScript_PassesLabPath verifies that execScriptWithRef translates the host
// script path to a lab path (/lab/...) before calling Exec.
func TestExecScript_PassesLabPath(t *testing.T) {
	root, _, mock, stdout, stderr := buildUpTestRoot(t)

	// First create the lab so the flat bootstrap command can run.
	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "gastown")
	require.NoError(t, err)

	// Reset ExecLog to only capture the next call.
	mock.ExecLog = nil

	// Run taxiway bootstrap gastown (flat alias) which calls execScriptWithRef with infra/commands/bootstrap.sh.
	_, _, err = execUpRoot(t, root, stdout, stderr, "bootstrap", "gastown")
	require.NoError(t, err)

	// The script should have been logged as "bootstrap.sh" (basename).
	require.True(t, containsStr(mock.ExecLog, "bootstrap.sh"),
		"expected bootstrap.sh in ExecLog, got %v", mock.ExecLog)
}

// TestExecScript_LabPath_Rejects_OutsideRepo verifies that a script path outside
// the repo directory causes hostScriptTolab to return an error (not reach Exec).
func TestExecScript_LabPath_Rejects_OutsideRepo(t *testing.T) {
	// Build state with a known repoDir.
	_, state, mock, _, _ := buildUpTestRoot(t)

	// Manually try to translate a path outside the repo.
	_, err := hostScriptToLab(state.RepoDir, "/tmp/evil.sh")
	require.Error(t, err, "expected error for script outside repo")

	// Exec should not have been called.
	require.Empty(t, mock.ExecLog)
}

// TestHostScriptToLab_WithRealPaths uses actual temp dir paths to verify
// the translation works end-to-end.
func TestHostScriptToLab_WithRealPaths(t *testing.T) {
	repoDir := t.TempDir()

	script := filepath.Join(repoDir, "infra", "commands", "bootstrap.sh")
	got, err := hostScriptToLab(repoDir, script)
	require.NoError(t, err)

	// Must start with /lab/ and end with infra/commands/bootstrap.sh
	require.True(t, strings.HasPrefix(got, "/lab/"),
		"lab path should start with /lab/, got: %s", got)
	require.True(t, strings.HasSuffix(got, "infra/commands/bootstrap.sh"),
		"lab path should end with infra/commands/bootstrap.sh, got: %s", got)
}
