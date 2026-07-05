package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/envfile"
)

const (
	labLiteLLMAPIKeyEnv            = "TAXIWAY_LITELLM_API_KEY"
	labLiteLLMBaseURLEnv           = "TAXIWAY_LITELLM_BASE_URL"
	labLangfuseProjectIDEnv        = "LANGFUSE_PROJECT_ID"
	labLangfuseProjectNameEnv      = "LANGFUSE_PROJECT_NAME"
	labLangfuseProjectPublicKeyEnv = "LANGFUSE_PUBLIC_KEY"
	labLangfuseProjectSecretKeyEnv = "LANGFUSE_SECRET_KEY"
)

type labLiteLLMSidecarFiles struct {
	ComposePath string
	Project     string
	Service     string
	DBService   string
}

type labLiteLLMRoute struct {
	Lab     string
	Service string
	Host    string
	Project string
}

type labLiteLLMSidecarStatus struct {
	Lab         string
	Project     string
	ComposePath string
}

var (
	ensureLabLiteLLMSidecarForUp = ensureLabLiteLLMSidecar
	stopLabLiteLLMSidecarForDown = stopLabLiteLLMSidecar
)

func labGatewayEnvPath(stateDir string, ref config.LabRef) string {
	return filepath.Join(stateDir, config.LabDirOf(idName(ref.Lab)), "gateway", "env")
}

func labGatewayDir(stateDir string, ref config.LabRef) string {
	return filepath.Join(stateDir, config.LabDirOf(idName(ref.Lab)), "gateway")
}

func labLiteLLMBaseURL(state *RootState, ref config.LabRef) string {
	return fmt.Sprintf("http://%s:%d", labLiteLLMHost(ref.Lab), state.proxyRuntime().Port)
}

func ensureLabGatewayEnv(state *RootState, ref config.LabRef) error {
	proxy, err := state.ensureProxyRuntime()
	if err != nil {
		return err
	}
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	values, err := readLabGatewayEnv(stateDir, ref)
	if err != nil {
		return err
	}

	changed := false
	if values[labLiteLLMAPIKeyEnv] == "" {
		key, err := generateSecret(24)
		if err != nil {
			return fmt.Errorf("generate lab LiteLLM key: %w", err)
		}
		values[labLiteLLMAPIKeyEnv] = "sk-taxiway-" + key
		changed = true
	}

	baseURL := fmt.Sprintf("http://%s:%d", labLiteLLMHost(ref.Lab), proxy.Port)
	if values[labLiteLLMBaseURLEnv] != baseURL {
		values[labLiteLLMBaseURLEnv] = baseURL
		changed = true
	}

	if !changed {
		return nil
	}
	return writeLabGatewayEnv(stateDir, ref, values)
}

func ensureLabLiteLLMSidecar(ctx context.Context, state *RootState, ref config.LabRef) error {
	_ = ctx
	if state.Driver.Name() == "mock" {
		return nil
	}
	if _, err := state.ensureProxyRuntime(); err != nil {
		return err
	}
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	files, err := prepareLabLiteLLMSidecarFiles(state, stateDir, authStateDir(state), ref)
	if err != nil {
		return err
	}
	proxyDir := state.proxyRuntime().StateDir
	if _, err := ensureProxyConfigForState(state, stateDir, proxyDir); err != nil {
		return err
	}
	if _, _, err := ensureProxyRunning(state, proxyDir); err != nil {
		return err
	}
	envPath := ""
	if err := composeProjectCmd(files.Project, filepath.Dir(files.ComposePath), envPath, []string{files.ComposePath}, "up", "-d", files.DBService, files.Service).Run(); err != nil {
		return fmt.Errorf("start lab LiteLLM sidecar: %w", err)
	}
	if err := connectProxyToNetwork(state, files.Project+"_default"); err != nil {
		return err
	}
	if err := waitForLabLiteLLMSidecarReady(ctx, files.Project, files.Service); err != nil {
		return err
	}
	if _, err := ensureProxyConfigForState(state, stateDir, proxyDir); err != nil {
		return err
	}
	if err := reloadProxy(state); err != nil {
		return err
	}
	return nil
}

func waitForLabLiteLLMSidecarReady(ctx context.Context, project, service string) error {
	return waitForLabLiteLLMSidecarReadyWithTimeout(ctx, project, service, 5*time.Minute, 2*time.Second)
}

func waitForLabLiteLLMSidecarReadyWithTimeout(ctx context.Context, project, service string, timeout, pollInterval time.Duration) error {
	container := project + "-" + service + "-1"
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, "docker", "exec", container, "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:4000/health/liveliness', timeout=2).read()")
		_, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return labLiteLLMSidecarWaitError(container, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func labLiteLLMSidecarWaitError(container string, lastErr error) error {
	return fmt.Errorf("wait for lab LiteLLM sidecar %s: %v", container, lastErr)
}

func removeLabLiteLLMSidecar(ctx context.Context, state *RootState, ref config.LabRef) error {
	_ = ctx
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	labDir := labGatewayDir(stateDir, ref)
	composePath := filepath.Join(labDir, "litellm.compose.yml")

	if _, err := os.Stat(labDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat lab LiteLLM sidecar state: %w", err)
	}

	if state.Driver.Name() != "mock" {
		runtime := state.proxyRuntime()
		_ = disconnectProxyFromNetwork(state, labLiteLLMComposeProject(runtime.Context, runtime.ContextID, ref.Lab)+"_default")
		if _, err := os.Stat(composePath); err == nil {
			if err := labLiteLLMSidecarDownCmd(state, labDir, composePath, ref, true).Run(); err != nil {
				return fmt.Errorf("stop lab LiteLLM sidecar: %w", err)
			}
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat lab LiteLLM sidecar compose: %w", err)
		}
	}

	if err := os.RemoveAll(labDir); err != nil {
		return fmt.Errorf("remove lab LiteLLM sidecar state: %w", err)
	}
	proxyDir := state.proxyRuntime().StateDir
	if _, err := ensureProxyConfigForState(state, stateDir, proxyDir); err != nil {
		return err
	}
	if state.Driver.Name() != "mock" {
		if action, err := reconcileProxyLifecycle(state, proxyDir); err != nil {
			return err
		} else if action == "running" {
			if err := reloadProxy(state); err != nil {
				return err
			}
		}
	}
	return nil
}

func stopLabLiteLLMSidecar(ctx context.Context, state *RootState, ref config.LabRef) error {
	_ = ctx
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	labDir := labGatewayDir(stateDir, ref)
	composePath := filepath.Join(labDir, "litellm.compose.yml")

	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat lab LiteLLM sidecar compose: %w", err)
	}
	if state.Driver.Name() == "mock" || state.Flags.DryRun {
		return nil
	}
	if _, err := state.ensureProxyRuntime(); err != nil {
		return err
	}
	if err := labLiteLLMSidecarStopCmd(state, labDir, composePath, ref).Run(); err != nil {
		return fmt.Errorf("stop lab LiteLLM sidecar: %w", err)
	}
	if err := reconcileProxyAfterLabSidecarStop(state); err != nil {
		return err
	}
	return nil
}

func reconcileProxyAfterLabSidecarStop(state *RootState) error {
	proxyDir := state.proxyRuntime().StateDir
	if !proxyGeneratedStateExists(proxyDir) {
		return nil
	}
	_, err := reconcileProxyLifecycle(state, proxyDir)
	return err
}

func labLiteLLMSidecarStopCmd(state *RootState, labDir, composePath string, ref config.LabRef) *exec.Cmd {
	runtime := state.proxyRuntime()
	project := labLiteLLMComposeProject(runtime.Context, runtime.ContextID, ref.Lab)
	return composeProjectCmd(project, labDir, "", []string{composePath}, "stop")
}

func labLiteLLMSidecarDownCmd(state *RootState, labDir, composePath string, ref config.LabRef, removeVolumes bool) *exec.Cmd {
	args := []string{"down"}
	if removeVolumes {
		args = append(args, "-v")
	}
	runtime := state.proxyRuntime()
	project := labLiteLLMComposeProject(runtime.Context, runtime.ContextID, ref.Lab)
	return composeProjectCmd(project, labDir, "", []string{composePath}, args...)
}

func prepareLabLiteLLMSidecarFiles(state *RootState, labStateDir, authDir string, ref config.LabRef) (labLiteLLMSidecarFiles, error) {
	values, err := readLabGatewayEnv(labStateDir, ref)
	if err != nil {
		return labLiteLLMSidecarFiles{}, err
	}
	liteLLMKey := values[labLiteLLMAPIKeyEnv]
	if liteLLMKey == "" {
		return labLiteLLMSidecarFiles{}, fmt.Errorf("lab LiteLLM key missing for %s", ref.Lab)
	}
	langfusePublicKey := values[labLangfuseProjectPublicKeyEnv]
	langfuseSecretKey := values[labLangfuseProjectSecretKeyEnv]
	langfuseEnabled := langfusePublicKey != "" && langfuseSecretKey != ""

	runtime := state.proxyRuntime()
	project := labLiteLLMComposeProject(runtime.Context, runtime.ContextID, ref.Lab)
	service := "litellm"
	dbService := "postgres"
	routeService := labGatewayDNSAlias(runtime, ref.Lab, service)
	dbAlias := labGatewayDNSAlias(runtime, ref.Lab, dbService)
	labDir := labGatewayDir(labStateDir, ref)
	composePath := filepath.Join(labDir, "litellm.compose.yml")
	if err := os.MkdirAll(labDir, 0o700); err != nil {
		return labLiteLLMSidecarFiles{}, fmt.Errorf("create lab LiteLLM sidecar dir: %w", err)
	}
	includeCodexModels := false
	if _, err := os.Stat(liteLLMChatGPTAuthStatePath(authDir)); err == nil {
		includeCodexModels = true
	} else if err != nil && !os.IsNotExist(err) {
		return labLiteLLMSidecarFiles{}, fmt.Errorf("stat LiteLLM ChatGPT auth: %w", err)
	}
	enableCodexSessionMapper, err := labUsesAgent(state, ref, "codex")
	if err != nil {
		return labLiteLLMSidecarFiles{}, err
	}
	models, err := labLiteLLMModelNames(state, ref)
	if err != nil {
		return labLiteLLMSidecarFiles{}, err
	}
	proxy, err := state.resolveProxyRuntime()
	if err != nil {
		return labLiteLLMSidecarFiles{}, err
	}
	if _, err := ensureLiteLLMConfig(state, labDir, includeCodexModels, enableCodexSessionMapper, models); err != nil {
		return labLiteLLMSidecarFiles{}, err
	}

	langfuseEnv := ""
	if langfuseEnabled {
		langfuseEnv = fmt.Sprintf(`      LANGFUSE_PUBLIC_KEY: %s
      LANGFUSE_SECRET_KEY: %s
      LANGFUSE_OTEL_HOST: %s
`, langfusePublicKey, langfuseSecretKey, proxy.InternalLangfuseBaseURL())
	}

	compose := fmt.Sprintf(`services:
  %[1]s:
    image: postgres:17
    restart: unless-stopped
    environment:
      POSTGRES_USER: litellm
      POSTGRES_PASSWORD: litellm
      POSTGRES_DB: litellm
    volumes:
      - %[1]s_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U litellm"]
      interval: 5s
      timeout: 3s
      retries: 10
    networks:
      default:
        aliases:
          - %[8]s

  %[2]s:
    image: litellm/litellm:1.88.1
    extra_hosts:
      - host.docker.internal:host-gateway
    depends_on:
      %[1]s:
        condition: service_healthy
    volumes:
      - ./litellm_config.yaml:/app/config.yaml:ro
      - %[5]s:/app/codex_session_mapper.py:ro
      - %[6]s:/app/chatgpt_token
    command: ["--config", "/app/config.yaml", "--port", "4000"]
    environment:
      LITELLM_MASTER_KEY: %[3]s
      DATABASE_URL: postgresql://litellm:litellm@%[8]s:5432/litellm
%[7]s
      CHATGPT_AUTH_FILE: /app/chatgpt_token/auth.json
    networks:
      default:
        aliases:
          - %[9]s

volumes:
  %[1]s_data:

networks:
  default:
    name: %[4]s
`, dbService, service, liteLLMKey, project+"_default", liteLLMCodexSessionMapperAssetPath(state), liteLLMChatGPTTokenStateDir(authDir), langfuseEnv, dbAlias, routeService)

	if err := os.WriteFile(composePath, []byte(compose), 0o600); err != nil {
		return labLiteLLMSidecarFiles{}, fmt.Errorf("write lab LiteLLM compose: %w", err)
	}
	if err := writeLabLiteLLMRoute(labStateDir, ref, labLiteLLMRoute{
		Lab:     ref.Lab,
		Service: routeService,
		Project: project,
	}); err != nil {
		return labLiteLLMSidecarFiles{}, err
	}
	return labLiteLLMSidecarFiles{ComposePath: composePath, Project: project, Service: service, DBService: dbService}, nil
}

func labLiteLLMModelNames(state *RootState, ref config.LabRef) ([]string, error) {
	var models []string
	seen := map[string]bool{}
	add := func(model string) {
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		models = append(models, model)
	}

	orchManifest, err := config.LoadOrchManifest(state.RepoDir, ref.Orch)
	if err != nil {
		return nil, err
	}
	addManifestModel := func(manifest *config.OrchManifest) {
		if manifest == nil {
			return
		}
		for _, setting := range manifest.Settings {
			if setting.Name != "model" {
				continue
			}
			if model := ref.Settings["model"]; model != "" {
				add(model)
			} else {
				add(setting.Default)
			}
			return
		}
	}
	addManifestModel(orchManifest)

	if orchManifest != nil {
		for _, agent := range orchManifest.Agents {
			agentManifest, err := config.LoadOrchManifest(state.RepoDir, agent)
			if err != nil {
				return nil, err
			}
			addManifestModel(agentManifest)
		}
	}
	return models, nil
}

func labUsesAgent(state *RootState, ref config.LabRef, agentName string) (bool, error) {
	manifest, err := config.LoadOrchManifest(state.RepoDir, ref.Orch)
	if err != nil {
		return false, err
	}
	agents := manifestAgents(manifest)
	if len(agents) == 0 {
		agents = []string{ref.Orch}
	}
	for _, agent := range agents {
		if agent == agentName {
			return true, nil
		}
	}
	return false, nil
}

func readLabGatewayEnv(stateDir string, ref config.LabRef) (map[string]string, error) {
	path := labGatewayEnvPath(stateDir, ref)
	values, err := envfile.Load(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lab gateway env: %w", err)
	}
	return values, nil
}

func writeLabGatewayEnv(stateDir string, ref config.LabRef, values map[string]string) error {
	path := labGatewayEnvPath(stateDir, ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create lab gateway state: %w", err)
	}

	var out strings.Builder
	for _, key := range envfile.SortedKeys(values) {
		if values[key] == "" {
			continue
		}
		fmt.Fprintf(&out, "%s=%s\n", key, values[key])
	}
	if err := os.WriteFile(path, []byte(out.String()), 0o600); err != nil {
		return fmt.Errorf("write lab gateway env: %w", err)
	}
	return nil
}

func labLiteLLMRoutePath(stateDir string, ref config.LabRef) string {
	return filepath.Join(labGatewayDir(stateDir, ref), "route.env")
}

func writeLabLiteLLMRoute(stateDir string, ref config.LabRef, route labLiteLLMRoute) error {
	path := labLiteLLMRoutePath(stateDir, ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create lab LiteLLM route dir: %w", err)
	}
	if route.Host == "" {
		route.Host = labLiteLLMHost(route.Lab)
	}
	content := fmt.Sprintf("LAB=%s\nSERVICE=%s\nHOST=%s\nPROJECT=%s\n", route.Lab, route.Service, route.Host, route.Project)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write lab LiteLLM route: %w", err)
	}
	return nil
}

func readLabLiteLLMRoute(stateDir string, ref config.LabRef) (labLiteLLMRoute, error) {
	values, err := envfile.Load(labLiteLLMRoutePath(stateDir, ref))
	if err != nil {
		return labLiteLLMRoute{}, err
	}
	return labLiteLLMRoute{Lab: values["LAB"], Service: values["SERVICE"], Host: values["HOST"], Project: values["PROJECT"]}, nil
}

func readLabLiteLLMRoutes(stateDir string) ([]labLiteLLMRoute, error) {
	entries, err := os.ReadDir(stateDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lab LiteLLM routes: %w", err)
	}
	var routes []labLiteLLMRoute
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		values, err := envfile.Load(filepath.Join(stateDir, entry.Name(), "gateway", "route.env"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if values["LAB"] == "" || values["SERVICE"] == "" {
			continue
		}
		routes = append(routes, labLiteLLMRoute{Lab: values["LAB"], Service: values["SERVICE"], Host: values["HOST"], Project: values["PROJECT"]})
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Lab < routes[j].Lab
	})
	return routes, nil
}

func readLabLiteLLMSidecarStatuses(stateDir string) ([]labLiteLLMSidecarStatus, error) {
	entries, err := os.ReadDir(stateDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lab LiteLLM sidecars: %w", err)
	}

	var statuses []labLiteLLMSidecarStatus
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		labGatewayDir := filepath.Join(stateDir, entry.Name(), "gateway")
		composePath := filepath.Join(labGatewayDir, "litellm.compose.yml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("stat lab LiteLLM compose: %w", err)
		}

		lab := entry.Name()
		project := ""
		values, err := envfile.Load(filepath.Join(labGatewayDir, "route.env"))
		if err == nil && values["LAB"] != "" {
			lab = values["LAB"]
			project = values["PROJECT"]
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if project == "" {
			project = labLiteLLMComposeProject("host", "", lab)
		}

		statuses = append(statuses, labLiteLLMSidecarStatus{
			Lab:         lab,
			Project:     project,
			ComposePath: composePath,
		})
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Lab < statuses[j].Lab
	})
	return statuses, nil
}

func labLiteLLMSlug(lab string) string {
	lower := strings.ToLower(lab)
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	slug := strings.Trim(re.ReplaceAllString(lower, "-"), "-")
	if slug == "" {
		return "lab"
	}
	return slug
}

func labLiteLLMHost(lab string) string {
	return config.LabLiteLLMHost(lab)
}

func labLiteLLMComposeProject(context, contextID, lab string) string {
	slug := labLiteLLMSlug(lab)
	if context == "" || context == "host" {
		return "taxiway-" + slug + "-gateway"
	}
	contextPrefix := context + "-" + contextID + "-"
	slug = strings.TrimPrefix(slug, contextPrefix)
	return fmt.Sprintf("taxiway-%s-%s-%s-gateway", context, contextID, slug)
}
