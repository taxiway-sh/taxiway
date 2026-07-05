package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func isGitHubURL(rawURL string) bool {
	return strings.Contains(strings.ToLower(rawURL), "github.com")
}

func remindManualWorkspaceForkCleanup(w io.Writer, ref config.LabRef) {
	if ref.Workspace == nil || ref.Workspace.Fork == "" || !isGitHubURL(ref.Workspace.Fork) {
		return
	}
	forkURL := strings.TrimSuffix(ref.Workspace.Fork, ".git")
	fmt.Fprintf(w, "Manual cleanup required: delete workspace fork %s\n", forkURL)
}
