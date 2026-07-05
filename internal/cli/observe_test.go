package cli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

// buildObserveTestRoot creates a minimal cobra root with the observe command
// wired, pointing at a temp directory as the repo root.
func buildObserveTestRoot(t *testing.T, repoDir string) (*cobra.Command, *RootState, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	state := &RootState{RepoDir: repoDir}

	var stdout, stderr bytes.Buffer
	root := &cobra.Command{
		Use:          "taxiway",
		SilenceUsage: true,
	}
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.AddCommand(newInitCmd(state), newDestroyCmd(state), newObserveCmd(state), newStatusCmd(state), newAccessCmd(state), newRepairCmd(state))

	return root, state, &stdout, &stderr
}

func writeFakeDocker(t *testing.T, script string) string {
	t.Helper()

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	require.NoError(t, os.WriteFile(dockerPath, []byte(script), 0o755))
	t.Setenv("PATH", binDir)
	return dockerPath
}

// ── .env generation ──────────────────────────────────────────────────────────

// TestObserveEnsureEnvFile_CreatesFile verifies that ensureEnvFile writes a
// .env with all required keys when the file does not yet exist.
func TestObserveEnsureEnvFile_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	created, err := ensureEnvFile(envPath)
	require.NoError(t, err)
	assert.True(t, created, "should report file was created")

	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "LANGFUSE_NEXTAUTH_SECRET=")
	assert.Contains(t, content, "LANGFUSE_SALT=")
	assert.Contains(t, content, "LANGFUSE_ENCRYPTION_KEY=")
	assert.NotContains(t, content, "LANGFUSE_ADMIN_API_KEY=")

	// Each secret line should be a non-empty value.
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		require.Len(t, parts, 2, "expected KEY=VALUE format for line: %q", line)
		assert.NotEmpty(t, parts[1], "secret value must not be empty for key %s", parts[0])
	}
}

// TestObserveEnsureEnvFile_Idempotent verifies that calling ensureEnvFile a
// second time does NOT overwrite an existing .env (idempotency).
func TestObserveEnsureEnvFile_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	// First call — creates the file.
	created, err := ensureEnvFile(envPath)
	require.NoError(t, err)
	require.True(t, created)

	original, err := os.ReadFile(envPath)
	require.NoError(t, err)

	// Second call — should be a no-op.
	created, err = ensureEnvFile(envPath)
	require.NoError(t, err)
	assert.False(t, created, "should report file was NOT created on second call")

	after, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(after), ".env must not be modified on second call")
}

// TestObserveEnsureEnvFile_IdempotentPartial verifies that calling ensureEnvFile
// on a .env that already has some keys only adds the missing ones.
func TestObserveEnsureEnvFile_IdempotentPartial(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	// Pre-populate with only the 3 original secrets.
	initial := "LANGFUSE_NEXTAUTH_SECRET=existing-secret\nLANGFUSE_SALT=existing-salt\nLANGFUSE_ENCRYPTION_KEY=existing-enc\n"
	require.NoError(t, os.WriteFile(envPath, []byte(initial), 0o600))

	created, err := ensureEnvFile(envPath)
	require.NoError(t, err)
	assert.True(t, created, "should report new keys were added")

	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	content := string(data)

	// Existing keys must be preserved unchanged.
	assert.Contains(t, content, "LANGFUSE_NEXTAUTH_SECRET=existing-secret")
	assert.Contains(t, content, "LANGFUSE_SALT=existing-salt")
	assert.Contains(t, content, "LANGFUSE_ENCRYPTION_KEY=existing-enc")

	// New INIT keys must have been added.
	assert.Contains(t, content, "LANGFUSE_INIT_ORG_ID=")
	assert.Contains(t, content, "LANGFUSE_INIT_USER_PASSWORD=")
	assert.Contains(t, content, "LANGFUSE_POSTGRES_PASSWORD=")
	assert.Contains(t, content, "LANGFUSE_CLICKHOUSE_PASSWORD=")
	assert.Contains(t, content, "LANGFUSE_MINIO_ROOT_PASSWORD=")
	assert.Contains(t, content, "LANGFUSE_REDIS_AUTH=")
	assert.NotContains(t, content, "LANGFUSE_INIT_PROJECT_")

	// Call again — must be a complete no-op.
	snapshot, _ := os.ReadFile(envPath)
	created, err = ensureEnvFile(envPath)
	require.NoError(t, err)
	assert.False(t, created, "second call must not modify .env")
	after, _ := os.ReadFile(envPath)
	assert.Equal(t, string(snapshot), string(after))
}

// TestObserveEnsureEnvFile_FileMode verifies the .env file is written with
// mode 0600 (owner read/write only).
func TestObserveEnsureEnvFile_FileMode(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	_, err := ensureEnvFile(envPath)
	require.NoError(t, err)

	info, err := os.Stat(envPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), ".env must be mode 0600")
}

// TestObserveEnsureEnvFile_SecretsUnique verifies that two separate .env
// files generated by ensureEnvFile have different secret values.
func TestObserveEnsureEnvFile_SecretsUnique(t *testing.T) {
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()
	env1 := filepath.Join(tmp1, ".env")
	env2 := filepath.Join(tmp2, ".env")

	_, err := ensureEnvFile(env1)
	require.NoError(t, err)
	_, err = ensureEnvFile(env2)
	require.NoError(t, err)

	data1, _ := os.ReadFile(env1)
	data2, _ := os.ReadFile(env2)
	assert.NotEqual(t, string(data1), string(data2), "each generation should produce unique secrets")
}

// TestObserveEnsureEnvFile_GeneratesInitKeys verifies that stack-level
// LANGFUSE_INIT_* keys are generated when the file does not yet exist.
func TestObserveEnsureEnvFile_GeneratesInitKeys(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	created, err := ensureEnvFile(envPath)
	require.NoError(t, err)
	require.True(t, created)

	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "LANGFUSE_INIT_ORG_ID=")
	assert.Contains(t, content, "LANGFUSE_INIT_USER_PASSWORD=")
	assert.NotContains(t, content, "LANGFUSE_INIT_PROJECT_")
}

func TestObserveEnsureEnvFile_DoesNotGenerateDefaultProject(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	_, err := ensureEnvFile(envPath)
	require.NoError(t, err)

	vals, _, err := readEnvFile(envPath)
	require.NoError(t, err)

	assert.NotContains(t, vals, "LANGFUSE_INIT_PROJECT_ID")
	assert.NotContains(t, vals, "LANGFUSE_INIT_PROJECT_PUBLIC_KEY")
	assert.NotContains(t, vals, "LANGFUSE_INIT_PROJECT_SECRET_KEY")
}

func TestObserveEnsureLiteLLMConfig_GeneratesFromModelCatalog(t *testing.T) {
	repoDir := t.TempDir()
	assetDir := filepath.Join(repoDir, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(assetDir, "litellm"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(assetDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: gpt-5.4
    provider: chatgpt
    upstream: gpt-5.4
    api: responses
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
    forward_client_headers: true
`),
		0o644,
	))

	stateDir := t.TempDir()
	written, err := ensureLiteLLMConfig(&RootState{RepoDir: repoDir}, stateDir, true, true, nil)
	require.NoError(t, err)
	assert.True(t, written)

	data, err := os.ReadFile(filepath.Join(stateDir, "litellm_config.yaml"))
	require.NoError(t, err)
	config := string(data)
	assert.Contains(t, config, "model_name: gpt-5.4")
	assert.Contains(t, config, "model: chatgpt/gpt-5.4")
	assert.Contains(t, config, "mode: responses")
	assert.Contains(t, config, "model_name: claude-opus-4-8")
	assert.Contains(t, config, "model: anthropic/claude-opus-4-8")
	assert.Contains(t, config, "forward_client_headers_to_llm_api:")
	assert.Contains(t, config, "master_key: os.environ/LITELLM_MASTER_KEY")
	assert.Contains(t, config, "database_url: os.environ/DATABASE_URL")
}

func TestObserveEnsureLiteLLMConfig_FiltersCodexModelsWhenAuthMissing(t *testing.T) {
	repoDir := t.TempDir()
	assetDir := filepath.Join(repoDir, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(assetDir, "litellm"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(assetDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: gpt-5.4
    provider: chatgpt
    upstream: gpt-5.4
    api: responses
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
`),
		0o644,
	))

	stateDir := t.TempDir()
	configPath := filepath.Join(stateDir, "litellm_config.yaml")
	written, err := ensureLiteLLMConfig(&RootState{RepoDir: repoDir}, stateDir, false, false, nil)
	require.NoError(t, err)
	assert.True(t, written)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	config := string(data)
	assert.NotContains(t, config, "model_name: gpt-5.4")
	assert.Contains(t, config, "model_name: claude-opus-4-8")
}

func TestObserveEnsureLiteLLMConfig_FiltersSelectedModels(t *testing.T) {
	repoDir := t.TempDir()
	assetDir := filepath.Join(repoDir, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(assetDir, "litellm"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(assetDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: gpt-5.5
    provider: chatgpt
    upstream: gpt-5.5
    api: responses
  - name: gpt-5.4
    provider: chatgpt
    upstream: gpt-5.4
    api: responses
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
    forward_client_headers: true
`),
		0o644,
	))

	stateDir := t.TempDir()
	written, err := ensureLiteLLMConfig(&RootState{RepoDir: repoDir}, stateDir, true, true, []string{"gpt-5.4"})
	require.NoError(t, err)
	assert.True(t, written)

	data, err := os.ReadFile(filepath.Join(stateDir, "litellm_config.yaml"))
	require.NoError(t, err)
	config := string(data)
	assert.Contains(t, config, "model_name: gpt-5.4")
	assert.NotContains(t, config, "model_name: gpt-5.5")
	assert.NotContains(t, config, "model_name: claude-opus-4-8")
}

func TestObserveEnsureLiteLLMConfig_RendersSelectedCodexModelWithoutAuth(t *testing.T) {
	repoDir := t.TempDir()
	assetDir := filepath.Join(repoDir, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(assetDir, "litellm"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(assetDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: gpt-5.5
    provider: chatgpt
    upstream: gpt-5.5
    api: responses
`),
		0o644,
	))

	stateDir := t.TempDir()
	written, err := ensureLiteLLMConfig(&RootState{RepoDir: repoDir}, stateDir, false, false, []string{"gpt-5.5"})
	require.NoError(t, err)
	assert.True(t, written)

	data, err := os.ReadFile(filepath.Join(stateDir, "litellm_config.yaml"))
	require.NoError(t, err)
	config := string(data)
	assert.Contains(t, config, "model_name: gpt-5.5")
	assert.Contains(t, config, "model: chatgpt/gpt-5.5")
}

func TestObserveEnsureLiteLLMConfig_RendersModelAPIBase(t *testing.T) {
	repoDir := t.TempDir()
	assetDir := filepath.Join(repoDir, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(assetDir, "litellm"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(assetDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: e2e-smoke
    provider: openai
    upstream: e2e-smoke
    api_base: http://taxiway-e2e-openai-upstream:8080/v1
    api_key: sk-e2e-upstream
`),
		0o644,
	))

	stateDir := t.TempDir()
	written, err := ensureLiteLLMConfig(&RootState{RepoDir: repoDir}, stateDir, true, false, []string{"e2e-smoke"})
	require.NoError(t, err)
	require.True(t, written)

	data, err := os.ReadFile(filepath.Join(stateDir, "litellm_config.yaml"))
	require.NoError(t, err)
	config := string(data)
	assert.Contains(t, config, "model_name: e2e-smoke")
	assert.Contains(t, config, "model: openai/e2e-smoke")
	assert.Contains(t, config, "api_base: http://taxiway-e2e-openai-upstream:8080/v1")
	assert.Contains(t, config, "api_key: sk-e2e-upstream")
}

func TestObservabilityRuntimeDefaultsToSharedDeveloperStack(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", "")
	t.Setenv("TAXIWAY_PROXY_DIR", "")
	state := &RootState{}

	runtime := state.observabilityRuntime()

	assert.Equal(t, "host", runtime.Context)
	assert.Empty(t, runtime.ContextID)
	assert.Equal(t, config.ObservabilityDir(), runtime.StateDir)
	assert.Equal(t, "taxiway-observability", runtime.ComposeProject)
	assert.Equal(t, "taxiway-observability_default", runtime.DockerNetwork())
	assert.Equal(t, "taxiway-observability-postgres-1", runtime.PostgresContainer())
	assert.Equal(t, "taxiway-observability-clickhouse-1", runtime.ClickHouseContainer())
}

func TestProxyRuntimeDefaultsToSharedDeveloperProxy(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")
	t.Setenv("TAXIWAY_PROXY_DIR", "")
	state := &RootState{}

	runtime := state.proxyRuntime()

	assert.Equal(t, "host", runtime.Context)
	assert.Empty(t, runtime.ContextID)
	assert.Equal(t, config.ProxyDir(), runtime.StateDir)
	assert.Equal(t, "taxiway-proxy", runtime.ComposeProject)
	assert.Equal(t, "taxiway-proxy", runtime.Container)
	assert.Equal(t, "taxiway-proxy_default", runtime.DockerNetwork())
	assert.Equal(t, "http://127.0.0.1:4000", runtime.BaseURL())
}

func TestObservabilityRuntimeUsesExplicitDevContext(t *testing.T) {
	observabilityDir := filepath.Join(t.TempDir(), ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	state := &RootState{}

	runtime := state.observabilityRuntime()

	assert.Equal(t, "dev", runtime.Context)
	assert.Equal(t, "a1b2c3d4", runtime.ContextID)
	assert.Equal(t, observabilityDir, runtime.StateDir)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-observability", runtime.ComposeProject)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-observability_default", runtime.DockerNetwork())
}

func TestProxyRuntimeUsesExplicitDevContext(t *testing.T) {
	proxyDir := filepath.Join(t.TempDir(), ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	state := &RootState{}

	runtime := state.proxyRuntime()

	assert.Equal(t, "dev", runtime.Context)
	assert.Equal(t, "a1b2c3d4", runtime.ContextID)
	assert.Equal(t, proxyDir, runtime.StateDir)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-proxy", runtime.ComposeProject)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-proxy", runtime.Container)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-proxy_default", runtime.DockerNetwork())
}

func TestObservabilityRuntimeUsesExplicitE2EContext(t *testing.T) {
	observabilityDir := filepath.Join(t.TempDir(), ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "e2e")
	t.Setenv("TAXIWAY_CONTEXT_ID", "b813f84d")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	state := &RootState{}

	runtime := state.observabilityRuntime()

	assert.Equal(t, "e2e", runtime.Context)
	assert.Equal(t, "b813f84d", runtime.ContextID)
	assert.Equal(t, observabilityDir, runtime.StateDir)
	assert.Equal(t, "taxiway-e2e-b813f84d-observability", runtime.ComposeProject)
	assert.Equal(t, "taxiway-e2e-b813f84d-observability_default", runtime.DockerNetwork())
}

func TestProxyRuntimeUsesExplicitE2EContext(t *testing.T) {
	proxyDir := filepath.Join(t.TempDir(), ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "e2e")
	t.Setenv("TAXIWAY_CONTEXT_ID", "b813f84d")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	state := &RootState{}

	runtime := state.proxyRuntime()

	assert.Equal(t, "e2e", runtime.Context)
	assert.Equal(t, "b813f84d", runtime.ContextID)
	assert.Equal(t, proxyDir, runtime.StateDir)
	assert.Equal(t, "taxiway-e2e-b813f84d-proxy", runtime.ComposeProject)
	assert.Equal(t, "taxiway-e2e-b813f84d-proxy", runtime.Container)
	assert.Equal(t, "taxiway-e2e-b813f84d-proxy_default", runtime.DockerNetwork())
}

func TestObservabilityRuntimeDoesNotPersistDevPortsBeforeLaunch(t *testing.T) {
	observabilityDir := filepath.Join(t.TempDir(), ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	state := &RootState{}

	runtime := state.observabilityRuntime()

	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Equal(t, "dev", runtime.Context)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-observability", runtime.ComposeProject)
	assert.Zero(t, runtime.LangfusePort)
	assert.Zero(t, runtime.WorkerPort)
}

func TestProxyRuntimeDoesNotPersistDevPortBeforeLaunch(t *testing.T) {
	proxyDir := filepath.Join(t.TempDir(), ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	state := &RootState{}

	runtime := state.proxyRuntime()

	require.NoFileExists(t, filepath.Join(proxyDir, "runtime.json"))
	assert.Equal(t, "dev", runtime.Context)
	assert.Equal(t, "taxiway-dev-a1b2c3d4-proxy", runtime.Container)
	assert.Zero(t, runtime.Port)
}

func TestEnsureObservabilityRuntimePersistsDevInitializationWithoutHostPorts(t *testing.T) {
	observabilityDir := filepath.Join(t.TempDir(), ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	state := &RootState{}

	first, err := state.ensureObservabilityRuntime()
	require.NoError(t, err)
	second, err := state.ensureObservabilityRuntime()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Zero(t, first.LangfusePort)
	assert.Zero(t, second.LangfusePort)
	assert.NotContains(t, first.ComposeEnv(proxyRuntime{Port: 45123}), "LANGFUSE_WEB_HOST_PORT=")
	assert.Contains(t, first.ComposeEnv(proxyRuntime{Port: 45123}), "NEXTAUTH_URL=http://langfuse.localhost:45123")
}

func TestEnsureObservabilityRuntimePersistsE2EInitializationWithoutHostPorts(t *testing.T) {
	observabilityDir := filepath.Join(t.TempDir(), ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "e2e")
	t.Setenv("TAXIWAY_CONTEXT_ID", "b813f84d")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	state := &RootState{}

	first, err := state.ensureObservabilityRuntime()
	require.NoError(t, err)
	second, err := state.ensureObservabilityRuntime()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Zero(t, first.LangfusePort)
	assert.Zero(t, second.LangfusePort)
}

func TestEnsureProxyRuntimeAllocatesDevPortWithoutPersistingBeforeLaunch(t *testing.T) {
	proxyDir := filepath.Join(t.TempDir(), ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	state := &RootState{}

	first, err := state.ensureProxyRuntime()
	require.NoError(t, err)

	// A free port is chosen before launch so the container can pin <port>:4000,
	// but nothing is persisted until the proxy actually starts.
	require.NoFileExists(t, filepath.Join(proxyDir, "runtime.json"))
	assert.NotZero(t, first.Port)
	assert.NotEqual(t, 4000, first.Port)
}

func TestObservabilityRuntimeKeepsLegacyRuntimeStateButDoesNotExposePorts(t *testing.T) {
	observabilityDir := filepath.Join(t.TempDir(), ".observability")
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{
  "worker_port": 32000,
  "langfuse_port": 32000
}
`), 0o600))
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	state := &RootState{}

	runtime, err := state.ensureObservabilityRuntime()

	require.NoError(t, err)
	assert.Zero(t, runtime.WorkerPort)
	assert.Zero(t, runtime.LangfusePort)
	assert.NotContains(t, runtime.ComposeEnv(proxyRuntime{Port: 45123}), "HOST_PORT")
}

func TestObservabilityRuntimeRejectsIncompleteDevContext(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", filepath.Join(t.TempDir(), ".observability"))
	state := &RootState{}

	_, err := state.resolveObservabilityRuntime()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TAXIWAY_CONTEXT_ID")
}

func TestObservabilityRuntimeRejectsInvalidDevContextID(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "../bad")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", filepath.Join(t.TempDir(), ".observability"))
	state := &RootState{}

	_, err := state.resolveObservabilityRuntime()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TAXIWAY_CONTEXT_ID")
}

func TestPrintObservabilityContextShowsDevRuntime(t *testing.T) {
	labStateDir := filepath.Join(t.TempDir(), ".lab-state")
	runtime := observabilityRuntime{
		Context:        "dev",
		ContextID:      "a1b2c3d4",
		StateDir:       "/repo/.observability",
		ComposeProject: "taxiway-dev-a1b2c3d4-observability",
	}
	proxy := proxyRuntime{
		Context:   "dev",
		ContextID: "a1b2c3d4",
		StateDir:  "/repo/.proxy",
		Container: "taxiway-dev-a1b2c3d4-proxy",
		Port:      45678,
	}
	var out bytes.Buffer

	printObservabilityContext(&out, "/repo", labStateDir, runtime, proxy)

	text := out.String()
	assert.Contains(t, text, "Context:")
	assert.Contains(t, text, "context: dev")
	assert.Contains(t, text, "context_id: a1b2c3d4")
	assert.Contains(t, text, "runtime_dir: /repo")
	assert.Contains(t, text, "lab_state_dir: "+labStateDir)
	assert.Contains(t, text, "observability_dir: /repo/.observability")
	assert.Contains(t, text, "compose_project: taxiway-dev-a1b2c3d4-observability")
	assert.Contains(t, text, "proxy_dir: /repo/.proxy")
	assert.Contains(t, text, "proxy_container: taxiway-dev-a1b2c3d4-proxy")
	assert.Contains(t, text, "proxy_url: http://localhost:45678")
}

func TestObservabilityRuntimeSupportsIsolatedE2EStack(t *testing.T) {
	state := &RootState{Observability: observabilityRuntime{
		StateDir:       "/tmp/taxiway-e2e-observability",
		ComposeProject: "taxiway-e2e-observability-123",
	}}

	runtime := state.observabilityRuntime()

	assert.Equal(t, "/tmp/taxiway-e2e-observability", runtime.StateDir)
	assert.Equal(t, "taxiway-e2e-observability-123", runtime.ComposeProject)
	assert.Equal(t, "taxiway-e2e-observability-123_default", runtime.DockerNetwork())
	assert.Equal(t, "taxiway-e2e-observability-123-postgres-1", runtime.PostgresContainer())
	assert.Equal(t, "taxiway-e2e-observability-123-clickhouse-1", runtime.ClickHouseContainer())
	assert.Contains(t, runtime.ComposeEnv(proxyRuntime{Port: 34001}), "NEXTAUTH_URL=http://langfuse.localhost:34001")
	assert.NotContains(t, strings.Join(runtime.ComposeEnv(proxyRuntime{Port: 34001}), "\n"), "HOST_PORT")
}

func TestProxyRuntimeSupportsIsolatedE2EStack(t *testing.T) {
	state := &RootState{Proxy: proxyRuntime{
		StateDir:       "/tmp/taxiway-e2e-proxy",
		ComposeProject: "taxiway-e2e-proxy-123",
		Container:      "taxiway-e2e-proxy-123",
		Port:           34001,
	}}

	runtime := state.proxyRuntime()

	assert.Equal(t, "/tmp/taxiway-e2e-proxy", runtime.StateDir)
	assert.Equal(t, "taxiway-e2e-proxy-123", runtime.ComposeProject)
	assert.Equal(t, "taxiway-e2e-proxy-123", runtime.Container)
	assert.Equal(t, "taxiway-e2e-proxy-123_default", runtime.DockerNetwork())
	assert.Equal(t, "http://127.0.0.1:34001", runtime.BaseURL())
}

func TestObserveEnsureLiteLLMConfig_RejectsUnknownSelectedModel(t *testing.T) {
	repoDir := t.TempDir()
	assetDir := filepath.Join(repoDir, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(assetDir, "litellm"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(assetDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
    forward_client_headers: true
  - name: claude-sonnet-4-6
    provider: anthropic
    upstream: claude-sonnet-4-6
    forward_client_headers: true
`),
		0o644,
	))

	stateDir := t.TempDir()
	_, err := ensureLiteLLMConfig(&RootState{RepoDir: repoDir}, stateDir, true, true, []string{"opus"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown LiteLLM model "opus"`)
}

func TestObserveLiteLLMAssets_MapCodexSessionBeforeLangfuse(t *testing.T) {
	repoDir := filepath.Join("..", "..")

	config, err := renderLiteLLMConfig(&RootState{RepoDir: repoDir}, true, true, nil)
	require.NoError(t, err)
	configText := string(config)
	require.Contains(t, configText, `codex_session_mapper.proxy_handler_instance`)
	require.Contains(t, configText, `langfuse_otel`)

	callbacks, err := os.ReadFile(filepath.Join(repoDir, "infra", "gateway", "litellm", "callbacks", "codex_session_mapper.py"))
	require.NoError(t, err)
	callbackText := string(callbacks)
	require.Contains(t, callbackText, `"x-codex-turn-metadata"`)
	require.NotContains(t, callbackText, `"x-client-request-id"`)
	require.Contains(t, callbackText, `async_logging_hook`)
	require.Contains(t, callbackText, `async_log_success_event`)
	require.Contains(t, callbackText, `async_log_stream_event`)
	require.Contains(t, callbackText, `standard_logging_object["trace_id"] = session_id`)
	require.Contains(t, callbackText, `litellm_params["litellm_session_id"] = session_id`)
	require.Contains(t, callbackText, `metadata["session_id"] = session_id`)
	require.Contains(t, callbackText, `request_metadata["session_id"] = session_id`)
}

func TestObserveLiteLLMAssets_OmitsCodexSessionMapperWhenDisabled(t *testing.T) {
	repoDir := filepath.Join("..", "..")

	config, err := renderLiteLLMConfig(&RootState{RepoDir: repoDir}, true, false, nil)
	require.NoError(t, err)
	configText := string(config)

	require.Contains(t, configText, `langfuse_otel`)
	require.NotContains(t, configText, `codex_session_mapper.proxy_handler_instance`)
}

func TestObserveLiteLLMAssets_ExposeAgentNativeModelNames(t *testing.T) {
	repoDir := filepath.Join("..", "..")

	config, err := renderLiteLLMConfig(&RootState{RepoDir: repoDir}, true, true, nil)
	require.NoError(t, err)
	configText := string(config)

	for _, modelName := range []string{
		"model_name: gpt-5.5",
		"model_name: gpt-5.4",
		"model_name: gpt-5.4-mini",
		"model_name: gpt-5.3-codex-spark",
		"model_name: claude-opus-4-8",
		"model_name: claude-sonnet-4-6",
		"model_name: claude-haiku-4-5-20251001",
	} {
		require.Contains(t, configText, modelName)
	}
	require.NotContains(t, configText, "model_name: taxiway-claude-code")
}

func TestProxyRunCmd(t *testing.T) {
	stateDir := filepath.Join("/state", "proxy")
	cmd := proxyRunCmd(&RootState{}, stateDir)

	assert.Equal(t, "docker", cmd.Args[0])
	assert.Equal(t, "run", cmd.Args[1])
	assert.Contains(t, cmd.Args, "--name")
	assert.Contains(t, cmd.Args, "taxiway-proxy")
	assert.Contains(t, cmd.Args, "--network")
	assert.Contains(t, cmd.Args, "taxiway-proxy_default")
	assert.Contains(t, cmd.Args, "4000:4000")
	assert.Contains(t, cmd.Args, filepath.Join(stateDir, "Caddyfile")+":/etc/caddy/Caddyfile:ro")
	assert.Equal(t, "caddy:2.8-alpine", cmd.Args[len(cmd.Args)-1])
	require.Contains(t, cmd.Args, "--network-alias")
	for i, arg := range cmd.Args {
		if arg == "--network-alias" {
			require.Less(t, i+1, len(cmd.Args))
			assert.Regexp(t, `^taxiway-[a-f0-9]{12}-proxy$`, cmd.Args[i+1])
			return
		}
	}
}

func TestProxyRunCmdPinsAllocatedPortForDevRuntime(t *testing.T) {
	stateDir := filepath.Join("/state", "proxy")
	cmd := proxyRunCmd(&RootState{Proxy: proxyRuntime{
		Context:        "dev",
		ContextID:      "a1b2c3d4",
		Port:           45123,
		StateDir:       stateDir,
		ComposeProject: "taxiway-dev-a1b2c3d4-proxy",
		Container:      "taxiway-dev-a1b2c3d4-proxy",
	}}, stateDir)

	publishIndex := -1
	for i, arg := range cmd.Args {
		if arg == "-p" {
			publishIndex = i
			break
		}
	}
	require.NotEqual(t, -1, publishIndex)
	require.Less(t, publishIndex+1, len(cmd.Args))
	assert.Equal(t, "45123:4000", cmd.Args[publishIndex+1])
}

func TestParseDockerPublishedPort(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		port int
	}{
		{name: "localhost", out: "127.0.0.1:58336\n", port: 58336},
		{name: "any ipv4", out: "0.0.0.0:58337\n", port: 58337},
		{name: "ipv6", out: "[::]:58338\n", port: 58338},
		{name: "dual stack", out: "0.0.0.0:58339\n[::]:58339\n", port: 58339},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port, err := parseDockerPublishedPort(tc.out)

			require.NoError(t, err)
			assert.Equal(t, tc.port, port)
		})
	}
}

func TestParseDockerPublishedPortRejectsConflictingBindings(t *testing.T) {
	_, err := parseDockerPublishedPort("0.0.0.0:58339\n[::]:58340\n")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting published ports")
}

func TestEnsureProxyConfigRoutesLabHostsWithCaddy(t *testing.T) {
	labStateDir := t.TempDir()
	proxyDir := t.TempDir()
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "mock"}
	require.NoError(t, writeLabLiteLLMRoute(labStateDir, ref, labLiteLLMRoute{
		Lab:     "gastown",
		Service: "taxiway-gastown-gateway-litellm-1",
	}))

	written, err := ensureProxyConfig(labStateDir, proxyDir)
	require.NoError(t, err)
	require.True(t, written)

	data, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)
	configText := string(data)
	require.Contains(t, configText, ":4000")
	require.Contains(t, configText, `@gastown host gastown.litellm.localhost`)
	require.Contains(t, configText, "reverse_proxy taxiway-gastown-gateway-litellm-1:4000")
	registryData, err := os.ReadFile(filepath.Join(proxyDir, "routes.json"))
	require.NoError(t, err)
	assert.Contains(t, string(registryData), `"id": "lab:gastown"`)
	assert.Contains(t, string(registryData), `"kind": "lab"`)
	assert.Contains(t, string(registryData), `"upstream": "taxiway-gastown-gateway-litellm-1:4000"`)

	written, err = ensureProxyConfig(labStateDir, proxyDir)
	require.NoError(t, err)
	require.False(t, written)
}

func TestProxyDefaultPageIncludesMinimalProxyShellAndRoutes(t *testing.T) {
	routes := []proxyRoute{
		observabilityProxyRoute(observabilityRuntime{ComposeProject: "taxiway-dev-a1b2c3d4-observability"}),
		proxyRouteFromLabLiteLLMRoute(labLiteLLMRoute{
			Lab:     "gastown",
			Service: "taxiway-dev-a1b2c3d4-gastown-gateway-litellm-1",
			Host:    "gastown.litellm.localhost",
			Project: "taxiway-dev-a1b2c3d4-gastown-gateway",
		}),
	}

	configText := string(renderProxyConfig(routes))

	require.Contains(t, configText, "Content-Type text/html")
	require.Contains(t, configText, `Cache-Control "no-store"`)
	require.Contains(t, configText, "respond <<TAXIWAY_PROXY_INDEX")
	require.Contains(t, configText, "<h1>Taxiway proxy</h1>")
	require.Contains(t, configText, `<span class="status">running</span>`)
	require.Contains(t, configText, "This endpoint only serves Taxiway runtime routes.")
	require.Contains(t, configText, `@observability_langfuse host langfuse.localhost`)
	require.Contains(t, configText, `@gastown host gastown.litellm.localhost`)
}

func TestReloadProxyIncludesCaddyOutputOnFailure(t *testing.T) {
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "exec" ]; then
  echo "caddy reload failed because config is invalid" >&2
  exit 1
fi
exit 0
`)
	state := &RootState{Proxy: proxyRuntime{Container: "taxiway-dev-a1b2c3d4-proxy"}}

	err := reloadProxy(state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reload proxy")
	assert.Contains(t, err.Error(), "caddy reload failed because config is invalid")
}

func TestRegisterObservabilityProxyRouteWritesLangfuseHostRoute(t *testing.T) {
	proxyDir := t.TempDir()
	runtime := observabilityRuntime{ComposeProject: "taxiway-dev-a1b2c3d4-observability"}

	written, err := upsertProxyRoute(proxyDir, observabilityProxyRoute(runtime))
	require.NoError(t, err)
	require.True(t, written)
	written, err = upsertProxyRoute(proxyDir, observabilityInternalProxyRoute(runtime))

	require.NoError(t, err)
	require.True(t, written)
	data, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)
	configText := string(data)
	assert.Contains(t, configText, `@observability_langfuse host langfuse.localhost`)
	assert.Contains(t, configText, `@observability_langfuse_internal path /_taxiway/langfuse /_taxiway/langfuse/*`)
	assert.Regexp(t, `reverse_proxy taxiway-[a-f0-9]{12}-langfuse-web:3000`, configText)
	registryData, err := os.ReadFile(proxyRoutesStatePath(proxyDir))
	require.NoError(t, err)
	assert.Contains(t, string(registryData), `"id": "observability:langfuse"`)
	assert.Contains(t, string(registryData), `"host": "langfuse.localhost"`)
	assert.Regexp(t, `"upstream": "taxiway-[a-f0-9]{12}-langfuse-web:3000"`, string(registryData))
	assert.Contains(t, string(registryData), `"id": "observability:langfuse-internal"`)
	assert.Contains(t, string(registryData), `"network": "taxiway-dev-a1b2c3d4-observability_default"`)
}

func TestObservabilityComposeDoesNotPublishHostPorts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "infra", "observability", "langfuse.compose.yml"))
	require.NoError(t, err)
	composeText := string(data)

	assert.NotContains(t, composeText, "\n    ports:")
	assert.NotContains(t, composeText, "HOST_PORT")
	assert.NotContains(t, composeText, "localhost:3000")
	for _, service := range observabilityComposeServices() {
		assert.Contains(t, composeText, "${TAXIWAY_OBSERVABILITY_"+strings.ToUpper(strings.ReplaceAll(service, "-", "_"))+"_DNS:-"+service+"}")
	}
}

func TestRepairDoesNotCreateProxyRuntimeWhenNothingExists(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy repair:")
	assert.Contains(t, out, "  Config: skipped (no routes)")
	assert.Contains(t, out, "  Container: skipped (no routes)")
	assert.Contains(t, out, "  Networks: skipped")
	assert.Contains(t, out, "Observability repair:")
	assert.Contains(t, out, "  Env: skipped (not initialized)")
	assert.Contains(t, out, "  Stack: skipped (not initialized)")
	assert.Contains(t, out, "  Routes: skipped (not initialized)")
	assert.Contains(t, out, "Lab routes repair:")
	assert.Contains(t, out, "  Routes: 0 found")
	assert.Less(t, strings.Index(out, "Proxy repair:"), strings.Index(out, "Observability repair:"))
	require.NoFileExists(t, filepath.Join(proxyDir, "runtime.json"))
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	require.NoDirExists(t, proxyDir)
	assert.Empty(t, stderr.String())
}

func TestRepairRestartsInitializedObservabilityStackWhenContainerIsMissing(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
exit 1
`, logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(observabilityEnvPath(&RootState{Observability: observabilityRuntime{StateDir: observabilityDir}}), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy repair:")
	assert.Contains(t, out, "  Config: ok")
	assert.Contains(t, out, "  Container: restarted")
	assert.Contains(t, out, "  Networks: connected")
	assert.Contains(t, out, "Observability repair:")
	assert.Contains(t, out, "  Env: updated")
	assert.Contains(t, out, "  Stack: restarted")
	assert.Contains(t, out, "  Routes: restored")
	assert.Contains(t, out, "Lab routes repair:")
	assert.Contains(t, out, "  Routes: 0 found")
	require.FileExists(t, observabilityRuntimeStatePath(observabilityDir))
	routeData, err := os.ReadFile(proxyRoutesStatePath(proxyDir))
	require.NoError(t, err)
	assert.Contains(t, string(routeData), `"id": "observability:langfuse"`)
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(logData)
	assert.Contains(t, log, "-p taxiway-dev-a1b2c3d4-observability")
	assert.Contains(t, log, "up -d")
	assert.Empty(t, stderr.String())
}

func TestObserveRepairWritesExistingGatewayProxyConfig(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRoutesStatePath(proxyDir), []byte(`{"routes":[]}`+"\n"), 0o600))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy repair:")
	assert.Contains(t, out, "  Config: updated")
	assert.Contains(t, out, "  Container: skipped (proxy is not initialized)")
	assert.Contains(t, out, "  Networks: skipped")
	assert.Contains(t, string(data), "<h1>Taxiway proxy</h1>")
	assert.Contains(t, string(data), "This endpoint only serves Taxiway runtime routes.")
	assert.Empty(t, stderr.String())
}

func TestRepairRestartsInitializedProxyWhenContainerIsMissing(t *testing.T) {
	tmp := t.TempDir()
	proxyDir := filepath.Join(tmp, ".proxy")
	observabilityDir := filepath.Join(tmp, ".observability")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
exit 0
`, logPath))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))
	require.NoError(t, os.WriteFile(proxyRoutesStatePath(proxyDir), []byte(`{"routes":[]}`+"\n"), 0o600))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy repair:")
	assert.Contains(t, out, "  Container: restarted")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "run -d")
	assert.Empty(t, stderr.String())
}

func TestRepairStartsProxyFromRoutesWithoutPersistedPort(t *testing.T) {
	tmp := t.TempDir()
	proxyDir := filepath.Join(tmp, ".proxy")
	observabilityDir := filepath.Join(tmp, ".observability")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "docker"}
	require.NoError(t, writeLabLiteLLMRoute(labStateDir, ref, labLiteLLMRoute{
		Lab:     "gastown",
		Service: "taxiway-dev-a1b2c3d4-gastown-gateway-litellm-1",
		Host:    "gastown.litellm.localhost",
		Project: "taxiway-dev-a1b2c3d4-gastown-gateway",
	}))
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
exit 0
`, logPath))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy repair:")
	assert.Contains(t, out, "  Container: restarted")
	require.FileExists(t, proxyRuntimeStatePath(proxyDir))
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "run -d")
	assert.Empty(t, stderr.String())
}

func TestInitStartsRuntimeWithoutPrintingProxyURL(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(serverURL.Host)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(fmt.Sprintf(`{"port":%s}`+"\n", port)), 0o600))
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:%s'; exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "create" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
if [ "$1" = "exec" ]; then exit 0; fi
exit 0
`, logPath, port))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"init"})

	err = root.Execute()

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Taxiway runtime initialized.")
	assert.Contains(t, stdout.String(), "✓ Proxy started")
	assert.Contains(t, stdout.String(), "Create your first lab with `taxiway up mylab --type codex`")
	assert.Empty(t, stderr.String())
	caddy, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)
	assert.Contains(t, string(caddy), "<h1>Taxiway proxy</h1>")
	assert.Contains(t, string(caddy), `Cache-Control "no-store"`)
	assert.Contains(t, string(caddy), "This endpoint only serves Taxiway runtime routes.")
	assert.Contains(t, string(caddy), "@observability_langfuse")
	dockerLog, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(dockerLog)
	assert.Contains(t, log, "run -d")
	proxyRunIndex := strings.Index(log, "run -d")
	observabilityUpIndex := strings.Index(log, "compose --project-directory")
	require.NotEqual(t, -1, proxyRunIndex)
	require.NotEqual(t, -1, observabilityUpIndex)
	assert.Less(t, proxyRunIndex, observabilityUpIndex)
	require.FileExists(t, proxyRuntimeStatePath(proxyDir))
	require.FileExists(t, observabilityRuntimeStatePath(observabilityDir))
}

func TestDestroyRemovesRuntimeStateAndProxy(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "rm" ]; then exit 0; fi
exit 0
`, logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(observabilityRuntimeStatePath(observabilityDir), []byte(`{"initialized":true}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))
	require.NoError(t, os.WriteFile(proxyRoutesStatePath(proxyDir), []byte(`{"routes":[]}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(labStateDir, 0o700))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"destroy", "--yes"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Destroying Taxiway runtime")
	assert.Contains(t, out, "Observability: removed")
	assert.Contains(t, out, "Proxy: removed")
	assert.Contains(t, out, "Labs: removed")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "compose")
	assert.Contains(t, log, "down -v")
	assert.Contains(t, log, "rm -f taxiway-dev-a1b2c3d4-proxy")
	proxyRemoveIndex := strings.Index(log, "rm -f taxiway-dev-a1b2c3d4-proxy")
	observabilityDownIndex := strings.Index(log, "-p taxiway-dev-a1b2c3d4-observability")
	require.NotEqual(t, -1, proxyRemoveIndex)
	require.NotEqual(t, -1, observabilityDownIndex)
	assert.Less(t, proxyRemoveIndex, observabilityDownIndex, "proxy must be removed before observability so Docker can remove the observability network")
	require.NoDirExists(t, proxyDir)
	require.NoDirExists(t, observabilityDir)
	require.NoDirExists(t, labStateDir)
	assert.Empty(t, stderr.String())
}

func TestDestroyRemovesLabGatewaySidecarsWithoutPreinitializedDriver(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "network" ]; then exit 0; fi
if [ "$1" = "rm" ]; then exit 0; fi
exit 0
`, logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(observabilityRuntimeStatePath(observabilityDir), []byte(`{"initialized":true}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))
	ref := config.LabRef{Lab: "test-observe", Orch: "codex", Driver: "docker"}
	require.NoError(t, config.WriteLabRef(labStateDir, "test-observe", ref))
	require.NoError(t, os.WriteFile(config.CreatedAtPath(labStateDir, "test-observe"), []byte("2026-06-24T20:00:00Z\n"), 0o644))
	gatewayDir := labGatewayDir(labStateDir, ref)
	require.NoError(t, os.MkdirAll(gatewayDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(gatewayDir, "litellm.compose.yml"), []byte("services: {}\n"), 0o600))

	root, state, _, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	require.Nil(t, state.Driver)
	root.SetArgs([]string{"destroy", "--yes"})

	err := root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "-p taxiway-dev-a1b2c3d4-test-observe-gateway")
	assert.Contains(t, log, "litellm.compose.yml")
	assert.Contains(t, log, "down -v")
	assert.Empty(t, stderr.String())
}

func TestObserveRepairRestoresConfiguredLabRoute(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	ref := config.LabRef{Lab: "test-codex", Orch: "codex", Driver: "docker"}
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ] && [ "$2" = "--format" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "run" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyConfigStatePath(proxyDir), []byte("broken\n"), 0o644))
	require.NoError(t, writeLabLiteLLMRoute(labStateDir, ref, labLiteLLMRoute{
		Lab:     "test-codex",
		Service: "taxiway-test-codex-gateway-litellm-1",
		Host:    "test-codex.litellm.localhost",
	}))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(proxyConfigStatePath(proxyDir))
	require.NoError(t, err)
	configText := string(data)
	assert.Contains(t, configText, "@test_codex host test-codex.litellm.localhost")
	assert.Contains(t, configText, "reverse_proxy taxiway-test-codex-gateway-litellm-1:4000")
	assert.Contains(t, stdout.String(), "  Config: updated")
	assert.Empty(t, stderr.String())
}

func TestObserveRepairReloadsRunningGatewayProxy(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	reloadMarker := filepath.Join(tmp, "reload-called")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	require.NoError(t, writeLabLiteLLMRoute(labStateDir, config.LabRef{Lab: "test-codex", Orch: "codex", Driver: "docker"}, labLiteLLMRoute{
		Lab:     "test-codex",
		Service: "taxiway-dev-a1b2c3d4-test-codex-gateway-litellm-1",
		Host:    "test-codex.litellm.localhost",
		Project: "taxiway-dev-a1b2c3d4-test-codex-gateway",
	}))
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "inspect" ]; then
  echo running
  exit 0
fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "exec" ]; then
  echo reloaded > %q
  exit 0
fi
exit 1
`, logPath, reloadMarker))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"repair"})

	err := root.Execute()

	require.NoError(t, err)
	require.FileExists(t, reloadMarker)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "network connect --alias ")
	assert.Contains(t, string(data), " taxiway-dev-a1b2c3d4-test-codex-gateway_default taxiway-dev-a1b2c3d4-proxy")
	assert.Contains(t, stdout.String(), "  Container: reloaded")
	assert.Empty(t, stderr.String())
}

// TestObserveEnsureEnvFile_InitKeysIdempotent verifies that calling
// ensureEnvFile twice produces the same LANGFUSE_INIT_* values.
func TestObserveEnsureEnvFile_InitKeysIdempotent(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	_, err := ensureEnvFile(envPath)
	require.NoError(t, err)

	vals1, _, err := readEnvFile(envPath)
	require.NoError(t, err)

	_, err = ensureEnvFile(envPath)
	require.NoError(t, err)

	vals2, _, err := readEnvFile(envPath)
	require.NoError(t, err)

	initKeys := []string{
		"LANGFUSE_INIT_ORG_ID",
		"LANGFUSE_INIT_USER_PASSWORD",
	}
	for _, k := range initKeys {
		assert.Equal(t, vals1[k], vals2[k],
			"key %s must not change between ensureEnvFile calls", k)
	}
}

func TestEnsureLiteLLMChatGPTAuth_OptionalMissingCodexAuthFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	enabled, err := ensureLiteLLMChatGPTAuth(tmp, false)

	require.NoError(t, err)
	assert.False(t, enabled)
}

func TestEnsureLiteLLMChatGPTAuth_RequiredMissingCodexAuthFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	enabled, err := ensureLiteLLMChatGPTAuth(tmp, true)

	require.Error(t, err)
	assert.False(t, enabled)
	assert.Contains(t, err.Error(), "Codex auth file not found")
	assert.Contains(t, err.Error(), "codex login")
}

func TestEnsureLiteLLMChatGPTAuth_ConvertsCodexAuthFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	sourcePath := filepath.Join(tmp, ".codex", "auth.json")
	stateDir := filepath.Join(tmp, "auth")
	require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o700))
	require.NoError(t, os.WriteFile(sourcePath, []byte(`{
  "tokens": {
    "access_token": "access",
    "refresh_token": "refresh",
    "id_token": "id",
    "account_id": "account"
  }
}`), 0o600))

	enabled, err := ensureLiteLLMChatGPTAuth(stateDir, false)

	require.NoError(t, err)
	assert.True(t, enabled)
	data, err := os.ReadFile(filepath.Join(stateDir, "providers", "codex", "chatgpt_token", "auth.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"access_token":"access"`)
	assert.Contains(t, string(data), `"refresh_token":"refresh"`)
	assert.Contains(t, string(data), `"id_token":"id"`)
	assert.Contains(t, string(data), `"account_id":"account"`)
}

func TestEnsureLiteLLMConfigFile_OverwritesGeneratedLabConfig(t *testing.T) {
	tmp := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	state := &RootState{RepoDir: tmp}

	runtimeConfigDir := filepath.Join(tmp, "infra", "gateway")
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeConfigDir, "litellm"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(runtimeConfigDir, "litellm", "models.yaml"),
		[]byte(`models:
  - name: claude-opus-4-8
    provider: anthropic
    upstream: claude-opus-4-8
`),
		0644,
	))

	ref := config.LabRef{Lab: "test-lab"}
	stateDir := filepath.Join(home, ".taxiway", "lab-state")
	stateConfigPath := filepath.Join(labGatewayDir(stateDir, ref), "litellm_config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(stateConfigPath), 0755))
	require.NoError(t, os.WriteFile(stateConfigPath, []byte("custom: true\n"), 0600))

	created, err := ensureLiteLLMConfig(state, labGatewayDir(stateDir, ref), true, false, []string{"claude-opus-4-8"})
	require.NoError(t, err)
	assert.True(t, created)

	data, err := os.ReadFile(stateConfigPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "model_name: claude-opus-4-8")
	assert.NotContains(t, string(data), "custom: true")
}

// ── docker availability check ────────────────────────────────────────────────

// TestObserveUp_DockerMissing verifies that `taxiway observe up` returns a clear
// error message when docker is not on the PATH.
func TestObserveUp_DockerMissing(t *testing.T) {
	// Override PATH to a directory that definitely has no docker binary.
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)

	// Verify docker really is absent (sanity-check for the test itself).
	_, lookErr := exec.LookPath("docker")
	if lookErr == nil {
		t.Skip("docker is present in the empty PATH; cannot simulate absence")
	}

	tmp := t.TempDir()
	// Create the observability asset directory so the path resolution works.
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0755))

	root, _, _, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "up"})
	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "docker",
		"error must mention 'docker' when the binary is missing")
}

// TestObserveDown_DockerMissing verifies that `taxiway observe down` also reports
// a clear error when docker is not on the PATH.
func TestObserveDown_DockerMissing(t *testing.T) {
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)

	_, lookErr := exec.LookPath("docker")
	if lookErr == nil {
		t.Skip("docker is present in the empty PATH; cannot simulate absence")
	}

	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0755))

	root, _, _, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "down"})
	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "docker")
}

func TestObserveDownStopsStackAndKeepsRuntimePorts(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "compose" ]; then exit 0; fi
exit 0
`, logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{
  "langfuse_port": 55123
}
`), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "down"})

	err := root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "compose")
	assert.Contains(t, log, "stop")
	assert.NotContains(t, log, "down")
	assert.Contains(t, stdout.String(), "Proxy was not running")
	require.FileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

func TestObserveDownKeepsProxyRunningWhenNoTargetsRemainRunning(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{
  "langfuse_port": 55123
}
`), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{
  "port": 55124
}
`), 0o600))
	require.NoError(t, os.MkdirAll(labStateDir, 0o700))
	_, err := upsertProxyRoute(proxyDir, observabilityProxyRoute(observabilityRuntime{
		Context:        "dev",
		ContextID:      "a1b2c3d4",
		StateDir:       observabilityDir,
		ComposeProject: "taxiway-dev-a1b2c3d4-observability",
		LangfusePort:   55123,
	}))
	require.NoError(t, err)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then
  last=""
  for arg do last="$arg"; done
  case "$last" in
    taxiway-dev-a1b2c3d4-proxy) echo running; exit 0 ;;
    *) echo exited; exit 0 ;;
  esac
fi
if [ "$1" = "network" ] && [ "$2" = "disconnect" ]; then exit 0; fi
if [ "$1" = "stop" ]; then exit 0; fi
if [ "$1" = "rm" ]; then exit 1; fi
exit 0
`, logPath))

	root, state, _, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"observe", "down"})

	err = root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "compose")
	assert.NotContains(t, log, "stop taxiway-dev-a1b2c3d4-proxy")
	assert.NotContains(t, log, "rm -f taxiway-dev-a1b2c3d4-proxy")
	require.FileExists(t, filepath.Join(proxyDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

func TestObserveRmVolumesRemovesComposeVolumesAndRuntimePorts(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 1; fi
if [ "$1" = "compose" ]; then exit 0; fi
exit 0
`, logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{
  "langfuse_port": 55123
}
`), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "rm", "--volumes"})

	err := root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "compose")
	assert.Contains(t, log, "down -v")
	assert.Contains(t, stdout.String(), "Proxy was not running")
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

func TestObserveRmKeepsProxyRuntimeWhenNoTargetsRemain(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{
  "langfuse_port": 55123
}
`), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{
  "port": 55124
}
`), 0o600))
	require.NoError(t, os.MkdirAll(labStateDir, 0o700))
	_, err := upsertProxyRoute(proxyDir, observabilityProxyRoute(observabilityRuntime{
		Context:        "dev",
		ContextID:      "a1b2c3d4",
		StateDir:       observabilityDir,
		ComposeProject: "taxiway-dev-a1b2c3d4-observability",
		LangfusePort:   55123,
	}))
	require.NoError(t, err)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then
  last=""
  for arg do last="$arg"; done
  case "$last" in
    taxiway-dev-a1b2c3d4-proxy) echo running; exit 0 ;;
    *) exit 1 ;;
  esac
fi
if [ "$1" = "network" ] && [ "$2" = "disconnect" ]; then exit 0; fi
if [ "$1" = "rm" ]; then exit 0; fi
exit 0
`, logPath))

	root, state, _, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"observe", "rm"})

	err = root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.NotContains(t, log, "rm -f taxiway-dev-a1b2c3d4-proxy")
	require.FileExists(t, filepath.Join(proxyDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

func TestObserveDownKeepsGatewayProxyWhenLabRoutesRemain(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "docker"}
	require.NoError(t, writeLabLiteLLMRoute(labStateDir, ref, labLiteLLMRoute{
		Lab:     "gastown",
		Service: "taxiway-dev-a1b2c3d4-gastown-gateway-litellm-1",
		Host:    "gastown.litellm.localhost",
		Project: "taxiway-dev-a1b2c3d4-gastown-gateway",
	}))
	_, err := ensureProxyConfig(labStateDir, proxyDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{
  "port": 55124
}
`), 0o600))
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:55124'; exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "ps" ]; then
  printf 'taxiway-dev-a1b2c3d4-gastown-gateway-litellm-1\trunning\tUp 1 minute\n'
  exit 0
fi
if [ "$1" = "inspect" ]; then
  last=""
  for arg do last="$arg"; done
  case "$last" in
    taxiway-dev-a1b2c3d4-proxy) echo running; exit 0 ;;
    taxiway-dev-a1b2c3d4-gastown-gateway-litellm-1) echo running; exit 0 ;;
    *) echo exited; exit 0 ;;
  esac
fi
if [ "$1" = "network" ] && [ "$2" = "connect" ]; then exit 0; fi
if [ "$1" = "rm" ]; then exit 1; fi
exit 0
`, logPath))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"observe", "down"})

	err = root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "compose")
	assert.NotContains(t, log, "rm -f taxiway-dev-a1b2c3d4-proxy")
	assert.Contains(t, stdout.String(), "Proxy kept running")
	assert.Empty(t, stderr.String())
}

func TestRemoveLabLiteLLMSidecarKeepsGatewayProxyAfterLastRoute(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".lab-state")
	proxyDir := filepath.Join(tmp, ".proxy")
	observabilityDir := filepath.Join(tmp, ".observability")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	ref := config.LabRef{Lab: "gastown", Orch: "gastown", Driver: "docker"}
	state := &RootState{
		RepoDir: tmp,
		Flags:   GlobalFlags{StateDir: stateDir},
		Driver:  &namedTestDriver{MockDriver: driver.NewMockDriver(stateDir), name: "docker"},
	}
	labDir := labGatewayDir(stateDir, ref)
	require.NoError(t, os.MkdirAll(labDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(labDir, "litellm.compose.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, writeLabLiteLLMRoute(stateDir, ref, labLiteLLMRoute{
		Lab:     "gastown",
		Service: "taxiway-dev-a1b2c3d4-gastown-gateway-litellm-1",
		Host:    "gastown.litellm.localhost",
		Project: "taxiway-dev-a1b2c3d4-gastown-gateway",
	}))
	_, err := ensureProxyConfig(stateDir, proxyDir)
	require.NoError(t, err)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "disconnect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "rm" ]; then exit 0; fi
if [ "$1" = "exec" ]; then exit 0; fi
exit 0
`, logPath))

	err = removeLabLiteLLMSidecar(context.Background(), state, ref)

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "compose")
	assert.Contains(t, log, "network disconnect taxiway-dev-a1b2c3d4-gastown-gateway_default taxiway-dev-a1b2c3d4-proxy")
	assert.NotContains(t, log, "rm -f taxiway-dev-a1b2c3d4-proxy")
	disconnectIndex := strings.Index(log, "network disconnect taxiway-dev-a1b2c3d4-gastown-gateway_default taxiway-dev-a1b2c3d4-proxy")
	composeDownIndex := strings.Index(log, "down -v")
	require.NotEqual(t, -1, disconnectIndex)
	require.NotEqual(t, -1, composeDownIndex)
	assert.Less(t, disconnectIndex, composeDownIndex)
	registry, err := os.ReadFile(proxyRoutesStatePath(proxyDir))
	require.NoError(t, err)
	assert.Contains(t, string(registry), `"routes": []`)
}

func TestObserveHelpOnlyShowsObservabilityActions(t *testing.T) {
	tmp := t.TempDir()
	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "--help"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "up")
	assert.Contains(t, out, "down")
	assert.Contains(t, out, "rm")
	assert.Contains(t, out, "reset")
	assert.Contains(t, out, "open")
	assert.Empty(t, stderr.String())
}

func TestStatus_DockerMissingOrdersGlobalRuntimeBeforeObservability(t *testing.T) {
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", filepath.Join(t.TempDir(), ".observability"))

	_, lookErr := exec.LookPath("docker")
	if lookErr == nil {
		t.Skip("docker is present in the empty PATH; cannot simulate absence")
	}

	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0755))

	root, _, stdout, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})
	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Runtime:")
	assert.Contains(t, out, "context: dev (a1b2c3d4)")
	assert.Contains(t, out, "driver:")
	assert.Contains(t, out, "docker: unavailable")
	assert.Contains(t, out, "dirs:")
	assert.Contains(t, out, "runtime:")
	assert.Contains(t, out, "labs:")
	assert.Contains(t, out, "auth:")
	assert.Contains(t, out, "proxy:")
	assert.Contains(t, out, "observability:")
	assert.Contains(t, out, "Labs:")
	assert.Contains(t, out, "Proxy:")
	assert.Contains(t, out, "Langfuse stack:")
	assert.Contains(t, out, "Status: not initialized")
	assert.Less(t, strings.Index(out, "Runtime:"), strings.Index(out, "Proxy:"))
	assert.Less(t, strings.Index(out, "Proxy:"), strings.Index(out, "Langfuse stack:"))
	assert.Less(t, strings.Index(out, "Langfuse stack:"), strings.Index(out, "Labs:"))
}

func TestStatusPrintsRuntimeContext(t *testing.T) {
	tmp := t.TempDir()
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	require.NoError(t, os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir)
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", filepath.Join(tmp, ".observability"))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Runtime:")
	assert.Contains(t, out, "context: dev (a1b2c3d4)")
	assert.Contains(t, out, "Langfuse stack:")
	assert.Empty(t, stderr.String())
}

func TestStatusDoesNotCreateRuntimeStateForUnstartedDevRuntime(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then exit 1; fi
if [ "$1" = "ps" ]; then
  printf 'litellm\trunning\n'
  printf 'postgres\tstopped\n'
  exit 0
fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy:")
	assert.Contains(t, out, "URL: unavailable (not started)")
	assert.Contains(t, out, "Langfuse stack:")
	require.NoFileExists(t, filepath.Join(proxyDir, "runtime.json"))
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

func TestStatusReportsRemovedProxyWhenGeneratedConfigAndStaleRuntimeRemain(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then exit 1; fi
if [ "$1" = "ps" ]; then
  printf 'litellm\trunning\n'
  printf 'postgres\tstopped\n'
  exit 0
fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(proxyDir), []byte(`{"port":55124}`+"\n"), 0o600))
	require.NoError(t, os.WriteFile(proxyRoutesStatePath(proxyDir), []byte(`{"routes":[]}`+"\n"), 0o600))
	require.NoError(t, os.WriteFile(proxyConfigStatePath(proxyDir), []byte(":4000 {}\n"), 0o644))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy:")
	assert.Contains(t, out, "Status: removed")
	assert.Contains(t, out, "URL: unavailable (not started)")
	assert.Contains(t, out, "Reason: proxy removed; generated config preserved; it will restart when a lab gateway or Langfuse starts")
	require.FileExists(t, filepath.Join(proxyDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

func TestObserveStatusReportsRemovedRuntimeWhenCredentialsRemain(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then exit 1; fi
if [ "$1" = "ps" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Langfuse stack:")
	assert.Contains(t, out, "Status: removed")
	assert.Contains(t, out, "Compose project: taxiway-dev-a1b2c3d4-observability")
	assert.Contains(t, out, "URL: unavailable (not started)")
	assert.Contains(t, out, "Reason: stack removed; credentials preserved; run `taxiway observe up`")
	assert.NotContains(t, out, "Services:")
	assert.Empty(t, stderr.String())
}

func TestObserveStatusReportsStoppedRuntimeWhenPortsRemain(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then exit 1; fi
if [ "$1" = "ps" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{"initialized":true}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{"port":55124}`+"\n"), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Langfuse stack:")
	assert.Contains(t, out, "Status: stopped")
	assert.Contains(t, out, "Compose project: taxiway-dev-a1b2c3d4-observability")
	assert.Contains(t, out, "URL: unavailable (not started)")
	assert.Contains(t, out, "Reason: no Langfuse containers found")
	assert.Contains(t, out, "Services:")
	assert.Empty(t, stderr.String())
}

func TestObserveStatusReportsLangfuseProxyURLWhenRunning(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "ps" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{"initialized":true}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{"port":55124}`+"\n"), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Langfuse stack:")
	assert.Contains(t, out, "Status: running")
	assert.Contains(t, out, "URL: http://langfuse.localhost:55124")
	assert.NotContains(t, out, "http://localhost:3000")
	assert.NotContains(t, out, "http://localhost:55123")
	assert.Empty(t, stderr.String())
}

func TestObserveStatusReportsPartialLangfuseRuntime(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then
  case "$4" in
    taxiway-dev-a1b2c3d4-observability-postgres-1) echo running; exit 0 ;;
    *) exit 1 ;;
  esac
fi
if [ "$1" = "ps" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{
  "langfuse_port": 55123
}
`), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Status: partial")
	assert.Contains(t, out, "postgres: running")
	assert.Contains(t, out, "minio: missing")
	assert.Empty(t, stderr.String())
}

func TestStatusReportsConfiguredSidecarWithoutContainers(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	stateDir := filepath.Join(tmp, ".lab-state")
	labGatewayDir := filepath.Join(stateDir, "test-codex", "gateway")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "ps" ]; then
  printf 'litellm\trunning\n'
  printf 'postgres\tstopped\n'
  exit 0
fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "test-codex"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "test-codex", "created_at"), []byte("now\n"), 0o600))
	require.NoError(t, config.WriteLabRef(stateDir, "test-codex", config.LabRef{Lab: "test-codex", Orch: "codex", Driver: "docker"}))
	require.NoError(t, os.MkdirAll(labGatewayDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(labGatewayDir, "litellm.compose.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(labGatewayDir, "route.env"), []byte("LAB=test-codex\nSERVICE=taxiway-dev-a1b2c3d4-test-codex-gateway-litellm-1\nHOST=test-codex.litellm.localhost\nPROJECT=taxiway-dev-a1b2c3d4-test-codex-gateway\n"), 0o600))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = stateDir
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Labs:")
	assert.Contains(t, out, "  test-codex:")
	assert.Contains(t, out, "    Status: degraded")
	assert.Contains(t, out, "    Phase: -")
	assert.Contains(t, out, "    State dir: "+filepath.Join(stateDir, "test-codex"))
	assert.Contains(t, out, "    Gateway:")
	assert.Contains(t, out, "      Status: partial")
	assert.Contains(t, out, "      State dir: "+filepath.Join(stateDir, "test-codex", "gateway"))
	assert.Contains(t, out, "      Compose project: taxiway-dev-a1b2c3d4-test-codex-gateway")
	assert.Contains(t, out, "      URL: -")
	assert.Contains(t, out, "      Services:")
	assert.Contains(t, out, "        litellm: running")
	assert.Contains(t, out, "        postgres: stopped")
	assert.Empty(t, stderr.String())
}

func TestStatusIgnoresNonCanonicalRuntimeContextLabDirs(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, ".lab-state")
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "59eb6a64")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	writeFakeDocker(t, `#!/bin/sh
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then exit 1; fi
if [ "$1" = "ps" ]; then exit 0; fi
exit 1
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "test-observe"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "test-observe", "created_at"), []byte("2026-01-01T00:00:00Z\n"), 0o644))
	require.NoError(t, config.WriteLabRef(stateDir, "test-observe", config.LabRef{Lab: "test-observe", Orch: "codex", Driver: "docker"}))
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "dev-59eb6a64-test-observe", "phases"), 0o755))

	root, state, stdout, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = stateDir
	root.SetArgs([]string{"status"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	rows := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  test-observe:") {
			rows++
		}
	}
	require.Equal(t, 1, rows)
	assert.Empty(t, stderr.String())
}

// ── taxiway access ───────────────────────────────────────────────────────────────

func TestAccessPrintsGatewayEndpointsWhenObservabilityEnvIsMissing(t *testing.T) {
	tmp := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0755))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"access"})
	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.NotContains(t, out, "Access:")
	assert.Contains(t, out, "Proxy:")
	assert.Contains(t, out, "Observability:")
	assert.Contains(t, out, "Langfuse:")
	assert.Contains(t, out, "State: not initialized")
	assert.Contains(t, out, "Run `taxiway observe up`")
	assert.Contains(t, out, "Gateway:")
	assert.Contains(t, out, "Template:")
	assert.Empty(t, stderr.String())
}

func TestAccessDoesNotCreateRuntimeStateForUnstartedDevRuntime(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"access"})

	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Proxy:")
	assert.Contains(t, out, "URL: unavailable (not started)")
	assert.Contains(t, out, "Observability:")
	assert.Contains(t, out, "Langfuse:")
	assert.Contains(t, out, "State: not initialized")
	require.NoFileExists(t, filepath.Join(proxyDir, "runtime.json"))
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
	assert.Empty(t, stderr.String())
}

// TestAccessShowsCredentials verifies that `taxiway access` prints
// UI access details when .env exists with INIT_* values.
func TestAccessShowsCredentials(t *testing.T) {
	tmp := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	observabilityDir := filepath.Join(home, ".taxiway", "observability")
	proxyDir := filepath.Join(home, ".taxiway", "proxy")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	require.NoError(t, os.MkdirAll(observabilityDir, 0755))
	require.NoError(t, os.MkdirAll(proxyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{"port":55124}`+"\n"), 0o600))

	envContent := strings.Join([]string{
		"LANGFUSE_NEXTAUTH_SECRET=abc123",
		"LANGFUSE_SALT=def456",
		"LANGFUSE_ENCRYPTION_KEY=ghi789",
		"LANGFUSE_INIT_USER_EMAIL=admin@taxiway.local",
		"LANGFUSE_INIT_USER_PASSWORD=supersecret",
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte(envContent), 0600))

	root, _, stdout, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"access"})
	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.NotContains(t, out, "Access:")
	assert.Contains(t, out, "Proxy:")
	assert.Contains(t, out, "Observability:")
	assert.Contains(t, out, "Langfuse:")
	assert.Contains(t, out, "Gateway:")
	assert.Contains(t, out, "Template:")
	assert.Contains(t, out, "admin@taxiway.local")
	assert.Contains(t, out, "supersecret")
	assert.Contains(t, out, "http://langfuse.localhost:55124")
	assert.NotContains(t, out, "http://localhost:3000")
	assert.Contains(t, out, "http://<lab>.litellm.localhost:55124/ui/login")
	assert.Contains(t, out, "API: http://<lab>.litellm.localhost:55124")
}

func TestAccessPrintsCustomEndpointsForExistingLabs(t *testing.T) {
	tmp := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	observabilityDir := filepath.Join(home, ".taxiway", "observability")
	require.NoError(t, os.MkdirAll(observabilityDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=supersecret\n"), 0600))

	stateDir := t.TempDir()
	ref := config.LabRef{Lab: "test-codex", Orch: "codex", Driver: "mock"}
	require.NoError(t, writeLabGatewayEnv(stateDir, ref, map[string]string{
		labLiteLLMAPIKeyEnv: "sk-taxiway-test",
	}))
	require.NoError(t, writeLabLiteLLMRoute(stateDir, ref, labLiteLLMRoute{
		Lab:     "test-codex",
		Service: "taxiway-test-codex-gateway-litellm-1",
	}))

	root, state, stdout, _ := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = stateDir
	root.SetArgs([]string{"access"})
	err := root.Execute()

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Lab gateways:")
	assert.Contains(t, out, "test-codex:")
	assert.Contains(t, out, "UI:  http://test-codex.litellm.localhost:4000/ui/login")
	assert.Contains(t, out, "API: http://test-codex.litellm.localhost:4000")
	require.Less(t,
		strings.Index(out, "API: http://test-codex.litellm.localhost:4000"),
		strings.Index(out, "UI:  http://test-codex.litellm.localhost:4000/ui/login"),
	)
	assert.Contains(t, out, "Username: admin")
	assert.Contains(t, out, "Password/API key: sk-taxiway-test")
}

func TestObserveResetHelpShowsRotateSecretsFlag(t *testing.T) {
	tmp := t.TempDir()
	root, _, stdout, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "reset", "--help"})

	err := root.Execute()

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "--rotate-secrets")
}

func TestObserveResetReloadsProxyAfterObservabilityRouteReturns(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	labStateDir := filepath.Join(tmp, ".lab-state")
	logPath := filepath.Join(tmp, "docker.log")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=test\nLANGFUSE_POSTGRES_PASSWORD=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{"initialized":true}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(serverURL.Host)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(fmt.Sprintf(`{"port":%s}`+"\n", port)), 0o600))
	require.NoError(t, os.MkdirAll(labStateDir, 0o700))
	_, err = upsertProxyRoute(proxyDir, observabilityProxyRoute(observabilityRuntime{
		Context:        "dev",
		ContextID:      "a1b2c3d4",
		StateDir:       observabilityDir,
		ComposeProject: "taxiway-dev-a1b2c3d4-observability",
	}))
	require.NoError(t, err)
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "version" ]; then exit 0; fi
if [ "$1" = "port" ]; then echo '127.0.0.1:%s'; exit 0; fi
if [ "$1" = "compose" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "inspect" ]; then echo running; exit 0; fi
if [ "$1" = "network" ]; then exit 0; fi
if [ "$1" = "exec" ]; then exit 0; fi
exit 0
`, logPath, port))

	root, state, _, stderr := buildObserveTestRoot(t, tmp)
	state.Flags.StateDir = labStateDir
	root.SetArgs([]string{"observe", "reset"})

	err = root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Equal(t, 1, strings.Count(log, "exec taxiway-dev-a1b2c3d4-proxy caddy reload"))
	downIndex := strings.Index(log, "down -v")
	upIndex := strings.Index(log, "up -d")
	reloadIndex := strings.Index(log, "exec taxiway-dev-a1b2c3d4-proxy caddy reload")
	require.NotEqual(t, -1, downIndex)
	require.NotEqual(t, -1, upIndex)
	require.NotEqual(t, -1, reloadIndex)
	assert.Less(t, downIndex, upIndex)
	assert.Less(t, upIndex, reloadIndex)
	assert.Empty(t, stderr.String())
}

func TestObserveOpenDoesNotCreateRuntimeStateForUnstartedDevRuntime(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))

	root, _, _, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "open"})

	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "run `taxiway observe up` first")
	require.NoFileExists(t, filepath.Join(observabilityDir, "runtime.json"))
}

func TestObserveOpenRequiresObservabilityEnvForHostRuntime(t *testing.T) {
	tmp := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TAXIWAY_CONTEXT", "")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", "")
	t.Setenv("TAXIWAY_PROXY_DIR", "")
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(config.ProxyDir(), 0o700))
	require.NoError(t, os.WriteFile(proxyRuntimeStatePath(config.ProxyDir()), []byte(`{"port":55124}`+"\n"), 0o600))

	root, _, _, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "open"})

	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "run `taxiway observe up` first")
	require.NoFileExists(t, observabilityEnvPath(&RootState{}))
}

func TestObserveOpenUsesLangfuseProxyURL(t *testing.T) {
	tmp := t.TempDir()
	observabilityDir := filepath.Join(tmp, ".observability")
	proxyDir := filepath.Join(tmp, ".proxy")
	binDir := t.TempDir()
	openLog := filepath.Join(tmp, "open.log")
	t.Setenv("PATH", binDir)
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", observabilityDir)
	t.Setenv("TAXIWAY_PROXY_DIR", proxyDir)
	openerScript := []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" > %q\n", openLog))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "open"), openerScript, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "xdg-open"), openerScript, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "infra", "observability"), 0o755))
	require.NoError(t, os.MkdirAll(observabilityDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, ".env"), []byte("LANGFUSE_INIT_USER_PASSWORD=secret\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(observabilityDir, "runtime.json"), []byte(`{"initialized":true}`+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(proxyDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(proxyDir, "runtime.json"), []byte(`{"port":55124}`+"\n"), 0o600))

	root, _, stdout, stderr := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"observe", "open"})

	err := root.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(openLog)
	require.NoError(t, err)
	assert.Equal(t, "http://langfuse.localhost:55124\n", string(data))
	assert.Contains(t, stdout.String(), "Opening http://langfuse.localhost:55124")
	assert.Empty(t, stderr.String())
}

func TestAccessRejectsIncompleteDevContext(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", filepath.Join(t.TempDir(), ".observability"))
	tmp := t.TempDir()
	root, _, _, _ := buildObserveTestRoot(t, tmp)
	root.SetArgs([]string{"access"})

	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TAXIWAY_CONTEXT_ID")
}

func TestCredentialsCodexHelp(t *testing.T) {
	root, _, _, stdout, _ := buildAliasTestRoot(t)
	root.SetArgs([]string{"credentials", "codex", "--help"})

	err := root.Execute()

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Prepare host Codex auth for Taxiway LiteLLM")
}

// ── newUUID ───────────────────────────────────────────────────────────────────

// TestNewUUID_Format verifies that newUUID returns a string matching the
// standard UUID v4 format (xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx).
func TestNewUUID_Format(t *testing.T) {
	for i := 0; i < 10; i++ {
		u, err := newUUID()
		require.NoError(t, err)
		parts := strings.Split(u, "-")
		require.Len(t, parts, 5, "UUID must have 5 dash-separated parts: %s", u)
		assert.Len(t, parts[0], 8)
		assert.Len(t, parts[1], 4)
		assert.Len(t, parts[2], 4)
		assert.Len(t, parts[3], 4)
		assert.Len(t, parts[4], 12)
		// Version nibble must be '4'.
		assert.Equal(t, "4", string(parts[2][0]), "version nibble must be '4'")
	}
}

// TestNewUUID_Unique verifies that two successive calls produce different values.
func TestNewUUID_Unique(t *testing.T) {
	u1, err := newUUID()
	require.NoError(t, err)
	u2, err := newUUID()
	require.NoError(t, err)
	assert.NotEqual(t, u1, u2, "successive UUIDs must be different")
}

func TestObservabilityDir(t *testing.T) {
	state := &RootState{RepoDir: "/repo"}
	expected := filepath.Join("/repo", "infra", "observability")
	assert.Equal(t, expected, observabilityDir(state))
}

func TestObservabilityComposeFile(t *testing.T) {
	state := &RootState{RepoDir: "/repo"}
	expected := filepath.Join("/repo", "infra", "observability", "langfuse.compose.yml")
	assert.Equal(t, expected, observabilityComposeFile(state))
}

func TestObservabilityComposeFilesOnlyIncludesLangfuse(t *testing.T) {
	state := &RootState{RepoDir: "/repo"}
	assert.Equal(t, []string{
		filepath.Join("/repo", "infra", "observability", "langfuse.compose.yml"),
	}, observabilityComposeFiles(state))
}

func TestObservabilityComposeServicesOnlyIncludesLangfuseStack(t *testing.T) {
	assert.Equal(t, []string{
		"minio",
		"postgres",
		"redis",
		"clickhouse",
		"langfuse-web",
		"langfuse-worker",
	}, observabilityComposeServices())
}

func TestComposeCmdUsesObservabilityProjectName(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))

	cmd := composeCmd("/state/observability", envPath, []string{
		"/runtime/infra/observability/langfuse.compose.yml",
	}, "up", "-d")

	assert.Equal(t, []string{
		"docker",
		"compose",
		"--project-directory",
		"/state/observability",
		"--env-file",
		envPath,
		"-p",
		"taxiway-observability",
		"-f",
		"/runtime/infra/observability/langfuse.compose.yml",
		"up",
		"-d",
	}, cmd.Args)
}

func TestComposeProjectCmdUsesProvidedProjectName(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("LANGFUSE_INIT_USER_PASSWORD=test\n"), 0o600))

	cmd := composeProjectCmd("taxiway-gastown-gateway", "/state/observability", envPath, []string{
		"/state/observability/labs/gastown/litellm.compose.yml",
	}, "ps")

	assert.Equal(t, []string{
		"docker",
		"compose",
		"--project-directory",
		"/state/observability",
		"--env-file",
		envPath,
		"-p",
		"taxiway-gastown-gateway",
		"-f",
		"/state/observability/labs/gastown/litellm.compose.yml",
		"ps",
	}, cmd.Args)
}

func TestLabLiteLLMSidecarDownCmdPreservesVolumes(t *testing.T) {
	tmp := t.TempDir()

	state := &RootState{
		Proxy: proxyRuntime{
			Context:        "dev",
			ContextID:      "abc12345",
			StateDir:       tmp,
			ComposeProject: "taxiway-dev-abc12345-proxy",
			Container:      "taxiway-dev-abc12345-proxy",
		},
	}
	ref := config.LabRef{Lab: "sample", Orch: "codex", Driver: "docker"}
	composePath := filepath.Join(tmp, "labs", "sample", "litellm.compose.yml")

	cmd := labLiteLLMSidecarDownCmd(state, filepath.Dir(composePath), composePath, ref, false)

	assert.Equal(t, []string{
		"docker",
		"compose",
		"--project-directory",
		filepath.Dir(composePath),
		"-p",
		"taxiway-dev-abc12345-sample-gateway",
		"-f",
		composePath,
		"down",
	}, cmd.Args)
	assert.NotContains(t, cmd.Args, "-v")
}

func TestLabLiteLLMSidecarStopCmdStopsWithoutRemovingContainers(t *testing.T) {
	tmp := t.TempDir()

	state := &RootState{
		Proxy: proxyRuntime{
			Context:        "dev",
			ContextID:      "abc12345",
			StateDir:       tmp,
			ComposeProject: "taxiway-dev-abc12345-proxy",
			Container:      "taxiway-dev-abc12345-proxy",
		},
	}
	ref := config.LabRef{Lab: "sample", Orch: "codex", Driver: "docker"}
	composePath := filepath.Join(tmp, "labs", "sample", "litellm.compose.yml")

	cmd := labLiteLLMSidecarStopCmd(state, filepath.Dir(composePath), composePath, ref)

	assert.Equal(t, []string{
		"docker",
		"compose",
		"--project-directory",
		filepath.Dir(composePath),
		"-p",
		"taxiway-dev-abc12345-sample-gateway",
		"-f",
		composePath,
		"stop",
	}, cmd.Args)
	assert.NotContains(t, cmd.Args, "down")
	assert.NotContains(t, cmd.Args, "-v")
}

func TestComposeCmdOmitsMissingEnvFile(t *testing.T) {
	cmd := composeCmd("/state/observability", "/state/observability/.env", []string{"/runtime/infra/observability/langfuse.compose.yml"}, "ps")

	assert.NotContains(t, cmd.Args, "--env-file")
}

func TestObservabilityEnvPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	state := &RootState{RepoDir: "/repo"}

	expected := filepath.Join(home, ".taxiway", "observability", ".env")
	assert.Equal(t, expected, observabilityEnvPath(state))
}

func TestDefaultCodexAuthFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	expected := filepath.Join(home, ".codex", "auth.json")
	assert.Equal(t, expected, defaultCodexAuthFile())
}

func TestLiteLLMChatGPTTokenStateDir(t *testing.T) {
	stateDir := filepath.Join("/state", "auth")
	expected := filepath.Join("/state", "auth", "providers", "codex", "chatgpt_token")

	assert.Equal(t, expected, liteLLMChatGPTTokenStateDir(stateDir))
}
