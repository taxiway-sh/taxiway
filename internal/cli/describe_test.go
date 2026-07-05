package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescribeShowsManifestSettings(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	setManifest(t, state, "gastown", `name: gastown
description: Gas Town HQ + workspace shell
settings:
  - name: version
    description: Gastown version/tag to install from release archive
    default: latest
    examples: ["1.1.0", "latest"]
    phases: [install]
`)

	out, _, err := execUpRoot(t, root, stdout, stderr, "describe", "gastown")
	require.NoError(t, err)
	require.Contains(t, out, "Orchestrator: gastown")
	require.Contains(t, out, "version")
	require.Contains(t, out, "Default: latest")
	require.Contains(t, out, "--set version=1.1.0")
}

func TestDescribeWithoutSettingsShowsNone(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	setManifest(t, state, "gastown", "name: gastown\ndescription: Gas Town\n")

	out, _, err := execUpRoot(t, root, stdout, stderr, "describe", "gastown")
	require.NoError(t, err)
	require.Contains(t, out, "Description: Gas Town")
	require.Contains(t, out, "Settings:")
	require.Contains(t, out, "(none declared)")
}

func TestDescribeShowsLiteLLMModelsForOrchestratorAgents(t *testing.T) {
	root, state, _, stdout, stderr := buildUpTestRoot(t)
	setManifest(t, state, "claude-code", `name: claude-code
description: Claude Code
agents:
  - claude-code
settings:
  - name: model
    description: Full Claude Code model name
    default: claude-opus-4-8
    phases: [start]
`)
	writeAgentManifest(t, state, "claude-code", `name: claude-code
command: claude
litellm:
  providers:
    - anthropic
`)
	require.NoError(t, os.MkdirAll(filepath.Join(state.RepoDir, "infra", "gateway", "litellm"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(state.RepoDir, "infra", "gateway", "litellm", "models.yaml"), []byte(`models:
  - name: gpt-5.5
    provider: chatgpt
    upstream: gpt-5.5
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
  - name: claude-sonnet-4-6
    provider: anthropic
    upstream: claude-sonnet-4-6
`), 0o644))

	out, _, err := execUpRoot(t, root, stdout, stderr, "describe", "claude-code")
	require.NoError(t, err)
	require.Contains(t, out, "Available LiteLLM models:")
	require.Contains(t, out, "  claude-opus-4-8")
	require.Contains(t, out, "  claude-sonnet-4-6")
	require.NotContains(t, out, "gpt-5.5")
	require.Contains(t, out, "Examples:")
	require.Contains(t, out, "  --set model=claude-sonnet-4-6")
	require.NotContains(t, out, "  --set model=claude-opus-4-8")
}
