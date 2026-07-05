package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// LabRepoRoot is the mount point of the repository inside the lab.
// All script paths passed to Exec use this prefix.
const LabRepoRoot = "/lab"

// LabWorkRoot is the default mutable lab workspace inside the lab/container.
const LabWorkRoot = "/lab/work"

// LabGitRoot is the mutable lab-local bare Git remotes directory.
const LabGitRoot = "/lab/git"

// LabRecordingsRoot is the mutable recordings mount inside the lab/container.
const LabRecordingsRoot = "/lab/recordings"

// hostScriptTolab translates an absolute host path under repoDir into its
// equivalent lab path under /lab.
//
// Example:
//
//	hostScriptToLab("/Users/x/repo", "/Users/x/repo/infra/commands/bootstrap.sh")
//	  → "/lab/infra/commands/bootstrap.sh", nil
//
// Returns an error if hostPath is not under repoDir.
func hostScriptToLab(repoDir, hostPath string) (string, error) {
	// filepath.Rel handles platform path separators and cleans both sides.
	rel, err := filepath.Rel(repoDir, hostPath)
	if err != nil {
		return "", fmt.Errorf("hostScriptTolab: cannot resolve %q relative to %q: %w", hostPath, repoDir, err)
	}
	// A relative path starting with ".." means hostPath is outside repoDir.
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("script path %q is not inside repo %q", hostPath, repoDir)
	}
	// path.Join uses forward slashes (correct for lab paths regardless of host OS).
	return path.Join(LabRepoRoot, filepath.ToSlash(rel)), nil
}
