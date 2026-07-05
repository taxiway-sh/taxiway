package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
)

const (
	defaultProxyComposeProjectName = "taxiway-proxy"
	defaultProxyContainerName      = "taxiway-proxy"
	proxyImage                     = "caddy:2.8-alpine"
	langfuseProxyHost              = "langfuse.localhost"
)

type proxyRuntime struct {
	Context        string
	ContextID      string
	StateDir       string
	ComposeProject string
	Container      string
	Port           int
}

type proxyRuntimeState struct {
	Port int `json:"port"`
}

type proxyRoute struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Host       string `json:"host,omitempty"`
	PathPrefix string `json:"path_prefix,omitempty"`
	Upstream   string `json:"upstream"`
	Network    string `json:"network,omitempty"`
}

type proxyRouteRegistry struct {
	Routes []proxyRoute `json:"routes"`
}

func (state *RootState) resolveProxyRuntime() (proxyRuntime, error) {
	runtime := state.Proxy
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
			runtime.StateDir = config.ProxyDir()
		case "dev", "e2e":
			runtime.StateDir = strings.TrimSpace(os.Getenv("TAXIWAY_PROXY_DIR"))
			if runtime.StateDir == "" {
				runtimeRoot := strings.TrimSpace(os.Getenv("TAXIWAY_RUNTIME_DIR"))
				if runtimeRoot == "" {
					runtimeRoot = state.RepoDir
				}
				if runtimeRoot == "" {
					return proxyRuntime{}, fmt.Errorf("TAXIWAY_PROXY_DIR or TAXIWAY_RUNTIME_DIR is required when TAXIWAY_CONTEXT=%s", runtime.Context)
				}
				runtime.StateDir = filepath.Join(runtimeRoot, ".proxy")
			}
		default:
			return proxyRuntime{}, fmt.Errorf("unsupported TAXIWAY_CONTEXT %q", runtime.Context)
		}
	}
	if (runtime.Context == "dev" || runtime.Context == "e2e") && runtime.ContextID == "" {
		return proxyRuntime{}, fmt.Errorf("TAXIWAY_CONTEXT_ID is required when TAXIWAY_CONTEXT=%s", runtime.Context)
	}
	if runtime.ContextID != "" && !isValidTaxiwayContextID(runtime.ContextID) {
		return proxyRuntime{}, fmt.Errorf("TAXIWAY_CONTEXT_ID must start with a lowercase letter or number and contain only lowercase letters, numbers, dashes, and underscores")
	}

	if runtime.ComposeProject == "" {
		if runtime.Context == "host" {
			runtime.ComposeProject = defaultProxyComposeProjectName
		} else {
			runtime.ComposeProject = fmt.Sprintf("taxiway-%s-%s-proxy", runtime.Context, runtime.ContextID)
		}
	}
	if runtime.Container == "" {
		if runtime.Context == "host" {
			runtime.Container = defaultProxyContainerName
		} else {
			runtime.Container = fmt.Sprintf("taxiway-%s-%s-proxy", runtime.Context, runtime.ContextID)
		}
	}
	if runtime.Context == "dev" || runtime.Context == "e2e" {
		persisted, ok, err := readDevProxyRuntimeState(runtime.StateDir)
		if err != nil {
			return proxyRuntime{}, err
		}
		if ok && runtime.Port == 0 {
			runtime.Port = persisted.Port
		}
	} else if runtime.Port == 0 {
		runtime.Port = 4000
	}
	return runtime, nil
}

func readDevProxyRuntimeState(stateDir string) (proxyRuntimeState, bool, error) {
	path := proxyRuntimeStatePath(stateDir)
	var state proxyRuntimeState
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, false, nil
	}
	if err != nil {
		return state, false, fmt.Errorf("read proxy runtime state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, false, fmt.Errorf("read proxy runtime state: %w", err)
	}
	return state, true, nil
}

func ensureDevProxyRuntimeState(runtime *proxyRuntime) error {
	state, _, err := readDevProxyRuntimeState(runtime.StateDir)
	if err != nil {
		return err
	}
	if runtime.Port == 0 {
		runtime.Port = state.Port
	}
	// Choose a free host port before launching so the proxy can be published on
	// a pinned <port>:4000 mapping. The port is persisted only after the
	// container actually starts (refreshProxyPublishedPort), so a launch that
	// loses the port race leaves no stale state and the next attempt re-picks.
	if runtime.Port == 0 {
		port, err := allocateLocalPort()
		if err != nil {
			return err
		}
		runtime.Port = port
	}
	return nil
}

func writeDevProxyRuntimeState(runtime proxyRuntime) error {
	if runtime.Context != "dev" && runtime.Context != "e2e" {
		return nil
	}
	if runtime.Port <= 0 {
		return fmt.Errorf("proxy published port is not available")
	}
	path := proxyRuntimeStatePath(runtime.StateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create proxy runtime state dir: %w", err)
	}
	data, err := json.MarshalIndent(proxyRuntimeState{Port: runtime.Port}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write proxy runtime state: %w", err)
	}
	return nil
}

func proxyRuntimeStatePath(stateDir string) string {
	return filepath.Join(stateDir, "runtime.json")
}

func (state *RootState) proxyRuntime() proxyRuntime {
	runtime, err := state.resolveProxyRuntime()
	if err != nil {
		return state.Proxy
	}
	return runtime
}

func (state *RootState) ensureProxyRuntime() (proxyRuntime, error) {
	runtime, err := state.resolveProxyRuntime()
	if err != nil {
		return proxyRuntime{}, err
	}
	if runtime.Context == "dev" || runtime.Context == "e2e" {
		if err := ensureDevProxyRuntimeState(&runtime); err != nil {
			return proxyRuntime{}, err
		}
	} else if runtime.Port == 0 {
		runtime.Port = 4000
	}
	state.Proxy = runtime
	return runtime, nil
}

func (runtime proxyRuntime) DockerNetwork() string {
	return runtime.ComposeProject + "_default"
}

func (runtime proxyRuntime) BaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", runtime.Port)
}

func (runtime proxyRuntime) LangfuseBaseURL() string {
	if runtime.Port == 0 {
		return "unavailable (not started)"
	}
	return fmt.Sprintf("http://%s:%d", langfuseProxyHost, runtime.Port)
}

func (runtime proxyRuntime) InternalLangfuseBaseURL() string {
	return fmt.Sprintf("http://%s:4000/_taxiway/langfuse", proxyDNSAlias(runtime))
}

func proxyConfigStatePath(stateDir string) string {
	return filepath.Join(stateDir, "Caddyfile")
}

func proxyRoutesStatePath(stateDir string) string {
	return filepath.Join(stateDir, "routes.json")
}

func proxyGeneratedStateExists(proxyDir string) bool {
	return fileExists(proxyRuntimeStatePath(proxyDir)) ||
		fileExists(proxyRoutesStatePath(proxyDir)) ||
		fileExists(proxyConfigStatePath(proxyDir))
}

func ensureProxyConfig(labStateDir, proxyDir string) (bool, error) {
	routes, err := readLabLiteLLMRoutes(labStateDir)
	if err != nil {
		return false, err
	}
	return ensureProxyConfigWithLabRoutes(proxyDir, routes)
}

func ensureProxyConfigForState(state *RootState, labStateDir, proxyDir string) (bool, error) {
	labRoutes, err := readLabLiteLLMRoutes(labStateDir)
	if err != nil {
		return false, err
	}
	existing, err := readProxyRoutes(proxyDir)
	if err != nil {
		return false, err
	}
	routes := mergeProxyRoutes(existing, labRoutes)
	routesChanged, err := writeProxyRoutes(proxyDir, routes)
	if err != nil {
		return false, err
	}
	configChanged, err := writeProxyConfig(proxyDir, renderProxyConfig(routes))
	if err != nil {
		return false, err
	}
	return routesChanged || configChanged, nil
}

func ensureProxyConfigWithLabRoutes(proxyDir string, labRoutes []labLiteLLMRoute) (bool, error) {
	existing, err := readProxyRoutes(proxyDir)
	if err != nil {
		return false, err
	}
	routes := mergeProxyRoutes(existing, labRoutes)

	routesChanged, err := writeProxyRoutes(proxyDir, routes)
	if err != nil {
		return false, err
	}

	configChanged, err := writeProxyConfig(proxyDir, renderProxyConfig(routes))
	if err != nil {
		return false, err
	}
	return routesChanged || configChanged, nil
}

func mergeProxyRoutes(existing []proxyRoute, labRoutes []labLiteLLMRoute) []proxyRoute {
	routes := make([]proxyRoute, 0, len(existing)+len(labRoutes))
	for _, route := range existing {
		if route.Kind != "lab" {
			routes = append(routes, route)
		}
	}
	for _, route := range labRoutes {
		routes = append(routes, proxyRouteFromLabLiteLLMRoute(route))
	}
	sortProxyRoutes(routes)
	return routes
}

func upsertProxyRoute(proxyDir string, route proxyRoute) (bool, error) {
	routes, err := readProxyRoutes(proxyDir)
	if err != nil {
		return false, err
	}
	replaced := false
	for i := range routes {
		if routes[i].ID == route.ID {
			routes[i] = route
			replaced = true
			break
		}
	}
	if !replaced {
		routes = append(routes, route)
	}
	sortProxyRoutes(routes)
	routesChanged, err := writeProxyRoutes(proxyDir, routes)
	if err != nil {
		return false, err
	}
	configChanged, err := writeProxyConfig(proxyDir, renderProxyConfig(routes))
	if err != nil {
		return false, err
	}
	return routesChanged || configChanged, nil
}

func removeProxyRoute(proxyDir, id string) (bool, error) {
	routes, err := readProxyRoutes(proxyDir)
	if err != nil {
		return false, err
	}
	next := routes[:0]
	for _, route := range routes {
		if route.ID != id {
			next = append(next, route)
		}
	}
	sortProxyRoutes(next)
	routesChanged, err := writeProxyRoutes(proxyDir, next)
	if err != nil {
		return false, err
	}
	configChanged, err := writeProxyConfig(proxyDir, renderProxyConfig(next))
	if err != nil {
		return false, err
	}
	return routesChanged || configChanged, nil
}

func proxyRouteFromLabLiteLLMRoute(route labLiteLLMRoute) proxyRoute {
	host := route.Host
	if host == "" {
		host = labLiteLLMHost(route.Lab)
	}
	project := route.Project
	network := ""
	if project != "" {
		network = project + "_default"
	}
	return proxyRoute{
		ID:       "lab:" + route.Lab,
		Kind:     "lab",
		Host:     host,
		Upstream: route.Service + ":4000",
		Network:  network,
	}
}

func observabilityProxyRoute(runtime observabilityRuntime) proxyRoute {
	return proxyRoute{
		ID:       "observability:langfuse",
		Kind:     "observability",
		Host:     langfuseProxyHost,
		Upstream: observabilityDNSAlias(runtime, "langfuse-web") + ":3000",
		Network:  runtime.DockerNetwork(),
	}
}

func observabilityInternalProxyRoute(runtime observabilityRuntime) proxyRoute {
	return proxyRoute{
		ID:         "observability:langfuse-internal",
		Kind:       "observability-internal",
		PathPrefix: "/_taxiway/langfuse",
		Upstream:   observabilityDNSAlias(runtime, "langfuse-web") + ":3000",
		Network:    runtime.DockerNetwork(),
	}
}

func observabilityProxyRoutes(runtime observabilityRuntime) []proxyRoute {
	return []proxyRoute{
		observabilityProxyRoute(runtime),
		observabilityInternalProxyRoute(runtime),
	}
}

func upsertObservabilityProxyRoutes(proxyDir string, runtime observabilityRuntime) (bool, error) {
	changed := false
	for _, route := range observabilityProxyRoutes(runtime) {
		updated, err := upsertProxyRoute(proxyDir, route)
		if err != nil {
			return false, err
		}
		if updated {
			changed = true
		}
	}
	return changed, nil
}

func readProxyRoutes(proxyDir string) ([]proxyRoute, error) {
	data, err := os.ReadFile(proxyRoutesStatePath(proxyDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read proxy routes: %w", err)
	}
	var registry proxyRouteRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse proxy routes: %w", err)
	}
	sortProxyRoutes(registry.Routes)
	return registry.Routes, nil
}

func writeProxyRoutes(proxyDir string, routes []proxyRoute) (bool, error) {
	sortProxyRoutes(routes)
	data, err := json.MarshalIndent(proxyRouteRegistry{Routes: routes}, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	dst := proxyRoutesStatePath(proxyDir)
	existing, err := os.ReadFile(dst)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading proxy routes: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false, fmt.Errorf("creating proxy route directory: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return false, fmt.Errorf("writing proxy routes: %w", err)
	}
	return true, nil
}

func writeProxyConfig(proxyDir string, data []byte) (bool, error) {
	dst := proxyConfigStatePath(proxyDir)

	existing, err := os.ReadFile(dst)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading proxy config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false, fmt.Errorf("creating proxy config directory: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return false, fmt.Errorf("writing proxy config: %w", err)
	}
	return true, nil
}

func sortProxyRoutes(routes []proxyRoute) {
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].ID < routes[j].ID
	})
}

func renderProxyConfig(routes []proxyRoute) []byte {
	return renderProxyConfigWithPage(routes, renderProxyDefaultPage(routes))
}

func renderProxyConfigWithPage(routes []proxyRoute, page string) []byte {
	var out strings.Builder
	out.WriteString(`{
	auto_https off
}

:4000 {
`)
	for _, route := range routes {
		matcher := proxyRouteMatcher(route)
		if route.PathPrefix != "" {
			fmt.Fprintf(&out, `	@%[1]s path %[2]s %[2]s/*
	handle @%[1]s {
		uri strip_prefix %[2]s
		reverse_proxy %[3]s
	}

`, matcher, route.PathPrefix, route.Upstream)
			continue
		}
		fmt.Fprintf(&out, `	@%[1]s host %[2]s
	handle @%[1]s {
		reverse_proxy %[3]s
	}

`, matcher, route.Host, route.Upstream)
	}
	out.WriteString(`	handle {
		header Content-Type text/html
		header Cache-Control "no-store"
		respond <<TAXIWAY_PROXY_INDEX
`)
	out.WriteString(page)
	out.WriteString(`
TAXIWAY_PROXY_INDEX
	}
}
`)
	return []byte(out.String())
}

type proxyStatusGateway struct {
	Lab         string
	Taxiway     string
	Docker      string
	Project     string
	UIURL       string
	APIURL      string
	APIKey      string
	OpenEnabled bool
	Services    []proxyStatusService
}

type proxyStatusService struct {
	Name  string
	State string
}

func collectProxyStatusGateways(labStateDir string, routes []proxyRoute, proxy proxyRuntime, docker dockerRuntimeStatus) ([]proxyStatusGateway, error) {
	statuses, err := readLabLiteLLMSidecarStatuses(labStateDir)
	if err != nil {
		return nil, err
	}
	routeByLab := map[string]proxyRoute{}
	for _, route := range routes {
		if route.Kind == "lab" {
			routeByLab[strings.TrimPrefix(route.ID, "lab:")] = route
		}
	}
	var rows []proxyStatusGateway
	for _, status := range statuses {
		dockerState := sidecarDockerState(status.Project, docker)
		route := routeByLab[status.Lab]
		host := route.Host
		if host == "" {
			host = labLiteLLMHost(status.Lab)
		}
		values, err := readLabGatewayEnv(labStateDir, config.LabRef{Lab: status.Lab})
		if err != nil {
			return nil, err
		}
		row := proxyStatusGateway{
			Lab:         status.Lab,
			Taxiway:     "configured",
			Docker:      dockerState,
			Project:     status.Project,
			OpenEnabled: dockerState == "running" && proxy.Port > 0,
			APIKey:      values[labLiteLLMAPIKeyEnv],
			Services:    labGatewayServiceStates(status.Project, docker),
		}
		if proxy.Port > 0 {
			row.UIURL = fmt.Sprintf("http://%s:%d/ui/login", host, proxy.Port)
			row.APIURL = fmt.Sprintf("http://%s:%d", host, proxy.Port)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Lab < rows[j].Lab })
	return rows, nil
}

func renderProxyDefaultPage(routes []proxyRoute) string {
	_ = routes
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Taxiway proxy</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f7f7f4;
      --text: #202124;
      --muted: #626760;
      --accent: #176b57;
      --accent-bg: #e7f4ef;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #171918;
        --text: #f1f2ee;
        --muted: #b2b7af;
        --accent: #74d4b3;
        --accent-bg: #17342b;
      }
    }
    * { box-sizing: border-box; }
    body { min-height: 100vh; margin: 0; display: grid; place-items: center; background: var(--bg); color: var(--text); line-height: 1.45; }
    main { text-align: center; padding: 32px; }
    h1 { margin: 0 0 10px; font-size: 28px; letter-spacing: 0; }
    .status { display: inline-flex; align-items: center; min-height: 24px; padding: 2px 10px; border-radius: 999px; background: var(--accent-bg); color: var(--accent); font-size: 12px; font-weight: 700; }
    p { margin: 12px 0 0; color: var(--muted); }
  </style>
</head>
<body>
<main>
  <h1>Taxiway proxy</h1>
  <span class="status">running</span>
  <p>This endpoint only serves Taxiway runtime routes.</p>
</main>
</body>
</html>`
}

func proxyRouteMatcher(route proxyRoute) string {
	if route.Kind == "lab" {
		name := strings.TrimPrefix(route.ID, "lab:")
		return strings.ReplaceAll(labLiteLLMSlug(name), "-", "_")
	}
	replacer := strings.NewReplacer(":", "_", "-", "_", ".", "_")
	return replacer.Replace(route.ID)
}

func proxyRunCmd(state *RootState, stateDir string) *exec.Cmd {
	runtime := state.proxyRuntime()
	publishedPort := "4000:4000"
	if runtime.Context == "dev" || runtime.Context == "e2e" {
		// dev/e2e: pin the free port allocated before launch (see
		// ensureDevProxyRuntimeState). A pinned <port>:4000 mapping binds 0.0.0.0
		// (reachable via 127.0.0.1, incl. on the GitHub Actions runner) and stays
		// stable across container restarts, unlike a Docker-assigned (::4000) port
		// whose host port is reallocated on restart and goes stale.
		publishedPort = fmt.Sprintf("%d:4000", runtime.Port)
	} else if runtime.Port > 0 {
		publishedPort = fmt.Sprintf("%d:4000", runtime.Port)
	}
	return exec.Command(
		"docker", "run", "-d",
		"--name", runtime.Container,
		"--restart", "unless-stopped",
		"--network", runtime.DockerNetwork(),
		"--network-alias", proxyDNSAlias(runtime),
		"-p", publishedPort,
		"-v", proxyConfigStatePath(stateDir)+":/etc/caddy/Caddyfile:ro",
		proxyImage,
	)
}

func parseDockerPublishedPort(out string) (int, error) {
	value := strings.TrimSpace(out)
	if value == "" {
		return 0, fmt.Errorf("docker port returned no published port")
	}
	var port int
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, portText, err := net.SplitHostPort(line)
		if err != nil {
			return 0, fmt.Errorf("parse docker published port %q: %w", line, err)
		}
		linePort, err := strconv.Atoi(portText)
		if err != nil || linePort <= 0 {
			return 0, fmt.Errorf("parse docker published port %q", line)
		}
		if port == 0 {
			port = linePort
			continue
		}
		if port != linePort {
			return 0, fmt.Errorf("conflicting published ports in docker port output %q", value)
		}
	}
	if port == 0 {
		return 0, fmt.Errorf("docker port returned no published port")
	}
	return port, nil
}

func inspectProxyPublishedPort(container string) (int, error) {
	out, err := exec.Command("docker", "port", container, "4000/tcp").CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return 0, fmt.Errorf("inspect proxy published port: %w\n%s", err, detail)
		}
		return 0, fmt.Errorf("inspect proxy published port: %w", err)
	}
	return parseDockerPublishedPort(string(out))
}

func refreshProxyPublishedPort(state *RootState) error {
	runtime := state.proxyRuntime()
	if runtime.Context != "dev" && runtime.Context != "e2e" {
		return nil
	}
	port, err := inspectProxyPublishedPort(runtime.Container)
	if err != nil {
		return err
	}
	runtime.Port = port
	state.Proxy = runtime
	return writeDevProxyRuntimeState(runtime)
}

func ensureProxyDockerNetwork(runtime proxyRuntime) error {
	if err := exec.Command("docker", "network", "inspect", runtime.DockerNetwork()).Run(); err == nil {
		return nil
	}
	if err := exec.Command("docker", "network", "create", runtime.DockerNetwork()).Run(); err != nil {
		return fmt.Errorf("create proxy network %q: %w", runtime.DockerNetwork(), err)
	}
	return nil
}

func startProxy(state *RootState, stateDir string) (bool, error) {
	runtime := state.proxyRuntime()
	if err := ensureProxyDockerNetwork(runtime); err != nil {
		return false, err
	}
	restarted, err := dockerRemoveContainerIfExists(runtime.Container)
	if err != nil {
		return false, err
	}
	cmd := proxyRunCmd(state, stateDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return false, fmt.Errorf("starting proxy: %w\n%s", err, detail)
		}
		return false, fmt.Errorf("starting proxy: %w", err)
	}
	if err := refreshProxyPublishedPort(state); err != nil {
		return false, err
	}
	return restarted, nil
}

func ensureProxyRunning(state *RootState, stateDir string) (bool, bool, error) {
	if containerState(state.proxyRuntime().Container) == "running" {
		if err := refreshProxyPublishedPort(state); err != nil {
			return false, false, err
		}
		return false, false, nil
	}
	restarted, err := startProxy(state, stateDir)
	if err != nil {
		return false, false, err
	}
	return true, restarted, nil
}

func stopProxyContainer(state *RootState) (bool, error) {
	runtime := state.proxyRuntime()
	exists, err := dockerContainerExists(runtime.Container)
	if err != nil {
		return false, fmt.Errorf("checking container %s: %w", runtime.Container, err)
	}
	if !exists {
		return false, nil
	}
	if containerState(runtime.Container) != "running" {
		return false, nil
	}
	if err := exec.Command("docker", "stop", runtime.Container).Run(); err != nil {
		return false, fmt.Errorf("stopping container %s: %w", runtime.Container, err)
	}
	return true, nil
}

func removeProxyContainer(state *RootState) (bool, error) {
	return dockerRemoveContainerIfExists(state.proxyRuntime().Container)
}

func removeProxyRuntimeState(runtime proxyRuntime) error {
	if runtime.Context != "dev" && runtime.Context != "e2e" {
		return nil
	}
	if err := os.Remove(proxyRuntimeStatePath(runtime.StateDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove proxy runtime state: %w", err)
	}
	return nil
}

func reconcileProxyLifecycle(state *RootState, proxyDir string) (string, error) {
	routes, err := readProxyRoutes(proxyDir)
	if err != nil {
		return "", err
	}
	proxy := state.proxyRuntime()
	if !proxyRuntimePortAvailable(proxy) && len(routes) == 0 {
		return "absent", nil
	}

	docker := detectDockerStatus()
	if !docker.Available {
		return "", fmt.Errorf("docker unavailable: %s", docker.Reason)
	}
	if len(routes) == 0 {
		if _, _, err := ensureProxyRunning(state, proxyDir); err != nil {
			return "", err
		}
		return "running", nil
	}
	hasRunning := false
	for _, route := range routes {
		switch proxyRouteTargetState(state, docker, route) {
		case "running":
			hasRunning = true
		}
	}
	if _, _, err := ensureProxyRunning(state, proxyDir); err != nil {
		return "", err
	}
	if hasRunning {
		if err := connectProxyToRouteNetworks(state, proxyDir); err != nil {
			return "", err
		}
	}
	return "running", nil
}

func proxyRouteTargetState(state *RootState, docker dockerRuntimeStatus, route proxyRoute) string {
	switch route.Kind {
	case "observability", "observability-internal":
		runtime := state.observabilityRuntime()
		credentialsPresent := fileExists(observabilityEnvPath(state))
		runtimePresent := observabilityRuntimeInitialized(runtime)
		status, _ := summarizeLangfuseStatus(credentialsPresent, runtimePresent, docker, langfuseServiceStates(runtime, docker))
		switch status {
		case "running", "partial":
			return "running"
		case "stopped":
			return "stopped"
		default:
			return "removed"
		}
	case "lab":
		project := strings.TrimSuffix(route.Network, "_default")
		if project == "" {
			return "stopped"
		}
		status := sidecarDockerState(project, docker)
		if status == "running" || status == "partial" {
			return "running"
		}
		return "stopped"
	default:
		return "removed"
	}
}

func reloadProxy(state *RootState) error {
	return reloadProxyWithAttempts(state, 3, 500*time.Millisecond)
}

func reloadProxyWithAttempts(state *RootState, attempts int, delay time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		out, err := exec.Command("docker", "exec", state.proxyRuntime().Container, "caddy", "reload", "--config", "/etc/caddy/Caddyfile").CombinedOutput()
		if err == nil {
			return nil
		}
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			lastErr = fmt.Errorf("reload proxy: %w\n%s", err, detail)
		} else {
			lastErr = fmt.Errorf("reload proxy: %w", err)
		}
		if attempt < attempts && delay > 0 {
			time.Sleep(delay)
		}
	}
	return lastErr
}

func connectProxyToNetwork(state *RootState, network string) error {
	runtime := state.proxyRuntime()
	out, err := exec.Command("docker", "network", "connect", "--alias", proxyDNSAlias(runtime), network, runtime.Container).CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(out))
	if strings.Contains(text, "already exists") {
		return nil
	}
	if text != "" {
		return fmt.Errorf("connect proxy %s to network %s: %w\n%s", runtime.Container, network, err, text)
	}
	return fmt.Errorf("connect proxy %s to network %s: %w", runtime.Container, network, err)
}

func connectProxyToRouteNetworks(state *RootState, proxyDir string) error {
	routes, err := readProxyRoutes(proxyDir)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, route := range routes {
		if route.Network == "" {
			continue
		}
		if _, ok := seen[route.Network]; ok {
			continue
		}
		seen[route.Network] = struct{}{}
		if err := connectProxyToNetwork(state, route.Network); err != nil {
			return err
		}
	}
	return nil
}

func disconnectProxyFromNetwork(state *RootState, network string) error {
	runtime := state.proxyRuntime()
	out, err := exec.Command("docker", "network", "disconnect", network, runtime.Container).CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(out))
	if strings.Contains(text, "not connected") || strings.Contains(text, "No such container") || strings.Contains(text, "No such network") {
		return nil
	}
	if text != "" {
		return fmt.Errorf("disconnect proxy %s from network %s: %w\n%s", runtime.Container, network, err, text)
	}
	return fmt.Errorf("disconnect proxy %s from network %s: %w", runtime.Container, network, err)
}
