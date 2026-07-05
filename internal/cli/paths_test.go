package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostScriptToLab_HappyPath(t *testing.T) {
	cases := []struct {
		name     string
		repoDir  string
		hostPath string
		want     string
	}{
		{
			name:     "infra script",
			repoDir:  "/home/lab/repo",
			hostPath: "/home/lab/repo/infra/commands/bootstrap.sh",
			want:     "/lab/infra/commands/bootstrap.sh",
		},
		{
			name:     "orchestrator install script",
			repoDir:  "/home/lab/repo",
			hostPath: "/home/lab/repo/orchestrators/gastown/install.sh",
			want:     "/lab/orchestrators/gastown/install.sh",
		},
		{
			name:     "nested deep path",
			repoDir:  "/Users/dev/projects/repo",
			hostPath: "/Users/dev/projects/repo/orchestrators/codex/verify.sh",
			want:     "/lab/orchestrators/codex/verify.sh",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hostScriptToLab(tc.repoDir, tc.hostPath)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestHostScriptToLab_OutsideRepo(t *testing.T) {
	_, err := hostScriptToLab("/home/lab/repo", "/tmp/evil.sh")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not inside repo")
}

func TestHostScriptToLab_DotDotTraversal(t *testing.T) {
	// A path that uses ../ to escape the repo dir must be rejected.
	_, err := hostScriptToLab("/home/lab/repo", "/home/lab/repo/../other/evil.sh")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not inside repo")
}

func TestHostScriptToLab_ExactlyRepoRoot(t *testing.T) {
	// A path equal to repoDir itself has rel = "." which maps to "/lab/."
	// cleaned to "/lab" — should succeed (edge case).
	got, err := hostScriptToLab("/home/lab/repo", "/home/lab/repo")
	require.NoError(t, err)
	require.Equal(t, "/lab", got)
}
