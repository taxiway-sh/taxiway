package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/taxiway-sh/taxiway/internal/config"
)

var runWorkspaceGit = runWorkspaceGitCommand

func workspaceRepoCloneDir(ref config.LabRef) string {
	if ref.Workspace == nil || ref.Workspace.Repo == "" {
		return ""
	}
	return path.Join(LabWorkRoot, "repo", repoBasename(ref.Workspace.Repo))
}

func workspaceBareRepoPath(ref config.LabRef) string {
	if ref.Workspace == nil || ref.Workspace.Repo == "" {
		return ""
	}
	return path.Join(LabGitRoot, repoBasename(ref.Workspace.Repo)+".git")
}

func workspaceBareRepoURL(ref config.LabRef) string {
	barePath := workspaceBareRepoPath(ref)
	if barePath == "" {
		return ""
	}
	return "file://" + barePath
}

func hostWorkspaceBareRepoPath(stateDir string, ref config.LabRef) string {
	return filepath.Join(stateDir, ref.Lab, "git", repoBasename(ref.Workspace.Repo)+".git")
}

func hostWorkspaceCloneDir(stateDir string, ref config.LabRef) string {
	return filepath.Join(stateDir, ref.Lab, "work", "repo", repoBasename(ref.Workspace.Repo))
}

func workspaceConfigured(ref config.LabRef) bool {
	return ref.Workspace != nil && ref.Workspace.Repo != ""
}

func prepareWorkspaceRepository(ctx context.Context, state *RootState, ref *config.LabRef) error {
	if ref == nil || !workspaceConfigured(*ref) {
		return nil
	}

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	hostBareRepo := hostWorkspaceBareRepoPath(stateDir, *ref)
	labBareURL := workspaceBareRepoURL(*ref)

	if ref.Workspace.Fork != labBareURL {
		ref.Workspace.Fork = labBareURL
		if err := state.Driver.WriteLabRef(ctx, idName(ref.Lab), *ref); err != nil {
			return fmt.Errorf("persist local workspace repo: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(hostBareRepo), 0o755); err != nil {
		return fmt.Errorf("create workspace git dir: %w", err)
	}

	if _, err := os.Stat(filepath.Join(hostBareRepo, "objects")); os.IsNotExist(err) {
		if err := runWorkspaceGit(ctx, "", "clone", "--mirror", ref.Workspace.Repo, hostBareRepo); err != nil {
			return fmt.Errorf("mirror workspace source: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect workspace bare repo: %w", err)
	} else {
		if err := runWorkspaceGit(ctx, hostBareRepo, "remote", "update", "--prune"); err != nil {
			return fmt.Errorf("update workspace bare repo: %w", err)
		}
	}

	if ref.Workspace.Ref != "" {
		if err := runWorkspaceGit(ctx, hostBareRepo, "fetch", "origin", ref.Workspace.Ref+":"+ref.Workspace.Ref); err != nil {
			return fmt.Errorf("fetch workspace ref into local remote: %w", err)
		}
	}

	return nil
}

func runWorkspaceGitCommand(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}
