package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func authStateDir(state *RootState) string {
	if v := strings.TrimSpace(os.Getenv("TAXIWAY_AUTH_DIR")); v != "" {
		return v
	}
	context := strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT"))
	switch context {
	case "dev", "e2e":
		runtimeRoot := strings.TrimSpace(os.Getenv("TAXIWAY_RUNTIME_DIR"))
		if runtimeRoot == "" {
			runtimeRoot = state.RepoDir
		}
		return filepath.Join(runtimeRoot, ".auth")
	default:
		return config.AuthDir()
	}
}

func liteLLMChatGPTTokenStateDir(stateDir string) string {
	return filepath.Join(stateDir, "providers", "codex", "chatgpt_token")
}

func liteLLMChatGPTAuthStatePath(stateDir string) string {
	return filepath.Join(liteLLMChatGPTTokenStateDir(stateDir), "auth.json")
}

func missingCodexAuthError(sourcePath string) error {
	return fmt.Errorf("Codex auth file not found at %s\n  run `codex login` on the host, then retry `taxiway credentials codex`", sourcePath)
}
