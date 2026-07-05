// Unit tests for package driver - no infrastructure dependency.
package driver

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/taxiway-sh/taxiway/internal/config"
)

// TestNormaliseDockerState verifies all state mappings including "created".
func TestNormaliseDockerState(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"running", "running"},
		{"exited", "stopped"},
		{"created", "stopped"}, // container stuck at docker run (never started)
		{"paused", "paused"},
		{"dead", "dead"},
		{"", ""},
	}
	for _, tc := range cases {
		got := normaliseDockerState(tc.in)
		require.Equal(t, tc.want, got, "input: %q", tc.in)
	}
}

// TestDockerList_SidecarGating verifies List only returns entries that
// have a completed created_at sidecar (i.e. Create finished successfully)
// and that entries without the sidecar are silently skipped.
func TestDockerList_SidecarGating(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")

	tmp := t.TempDir()
	d := NewDockerDriver(tmp)
	labsDir := tmp

	// Entry 1: complete sidecar — should appear in list (state will be "absent"
	// because there's no real Docker container, but the entry IS returned).
	// State dir uses lab name (no taxiway- prefix); List returns Name = "taxiway-complete".
	require.NoError(t, os.MkdirAll(filepath.Join(labsDir, "complete"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(labsDir, "complete", "created_at"),
		[]byte("2026-05-12T10:00:00Z"), 0o644,
	))

	// Entry 2: directory exists but no created_at (Create crashed mid-way).
	// Must be silently skipped by List.
	require.NoError(t, os.MkdirAll(filepath.Join(labsDir, "orphan"), 0o755))

	// Entry 3: plain file (not a directory) — must be skipped.
	require.NoError(t, os.WriteFile(
		filepath.Join(labsDir, "somefile"),
		[]byte(""), 0o644,
	))

	list, err := d.List(context.Background())
	require.NoError(t, err)

	// Only "taxiway-complete" must appear (orphan has no created_at, somefile is not a dir).
	require.Len(t, list, 1, "expected exactly 1 entry, orphan/file must be skipped")
	require.Equal(t, "taxiway-complete", list[0].Name)
	require.Equal(t, "docker", list[0].Driver)
	// Docker inspect will fail (no real daemon) → state normalises to "absent".
	require.Equal(t, "absent", list[0].State)
	// Created timestamp must be parsed correctly.
	require.False(t, list[0].Created.IsZero(), "Created must be parsed from sidecar")
}

func TestDockerList_EmptyStateDir(t *testing.T) {
	d := NewDockerDriver(t.TempDir())
	list, err := d.List(context.Background())
	require.NoError(t, err)
	require.Nil(t, list)
}

func TestDockerCreateRequiresSharedCreateOptions(t *testing.T) {
	d := NewDockerDriver(t.TempDir())

	err := d.Create(context.Background(), "taxiway-demo", CreateOptions{})

	require.EqualError(t, err, "docker driver: Lab, Orch, RepoDir, GitDir, RecordingsDir are all required")
}

func TestDockerCreateInvokesExpectedDockerCommands(t *testing.T) {
	logPath := installFakeDocker(t, `#!/bin/sh
printf '%s\n' "$*" >> "$TAXIWAY_FAKE_DOCKER_LOG"
exit 0
`)
	stateDir := t.TempDir()
	repoDir := t.TempDir()
	gitDir := filepath.Join(stateDir, "demo", "git")
	recordingsDir := filepath.Join(stateDir, "demo", "recordings")
	d := NewDockerDriver(stateDir)
	d.image = "ubuntu:test"

	err := d.Create(context.Background(), "taxiway-demo", CreateOptions{
		Lab:           "demo",
		Orch:          "gastown",
		RepoDir:       repoDir,
		GitDir:        gitDir,
		RecordingsDir: recordingsDir,
	})

	require.NoError(t, err)
	lines := readCommandLog(t, logPath)
	require.Equal(t, []string{
		"volume create taxiway-taxiway-demo-lab",
		"run -d --name taxiway-demo --add-host demo.litellm.localhost:host-gateway -e USER=taxiway -e HOME=/home/taxiway -e SHELL=/bin/bash -e LANG=C.UTF-8 -e LC_ALL=C.UTF-8 -e LC_CTYPE=C.UTF-8 -e TERM=xterm-256color -v taxiway-taxiway-demo-lab:/lab -v " + repoDir + "/infra:/lab/infra:ro -v " + repoDir + "/agents:/lab/agents:ro -v " + repoDir + "/orchestrators/gastown:/lab/orchestrators/gastown:ro -v " + gitDir + ":/lab/git -v " + recordingsDir + ":/lab/recordings ubuntu:test sleep infinity",
		"exec taxiway-demo apt-get update -qq",
		"exec taxiway-demo apt-get install -y -qq sudo",
		"exec taxiway-demo sh -c id -u 'taxiway' >/dev/null 2>&1 || useradd -m -s '/bin/bash' 'taxiway' && usermod -aG sudo 'taxiway' && printf '%s\\n' 'taxiway ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/taxiway-taxiway && chmod 0440 /etc/sudoers.d/taxiway-taxiway && mkdir -p /lab/work /lab/git /lab/recordings && chown -R 'taxiway:taxiway' /lab/work /lab/git /lab/recordings",
		"exec -i taxiway-demo tee /usr/local/bin/systemctl",
		"exec taxiway-demo chmod +x /usr/local/bin/systemctl",
	}, lines)
	require.FileExists(t, filepath.Join(stateDir, "demo", "created_at"))
	require.DirExists(t, gitDir)
	require.DirExists(t, recordingsDir)
}

func TestDockerRunArgsInjectsUtf8TerminalEnvironment(t *testing.T) {
	args := dockerRunArgs("taxiway-demo", "taxiway-demo-lab", CreateOptions{
		Lab:           "demo",
		Orch:          "claude-code",
		RepoDir:       "/repo",
		GitDir:        "/git",
		RecordingsDir: "/recordings",
	}, "ubuntu:test")

	require.Contains(t, args, "LANG=C.UTF-8")
	require.Contains(t, args, "LC_ALL=C.UTF-8")
	require.Contains(t, args, "LC_CTYPE=C.UTF-8")
	require.Contains(t, args, "TERM=xterm-256color")
}

func TestDockerExecBuildsDockerExecCommand(t *testing.T) {
	logPath := installFakeDocker(t, `#!/bin/sh
printf '%s\n' "$*" >> "$TAXIWAY_FAKE_DOCKER_LOG"
exit 7
`)
	d := NewDockerDriver(t.TempDir())
	var stdout, stderr bytes.Buffer

	result, err := d.Exec(context.Background(), "taxiway-demo", ExecRequest{
		Workdir: "/lab/work",
		Argv:    []string{"sh", "-c", "echo hi"},
		Env:     map[string]string{"TAXIWAY_AGENT": "codex"},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})

	require.NoError(t, err)
	require.Equal(t, 7, result.ExitCode)
	require.Equal(t, []string{
		"exec -u taxiway -e USER=taxiway -e HOME=/home/taxiway -e SHELL=/bin/bash -e TAXIWAY_AGENT=codex -w /lab/work taxiway-demo sh -c echo hi",
	}, readCommandLog(t, logPath))
}

func TestDockerShellExecUsesRawInteractiveOutput(t *testing.T) {
	installFakeDocker(t, `#!/bin/sh
printf '%s\n' '[detached (from session codex)]'
printf '%s\n' 'interactive shell output'
exit 0
`)
	d := NewDockerDriver(t.TempDir())

	stdout, stderr := captureProcessOutput(t, func() error {
		return d.ShellExec(context.Background(), "taxiway-demo", "/lab", "tmux attach-session -t codex")
	})

	require.Contains(t, stdout, "[detached (from session codex)]")
	require.Contains(t, stdout, "interactive shell output")
	require.Empty(t, stderr)
}

func TestDockerLifecycleCommands(t *testing.T) {
	logPath := installFakeDocker(t, `#!/bin/sh
printf '%s\n' "$*" >> "$TAXIWAY_FAKE_DOCKER_LOG"
exit 0
`)
	stateDir := t.TempDir()
	d := NewDockerDriver(stateDir)
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "demo"), 0o755))

	require.NoError(t, d.Start(context.Background(), "taxiway-demo"))
	require.NoError(t, d.Stop(context.Background(), "taxiway-demo"))
	require.NoError(t, d.Copy(context.Background(), "taxiway-demo", "/host/file", "/lab/file"))
	require.NoError(t, d.Delete(context.Background(), "taxiway-demo"))

	require.Equal(t, []string{
		"start taxiway-demo",
		"stop taxiway-demo",
		"cp /host/file taxiway-demo:/lab/file",
		"exec taxiway-demo chown -R taxiway:taxiway /lab/file",
		"rm -f taxiway-demo",
		"volume rm taxiway-taxiway-demo-lab",
	}, readCommandLog(t, logPath))
	require.NoDirExists(t, filepath.Join(stateDir, "demo"))
}

func TestDockerDeleteToleratesMissingContainerAndVolume(t *testing.T) {
	logPath := installFakeDocker(t, `#!/bin/sh
printf '%s\n' "$*" >> "$TAXIWAY_FAKE_DOCKER_LOG"
if [ "$1 $2" = "rm -f" ]; then
  echo 'No such container' >&2
  exit 1
fi
if [ "$1 $2" = "volume rm" ]; then
  echo 'No such volume' >&2
  exit 1
fi
exit 0
`)
	d := NewDockerDriver(t.TempDir())

	require.NoError(t, d.Delete(context.Background(), "taxiway-demo"))

	require.Equal(t, []string{
		"rm -f taxiway-demo",
		"volume rm taxiway-taxiway-demo-lab",
	}, readCommandLog(t, logPath))
}

func TestDockerInspectBasedState(t *testing.T) {
	installFakeDocker(t, `#!/bin/sh
case "$*" in
  'inspect --format={{.Name}} taxiway-demo')
    echo '/taxiway-demo'
    exit 0
    ;;
  'inspect --format={{.State.Running}} taxiway-demo')
    echo 'true'
    exit 0
    ;;
  'inspect --format {{.State.Status}} taxiway-demo')
    echo 'created'
    exit 0
    ;;
esac
exit 1
`)
	stateDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "demo", "created_at"), []byte("2026-05-12T10:00:00Z"), 0o644))
	d := NewDockerDriver(stateDir)

	exists, err := d.Exists(context.Background(), "taxiway-demo")
	require.NoError(t, err)
	require.True(t, exists)

	running, err := d.Running(context.Background(), "taxiway-demo")
	require.NoError(t, err)
	require.True(t, running)

	status, err := d.Status(context.Background(), "taxiway-demo")
	require.NoError(t, err)
	require.Equal(t, "taxiway-demo", status.Name)
	require.Equal(t, "stopped", status.State)
	require.Equal(t, "docker", status.Driver)
	require.False(t, status.Created.IsZero())
}

func TestDockerInspectFailuresMeanAbsent(t *testing.T) {
	installFakeDocker(t, `#!/bin/sh
exit 1
`)
	d := NewDockerDriver(t.TempDir())

	exists, err := d.Exists(context.Background(), "taxiway-missing")
	require.NoError(t, err)
	require.False(t, exists)

	running, err := d.Running(context.Background(), "taxiway-missing")
	require.NoError(t, err)
	require.False(t, running)

	status, err := d.Status(context.Background(), "taxiway-missing")
	require.NoError(t, err)
	require.Equal(t, Status{Name: "taxiway-missing", State: "absent", Driver: "docker"}, status)
}

func TestDockerLabRefRoundTrip(t *testing.T) {
	d := NewDockerDriver(t.TempDir())
	ref := config.LabRef{Lab: "demo", Orch: "gastown", Driver: "docker"}

	require.NoError(t, d.WriteLabRef(context.Background(), "taxiway-demo", ref))
	got, ok, err := d.ReadLabRef(context.Background(), "taxiway-demo")

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, ref, got)
}

func captureProcessOutput(t *testing.T, fn func() error) (string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = stdoutW
	os.Stderr = stderrW
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	runErr := fn()
	require.NoError(t, stdoutW.Close())
	require.NoError(t, stderrW.Close())
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdout, err := io.ReadAll(stdoutR)
	require.NoError(t, err)
	stderr, err := io.ReadAll(stderrR)
	require.NoError(t, err)
	require.NoError(t, runErr)
	return string(stdout), string(stderr)
}

func installFakeDocker(t *testing.T, script string) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	path := filepath.Join(binDir, "docker")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	t.Setenv("PATH", binDir)
	t.Setenv("TAXIWAY_FAKE_DOCKER_LOG", logPath)
	return logPath
}

func readCommandLog(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}
