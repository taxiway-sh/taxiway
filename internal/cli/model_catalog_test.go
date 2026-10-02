package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func modelCatalogState(t *testing.T, catalog string) *RootState {
	t.Helper()
	state := &RootState{RepoDir: t.TempDir()}
	path := liteLLMModelsAssetPath(state)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(catalog), 0644))
	return state
}

func TestModelCatalogRetiredSelection(t *testing.T) {
	for _, status := range []string{"status: retired", "status: deprecated\n    retirement_date: '2000-01-01'"} {
		t.Run(status, func(t *testing.T) {
			state := modelCatalogState(t, `models:
  - name: old-model
    provider: chatgpt
    upstream: old-model
    `+status+`
    replacement: new-model
  - name: new-model
    provider: chatgpt
    upstream: new-model
`)
			_, err := renderLiteLLMConfig(state, true, false, []string{"old-model"})
			require.ErrorContains(t, err, "retired")
			require.ErrorContains(t, err, "new-model")
		})
	}
}

func TestModelCatalogDefaultExposureExcludesUnavailableModels(t *testing.T) {
	state := modelCatalogState(t, `models:
  - name: active-model
    provider: anthropic
    upstream: active-model
  - name: deprecated-model
    provider: anthropic
    upstream: deprecated-model
    status: deprecated
    retirement_date: '2099-01-01'
  - name: retired-model
    provider: anthropic
    upstream: retired-model
    status: retired
`)
	data, err := renderLiteLLMConfig(state, false, false, nil)
	require.NoError(t, err)
	var generated liteLLMGeneratedConfig
	require.NoError(t, yaml.Unmarshal(data, &generated))
	require.Len(t, generated.ModelList, 1)
	require.Equal(t, "active-model", generated.ModelList[0].ModelName)
	// A still-supported explicit choice must retain its real model identity.
	data, err = renderLiteLLMConfig(state, false, false, []string{"deprecated-model"})
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &generated))
	require.Len(t, generated.ModelList, 1)
	require.Equal(t, "anthropic/deprecated-model", generated.ModelList[0].LiteLLMParams.Model)
}

func TestModelCatalogValidation(t *testing.T) {
	for name, data := range map[string]string{
		"duplicate":        "models: [{name: model, provider: chatgpt, upstream: model}, {name: model, provider: chatgpt, upstream: model}]",
		"status":           "models: [{name: model, provider: chatgpt, upstream: model, status: invented}]",
		"date":             "models: [{name: model, provider: chatgpt, upstream: model, retirement_date: tomorrow}]",
		"missing upstream": "models: [{name: model, provider: chatgpt}]",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseLiteLLMModelCatalog([]byte(data))
			require.Error(t, err)
		})
	}
}

func TestGatewayModelAvailabilityFollowsAgentProvider(t *testing.T) {
	state := &RootState{RepoDir: filepath.Join("..", "..")}
	for _, orch := range []string{"claude-code", "gastown", "codex"} {
		t.Run(orch, func(t *testing.T) {
			models, err := labLiteLLMModelNames(state, config.LabRef{Orch: orch})
			require.NoError(t, err)
			if orch == "codex" {
				require.Contains(t, models, "gpt-6.1-sol")
				require.Contains(t, models, "gpt-6-luna")
				require.NotContains(t, models, "gpt-5.4")
				require.NotContains(t, models, "claude-opus-5-5")
			} else {
				require.Contains(t, models, "claude-opus-5-5")
				require.Contains(t, models, "claude-sonnet-5-5")
				require.Contains(t, models, "claude-haiku-4-5-20251001")
				require.NotContains(t, models, "gpt-6.1-sol")
				require.NotContains(t, models, "claude-sonnet-4-5-20250929")
			}
		})
	}
}

func TestGatewayModelSelectionRejectsUnrelatedProvider(t *testing.T) {
	state := &RootState{RepoDir: filepath.Join("..", "..")}
	_, err := labLiteLLMModelNames(state, config.LabRef{
		Orch: "claude-code", Settings: map[string]string{"model": "gpt-5.5"},
	})
	require.ErrorContains(t, err, "provider")
}

func TestDeprecatedAliasDefaultRemainsRoutable(t *testing.T) {
	state := modelCatalogState(t, `defaults:
  anthropic:
    sonnet: deprecated-sonnet
models:
  - name: opus
    provider: anthropic
    upstream: opus
  - name: deprecated-sonnet
    provider: anthropic
    upstream: deprecated-sonnet
    status: deprecated
    retirement_date: '2099-01-01'
`)
	for name, data := range map[string]string{
		"orchestrators/claude-code/manifest.yaml": "name: claude-code\nagents: [claude-code]\nsettings: [{name: model, default: opus}]\n",
		"agents/claude-code/manifest.yaml":        "name: claude-code\nlitellm:\n  providers: [anthropic]\n",
	} {
		path := filepath.Join(state.RepoDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(data), 0644))
	}
	models, err := labLiteLLMModelNames(state, config.LabRef{Orch: "claude-code"})
	require.NoError(t, err)
	require.Contains(t, models, "deprecated-sonnet")
	data, err := renderLiteLLMConfig(state, false, false, models)
	require.NoError(t, err)
	require.Contains(t, string(data), "anthropic/deprecated-sonnet")
}

func TestClaudeModelAliasesAgreeWithGateway(t *testing.T) {
	repo := filepath.Join("..", "..")
	for _, orch := range []string{"claude-code", "gastown"} {
		t.Run(orch, func(t *testing.T) {
			env, err := buildBaseEnv(repo, config.LabRef{
				Orch: orch, Settings: map[string]string{"model": "claude-opus-4-8"},
			})
			require.NoError(t, err)
			require.Equal(t, "claude-opus-4-8", env["TAXIWAY_SET_MODEL"])
			require.Equal(t, "claude-opus-5-5", env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
			require.Equal(t, "claude-sonnet-5-5", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
			require.Equal(t, "claude-haiku-4-5-20251001", env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
		})
	}
}
