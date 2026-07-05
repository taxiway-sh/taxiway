package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

// observabilityDir returns the absolute path to the bundled observability assets under
// the taxiway runtime root (state.RepoDir).
func observabilityDir(state *RootState) string {
	return filepath.Join(state.RepoDir, "infra", "observability")
}

func observabilityComposeFile(state *RootState) string {
	return filepath.Join(observabilityDir(state), "langfuse.compose.yml")
}

func observabilityComposeFiles(state *RootState) []string {
	return []string{observabilityComposeFile(state)}
}

func observabilityComposeServices() []string {
	return []string{
		"minio",
		"postgres",
		"redis",
		"clickhouse",
		"langfuse-web",
		"langfuse-worker",
	}
}

func observabilityStateDir(state *RootState) string {
	return state.observabilityRuntime().StateDir
}

// observabilityEnvPath returns the path to the mutable observability .env state file.
func observabilityEnvPath(state *RootState) string {
	return filepath.Join(observabilityStateDir(state), ".env")
}

func liteLLMModelsAssetPath(state *RootState) string {
	return filepath.Join(state.RepoDir, "infra", "gateway", "litellm", "models.yaml")
}

func liteLLMConfigStatePath(stateDir string) string {
	return filepath.Join(stateDir, "litellm_config.yaml")
}

func liteLLMCodexSessionMapperAssetPath(state *RootState) string {
	return filepath.Join(state.RepoDir, "infra", "gateway", "litellm", "callbacks", "codex_session_mapper.py")
}

func defaultCodexAuthFile() string {
	return filepath.Join(os.Getenv("HOME"), ".codex", "auth.json")
}

const defaultObservabilityComposeProjectName = "taxiway-observability"

type observabilityRuntime struct {
	Context              string
	ContextID            string
	StateDir             string
	ComposeProject       string
	WorkerPort           int
	LangfusePort         int
	ClickHouseHTTPPort   int
	ClickHouseNativePort int
	MinIOPort            int
	MinIOConsolePort     int
	RedisPort            int
	PostgresPort         int
}

type observabilityRuntimeState struct {
	Initialized          bool `json:"initialized,omitempty"`
	WorkerPort           int  `json:"worker_port,omitempty"`
	LangfusePort         int  `json:"langfuse_port,omitempty"`
	ClickHouseHTTPPort   int  `json:"clickhouse_http_port,omitempty"`
	ClickHouseNativePort int  `json:"clickhouse_native_port,omitempty"`
	MinIOPort            int  `json:"minio_port,omitempty"`
	MinIOConsolePort     int  `json:"minio_console_port,omitempty"`
	RedisPort            int  `json:"redis_port,omitempty"`
	PostgresPort         int  `json:"postgres_port,omitempty"`
}

func (state *RootState) resolveObservabilityRuntime() (observabilityRuntime, error) {
	runtime := state.Observability
	if runtime.Context == "" {
		runtime.Context = strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT"))
	}
	if runtime.Context == "" {
		runtime.Context = "host"
	}
	if runtime.ContextID == "" {
		runtime.ContextID = strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT_ID"))
	}

	if runtime.StateDir == "" {
		switch runtime.Context {
		case "host":
			runtime.StateDir = config.ObservabilityDir()
		case "dev", "e2e":
			runtime.StateDir = strings.TrimSpace(os.Getenv("TAXIWAY_OBSERVABILITY_DIR"))
			if runtime.StateDir == "" {
				return observabilityRuntime{}, fmt.Errorf("TAXIWAY_OBSERVABILITY_DIR is required when TAXIWAY_CONTEXT=%s", runtime.Context)
			}
		default:
			return observabilityRuntime{}, fmt.Errorf("unsupported TAXIWAY_CONTEXT %q", runtime.Context)
		}
	}

	if (runtime.Context == "dev" || runtime.Context == "e2e") && runtime.ContextID == "" {
		return observabilityRuntime{}, fmt.Errorf("TAXIWAY_CONTEXT_ID is required when TAXIWAY_CONTEXT=%s", runtime.Context)
	}
	if runtime.ContextID != "" && !isValidTaxiwayContextID(runtime.ContextID) {
		return observabilityRuntime{}, fmt.Errorf("TAXIWAY_CONTEXT_ID must start with a lowercase letter or number and contain only lowercase letters, numbers, dashes, and underscores")
	}

	if runtime.ComposeProject == "" {
		if runtime.Context == "host" {
			runtime.ComposeProject = defaultObservabilityComposeProjectName
		} else {
			runtime.ComposeProject = fmt.Sprintf("taxiway-%s-%s-observability", runtime.Context, runtime.ContextID)
		}
	}
	if runtime.Context == "dev" || runtime.Context == "e2e" {
		if _, _, err := readDevObservabilityRuntimeState(runtime.StateDir); err != nil {
			return observabilityRuntime{}, err
		}
	}
	return runtime, nil
}

func readDevObservabilityRuntimeState(stateDir string) (observabilityRuntimeState, bool, error) {
	path := observabilityRuntimeStatePath(stateDir)
	var state observabilityRuntimeState
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, false, nil
	}
	if err != nil {
		return state, false, fmt.Errorf("read observability runtime state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, false, fmt.Errorf("read observability runtime state: %w", err)
	}
	return state, true, nil
}

func applyObservabilityRuntimeState(runtime *observabilityRuntime, state observabilityRuntimeState) {
	_ = runtime
	_ = state
}

func applyDefaultObservabilityRuntimePorts(runtime *observabilityRuntime) {
	_ = runtime
}

func ensureDevObservabilityRuntimeState(runtime *observabilityRuntime) error {
	path := observabilityRuntimeStatePath(runtime.StateDir)
	state, ok, err := readDevObservabilityRuntimeState(runtime.StateDir)
	if err != nil {
		return err
	}

	current := observabilityRuntimeState{
		Initialized: true,
	}
	if ok && current == state {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create observability runtime state dir: %w", err)
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write observability runtime state: %w", err)
	}
	return nil
}

func observabilityRuntimeStatePath(stateDir string) string {
	return filepath.Join(stateDir, "runtime.json")
}

func isValidTaxiwayContextID(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
			return false
		case r >= '0' && r <= '9':
		case i > 0 && (r == '-' || r == '_'):
		default:
			return false
		}
	}
	return true
}

func allocateLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate observability port: %w", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (state *RootState) observabilityRuntime() observabilityRuntime {
	runtime, err := state.resolveObservabilityRuntime()
	if err != nil {
		return state.Observability
	}
	return runtime
}

func (state *RootState) ensureObservabilityRuntime() (observabilityRuntime, error) {
	runtime, err := state.resolveObservabilityRuntime()
	if err != nil {
		return observabilityRuntime{}, err
	}
	if runtime.Context == "dev" || runtime.Context == "e2e" {
		if err := ensureDevObservabilityRuntimeState(&runtime); err != nil {
			return observabilityRuntime{}, err
		}
	} else {
		applyDefaultObservabilityRuntimePorts(&runtime)
	}
	state.Observability = runtime
	return runtime, nil
}

func (runtime observabilityRuntime) DockerNetwork() string {
	return runtime.ComposeProject + "_default"
}

func (runtime observabilityRuntime) PostgresContainer() string {
	return runtime.ComposeProject + "-postgres-1"
}

func (runtime observabilityRuntime) ClickHouseContainer() string {
	return runtime.ComposeProject + "-clickhouse-1"
}

func localRuntimeURL(port int) string {
	if port == 0 {
		return "unavailable (not started)"
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

func (runtime observabilityRuntime) ComposeEnv(proxy proxyRuntime) []string {
	env := []string{
		"NEXTAUTH_URL=" + proxy.LangfuseBaseURL(),
	}
	for _, service := range observabilityComposeServices() {
		env = append(env, observabilityDNSAliasEnvKey(service)+"="+observabilityDNSAlias(runtime, service))
	}
	return env
}

type liteLLMModelCatalog struct {
	Models []liteLLMModelDefinition `yaml:"models"`
}

type liteLLMModelDefinition struct {
	Name                 string `yaml:"name"`
	Provider             string `yaml:"provider"`
	Upstream             string `yaml:"upstream"`
	APIBase              string `yaml:"api_base,omitempty"`
	APIKey               string `yaml:"api_key,omitempty"`
	API                  string `yaml:"api,omitempty"`
	ForwardClientHeaders bool   `yaml:"forward_client_headers,omitempty"`
}

type liteLLMGeneratedConfig struct {
	ModelList       []liteLLMGeneratedModelEntry `yaml:"model_list"`
	LiteLLMSettings liteLLMGeneratedSettings     `yaml:"litellm_settings"`
	GeneralSettings liteLLMGeneratedGeneral      `yaml:"general_settings"`
}

type liteLLMGeneratedModelEntry struct {
	ModelName     string                      `yaml:"model_name"`
	ModelInfo     *liteLLMGeneratedModelInfo  `yaml:"model_info,omitempty"`
	LiteLLMParams liteLLMGeneratedModelParams `yaml:"litellm_params"`
}

type liteLLMGeneratedModelInfo struct {
	Mode string `yaml:"mode"`
}

type liteLLMGeneratedModelParams struct {
	Model   string `yaml:"model"`
	APIBase string `yaml:"api_base,omitempty"`
	APIKey  string `yaml:"api_key,omitempty"`
}

type liteLLMGeneratedSettings struct {
	MasterKey          string                              `yaml:"master_key"`
	Callbacks          []string                            `yaml:"callbacks"`
	LogRawRequest      bool                                `yaml:"log_raw_request_response"`
	RedactAPIKeyInfo   bool                                `yaml:"redact_user_api_key_info"`
	ModelGroupSettings *liteLLMGeneratedModelGroupSettings `yaml:"model_group_settings,omitempty"`
}

type liteLLMGeneratedModelGroupSettings struct {
	ForwardClientHeadersToLLMAPI []string `yaml:"forward_client_headers_to_llm_api,omitempty"`
}

type liteLLMGeneratedGeneral struct {
	DatabaseURL                  string `yaml:"database_url"`
	ForwardClientHeadersToLLMAPI bool   `yaml:"forward_client_headers_to_llm_api"`
}

func ensureLiteLLMConfig(state *RootState, stateDir string, includeCodexModels bool, enableCodexSessionMapper bool, selectedModels []string) (bool, error) {
	dst := liteLLMConfigStatePath(stateDir)
	data, err := renderLiteLLMConfig(state, includeCodexModels, enableCodexSessionMapper, selectedModels)
	if err != nil {
		return false, err
	}

	existing, err := os.ReadFile(dst)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading LiteLLM config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false, fmt.Errorf("creating LiteLLM config directory: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return false, fmt.Errorf("writing LiteLLM config: %w", err)
	}
	return true, nil
}

func renderLiteLLMConfig(state *RootState, includeCodexModels bool, enableCodexSessionMapper bool, selectedModels []string) ([]byte, error) {
	data, err := os.ReadFile(liteLLMModelsAssetPath(state))
	if err != nil {
		return nil, fmt.Errorf("reading LiteLLM model catalog: %w", err)
	}
	catalog, err := parseLiteLLMModelCatalog(data)
	if err != nil {
		return nil, err
	}

	var models []liteLLMGeneratedModelEntry
	var forwardHeaders []string
	selected := map[string]bool{}
	for _, name := range selectedModels {
		if name != "" {
			selected[name] = true
		}
	}
	matchedSelected := map[string]bool{}
	addForwardHeader := func(name string) {
		if name == "" {
			return
		}
		for _, existing := range forwardHeaders {
			if existing == name {
				return
			}
		}
		forwardHeaders = append(forwardHeaders, name)
	}
	for _, model := range catalog.Models {
		if len(selected) > 0 && !selected[model.Name] {
			continue
		}
		if selected[model.Name] {
			matchedSelected[model.Name] = true
		}
		if model.Provider == "chatgpt" && !includeCodexModels && len(selected) == 0 {
			continue
		}
		entry := liteLLMGeneratedModelEntry{
			ModelName: model.Name,
			LiteLLMParams: liteLLMGeneratedModelParams{
				Model:   model.Provider + "/" + model.Upstream,
				APIBase: model.APIBase,
				APIKey:  model.APIKey,
			},
		}
		if model.API != "" {
			entry.ModelInfo = &liteLLMGeneratedModelInfo{Mode: model.API}
		}
		models = append(models, entry)
		if model.ForwardClientHeaders {
			addForwardHeader(model.Name)
		}
	}
	for name := range selected {
		if !matchedSelected[name] {
			return nil, fmt.Errorf("unknown LiteLLM model %q", name)
		}
	}

	callbacks := []string{"langfuse_otel"}
	if enableCodexSessionMapper {
		callbacks = append([]string{"codex_session_mapper.proxy_handler_instance"}, callbacks...)
	}

	generated := liteLLMGeneratedConfig{
		ModelList: models,
		LiteLLMSettings: liteLLMGeneratedSettings{
			MasterKey:        "os.environ/LITELLM_MASTER_KEY",
			Callbacks:        callbacks,
			LogRawRequest:    true,
			RedactAPIKeyInfo: true,
		},
		GeneralSettings: liteLLMGeneratedGeneral{
			DatabaseURL:                  "os.environ/DATABASE_URL",
			ForwardClientHeadersToLLMAPI: true,
		},
	}
	if len(forwardHeaders) > 0 {
		generated.LiteLLMSettings.ModelGroupSettings = &liteLLMGeneratedModelGroupSettings{
			ForwardClientHeadersToLLMAPI: forwardHeaders,
		}
	}

	out, err := yaml.Marshal(generated)
	if err != nil {
		return nil, fmt.Errorf("rendering LiteLLM config: %w", err)
	}
	return out, nil
}

func parseLiteLLMModelCatalog(data []byte) (liteLLMModelCatalog, error) {
	var catalog liteLLMModelCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return liteLLMModelCatalog{}, fmt.Errorf("parsing LiteLLM model catalog: %w", err)
	}
	return catalog, nil
}

func ensureLiteLLMChatGPTAuth(stateDir string, required bool) (bool, error) {
	sourcePath := defaultCodexAuthFile()

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			if required {
				return false, missingCodexAuthError(sourcePath)
			}
			if removeErr := os.Remove(liteLLMChatGPTAuthStatePath(stateDir)); removeErr != nil && !os.IsNotExist(removeErr) {
				return false, fmt.Errorf("removing stale LiteLLM ChatGPT auth file: %w", removeErr)
			}
			return false, nil
		}
		return false, fmt.Errorf("checking Codex auth file: %w", err)
	}
	if sourceInfo.IsDir() {
		return false, fmt.Errorf("Codex auth path is a directory, expected a file: %s", sourcePath)
	}

	dst := liteLLMChatGPTAuthStatePath(stateDir)
	if dstInfo, statErr := os.Stat(dst); statErr == nil && !sourceInfo.ModTime().After(dstInfo.ModTime()) {
		return true, nil
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return false, fmt.Errorf("reading Codex auth file: %w", err)
	}
	converted, err := convertCodexAuthToLiteLLMChatGPTAuth(data)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false, fmt.Errorf("creating LiteLLM ChatGPT auth directory: %w", err)
	}
	if err := os.WriteFile(dst, converted, 0o600); err != nil {
		return false, fmt.Errorf("writing LiteLLM ChatGPT auth file: %w", err)
	}
	return true, nil
}

func convertCodexAuthToLiteLLMChatGPTAuth(data []byte) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing Codex auth file: %w", err)
	}

	source := raw
	if tokens, ok := raw["tokens"].(map[string]any); ok {
		source = tokens
	}

	auth := map[string]any{}
	for _, key := range []string{"access_token", "refresh_token", "id_token", "account_id", "expires_at"} {
		if value, ok := source[key]; ok {
			auth[key] = value
		}
	}
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if value, ok := auth[key].(string); !ok || value == "" {
			return nil, fmt.Errorf("Codex auth file is missing %s for LiteLLM ChatGPT auth", key)
		}
	}

	converted, err := json.Marshal(auth)
	if err != nil {
		return nil, fmt.Errorf("encoding LiteLLM ChatGPT auth file: %w", err)
	}
	return converted, nil
}

// newObserveCmd wires the `taxiway observe` command group.
func newObserveCmd(state *RootState) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "observe",
		Short:       "Manage the local Langfuse observability stack",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newObserveUpCmd(state),
		newObserveDownCmd(state),
		newObserveRmCmd(state),
		newObserveResetCmd(state),
		newObserveOpenCmd(state),
	)
	return cmd
}

// checkDockerAvailable returns a clear error if docker is not on PATH or the
// daemon is not reachable.
func checkDockerAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found on PATH — install Docker Desktop or Docker Engine first")
	}
	if err := exec.Command("docker", "version").Run(); err != nil {
		return fmt.Errorf("docker daemon is not running (or not reachable): %w\n  hint: start Docker Desktop or run: dockerd &", err)
	}
	return nil
}

func observabilityDockerNetwork(state *RootState) string {
	return state.observabilityRuntime().DockerNetwork()
}

func dockerContainerExists(name string) (bool, error) {
	cmd := exec.Command("docker", "container", "inspect", name)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func dockerRemoveContainerIfExists(name string) (bool, error) {
	exists, err := dockerContainerExists(name)
	if err != nil {
		return false, fmt.Errorf("checking container %s: %w", name, err)
	}
	if !exists {
		return false, nil
	}
	if err := exec.Command("docker", "rm", "-f", name).Run(); err != nil {
		return false, fmt.Errorf("removing container %s: %w", name, err)
	}
	return true, nil
}

// generateSecret generates n random bytes and returns them as a hex string.
func generateSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// newUUID generates a random UUID v4 string using crypto/rand.
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	// Set version 4 (bits 12-15 of byte 6).
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant bits (bits 6-7 of byte 8).
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// readEnvFile parses key=value pairs from an env file.
// Lines that are empty or start with '#' are ignored.
func readEnvFile(path string) (map[string]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	vals := make(map[string]string)
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			vals[parts[0]] = parts[1]
		}
	}
	return vals, lines, scanner.Err()
}

// ensureEnvFile creates (or updates) the .env file, adding any missing keys.
// It is idempotent: existing keys are never overwritten.
// Returns true if any new keys were added (or the file was created).
func ensureEnvFile(envPath string) (created bool, err error) {
	existing := make(map[string]string)
	var existingLines []string

	if _, statErr := os.Stat(envPath); statErr == nil {
		existing, existingLines, err = readEnvFile(envPath)
		if err != nil {
			return false, fmt.Errorf("reading .env: %w", err)
		}
	}

	// Build the list of keys to generate if absent.
	type entry struct {
		key string
		gen func() (string, error)
	}

	entries := []entry{
		{
			"LANGFUSE_NEXTAUTH_SECRET",
			func() (string, error) { return generateSecret(32) },
		},
		{
			"LANGFUSE_SALT",
			func() (string, error) { return generateSecret(16) },
		},
		{
			"LANGFUSE_ENCRYPTION_KEY",
			func() (string, error) { return generateSecret(32) },
		},
		{
			"LANGFUSE_INIT_ORG_ID",
			func() (string, error) { return newUUID() },
		},
		{
			"LANGFUSE_INIT_USER_PASSWORD",
			func() (string, error) { return generateSecret(16) },
		},
		{
			"LANGFUSE_POSTGRES_PASSWORD",
			func() (string, error) { return generateSecret(24) },
		},
		{
			"LANGFUSE_CLICKHOUSE_PASSWORD",
			func() (string, error) { return generateSecret(24) },
		},
		{
			"LANGFUSE_MINIO_ROOT_PASSWORD",
			func() (string, error) { return generateSecret(24) },
		},
		{
			"LANGFUSE_REDIS_AUTH",
			func() (string, error) { return generateSecret(24) },
		},
	}

	var newLines []string
	for _, e := range entries {
		if _, ok := existing[e.key]; !ok {
			val, genErr := e.gen()
			if genErr != nil {
				return false, genErr
			}
			newLines = append(newLines, fmt.Sprintf("%s=%s", e.key, val))
			created = true
		}
	}

	if !created {
		return false, nil
	}

	// Rebuild file: existing lines + new lines.
	all := append(existingLines, newLines...)
	content := strings.Join(all, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		return false, fmt.Errorf("creating .env directory: %w", err)
	}
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		return false, fmt.Errorf("writing .env: %w", err)
	}
	return true, nil
}

// readEnvValue reads a single key from an env file.
func readEnvValue(envPath, key string) string {
	vals, _, err := readEnvFile(envPath)
	if err != nil {
		return ""
	}
	return vals[key]
}

func printObservabilityAccess(w interface{ Write([]byte) (int, error) }, envPath string, includeStartedHeader bool, runtime observabilityRuntime, proxy proxyRuntime) {
	_ = runtime
	password := readEnvValue(envPath, "LANGFUSE_INIT_USER_PASSWORD")
	email := readEnvValue(envPath, "LANGFUSE_INIT_USER_EMAIL")
	if email == "" {
		email = "admin@taxiway.local"
	}

	if includeStartedHeader {
		fmt.Fprintf(w, "\n✓ Observability stack started\n")
	}
	fmt.Fprintf(w, "\nAccess:\n")
	fmt.Fprintf(w, "  Langfuse:\n")
	fmt.Fprintf(w, "    URL:      %s\n", proxy.LangfuseBaseURL())
	fmt.Fprintf(w, "    Email:    %s\n", email)
	fmt.Fprintf(w, "    Password: %s\n", password)
	fmt.Fprintf(w, "\nRun `taxiway access` for proxy and lab gateway endpoints.\n")
}

func printRuntimeAccess(w interface{ Write([]byte) (int, error) }, state *RootState) {
	proxy := state.proxyRuntime()
	envPath := observabilityEnvPath(state)

	fmt.Fprintf(w, "Proxy:\n")
	fmt.Fprintf(w, "  URL: %s\n", localRuntimeURL(proxy.Port))
	fmt.Fprintf(w, "\nObservability:\n")
	fmt.Fprintf(w, "  Langfuse:\n")
	fmt.Fprintf(w, "    URL: %s\n", proxy.LangfuseBaseURL())

	values, _, err := readEnvFile(envPath)
	if err != nil {
		fmt.Fprintf(w, "    State: not initialized\n")
		fmt.Fprintf(w, "    Run `taxiway observe up` to enable Langfuse traces.\n")
	} else {
		email := values["LANGFUSE_INIT_USER_EMAIL"]
		if email == "" {
			email = "admin@taxiway.local"
		}
		fmt.Fprintf(w, "    Email: %s\n", email)
		fmt.Fprintf(w, "    Password: %s\n", values["LANGFUSE_INIT_USER_PASSWORD"])
	}

	fmt.Fprintf(w, "\nGateway:\n")
	fmt.Fprintf(w, "  Template:\n")
	if proxy.Port == 0 {
		fmt.Fprintf(w, "    unavailable (proxy not started)\n")
	} else {
		fmt.Fprintf(w, "    UI:  http://<lab>.litellm.localhost:%d/ui/login\n", proxy.Port)
		fmt.Fprintf(w, "    API: http://<lab>.litellm.localhost:%d\n", proxy.Port)
	}
}

func printObservabilityContext(w interface{ Write([]byte) (int, error) }, runtimeDir, labStateDir string, runtime observabilityRuntime, proxy proxyRuntime) {
	fmt.Fprintf(w, "Context:\n")
	fmt.Fprintf(w, "  context: %s\n", runtime.Context)
	if runtime.ContextID != "" {
		fmt.Fprintf(w, "  context_id: %s\n", runtime.ContextID)
	}
	fmt.Fprintf(w, "  runtime_dir: %s\n", runtimeDir)
	fmt.Fprintf(w, "  lab_state_dir: %s\n", labStateDir)
	fmt.Fprintf(w, "  observability_dir: %s\n", runtime.StateDir)
	fmt.Fprintf(w, "  compose_project: %s\n", runtime.ComposeProject)
	fmt.Fprintf(w, "  proxy_dir: %s\n", proxy.StateDir)
	fmt.Fprintf(w, "  proxy_container: %s\n", proxy.Container)
	fmt.Fprintf(w, "  proxy_url: %s\n", localRuntimeURL(proxy.Port))
}

func printLabLiteLLMEndpoints(w interface{ Write([]byte) (int, error) }, labStateDir string, routes []labLiteLLMRoute, proxy proxyRuntime) error {
	if len(routes) == 0 {
		return nil
	}
	fmt.Fprintf(w, "\nLab gateways:\n")
	if proxy.Port == 0 {
		fmt.Fprintf(w, "  unavailable (proxy not started)\n")
		return nil
	}
	for _, route := range routes {
		host := route.Host
		if host == "" {
			host = labLiteLLMHost(route.Lab)
		}
		values, err := readLabGatewayEnv(labStateDir, config.LabRef{Lab: route.Lab})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  %s:\n", route.Lab)
		fmt.Fprintf(w, "    API: http://%s:%d\n", host, proxy.Port)
		fmt.Fprintf(w, "    UI:  http://%s:%d/ui/login\n", host, proxy.Port)
		if key := values[labLiteLLMAPIKeyEnv]; key != "" {
			fmt.Fprintf(w, "    Username: admin\n")
			fmt.Fprintf(w, "    Password/API key: %s\n", key)
		}
	}
	return nil
}

// waitForLangfuse polls GET /api/public/health until 200 OK or timeout.
func waitForLangfuse(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func waitForLangfuseViaProxy(proxy proxyRuntime, timeout time.Duration) bool {
	if proxy.Port == 0 {
		return false
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, proxy.BaseURL()+"/api/public/health", nil)
		if err != nil {
			return false
		}
		req.Host = langfuseProxyHost
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// composeCmd builds an exec.Cmd for the global observability compose project
// with stdout/stderr wired to the process stdio.
func composeCmd(projectDir, envFile string, composeFiles []string, args ...string) *exec.Cmd {
	return composeProjectCmd(defaultObservabilityComposeProjectName, projectDir, envFile, composeFiles, args...)
}

func observabilityComposeCmd(state *RootState, projectDir, envFile string, composeFiles []string, args ...string) *exec.Cmd {
	cmd := composeProjectCmd(state.observabilityRuntime().ComposeProject, projectDir, envFile, composeFiles, args...)
	cmd.Env = append(os.Environ(), state.observabilityRuntime().ComposeEnv(state.proxyRuntime())...)
	return cmd
}

func composeProjectCmd(projectName, projectDir, envFile string, composeFiles []string, args ...string) *exec.Cmd {
	argv := []string{"compose", "--project-directory", projectDir}
	if envFile != "" {
		if _, err := os.Stat(envFile); err == nil {
			argv = append(argv, "--env-file", envFile)
		}
	}
	argv = append(argv, "-p", projectName)
	for _, composeFile := range composeFiles {
		argv = append(argv, "-f", composeFile)
	}
	argv = append(argv, args...)
	cmd := exec.Command("docker", argv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// ── taxiway observe up ────────────────────────────────────────────────────────────

type taxiwayRuntimeInitResult struct {
	Observability observabilityRuntime
	Proxy         proxyRuntime
	EnvPath       string
}

func initializeTaxiwayRuntime(state *RootState, w io.Writer, printAccess bool) (taxiwayRuntimeInitResult, error) {
	runtime, err := state.ensureObservabilityRuntime()
	if err != nil {
		return taxiwayRuntimeInitResult{}, err
	}
	proxy, err := state.ensureProxyRuntime()
	if err != nil {
		return taxiwayRuntimeInitResult{}, err
	}

	envPath := observabilityEnvPath(state)
	updated, err := ensureEnvFile(envPath)
	if err != nil {
		return taxiwayRuntimeInitResult{}, err
	}
	if updated {
		fmt.Fprintf(w, "Generated %s\n", envPath)
	}
	labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	if updated, err := ensureProxyConfigForState(state, labStateDir, proxy.StateDir); err != nil {
		return taxiwayRuntimeInitResult{}, err
	} else if updated {
		fmt.Fprintf(w, "Generated %s\n", proxyConfigStatePath(proxy.StateDir))
	}
	updatedRoute, err := upsertObservabilityProxyRoutes(proxy.StateDir, runtime)
	if err != nil {
		return taxiwayRuntimeInitResult{}, err
	}
	if updatedRoute {
		fmt.Fprintf(w, "Registered Langfuse route in %s\n", proxyRoutesStatePath(proxy.StateDir))
	}

	fmt.Fprintf(w, "Starting proxy %s...\n", proxy.Container)
	started, restarted, err := ensureProxyRunning(state, proxy.StateDir)
	if err != nil {
		return taxiwayRuntimeInitResult{}, err
	}
	proxy = state.proxyRuntime()
	if !started {
		fmt.Fprintln(w, "✓ Proxy reloaded")
	} else if restarted {
		fmt.Fprintln(w, "✓ Proxy restarted")
	} else {
		fmt.Fprintln(w, "✓ Proxy started")
	}

	if err := observabilityComposeCmd(state, observabilityStateDir(state), envPath, observabilityComposeFiles(state), "up", "-d").Run(); err != nil {
		return taxiwayRuntimeInitResult{}, fmt.Errorf("docker compose up: %w", err)
	}
	if err := connectProxyToRouteNetworks(state, proxy.StateDir); err != nil {
		return taxiwayRuntimeInitResult{}, err
	}
	proxyConfigUpdated, err := ensureProxyConfigForState(state, labStateDir, proxy.StateDir)
	if err != nil {
		return taxiwayRuntimeInitResult{}, err
	}
	if proxyConfigUpdated || (updatedRoute && !started) {
		if err := reloadProxy(state); err != nil {
			return taxiwayRuntimeInitResult{}, err
		}
	}

	fmt.Fprintf(w, "\nWaiting for observability to be ready...")
	if waitForLangfuseViaProxy(proxy, 60*time.Second) {
		if printAccess {
			printObservabilityAccess(w, envPath, true, runtime, proxy)
		} else {
			fmt.Fprintln(w, " ready.")
		}
	} else {
		fmt.Fprintf(w, "\nWarning: Langfuse may still be starting. Check with: taxiway status\n")
	}

	return taxiwayRuntimeInitResult{Observability: runtime, Proxy: proxy, EnvPath: envPath}, nil
}

func newObserveUpCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "up",
		Short:       "Start the Langfuse observability stack",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			_, err := initializeTaxiwayRuntime(state, cmd.OutOrStdout(), true)
			return err
		},
	}
}

// ── taxiway observe down ──────────────────────────────────────────────────────────

func newObserveDownCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "down",
		Short:       "Stop the observability stack",
		Long:        "Stop the observability stack. Containers, data volumes, and runtime port state are preserved.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			runtime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.resolveProxyRuntime()
			if err != nil {
				return err
			}
			state.Observability = runtime
			state.Proxy = proxy
			if err := observabilityComposeCmd(state, observabilityStateDir(state), observabilityEnvPath(state), observabilityComposeFiles(state), "stop").Run(); err != nil {
				return err
			}
			labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			if _, err := ensureProxyConfigForState(state, labStateDir, proxy.StateDir); err != nil {
				return err
			}
			if err := reconcileProxyLifecycleAndPrint(cmd.OutOrStdout(), state, proxy.StateDir); err != nil {
				return err
			}
			return nil
		},
	}
}

// ── taxiway observe rm ────────────────────────────────────────────────────────────

func newObserveRmCmd(state *RootState) *cobra.Command {
	var volumes bool
	cmd := &cobra.Command{
		Use:         "rm",
		Short:       "Remove the observability stack",
		Long:        "Remove the observability stack. Runtime port state is cleared so the next up can allocate currently available ports.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			runtime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.resolveProxyRuntime()
			if err != nil {
				return err
			}
			state.Observability = runtime
			state.Proxy = proxy
			if err := removeObservabilityProxyRoute(state, runtime, proxy); err != nil {
				return err
			}
			args := []string{"down"}
			if volumes {
				args = append(args, "-v")
			}
			if err := observabilityComposeCmd(state, observabilityStateDir(state), observabilityEnvPath(state), observabilityComposeFiles(state), args...).Run(); err != nil {
				return err
			}
			if err := removeObservabilityRuntimeState(runtime); err != nil {
				return err
			}
			labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			if _, err := ensureProxyConfigForState(state, labStateDir, proxy.StateDir); err != nil {
				return err
			}
			if err := reconcileProxyLifecycleAndPrint(cmd.OutOrStdout(), state, proxy.StateDir); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&volumes, "volumes", false, "remove observability Docker volumes")
	return cmd
}

func removeObservabilityProxyRoute(state *RootState, runtime observabilityRuntime, proxy proxyRuntime) error {
	labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	if proxyGeneratedStateExists(proxy.StateDir) {
		for _, route := range observabilityProxyRoutes(runtime) {
			if _, err := removeProxyRoute(proxy.StateDir, route.ID); err != nil {
				return err
			}
		}
		if _, err := ensureProxyConfig(labStateDir, proxy.StateDir); err != nil {
			return err
		}
		if err := disconnectProxyFromNetwork(state, runtime.DockerNetwork()); err != nil {
			return err
		}
	}
	return nil
}

func reconcileProxyLifecycleAndPrint(w io.Writer, state *RootState, proxyDir string) error {
	action, err := reconcileProxyLifecycle(state, proxyDir)
	if err != nil {
		return err
	}
	if action == "running" {
		if err := reloadProxy(state); err != nil {
			return err
		}
		fmt.Fprintf(w, "Proxy kept running\n")
		return nil
	}
	fmt.Fprintf(w, "Proxy was not running\n")
	return nil
}

func removeObservabilityRuntimeState(runtime observabilityRuntime) error {
	if runtime.Context != "dev" && runtime.Context != "e2e" {
		return nil
	}
	if err := os.Remove(observabilityRuntimeStatePath(runtime.StateDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove observability runtime state: %w", err)
	}
	return nil
}

// ── taxiway status ───────────────────────────────────────────────────────────────

func newStatusCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "Show Taxiway runtime status",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.resolveProxyRuntime()
			if err != nil {
				return err
			}
			state.Observability = runtime
			state.Proxy = proxy
			docker := detectDockerStatus()
			printTaxiwayRuntimeStatus(cmd.OutOrStdout(), state, docker)
			fmt.Fprintln(cmd.OutOrStdout())
			printProxyRuntimeStatus(cmd.OutOrStdout(), state, docker)
			fmt.Fprintln(cmd.OutOrStdout())
			printLangfuseStackStatus(cmd.OutOrStdout(), state, docker)
			fmt.Fprintln(cmd.OutOrStdout())
			if err := printStatusLabs(cmd.OutOrStdout(), state, docker); err != nil {
				return err
			}
			return nil
		},
	}
}

func newInitCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "init",
		Short:       "Initialize Taxiway runtime",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			if _, err := initializeTaxiwayRuntime(state, cmd.OutOrStdout(), false); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Taxiway runtime initialized.")
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Next:")
			fmt.Fprintln(cmd.OutOrStdout(), "  Create your first lab with `taxiway up mylab --type codex`")
			return nil
		},
	}
}

func newDestroyCmd(state *RootState) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:         "destroy",
		Short:       "Destroy Taxiway runtime",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			if !yes {
				fmt.Fprint(cmd.OutOrStdout(), "Destroy Taxiway runtime? This removes labs, gateways, observability, proxy, and volumes. [y/N] ")
				scanner := bufio.NewScanner(os.Stdin)
				scanner.Scan()
				if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			return destroyTaxiwayRuntime(cmd.OutOrStdout(), state)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func destroyTaxiwayRuntime(w io.Writer, state *RootState) error {
	runtime, err := state.resolveObservabilityRuntime()
	if err != nil {
		return err
	}
	proxy, err := state.resolveProxyRuntime()
	if err != nil {
		return err
	}
	state.Observability = runtime
	state.Proxy = proxy
	labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	fmt.Fprintln(w, "Destroying Taxiway runtime")

	labs, err := collectDestroyLabRefs(labStateDir)
	if err != nil {
		return err
	}
	for _, ref := range labs {
		d, err := destroyDriverForRef(state, labStateDir, ref)
		if err != nil {
			return err
		}
		previousDriver := state.Driver
		state.Driver = d
		if err := removeLabLiteLLMSidecar(context.Background(), state, ref); err != nil {
			state.Driver = previousDriver
			return err
		}
		state.Driver = previousDriver
		if ref.Driver != "" {
			if err := d.Delete(context.Background(), idName(ref.Lab)); err != nil {
				return err
			}
		}
	}
	if err := os.RemoveAll(labStateDir); err != nil {
		return fmt.Errorf("remove lab state: %w", err)
	}
	fmt.Fprintln(w, "Labs: removed")

	if _, err := removeProxyContainer(state); err != nil {
		return err
	}
	if err := os.RemoveAll(proxy.StateDir); err != nil {
		return fmt.Errorf("remove proxy state: %w", err)
	}
	fmt.Fprintln(w, "Proxy: removed")

	if err := observabilityComposeCmd(state, observabilityStateDir(state), observabilityEnvPath(state), observabilityComposeFiles(state), "down", "-v").Run(); err != nil {
		return fmt.Errorf("docker compose down -v: %w", err)
	}
	if err := os.RemoveAll(observabilityStateDir(state)); err != nil {
		return fmt.Errorf("remove observability state: %w", err)
	}
	fmt.Fprintln(w, "Observability: removed")
	return nil
}

func collectDestroyLabRefs(stateDir string) ([]config.LabRef, error) {
	entries, err := os.ReadDir(stateDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refs := make([]config.LabRef, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		lab := entry.Name()
		if config.LabDirOf(lab) != lab {
			continue
		}
		ref, ok, err := config.ReadLabRef(stateDir, lab)
		if err != nil {
			return nil, err
		}
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func destroyDriverForRef(state *RootState, stateDir string, ref config.LabRef) (driver.Driver, error) {
	if ref.Driver == "" {
		return nil, fmt.Errorf("lab %q is missing driver in ref.json", ref.Lab)
	}
	if state.Driver != nil && ref.Driver == state.Driver.Name() {
		return state.Driver, nil
	}
	return newDriverByName(ref.Driver, stateDir)
}

// ── taxiway access ───────────────────────────────────────────────────────────────

func newAccessCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "access",
		Short:       "Show Taxiway service access details",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.resolveProxyRuntime()
			if err != nil {
				return err
			}
			state.Observability = runtime
			state.Proxy = proxy

			out := cmd.OutOrStdout()
			printRuntimeAccess(out, state)
			labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			routes, err := readLabLiteLLMRoutes(labStateDir)
			if err != nil {
				return err
			}
			return printLabLiteLLMEndpoints(out, labStateDir, routes, state.proxyRuntime())
		},
	}
}

// ── taxiway repair ───────────────────────────────────────────────────────────────

func newRepairCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "repair",
		Short:       "Repair generated Taxiway runtime state",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.resolveProxyRuntime()
			if err != nil {
				return err
			}
			state.Observability = runtime
			state.Proxy = proxy
			labStateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

			observabilityRepair, err := repairObservabilityStack(state)
			if err != nil {
				return err
			}

			labRoutes, err := readLabLiteLLMRoutes(labStateDir)
			if err != nil {
				return err
			}
			proxyRoutes, err := readProxyRoutes(proxy.StateDir)
			if err != nil {
				return err
			}
			proxyRepair := proxyRepairResult{
				Config:    "skipped (no routes)",
				Container: "skipped (no routes)",
				Networks:  "skipped",
			}
			hasProxyState := proxyGeneratedStateExists(proxy.StateDir)
			if hasProxyState || len(labRoutes) > 0 {
				updated, err := ensureProxyConfigForState(state, labStateDir, proxy.StateDir)
				if err != nil {
					return err
				}
				proxyRepair.Config = "ok"
				if updated {
					proxyRepair.Config = "updated"
				}

				proxyRepair.Container = "skipped"
				docker := detectDockerStatus()
				if !docker.Available {
					proxyRepair.Container = fmt.Sprintf("skipped (docker unavailable: %s)", docker.Reason)
				} else {
					switch containerState(proxy.Container) {
					case "running":
						if err := refreshProxyPublishedPort(state); err != nil {
							return err
						}
						if err := connectProxyToRouteNetworks(state, proxy.StateDir); err != nil {
							return err
						}
						if err := reloadProxy(state); err != nil {
							return err
						}
						proxyRepair.Container = "reloaded"
						proxyRepair.Networks = "connected"
					default:
						if proxyRuntimePortAvailable(proxy) || len(labRoutes) > 0 || len(proxyRoutes) > 0 {
							if _, _, err := ensureProxyRunning(state, proxy.StateDir); err != nil {
								return err
							}
							if err := connectProxyToRouteNetworks(state, proxy.StateDir); err != nil {
								return err
							}
							proxyRepair.Container = "restarted"
							proxyRepair.Networks = "connected"
						} else {
							proxyRepair.Container = "skipped (proxy is not initialized)"
						}
					}
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Proxy repair:")
			fmt.Fprintf(cmd.OutOrStdout(), "  Config: %s\n", proxyRepair.Config)
			fmt.Fprintf(cmd.OutOrStdout(), "  Container: %s\n", proxyRepair.Container)
			fmt.Fprintf(cmd.OutOrStdout(), "  Networks: %s\n", proxyRepair.Networks)
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Observability repair:")
			fmt.Fprintf(cmd.OutOrStdout(), "  Env: %s\n", observabilityRepair.Env)
			fmt.Fprintf(cmd.OutOrStdout(), "  Stack: %s\n", observabilityRepair.Stack)
			fmt.Fprintf(cmd.OutOrStdout(), "  Routes: %s\n", observabilityRepair.Routes)
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Lab routes repair:")
			fmt.Fprintf(cmd.OutOrStdout(), "  Routes: %d found\n", len(labRoutes))
			return nil
		},
	}
}

type proxyRepairResult struct {
	Config    string
	Container string
	Networks  string
}

type observabilityRepairResult struct {
	Env    string
	Stack  string
	Routes string
}

func repairObservabilityStack(state *RootState) (observabilityRepairResult, error) {
	result := observabilityRepairResult{
		Env:    "skipped (not initialized)",
		Stack:  "skipped (not initialized)",
		Routes: "skipped (not initialized)",
	}
	envPath := observabilityEnvPath(state)
	if !fileExists(envPath) {
		return result, nil
	}

	runtime, err := state.ensureObservabilityRuntime()
	if err != nil {
		return result, err
	}
	updatedEnv, err := ensureEnvFile(envPath)
	if err != nil {
		return result, err
	}
	result.Env = "ok"
	if updatedEnv {
		result.Env = "updated"
	}
	updatedRoutes, err := upsertObservabilityProxyRoutes(state.proxyRuntime().StateDir, runtime)
	if err != nil {
		return result, err
	}
	result.Routes = "ok"
	if updatedRoutes {
		result.Routes = "restored"
	}

	docker := detectDockerStatus()
	if !docker.Available {
		result.Stack = fmt.Sprintf("skipped (docker unavailable: %s)", docker.Reason)
		return result, nil
	}

	status, _ := summarizeLangfuseStatus(true, observabilityRuntimeInitialized(runtime), docker, langfuseServiceStates(runtime, docker))
	if status == "running" {
		result.Stack = "running"
		return result, nil
	}
	if err := observabilityComposeCmd(state, observabilityStateDir(state), envPath, observabilityComposeFiles(state), "up", "-d").Run(); err != nil {
		return result, fmt.Errorf("docker compose up: %w", err)
	}
	result.Stack = "restarted"
	return result, nil
}

type dockerRuntimeStatus struct {
	Available bool
	Reason    string
}

func detectDockerStatus() dockerRuntimeStatus {
	if _, err := exec.LookPath("docker"); err != nil {
		return dockerRuntimeStatus{Available: false, Reason: "docker not found on PATH"}
	}
	if out, err := exec.Command("docker", "version").CombinedOutput(); err != nil {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = err.Error()
		}
		return dockerRuntimeStatus{Available: false, Reason: reason}
	}
	return dockerRuntimeStatus{Available: true}
}

func printLangfuseStackStatus(w interface{ Write([]byte) (int, error) }, state *RootState, docker dockerRuntimeStatus) {
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()
	credentialsPresent := fileExists(observabilityEnvPath(state))
	runtimePresent := observabilityRuntimeInitialized(runtime)
	services := langfuseServiceStates(runtime, docker)
	status, reason := summarizeLangfuseStatus(credentialsPresent, runtimePresent, docker, services)

	fmt.Fprintln(w, "Langfuse stack:")
	fmt.Fprintf(w, "  Status: %s\n", status)
	fmt.Fprintf(w, "  Compose project: %s\n", runtime.ComposeProject)
	fmt.Fprintf(w, "  State dir: %s\n", runtime.StateDir)
	url := "unavailable (not started)"
	if status == "running" {
		url = proxy.LangfuseBaseURL()
	}
	fmt.Fprintf(w, "  URL: %s\n", url)
	if reason != "" {
		fmt.Fprintf(w, "  Reason: %s\n", reason)
	}
	if credentialsPresent && runtimePresent && docker.Available {
		fmt.Fprintln(w, "  Services:")
		for _, service := range observabilityComposeServices() {
			fmt.Fprintf(w, "    %s: %s\n", service, services[service])
		}
	}
}

func observabilityRuntimeInitialized(runtime observabilityRuntime) bool {
	if runtime.Context == "host" {
		return true
	}
	return fileExists(observabilityRuntimeStatePath(runtime.StateDir))
}

func printTaxiwayRuntimeStatus(w interface{ Write([]byte) (int, error) }, state *RootState, docker dockerRuntimeStatus) {
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()
	runtimeDir := state.RepoDir
	if runtimeDir == "" {
		runtimeDir = config.RuntimeDir("", Version)
	}
	labStateDir := config.StateDir(state.Flags.StateDir, runtimeDir)
	driverName, err := selectDriverName(state.Flags.DriverName)
	if err != nil {
		driverName = "unavailable"
	}

	fmt.Fprintln(w, "Runtime:")
	context := runtime.Context
	if runtime.ContextID != "" {
		context = fmt.Sprintf("%s (%s)", runtime.Context, runtime.ContextID)
	}
	fmt.Fprintf(w, "  context: %s\n", context)
	fmt.Fprintf(w, "  driver: %s\n", driverName)
	if docker.Available {
		fmt.Fprintln(w, "  docker: available")
	} else {
		fmt.Fprintf(w, "  docker: unavailable (%s)\n", docker.Reason)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  dirs:")
	fmt.Fprintf(w, "    runtime: %s\n", runtimeDir)
	fmt.Fprintf(w, "    labs: %s\n", labStateDir)
	fmt.Fprintf(w, "    auth: %s\n", authStateDir(state))
	fmt.Fprintf(w, "    proxy: %s\n", proxy.StateDir)
	fmt.Fprintf(w, "    observability: %s\n", runtime.StateDir)
}

func printStatusLabs(w interface{ Write([]byte) (int, error) }, state *RootState, docker dockerRuntimeStatus) error {
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	routes, err := collectLabProxyRoutes(stateDir)
	if err != nil {
		return err
	}
	rows, err := collectLabRuntimeRows(context.Background(), state, stateDir, routes, state.proxyRuntime(), docker)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "Labs:")
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no labs found)")
		return nil
	}
	printStatusLabDetails(w, rows)
	return nil
}

func printStatusLabDetails(w interface{ Write([]byte) (int, error) }, rows []labRuntimeRow) {
	for _, row := range rows {
		fmt.Fprintf(w, "  %s:\n", row.lab)
		fmt.Fprintf(w, "    Status: %s\n", row.status)
		fmt.Fprintf(w, "    Phase: %s\n", row.phase)
		fmt.Fprintf(w, "    State dir: %s\n", row.stateDir)
		fmt.Fprintln(w, "    Gateway:")
		fmt.Fprintf(w, "      Status: %s\n", row.gateway)
		fmt.Fprintf(w, "      State dir: %s\n", row.gatewayDir)
		if row.project != "" {
			fmt.Fprintf(w, "      Compose project: %s\n", row.project)
		}
		fmt.Fprintf(w, "      URL: %s\n", row.access)
		if len(row.services) > 0 {
			fmt.Fprintln(w, "      Services:")
			for _, service := range row.services {
				fmt.Fprintf(w, "        %s: %s\n", service.Name, service.State)
			}
		}
	}
}

func summarizeLangfuseStatus(credentialsPresent, runtimePresent bool, docker dockerRuntimeStatus, services map[string]string) (string, string) {
	if !credentialsPresent {
		return "not initialized", ".env not found; run `taxiway observe up`"
	}
	if !runtimePresent {
		return "removed", "stack removed; credentials preserved; run `taxiway observe up`"
	}
	if !docker.Available {
		return "docker unavailable", docker.Reason
	}

	present := 0
	running := 0
	for _, service := range observabilityComposeServices() {
		switch services[service] {
		case "running":
			present++
			running++
		case "missing":
		default:
			present++
		}
	}
	if present == 0 {
		return "stopped", "no Langfuse containers found"
	}
	if running == len(observabilityComposeServices()) {
		return "running", ""
	}
	if running > 0 {
		return "partial", "only some Langfuse services are running"
	}
	return "stopped", "Langfuse containers exist but are not running"
}

func langfuseServiceStates(runtime observabilityRuntime, docker dockerRuntimeStatus) map[string]string {
	states := map[string]string{}
	for _, service := range observabilityComposeServices() {
		states[service] = "unknown"
		if !docker.Available {
			continue
		}
		states[service] = containerState(runtime.ComposeProject + "-" + service + "-1")
	}
	return states
}

func printProxyRuntimeStatus(w interface{ Write([]byte) (int, error) }, state *RootState, docker dockerRuntimeStatus) {
	runtime := state.proxyRuntime()
	status := "docker unavailable"
	reason := docker.Reason
	if docker.Available {
		switch containerState(runtime.Container) {
		case "running":
			status = "running"
			reason = ""
		case "missing":
			if proxyGeneratedStateExists(runtime.StateDir) {
				status = "removed"
				reason = "proxy removed; generated config preserved; it will restart when a lab gateway or Langfuse starts"
			} else if !proxyRuntimePortAvailable(runtime) {
				status = "not initialized"
				reason = "proxy has not been started"
			} else {
				status = "stopped"
				reason = "proxy container is not running"
			}
		default:
			status = "stopped"
			reason = "proxy container exists but is not running"
		}
	}

	fmt.Fprintln(w, "Proxy:")
	fmt.Fprintf(w, "  Status: %s\n", status)
	fmt.Fprintf(w, "  Container: %s\n", runtime.Container)
	url := "unavailable (not started)"
	if status == "running" {
		url = localRuntimeURL(runtime.Port)
	}
	fmt.Fprintf(w, "  URL: %s\n", url)
	if reason != "" {
		fmt.Fprintf(w, "  Reason: %s\n", reason)
	}
}

func proxyRuntimePortAvailable(runtime proxyRuntime) bool {
	return runtime.Port > 0
}

func containerState(name string) string {
	out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", name).CombinedOutput()
	if err != nil {
		return "missing"
	}
	state := strings.TrimSpace(string(out))
	if state == "" {
		return "unknown"
	}
	if state == "running" {
		return "running"
	}
	return "stopped"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sidecarDockerState(project string, docker dockerRuntimeStatus) string {
	if !docker.Available {
		return "unavailable"
	}
	containers, err := composeProjectContainerStates(project)
	if err != nil || len(containers) == 0 {
		return "stopped"
	}
	running := 0
	for _, state := range containers {
		if state == "running" {
			running++
		}
	}
	if running == len(containers) {
		return "running"
	}
	if running > 0 {
		return "partial"
	}
	return "stopped"
}

func labGatewayServices() []string {
	return []string{"litellm", "postgres"}
}

func labGatewayServiceStates(project string, docker dockerRuntimeStatus) []proxyStatusService {
	services := labGatewayServices()
	states := make([]proxyStatusService, 0, len(services))
	if !docker.Available {
		for _, service := range services {
			states = append(states, proxyStatusService{Name: service, State: "unavailable"})
		}
		return states
	}
	byService, err := composeProjectServiceStates(project)
	if err != nil {
		byService = map[string]string{}
	}
	for _, service := range services {
		state := byService[service]
		if state == "" {
			state = "missing"
		}
		states = append(states, proxyStatusService{Name: service, State: state})
	}
	return states
}

func composeProjectContainerStates(project string) ([]string, error) {
	out, err := exec.Command("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}\t{{.State}}\t{{.Status}}").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var states []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		state := strings.TrimSpace(parts[1])
		if state == "" {
			state = "unknown"
		}
		if state != "running" {
			state = "stopped"
		}
		states = append(states, state)
	}
	return states, nil
}

func composeProjectServiceStates(project string) (map[string]string, error) {
	out, err := exec.Command("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}\t{{.State}}\t{{.Label \"com.docker.compose.service\"}}").CombinedOutput()
	if err != nil {
		return nil, err
	}
	states := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		state := normalizeDockerState(strings.TrimSpace(parts[1]))
		service := ""
		if len(parts) >= 3 {
			service = strings.TrimSpace(parts[2])
		}
		if service == "" {
			service = inferGatewayServiceFromContainerName(name)
		}
		if service != "" {
			states[service] = state
		}
	}
	return states, nil
}

func normalizeDockerState(state string) string {
	if state == "" {
		return "unknown"
	}
	if state != "running" {
		return "stopped"
	}
	return state
}

func inferGatewayServiceFromContainerName(name string) string {
	for _, service := range labGatewayServices() {
		if strings.Contains(name, service) {
			return service
		}
	}
	return ""
}

// ── taxiway observe reset ─────────────────────────────────────────────────────────

func newObserveResetCmd(state *RootState) *cobra.Command {
	var rotateSecrets bool
	cmd := &cobra.Command{
		Use:         "reset",
		Short:       "Wipe all traces and restart the stack",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			runtime, err := state.ensureObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.ensureProxyRuntime()
			if err != nil {
				return err
			}
			envPath := observabilityEnvPath(state)

			if err := removeObservabilityProxyRoute(state, runtime, proxy); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if err := observabilityComposeCmd(state, observabilityStateDir(state), envPath, observabilityComposeFiles(state), "down", "-v").Run(); err != nil {
				return fmt.Errorf("docker compose down -v: %w", err)
			}
			if err := removeObservabilityRuntimeState(runtime); err != nil {
				return err
			}
			state.Observability = observabilityRuntime{
				Context:        runtime.Context,
				ContextID:      runtime.ContextID,
				StateDir:       runtime.StateDir,
				ComposeProject: runtime.ComposeProject,
			}
			runtime, err = state.ensureObservabilityRuntime()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nWarning: All traces wiped.\n\n")

			if rotateSecrets {
				if err := os.Remove(envPath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("removing observability credentials: %w", err)
				}
			}

			_, err = initializeTaxiwayRuntime(state, cmd.OutOrStdout(), true)
			return err
		},
	}
	cmd.Flags().BoolVar(&rotateSecrets, "rotate-secrets", false, "regenerate observability credentials during reset")
	return cmd
}

// ── taxiway observe open ──────────────────────────────────────────────────────────

func newObserveOpenCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "open",
		Short:       "Open Langfuse UI in the default browser",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			obsRuntime, err := state.resolveObservabilityRuntime()
			if err != nil {
				return err
			}
			proxy, err := state.resolveProxyRuntime()
			if err != nil {
				return err
			}
			state.Observability = obsRuntime
			state.Proxy = proxy
			if !fileExists(observabilityEnvPath(state)) || !observabilityRuntimeInitialized(obsRuntime) || proxy.Port == 0 {
				return fmt.Errorf("Langfuse URL is unavailable; run `taxiway observe up` first")
			}
			url := proxy.LangfuseBaseURL()
			opener, args := browserOpenCommand(url)
			if _, err := exec.LookPath(opener); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Open manually: %s\n", url)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Opening %s\n", url)
			return exec.Command(opener, args...).Run()
		},
	}
}
