package cli

import (
	"context"
	"fmt"

	"github.com/taxiway-sh/taxiway/internal/config"
)

const workspaceTrustPathEnv = "TAXIWAY_WORKSPACE_TRUST_PATH"

func runAgentWorkspaceTrustHooks(ctx context.Context, state *RootState, ref config.LabRef, workspacePath string, baseEnv map[string]string) error {
	if workspacePath == "" {
		return nil
	}
	manifest, err := config.LoadOrchManifest(state.RepoDir, ref.Orch)
	if err != nil {
		return err
	}
	for _, agent := range manifestAgents(manifest) {
		script, err := agentScript(state.RepoDir, agent, "trust-workspace.sh")
		if err != nil {
			return err
		}
		if script == "" {
			continue
		}
		env := agentEnv(baseEnv, agent)
		env[workspaceTrustPathEnv] = workspacePath
		if err := execScriptWithRef(ctx, state, ref, script, env); err != nil {
			return fmt.Errorf("agent %q trust workspace %q: %w", agent, workspacePath, err)
		}
	}
	return nil
}
