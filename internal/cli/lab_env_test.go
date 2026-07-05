package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

type composeServiceForTest struct {
	Image   string `yaml:"image"`
	Restart string `yaml:"restart"`
}

type composeFileForTest struct {
	Services map[string]composeServiceForTest `yaml:"services"`
}

// buildLabEnvTestState creates a minimal RootState with a MockDriver and the
// gateway LiteLLM runtime assets.
func buildLabEnvTestState(t *testing.T) (*RootState, *driver.MockDriver) {
	t.Helper()
	tmp := t.TempDir()

	infraDir := filepath.Join(tmp, "infra")
	require.NoError(t, os.MkdirAll(infraDir, 0755))
	gatewayDir := filepath.Join(infraDir, "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(gatewayDir, "litellm"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(gatewayDir, "litellm", "models.yaml"), []byte(`models:
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
    forward_client_headers: true
`), 0644))

	mock := driver.NewMockDriver(t.TempDir())
	// Pre-create the lab so Exec doesn't fail.
	require.NoError(t, mock.Create(context.Background(), "taxiway-gastown", driver.CreateOptions{}))

	state := &RootState{
		RepoDir: tmp,
		Driver:  mock,
	}
	return state, mock
}

func TestPrepareLabLiteLLMSidecarFilesWritesComposeAndRoute(t *testing.T) {
	state, _ := buildLabEnvTestState(t)
	stateDir := t.TempDir()
	observabilityDir := t.TempDir()
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{
		labLiteLLMAPIKeyEnv:            "sk-litellm-lab",
		labLiteLLMBaseURLEnv:           "http://gastown.litellm.localhost:4000",
		labLangfuseProjectPublicKeyEnv: "pk-lf-lab",
		labLangfuseProjectSecretKeyEnv: "sk-lf-lab",
	}))

	files, err := prepareLabLiteLLMSidecarFiles(state, stateDir, observabilityDir, ref)
	require.NoError(t, err)
	assert.Equal(t, "taxiway-gastown-gateway", files.Project)
	assert.Equal(t, "litellm", files.Service)
	assert.Equal(t, "postgres", files.DBService)
	assert.Equal(t, filepath.Join(labGatewayDir(stateDir, ref), "litellm.compose.yml"), files.ComposePath)

	compose, err := os.ReadFile(files.ComposePath)
	require.NoError(t, err)
	composeText := string(compose)
	assert.Contains(t, composeText, "litellm:")
	assert.Contains(t, composeText, "postgres:")
	var composeFile composeFileForTest
	require.NoError(t, yaml.Unmarshal(compose, &composeFile))
	require.Contains(t, composeFile.Services, "postgres")
	require.Contains(t, composeFile.Services, "litellm")
	assert.Equal(t, "litellm/litellm:1.88.1", composeFile.Services["litellm"].Image)
	assert.Equal(t, "unless-stopped", composeFile.Services["postgres"].Restart)
	assert.Empty(t, composeFile.Services["litellm"].Restart)
	assert.Contains(t, composeText, "LITELLM_MASTER_KEY: sk-litellm-lab")
	assert.Contains(t, composeText, "LANGFUSE_PUBLIC_KEY: pk-lf-lab")
	assert.Contains(t, composeText, "LANGFUSE_SECRET_KEY: sk-lf-lab")
	assert.Regexp(t, `LANGFUSE_OTEL_HOST: http://taxiway-[a-f0-9]{12}-proxy:4000/_taxiway/langfuse`, composeText)
	assert.Regexp(t, `DATABASE_URL: postgresql://litellm:litellm@taxiway-[a-f0-9]{12}-postgres:5432/litellm`, composeText)
	assert.Regexp(t, `(?m)^\s+- taxiway-[a-f0-9]{12}-postgres$`, composeText)
	assert.Regexp(t, `(?m)^\s+- taxiway-[a-f0-9]{12}-litellm$`, composeText)
	assert.Contains(t, composeText, "name: taxiway-gastown-gateway_default")
	assert.Contains(t, composeText, "host.docker.internal:host-gateway")
	assert.Contains(t, composeText, liteLLMCodexSessionMapperAssetPath(state))
	assert.Contains(t, composeText, liteLLMChatGPTTokenStateDir(observabilityDir))

	config, err := os.ReadFile(filepath.Join(labGatewayDir(stateDir, ref), "litellm_config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(config), "model_name: claude-opus-4-8")
	assert.NotContains(t, string(config), "codex_session_mapper.proxy_handler_instance")

	route, err := readLabLiteLLMRoute(stateDir, ref)
	require.NoError(t, err)
	assert.Equal(t, "gastown", route.Lab)
	assert.Regexp(t, `^taxiway-[a-f0-9]{12}-litellm$`, route.Service)
	assert.Contains(t, composeText, "- "+route.Service)
}

func TestPrepareLabLiteLLMSidecarFilesDoesNotRequireLangfuseKeys(t *testing.T) {
	state, _ := buildLabEnvTestState(t)
	stateDir := t.TempDir()
	observabilityDir := t.TempDir()
	ref := config.LabRef{Lab: "gateway-only", Orch: "gastown", Driver: "mock"}
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{
		labLiteLLMAPIKeyEnv:  "sk-litellm-lab",
		labLiteLLMBaseURLEnv: "http://gateway-only.litellm.localhost:4000",
	}))

	files, err := prepareLabLiteLLMSidecarFiles(state, stateDir, observabilityDir, ref)
	require.NoError(t, err)

	compose, err := os.ReadFile(files.ComposePath)
	require.NoError(t, err)
	composeText := string(compose)
	assert.NotContains(t, composeText, "LANGFUSE_PUBLIC_KEY:")
	assert.NotContains(t, composeText, "LANGFUSE_SECRET_KEY:")
	assert.NotContains(t, composeText, "LANGFUSE_OTEL_HOST:")
}

func TestPrepareLabLiteLLMSidecarFilesUsesDevContextProjectAndNetwork(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", filepath.Join(t.TempDir(), ".observability"))
	state, _ := buildLabEnvTestState(t)
	stateDir := t.TempDir()
	observabilityDir := t.TempDir()
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{
		labLiteLLMAPIKeyEnv:            "sk-litellm-lab",
		labLiteLLMBaseURLEnv:           "http://gastown.litellm.localhost:4000",
		labLangfuseProjectPublicKeyEnv: "pk-lf-lab",
		labLangfuseProjectSecretKeyEnv: "sk-lf-lab",
	}))

	files, err := prepareLabLiteLLMSidecarFiles(state, stateDir, observabilityDir, ref)
	require.NoError(t, err)

	assert.Equal(t, "taxiway-dev-a1b2c3d4-gastown-gateway", files.Project)
	compose, err := os.ReadFile(files.ComposePath)
	require.NoError(t, err)
	assert.Contains(t, string(compose), "name: taxiway-dev-a1b2c3d4-gastown-gateway_default")
	statuses, err := readLabLiteLLMSidecarStatuses(stateDir)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-gastown-gateway", statuses[0].Project)
}

func TestLabLiteLLMComposeProjectDoesNotDuplicateContextPrefix(t *testing.T) {
	project := labLiteLLMComposeProject("e2e", "deadbeef", "e2e-deadbeef-claude-code-up")

	assert.Equal(t, "taxiway-e2e-deadbeef-claude-code-up-gateway", project)
}

func TestWaitForLabLiteLLMSidecarReadyUsesContainerHealthProbe(t *testing.T) {
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "exec" ] && [ "$2" = "taxiway-e2e-deadbeef-claude-code-up-gateway-litellm-1" ]; then
  case "$*" in
    *"http://127.0.0.1:4000/health/liveliness"*) exit 0 ;;
  esac
fi
exit 1
`)

	err := waitForLabLiteLLMSidecarReady(context.Background(), "taxiway-e2e-deadbeef-claude-code-up-gateway", "litellm")

	require.NoError(t, err)
}

func TestWaitForLabLiteLLMSidecarReadyReturnsConciseTimeoutError(t *testing.T) {
	writeFakeDocker(t, `#!/bin/sh
case "$1" in
  exec)
    exit 1
    ;;
esac
exit 1
`)

	err := waitForLabLiteLLMSidecarReadyWithTimeout(
		context.Background(),
		"taxiway-e2e-deadbeef-claude-code-up-gateway",
		"litellm",
		1*time.Millisecond,
		1*time.Millisecond,
	)

	require.Error(t, err)
	assert.Equal(t, "wait for lab LiteLLM sidecar taxiway-e2e-deadbeef-claude-code-up-gateway-litellm-1: exit status 1", err.Error())
}

func TestPrepareLabLiteLLMSidecarFilesEnablesCodexSessionMapperForCodexLabs(t *testing.T) {
	state, _ := buildLabEnvTestState(t)
	stateDir := t.TempDir()
	observabilityDir := t.TempDir()
	ref := config.LabRef{Lab: "test-codex", Orch: "codex", Driver: "mock"}
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{
		labLiteLLMAPIKeyEnv:            "sk-litellm-lab",
		labLiteLLMBaseURLEnv:           "http://test-codex.litellm.localhost:4000",
		labLangfuseProjectPublicKeyEnv: "pk-lf-lab",
		labLangfuseProjectSecretKeyEnv: "sk-lf-lab",
	}))

	_, err := prepareLabLiteLLMSidecarFiles(state, stateDir, observabilityDir, ref)
	require.NoError(t, err)

	config, err := os.ReadFile(filepath.Join(labGatewayDir(stateDir, ref), "litellm_config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(config), "codex_session_mapper.proxy_handler_instance")
}

func TestReadLabLiteLLMSidecarStatuses(t *testing.T) {
	stateDir := t.TempDir()
	ref := config.LabRef{Lab: "test-codex", Orch: "codex", Driver: "mock"}
	labDir := labGatewayDir(stateDir, ref)
	require.NoError(t, os.MkdirAll(labDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(labDir, "litellm.compose.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(labDir, "route.env"), []byte("LAB=test-codex\nSERVICE=taxiway-test-codex-gateway-litellm-1\n"), 0o600))

	statuses, err := readLabLiteLLMSidecarStatuses(stateDir)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "test-codex", statuses[0].Lab)
	assert.Equal(t, "taxiway-test-codex-gateway", statuses[0].Project)
	assert.Equal(t, filepath.Join(labDir, "litellm.compose.yml"), statuses[0].ComposePath)
}

// TestShellQuote verifies shellQuote escapes correctly.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's tricky", `'it'\''s tricky'`},
		{"double\"quote", `'double"quote'`},
		{"", "''"},
	}
	for _, c := range cases {
		got := shellQuote(c.in)
		if got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderManagedEnvBlockWritesGatewayEnv(t *testing.T) {
	env := map[string]string{
		labLiteLLMAPIKeyEnv:  "sk-litellm-lab",
		labLiteLLMBaseURLEnv: "http://gastown.litellm.localhost:4000",
	}

	got := renderManagedEnvBlock("gateway", "gateway", env)

	assert.Equal(t, `# >>> taxiway gateway scope=gateway
TAXIWAY_LITELLM_API_KEY='sk-litellm-lab'
TAXIWAY_LITELLM_BASE_URL='http://gastown.litellm.localhost:4000'
# <<< taxiway gateway scope=gateway
`, got)
}

func TestUpsertManagedEnvBlockReplacesOnlyGatewayBlock(t *testing.T) {
	existing := `# hand-written setting
CUSTOM_FLAG=true

# >>> taxiway gateway scope=gateway
TAXIWAY_LITELLM_API_KEY='old'
# <<< taxiway gateway scope=gateway
`
	block := `# >>> taxiway gateway scope=gateway
TAXIWAY_LITELLM_API_KEY='new'
# <<< taxiway gateway scope=gateway
`

	got := upsertManagedEnvBlock(existing, "gateway", "gateway", block)

	assert.Contains(t, got, "CUSTOM_FLAG=true")
	assert.Contains(t, got, "TAXIWAY_LITELLM_API_KEY='new'")
	assert.NotContains(t, got, "TAXIWAY_LITELLM_API_KEY='old'")
}
