package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

func TestCommandsFromDeletedWorkingDirectory(t *testing.T) {
	for _, name := range []string{"up", "workspace", "existing-up", "existing-up-profile", "existing-run", "workspace-new-repo", "cached-up", "mirror-workspace", "prepare", "create", "no-workspace", "skip-workspace", "version", "help", "dry-run", "relative-state"} {
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
	existing := strings.HasPrefix(name, "existing-") || name == "workspace" || name == "workspace-new-repo" || name == "cached-up" || name == "mirror-workspace"
	var refBefore []byte
	markersBefore := map[string][]byte{}
	if existing {
		createCLITestLabWithOrch(t, state, "demo", "claude-code")
		ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("demo"))
		require.NoError(t, err)
		require.True(t, ok)
		if name != "workspace-new-repo" {
			ref.Workspace = &config.Workspace{Repo: "https://github.com/example/project.git"}
		}
		if name == "mirror-workspace" {
			source := filepath.Join(t.TempDir(), "source.git")
			out, err := exec.Command("git", "init", "--bare", source).CombinedOutput()
			require.NoError(t, err, string(out))
			ref.Workspace.Repo = source
			mirror := hostWorkspaceBareRepoPath(state.Flags.StateDir, ref)
			require.NoError(t, os.MkdirAll(filepath.Dir(mirror), 0700))
			out, err = exec.Command("git", "clone", "--mirror", source, mirror).CombinedOutput()
			require.NoError(t, err, string(out))
			runWorkspaceGit = func(ctx context.Context, dir string, args ...string) error {
				gitCommands = append(gitCommands, dir+" git "+strings.Join(args, " "))
				return runWorkspaceGitCommand(ctx, dir, args...)
			}
		}
		require.NoError(t, state.Driver.WriteLabRef(testCtx(t), idName("demo"), ref))
		refBefore, err = os.ReadFile(filepath.Join(state.Flags.StateDir, "demo", "ref.json"))
		require.NoError(t, err)
		for _, phase := range phases.Order {
			require.NoError(t, phases.Mark(state.Flags.StateDir, idName("demo"), phase))
			marker := filepath.Join(phases.Dir(state.Flags.StateDir, idName("demo")), string(phase)+".done")
			markersBefore[marker], err = os.ReadFile(marker)
			require.NoError(t, err)
		}
		if name == "existing-up-profile" {
			require.NoError(t, os.MkdirAll(filepath.Join(state.RepoDir, "orchestrators", ref.Orch, "profiles", "new"), 0700))
		}
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
	case "existing-up":
		args = []string{"up", "demo", "--repo", "https://github.com/example/project.git", "--set", "codex-version=new", "--skip-auth-check", "--skip-gateway"}
	case "existing-up-profile":
		args = []string{"up", "demo", "--profile", "new", "--skip-auth-check", "--skip-gateway"}
	case "existing-run":
		args = []string{"run", "demo", "--set", "codex-version=new", "--skip-auth-check", "--skip-gateway"}
	case "workspace-new-repo":
		args = []string{"workspace", "demo", "--repo", "https://github.com/example/project.git", "--set", "codex-version=new"}
	case "cached-up":
		args = []string{"up", "demo", "--skip-auth-check", "--skip-gateway"}
	case "mirror-workspace":
		args = []string{"workspace", "demo"}
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
	wantError := name == "up" || name == "workspace" || strings.HasPrefix(name, "existing-") || name == "workspace-new-repo"
	if !wantError {
		require.NoError(t, err)
		if name == "relative-state" {
			require.FileExists(t, filepath.Join(cwd, "relative-state", "demo", "ref.json"))
		} else if name == "version" || name == "help" || name == "dry-run" {
			require.NoDirExists(t, filepath.Join(state.Flags.StateDir, "demo"))
		} else {
			require.FileExists(t, filepath.Join(state.Flags.StateDir, "demo", "ref.json"))
		}
		if name == "mirror-workspace" {
			require.Len(t, gitCommands, 1)
			require.Contains(t, gitCommands[0], "git remote update --prune")
			require.NotContains(t, gitCommands[0], "clone")
		}
		return
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "current working directory")
	require.Contains(t, err.Error(), "cd")
	require.Contains(t, err.Error(), "rerun")
	require.Empty(t, gitCommands, "Git must not run from the deleted directory")
	require.Empty(t, mock.ExecLog, "no lifecycle script should run before the diagnostic")
	if existing {
		refAfter, err := os.ReadFile(filepath.Join(state.Flags.StateDir, "demo", "ref.json"))
		require.NoError(t, err)
		require.Equal(t, refBefore, refAfter, "preflight must precede persisted workspace, settings and profile changes")
		for marker, before := range markersBefore {
			after, err := os.ReadFile(marker)
			require.NoError(t, err, "preflight must precede phase invalidation")
			require.Equal(t, before, after)
		}
		ref, ok, err := state.Driver.ReadLabRef(testCtx(t), idName("demo"))
		require.NoError(t, err)
		require.True(t, ok)
		if ref.Workspace != nil {
			require.Empty(t, ref.Workspace.Fork, "workspace preparation must not persist a mirror before the diagnostic")
		}
	} else {
		require.NoDirExists(t, filepath.Join(state.Flags.StateDir, "demo"), "preflight must precede provisioning")
	}
}
