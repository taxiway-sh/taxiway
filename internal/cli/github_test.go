package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func TestIsGitHubURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "github https", url: "https://github.com/org/repo.git", want: true},
		{name: "github ssh", url: "git@github.com:org/repo.git", want: true},
		{name: "case insensitive", url: "https://GitHub.com/org/repo", want: true},
		{name: "non github", url: "https://gitlab.com/org/repo.git", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isGitHubURL(tt.url))
		})
	}
}

func TestRemindManualWorkspaceForkCleanup(t *testing.T) {
	var out bytes.Buffer
	ref := config.LabRef{
		Lab: "demo",
		Workspace: &config.Workspace{
			Fork: "https://github.com/me/repo-lab-demo.git",
		},
	}

	remindManualWorkspaceForkCleanup(&out, ref)

	require.Contains(t, out.String(), "Manual cleanup required")
	require.Contains(t, out.String(), "https://github.com/me/repo-lab-demo")
	require.NotContains(t, out.String(), ".git")
}

func TestRemindManualWorkspaceForkCleanupSkipsMissingFork(t *testing.T) {
	var out bytes.Buffer

	remindManualWorkspaceForkCleanup(&out, config.LabRef{Lab: "demo"})

	require.Empty(t, out.String())
}

func TestRemindManualWorkspaceForkCleanupSkipsLocalFork(t *testing.T) {
	var out bytes.Buffer

	remindManualWorkspaceForkCleanup(&out, config.LabRef{
		Lab: "demo",
		Workspace: &config.Workspace{
			Fork: "file:///lab/git/repo.git",
		},
	})

	require.Empty(t, out.String())
}
