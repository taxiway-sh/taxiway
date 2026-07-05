package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/recording"
)

func buildRecordTestRoot(t *testing.T) (*cobra.Command, *RootState, *driver.MockDriver, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root, state, _, _ := buildTestRoot(t)
	mock, ok := state.Driver.(*driver.MockDriver)
	require.True(t, ok)
	root.AddCommand(newRecordCmd(state))

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	return root, state, mock, &stdout, &stderr
}

func createRecordLab(t *testing.T, state *RootState, mock *driver.MockDriver, lab, orch string) {
	t.Helper()
	ctx := context.Background()
	id := config.IDOf(lab)
	require.NoError(t, mock.Create(ctx, id, driver.CreateOptions{Lab: lab, Orch: orch}))
	require.NoError(t, mock.WriteLabRef(ctx, id, config.LabRef{
		Lab:    lab,
		Orch:   orch,
		Driver: mock.Name(),
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(state.RepoDir, "orchestrators", orch), 0o755))
}

func TestRecordHelpIsRegistered(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Manage lab recordings")
	require.Contains(t, out, "start")
	require.Contains(t, out, "stop")
	require.Contains(t, out, "list")
	require.Contains(t, out, "rm")
	require.Contains(t, out, "player")
	require.Contains(t, out, "analyze")
	require.Empty(t, errOut)
}

func TestRecordPlayerHelpDocumentsNoOpen(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "player", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Serve the browser player")
	require.Contains(t, out, "no-open")
	require.Contains(t, out, "stable per lab")
	require.NotContains(t, out, "default 8000")
	require.Empty(t, errOut)
}

func TestRecordRmHelpDocumentsForce(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "rm", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Remove a lab recording")
	require.Contains(t, out, "--name")
	require.Contains(t, out, "Removal is explicit")
	require.Contains(t, out, "force")
	require.Empty(t, errOut)
}

func TestRecordStopHelpDocumentsDefaultLatest(t *testing.T) {
	root, _, _, stdout, stderr := buildRecordTestRoot(t)

	out, errOut, err := execRoot(t, root, stdout, stderr, "record", "stop", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "Stop a running lab recording")
	require.Contains(t, out, "When --name is omitted, Taxiway stops the latest active recording")
	require.Contains(t, out, "--name")
	require.NotContains(t, out, "--latest")
	require.Empty(t, errOut)
}

func TestRecordStartUsesDefaultShellTarget(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	var commands [][]string
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		commands = append(commands, slices.Clone(req.Argv))
		return driver.MockExecResponse{ExitCode: 0}
	}

	out, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)
	require.Contains(t, out, "Recording started")
	require.Contains(t, out, "walkthrough")
	require.Contains(t, joinedCommands(commands), "tmux has-session -t sampleorch")
	require.Contains(t, joinedCommands(commands), "asciinema rec")
	require.Contains(t, joinedCommands(commands), "tmux attach-session -f read-only,ignore-size -t sampleorch")
	require.Contains(t, joinedCommands(commands), "/lab/recordings/")
	require.Contains(t, joinedCommands(commands), "test -d '/lab/recordings'")
	require.NotContains(t, joinedCommands(commands), "mkdir -p /lab/recordings")
	require.NotContains(t, joinedCommands(commands), "/lab/work/recordings")
	require.Contains(t, out, filepath.Join(stateDir, "demo", "recordings"))
	require.NotContains(t, out, filepath.Join(stateDir, "demo", "work", "recordings"))
	require.NotContains(t, joinedCommands(commands), "gastown")
}

func TestRecordStartUsesStableRecordingSizeAndResizesTargetTmux(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")

	var commands [][]string
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		commands = append(commands, slices.Clone(req.Argv))
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)

	joined := joinedCommands(commands)
	resizeCommand := fmt.Sprintf("tmux resize-window -t sampleorch -x %d -y %d", recordingDefaultCols, recordingDefaultRows)
	startCommand := fmt.Sprintf("tmux new-session -d -x %d -y %d -s", recordingDefaultCols, recordingDefaultRows)
	require.Contains(t, joined, resizeCommand)
	require.Contains(t, joined, startCommand)
	require.Less(t, strings.Index(joined, resizeCommand), strings.Index(joined, startCommand), "target tmux must be resized before recorder starts")
	require.Contains(t, joined, "tmux attach-session -f read-only,ignore-size -t sampleorch")
}

func TestRecordStartUsesManifestShellCommand(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "customorch")
	require.NoError(t, os.WriteFile(filepath.Join(state.RepoDir, "orchestrators", "customorch", "manifest.yaml"), []byte("name: customorch\nshell:\n  command: [\"custom\", \"attach\", \"--main\"]\n"), 0o644))

	var commands [][]string
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		commands = append(commands, slices.Clone(req.Argv))
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "custom")
	require.NoError(t, err)
	require.Contains(t, joinedCommands(commands), "custom attach --main")
}

func TestRecordStartFailsWhenAnyRecordingIsAlreadyActive(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "first")
	require.NoError(t, err)

	_, _, err = execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "second")
	require.Error(t, err)
	require.Contains(t, err.Error(), "recording \"first\" is already active for lab \"demo\"")
	require.Contains(t, err.Error(), "stop it before starting a new one")
}

func TestRecordStartFailsWhenShellTargetMissing(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		if strings.Contains(strings.Join(req.Argv, " "), "tmux has-session") {
			return driver.MockExecResponse{ExitCode: 1}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo")
	require.Error(t, err)
	require.Contains(t, err.Error(), `lab "demo" shell session "sampleorch" is not running`)
	require.Contains(t, err.Error(), "tmux session may have been closed")
	require.Contains(t, err.Error(), "taxiway up demo --from start --force")
	require.Contains(t, err.Error(), "taxiway doctor demo")
}

func TestRecordListShowsNoRecordings(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")

	out, _, err := execRoot(t, root, stdout, stderr, "record", "list", "demo")
	require.NoError(t, err)
	require.Contains(t, out, "(no recordings found)")
}

func TestRecordListLsAlias(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")

	out, _, err := execRoot(t, root, stdout, stderr, "record", "ls", "demo")
	require.NoError(t, err)
	require.Contains(t, out, "(no recordings found)")
}

func TestRecordListAllShowsRecordingsForAllLabs(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "alpha", "sampleorch")
	createRecordLab(t, state, mock, "beta", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "alpha", "--name", "demo-alpha")
	require.NoError(t, err)
	_, _, err = execRoot(t, root, stdout, stderr, "record", "start", "beta", "--name", "demo-beta")
	require.NoError(t, err)

	out, _, err := execRoot(t, root, stdout, stderr, "record", "list")
	require.NoError(t, err)
	require.Contains(t, out, "LAB")
	require.Contains(t, out, "alpha")
	require.Contains(t, out, "demo-alpha")
	require.Contains(t, out, "beta")
	require.Contains(t, out, "demo-beta")
}

func TestRecordListAllAlignsLongLabNames(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	lab := "recording-lab-with-long-name"
	createRecordLab(t, state, mock, lab, "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", lab, "--name", "demo")
	require.NoError(t, err)

	out, _, err := execRoot(t, root, stdout, stderr, "record", "list")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	headerNameIdx := strings.Index(lines[0], "NAME")
	rowNameIdx := strings.Index(lines[1], "demo")
	require.NotEqual(t, -1, headerNameIdx)
	require.Equal(t, headerNameIdx, rowNameIdx, out)
}

func TestRecordStopWithoutNameStopsLatestActiveRecording(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)

	var stopCommands [][]string
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		stopCommands = append(stopCommands, slices.Clone(req.Argv))
		return driver.MockExecResponse{ExitCode: 0}
	}
	out, _, err := execRoot(t, root, stdout, stderr, "record", "stop", "demo")
	require.NoError(t, err)
	require.Contains(t, out, "Recording stopped")
	require.Contains(t, joinedCommands(stopCommands), "tmux send-keys")
	require.Contains(t, joinedCommands(stopCommands), "C-b d")

	out, _, err = execRoot(t, root, stdout, stderr, "record", "list", "demo")
	require.NoError(t, err)
	require.Contains(t, out, "stopped")
	require.Contains(t, out, "-walkthrough.cast")
	require.Contains(t, out, ".cast")
	require.NotContains(t, out, filepath.Join(state.Flags.StateDir, "demo", "recordings"))
}

func TestRecordRmDeletesStoppedRecordingAndCast(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)
	_, _, err = execRoot(t, root, stdout, stderr, "record", "stop", "demo", "--name", "walkthrough")
	require.NoError(t, err)

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, "demo")
	idx, err := store.Load()
	require.NoError(t, err)
	require.Len(t, idx.Sessions, 1)
	castPath := idx.Sessions[0].CastPathHost
	require.NoError(t, os.WriteFile(castPath, []byte("cast"), 0o644))

	out, _, err := execRoot(t, root, stdout, stderr, "record", "rm", "demo", "--name", "walkthrough")
	require.NoError(t, err)
	require.Contains(t, out, "Recording removed: walkthrough")
	require.NoFileExists(t, castPath)

	idx, err = store.Load()
	require.NoError(t, err)
	require.Empty(t, idx.Sessions)
}

func TestRecordRmRefusesActiveRecording(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)

	_, _, err = execRoot(t, root, stdout, stderr, "record", "rm", "demo", "--name", "walkthrough")
	require.Error(t, err)
	require.Contains(t, err.Error(), "recording \"walkthrough\" is active")
	require.Contains(t, err.Error(), "stop it before removing it")
}

func TestRecordRmForceStopsActiveRecordingThenDeletesIt(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	store := recording.NewStore(stateDir, "demo")
	idx, err := store.Load()
	require.NoError(t, err)
	require.Len(t, idx.Sessions, 1)
	castPath := idx.Sessions[0].CastPathHost
	require.NoError(t, os.WriteFile(castPath, []byte("cast"), 0o644))

	var removeCommands [][]string
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		removeCommands = append(removeCommands, slices.Clone(req.Argv))
		return driver.MockExecResponse{ExitCode: 0}
	}
	out, _, err := execRoot(t, root, stdout, stderr, "record", "rm", "demo", "--name", "walkthrough", "--force")
	require.NoError(t, err)
	require.Contains(t, out, "Recording removed: walkthrough")
	require.Contains(t, joinedCommands(removeCommands), "tmux send-keys")
	require.Contains(t, joinedCommands(removeCommands), "C-b d")
	require.NoFileExists(t, castPath)

	idx, err = store.Load()
	require.NoError(t, err)
	require.Empty(t, idx.Sessions)
}

func TestRecordListAllShowsCastBasename(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	mock.ExecResponder = func(_ string, _ driver.ExecRequest) driver.MockExecResponse {
		return driver.MockExecResponse{ExitCode: 0}
	}
	_, _, err := execRoot(t, root, stdout, stderr, "record", "start", "demo", "--name", "walkthrough")
	require.NoError(t, err)

	out, _, err := execRoot(t, root, stdout, stderr, "record", "list")
	require.NoError(t, err)
	require.Contains(t, out, "demo")
	require.Contains(t, out, "walkthrough")
	require.Contains(t, out, ".cast")
	require.NotContains(t, out, filepath.Join(state.Flags.StateDir, "demo", "recordings"))
}

func TestRecordPlayerWriteOnlyWritesIndex(t *testing.T) {
	root, state, mock, stdout, stderr := buildRecordTestRoot(t)
	createRecordLab(t, state, mock, "demo", "sampleorch")
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	recordingsDir := filepath.Join(stateDir, "demo", "recordings")
	require.NoError(t, os.MkdirAll(recordingsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(recordingsDir, "recordings.json"), []byte(`{"sessions":[]}`+"\n"), 0o644))

	out, _, err := execRoot(t, root, stdout, stderr, "record", "player", "demo", "--write-only")
	require.NoError(t, err)
	require.Contains(t, out, "Recording player ready")
	require.Contains(t, out, "\n\n")
	require.Contains(t, out, "  Player:")
	require.Contains(t, out, "  Recordings:")
	require.Contains(t, out, filepath.Join(recordingsDir, "index.html"))
	require.Contains(t, out, recordingsDir)

	data, err := os.ReadFile(filepath.Join(recordingsDir, "index.html"))
	require.NoError(t, err)
	require.Contains(t, string(data), `fetch("recordings.json"`)
}

func TestRecordPlayerPortAllocatesStablePortPerLab(t *testing.T) {
	stateDir := t.TempDir()

	port, err := recordPlayerPort(stateDir, "demo", 0, false)
	require.NoError(t, err)
	require.NotZero(t, port)

	again, err := recordPlayerPort(stateDir, "demo", 0, false)
	require.NoError(t, err)
	require.Equal(t, port, again)

	data, err := os.ReadFile(filepath.Join(stateDir, "demo", recordPlayerPortFile))
	require.NoError(t, err)
	require.Contains(t, string(data), fmt.Sprintf("%d", port))
}

func TestRecordPlayerPortSkipsPortsAllocatedToOtherLabs(t *testing.T) {
	stateDir := t.TempDir()
	alphaPort, err := recordPlayerPort(stateDir, "alpha", 0, false)
	require.NoError(t, err)

	betaPort, err := recordPlayerPort(stateDir, "beta", 0, false)
	require.NoError(t, err)
	require.NotEqual(t, alphaPort, betaPort)
}

func TestRecordPlayerPortUsesExplicitPortWithoutPersisting(t *testing.T) {
	stateDir := t.TempDir()

	port, err := recordPlayerPort(stateDir, "demo", 8123, false)
	require.NoError(t, err)
	require.Equal(t, 8123, port)

	require.NoFileExists(t, filepath.Join(stateDir, "demo", recordPlayerPortFile))
}

func joinedCommands(commands [][]string) string {
	parts := make([]string, 0, len(commands))
	for _, cmd := range commands {
		parts = append(parts, strings.Join(cmd, " "))
	}
	return strings.Join(parts, "\n")
}
