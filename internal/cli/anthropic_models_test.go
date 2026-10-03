package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/taxiway-sh/taxiway/internal/config"
)

func TestAnthropicCatalog_SelectedModelRouting(t *testing.T) {
	state := providerModelCatalogState(t)
	data, err := os.ReadFile(liteLLMModelsAssetPath(state))
	require.NoError(t, err)
	catalog, err := parseLiteLLMModelCatalog(data)
	require.NoError(t, err)
	for _, entry := range catalog.Models {
		if entry.Provider != "anthropic" || entry.StatusAt(modelNow()) == "retired" {
			continue
		}
		model := entry.Name
		t.Run(model, func(t *testing.T) {
			data, err := renderLiteLLMConfig(state, false, false, []string{model})
			require.NoError(t, err)
			var generated liteLLMGeneratedConfig
			require.NoError(t, yaml.Unmarshal(data, &generated))
			require.Len(t, generated.ModelList, 1)
			require.Equal(t, model, generated.ModelList[0].ModelName)
			require.Equal(t, "anthropic/"+model, generated.ModelList[0].LiteLLMParams.Model)
			require.NotNil(t, generated.LiteLLMSettings.ModelGroupSettings)
			require.Equal(t, []string{model}, generated.LiteLLMSettings.ModelGroupSettings.ForwardClientHeadersToLLMAPI)
		})
	}
}

func TestClaudeCodeGatewayModelSelection(t *testing.T) {
	state := providerModelCatalogState(t)
	for _, tc := range []struct {
		name     string
		selected string
		want     string
	}{
		{"default", "", "claude-opus-5-5"},
		{"existing explicit selection", "claude-opus-4-8", "claude-opus-4-8"},
		{"alternative", "claude-sonnet-5", "claude-sonnet-5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models, err := labLiteLLMModelNames(state, config.LabRef{
				Orch: "claude-code", Settings: map[string]string{"model": tc.selected},
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, models[0])
			require.Contains(t, models, "claude-haiku-4-5-20251001")
			data, err := renderLiteLLMConfig(state, false, false, models)
			require.NoError(t, err)
			var generated liteLLMGeneratedConfig
			require.NoError(t, yaml.Unmarshal(data, &generated))
			require.Len(t, generated.ModelList, len(models))
			found := false
			for _, model := range generated.ModelList {
				if model.ModelName == tc.want {
					require.Equal(t, "anthropic/"+tc.want, model.LiteLLMParams.Model)
					found = true
				}
			}
			require.True(t, found)
		})
	}
}

func TestAnthropicCatalog_RejectsRetiredAndUnknownModels(t *testing.T) {
	state := providerModelCatalogState(t)
	for _, model := range []string{
		"claude-opus-4-1-20250805",
		"claude-opus-4-20250514",
		"claude-sonnet-4-20250514",
		"claude-3-7-sonnet-20250219",
		"claude-3-5-haiku-20241022",
		"claude-3-haiku-20240307",
		"claude-not-a-model",
	} {
		t.Run(model, func(t *testing.T) {
			_, err := renderLiteLLMConfig(state, false, false, []string{model})
			require.ErrorContains(t, err, "unknown LiteLLM model")
		})
	}
}

func TestAnthropicCatalog_DiscoveryRespectsAgentProvider(t *testing.T) {
	state := providerModelCatalogState(t)
	for _, orch := range []string{"claude-code", "gastown", "codex"} {
		t.Run(orch, func(t *testing.T) {
			manifest, err := config.LoadOrchManifest(state.RepoDir, orch)
			require.NoError(t, err)
			records, err := describeLiteLLMModels(state, manifest)
			models := []string{}
			for _, record := range records {
				models = append(models, record.Name)
			}
			require.NoError(t, err)
			if orch == "codex" {
				require.Contains(t, models, "gpt-6.1-sol")
				require.NotContains(t, models, "claude-opus-5")
				require.NotContains(t, models, "claude-sonnet-5")
			} else {
				require.Contains(t, models, "claude-opus-5")
				require.Contains(t, models, "claude-sonnet-5")
				require.Contains(t, models, "claude-haiku-4-5-20251001")
				require.NotContains(t, models, "gpt-5.5")
			}
		})
	}
}
