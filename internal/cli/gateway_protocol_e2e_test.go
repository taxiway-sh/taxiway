//go:build e2e

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// Exercise the shipped proxy and generated configuration without provider credentials.
func TestE2E_GatewayAnthropicProtocol(t *testing.T) {
	runGatewayProtocol(t, "claude-code", []string{"subscription", "api-key"})
}

func TestE2E_GatewayChatGPTProtocol(t *testing.T) {
	runGatewayProtocol(t, "codex", []string{"chatgpt"})
}

func runGatewayProtocol(t *testing.T, orch string, modes []string) {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	state := &RootState{RepoDir: repo}
	dir := t.TempDir()
	ref := config.LabRef{
		Lab: "protocol-test", Orch: orch, Driver: "docker",
	}
	require.NoError(t, writeLabGatewayEnv(dir, ref, map[string]string{
		labLiteLLMAPIKeyEnv: "sk-test-only-protocol",
	}))
	files, err := prepareLabLiteLLMSidecarFiles(state, dir, dir, ref)
	require.NoError(t, err)
	compose, err := os.ReadFile(files.ComposePath)
	require.NoError(t, err)
	var services composeFileForTest
	require.NoError(t, yaml.Unmarshal(compose, &services))
	models, err := labLiteLLMModelNames(state, ref)
	require.NoError(t, err)
	data, err := renderLiteLLMConfig(state, true, orch == "codex", models)
	require.NoError(t, err)
	configPath := filepath.Join(dir, "protocol.yaml")
	require.NoError(t, os.WriteFile(configPath, data, 0600))
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			name := "taxiway-protocol-" + filepath.Base(dir) + "-" + mode
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cleanupCancel()
				_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", name).Run()
			})
			cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--name", name,
				"--network", "none", "-e", "LITELLM_LOCAL_MODEL_COST_MAP=true",
				"-e", "LITELLM_MASTER_KEY=sk-test-only-protocol",
				"-e", "TEST_AUTH_MODE="+mode,
				"-v", configPath+":/test/config.yaml:ro",
				"-v", filepath.Join(repo, "internal", "cli", "testdata", "gateway_protocol.py")+":/test/protocol.py:ro",
				"-v", filepath.Join(repo, "infra", "gateway", "litellm", "callbacks", "anthropic_protocol.py")+":/app/anthropic_protocol.py:ro",
				"-v", liteLLMCodexSessionMapperAssetPath(state)+":/app/codex_session_mapper.py:ro",
				"--entrypoint", "python", services.Services[files.Service].Image, "/test/protocol.py")
			output, err := cmd.CombinedOutput()
			t.Log(string(output))
			require.NoError(t, err)
		})
	}
}
