package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	state := providerModelCatalogState(t)
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
	state := providerModelCatalogState(t)
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
	repo := providerModelCatalogState(t).RepoDir
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

func TestExpiredAnthropicAliasDoesNotBlockCodex(t *testing.T) {
	previous := modelNow
	boundary := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	modelNow = func() time.Time { return boundary.Add(-time.Nanosecond) }
	t.Cleanup(func() { modelNow = previous })
	state := modelCatalogState(t, `defaults:
  anthropic:
    sonnet: old-sonnet
models:
  - {name: old-sonnet, provider: anthropic, upstream: old-sonnet, status: deprecated, retirement_date: '2030-01-01', replacement: new-sonnet}
  - {name: opus, provider: anthropic, upstream: opus}
  - {name: codex, provider: chatgpt, upstream: codex}
`)
	for name, data := range map[string]string{
		"orchestrators/codex/manifest.yaml":       "name: codex\nagents: [codex]\nsettings: [{name: model, default: codex}]\n",
		"agents/codex/manifest.yaml":              "name: codex\nlitellm:\n  providers: [chatgpt]\n",
		"orchestrators/claude-code/manifest.yaml": "name: claude-code\nagents: [claude-code]\nsettings: [{name: model, default: opus}]\n",
		"agents/claude-code/manifest.yaml":        "name: claude-code\nlitellm:\n  providers: [anthropic]\n",
	} {
		path := filepath.Join(state.RepoDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(data), 0644))
	}
	_, err := buildBaseEnv(state.RepoDir, config.LabRef{Orch: "claude-code"})
	require.NoError(t, err)
	modelNow = func() time.Time { return boundary }
	models, err := labLiteLLMModelNames(state, config.LabRef{Orch: "codex"})
	require.NoError(t, err)
	_, err = renderLiteLLMConfig(state, true, false, models)
	require.NoError(t, err)
	_, err = buildBaseEnv(state.RepoDir, config.LabRef{Orch: "codex"})
	require.NoError(t, err)
	_, err = buildBaseEnv(state.RepoDir, config.LabRef{Orch: "claude-code"})
	require.ErrorContains(t, err, "new-sonnet")
}
func TestAnthropicAPIKeyRouteIncludesProtocolShim(t *testing.T) {
	state := modelCatalogState(t, "models: [{name: api-model, provider: anthropic, upstream: api-model, api_key: os.environ/ANTHROPIC_API_KEY}]")
	data, err := renderLiteLLMConfig(state, false, false, nil)
	require.NoError(t, err)
	require.Contains(t, string(data), "anthropic_protocol.proxy_handler_instance")
}

// Logic fixtures intentionally do not read the evolving shipped catalog.
func providerModelCatalogState(t *testing.T) *RootState {
	t.Helper()
	previous := modelNow
	modelNow = func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { modelNow = previous })
	state := modelCatalogState(t, `defaults:
  anthropic:
    opus: claude-opus-5-5
    sonnet: claude-sonnet-5-5
    haiku: claude-haiku-4-5-20251001
models:
  - {name: gpt-6.1-sol, provider: chatgpt, upstream: gpt-6.1-sol, api: responses}
  - {name: gpt-6-astra, provider: chatgpt, upstream: gpt-6-astra, api: responses}
  - {name: gpt-6-luna, provider: chatgpt, upstream: gpt-6-luna, api: responses}
  - {name: gpt-5.6-sol, provider: chatgpt, upstream: gpt-5.6-sol, api: responses}
  - {name: gpt-5.5, provider: chatgpt, upstream: gpt-5.5, status: deprecated, retirement_date: '2026-10-14'}
  - {name: gpt-5.4, provider: chatgpt, upstream: gpt-5.4, status: retired}
  - {name: claude-opus-5, provider: anthropic, forward_client_headers: true, upstream: claude-opus-5}
  - {name: claude-sonnet-5, provider: anthropic, forward_client_headers: true, upstream: claude-sonnet-5}
  - {name: claude-opus-5-5, provider: anthropic, forward_client_headers: true, upstream: claude-opus-5-5}
  - {name: claude-opus-4-8, provider: anthropic, forward_client_headers: true, upstream: claude-opus-4-8}
  - {name: claude-sonnet-5-5, provider: anthropic, forward_client_headers: true, upstream: claude-sonnet-5-5}
  - {name: claude-sonnet-4-6, provider: anthropic, forward_client_headers: true, upstream: claude-sonnet-4-6}
  - {name: claude-haiku-4-5-20251001, provider: anthropic, forward_client_headers: true, upstream: claude-haiku-4-5-20251001}
  - {name: claude-sonnet-4-5-20250929, provider: anthropic, forward_client_headers: true, upstream: claude-sonnet-4-5-20250929, status: deprecated}
`)
	for name, data := range map[string]string{
		"orchestrators/codex/manifest.yaml":       "name: codex\nagents: [codex]\nsettings: [{name: model, default: gpt-6.1-sol}]\n",
		"agents/codex/manifest.yaml":              "name: codex\nlitellm:\n  providers: [chatgpt]\n",
		"orchestrators/claude-code/manifest.yaml": "name: claude-code\nagents: [claude-code]\nsettings: [{name: model, default: claude-opus-5-5}]\n",
		"orchestrators/gastown/manifest.yaml":     "name: gastown\nagents: [claude-code]\nsettings: [{name: model, default: claude-opus-5-5}]\n",
		"agents/claude-code/manifest.yaml":        "name: claude-code\nlitellm:\n  providers: [anthropic]\n",
	} {
		path := filepath.Join(state.RepoDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(data), 0644))
	}
	return state
}
func TestShippedManifestModelsAreSelectableToday(t *testing.T) {
	repo := filepath.Join("..", "..")
	for _, orch := range []string{"codex", "claude-code", "gastown"} {
		_, err := buildBaseEnv(repo, config.LabRef{Orch: orch})
		require.NoError(t, err, "shipped %s defaults need explicit migration", orch)
	}
}
