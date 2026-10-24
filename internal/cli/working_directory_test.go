package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestCommandsFromDeletedWorkingDirectory(t *testing.T) {
	for _, name := range []string{"up", "workspace", "prepare", "create", "no-workspace", "skip-workspace", "version", "help", "dry-run", "relative-state"} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestWorkingDirectoryProcess$")
			cmd.Env = append(os.Environ(), "TAXIWAY_TEST_WORKING_DIRECTORY="+name)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
		})
	}
}

// A subprocess owns the cwd mutation so neither parallel tests nor the parent
// process can observe the deleted directory.
func TestWorkingDirectoryProcess(t *testing.T) {
	name := os.Getenv("TAXIWAY_TEST_WORKING_DIRECTORY")
	if name == "" {
		return
	}
	root, state, mock, stdout, stderr := buildUpTestRoot(t)
	var gitCommands []string
	installFakeWorkspaceGit(t, &gitCommands)
	if name == "workspace" {
		createCLITestLab(t, state, "demo")
		ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("demo"))
		require.NoError(t, err)
		require.True(t, ok)
		ref.Workspace = &config.Workspace{Repo: "https://github.com/example/project.git"}
		require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("demo"), ref))
	}
	original, err := os.Getwd()
	require.NoError(t, err)
	cwd := filepath.Join(t.TempDir(), "cwd")
	require.NoError(t, os.Mkdir(cwd, 0700))
	require.NoError(t, os.Chdir(cwd))
	defer func() { require.NoError(t, os.Chdir(original)) }()
	if name != "relative-state" {
		require.NoError(t, os.Remove(cwd))
		_, err = os.Getwd()
		require.Error(t, err, "the regression must start from a genuinely deleted cwd")
	}
	args := []string{name, "demo"}
	switch name {
	case "up":
		args = append(args, "--repo", "https://github.com/example/project.git", "--skip-auth-check", "--skip-gateway")
	case "version":
		args = []string{"version"}
	case "help":
		args = []string{"--help"}
	case "no-workspace":
		args = []string{"up", "demo", "--skip-auth-check", "--skip-gateway"}
	case "skip-workspace":
		args = []string{"up", "demo", "--repo", "https://github.com/example/project.git", "--skip-workspace", "--skip-auth-check", "--skip-gateway"}
	case "dry-run":
		args = []string{"create", "demo", "--dry-run"}
	case "relative-state":
		state.Flags.StateDir = "relative-state"
		state.Driver = driver.NewMockDriver(state.Flags.StateDir)
		args = []string{"create", "demo"}
	}
	_, _, err = execRoot(t, root, stdout, stderr, args...)
	if name != "up" && name != "workspace" {
		require.NoError(t, err)
		if name == "relative-state" {
			require.FileExists(t, filepath.Join(cwd, "relative-state", "demo", "ref.json"))
		} else if name == "version" || name == "help" || name == "dry-run" {
			require.NoDirExists(t, filepath.Join(state.Flags.StateDir, "demo"))
		} else {
			require.FileExists(t, filepath.Join(state.Flags.StateDir, "demo", "ref.json"))
		}
		return
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "current working directory")
	require.Contains(t, err.Error(), "cd")
	require.Contains(t, err.Error(), "rerun")
	require.Empty(t, gitCommands, "Git must not run from the deleted directory")
	require.Empty(t, mock.ExecLog, "no lifecycle script should run before the diagnostic")
	if name == "workspace" {
		ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("demo"))
		require.NoError(t, err)
		require.True(t, ok)
		require.Empty(t, ref.Workspace.Fork, "workspace preparation must not persist a mirror before the diagnostic")
	} else {
		require.NoDirExists(t, filepath.Join(state.Flags.StateDir, "demo"), "preflight must precede provisioning")
	}
}
