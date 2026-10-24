package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestroyProxyNetworkContextAndPartialStates(t *testing.T) {
	for _, context := range []string{"host", "dev", "e2e"} {
		for _, resources := range []string{"both", "network", "container", "neither"} {
			t.Run(context+"/"+resources, func(t *testing.T) {
				tmp := t.TempDir()
				t.Setenv("TAXIWAY_CONTEXT", context)
				t.Setenv("TAXIWAY_CONTEXT_ID", "cleanup-test")
				_, state, _, _ := buildObserveTestRoot(t, tmp)
				state.Flags.StateDir = filepath.Join(tmp, "labs")
				state.Proxy = proxyRuntime{Context: context, ContextID: "cleanup-test", StateDir: filepath.Join(tmp, "proxy")}
				state.Observability.StateDir = filepath.Join(tmp, "observability")
				proxy, err := state.resolveProxyRuntime()
				require.NoError(t, err)
				containerFile := filepath.Join(tmp, "container")
				networkFile := filepath.Join(tmp, "network")
				otherFile := filepath.Join(tmp, "other-context-network")
				require.NoError(t, os.WriteFile(otherFile, nil, 0o600))
				if resources == "both" || resources == "container" {
					require.NoError(t, os.WriteFile(containerFile, nil, 0o600))
				}
				if resources == "both" || resources == "network" {
					require.NoError(t, os.WriteFile(networkFile, nil, 0o600))
				}
				require.NoError(t, os.MkdirAll(proxy.StateDir, 0o700))
				writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "container" ] && [ "$2" = "inspect" ] && [ "$3" = %q ]; then test -f %q; exit $?; fi
if [ "$1" = "rm" ] && [ "$2" = "-f" ] && [ "$3" = %q ]; then /bin/rm -f %q; exit 0; fi
if [ "$1" = "network" ] && [ "$2" = "rm" ] && [ "$3" = %q ] && [ "$#" = 3 ]; then
  test ! -f %q || { echo 'container still attached' >&2; exit 1; }
  test ! -f %q || test -d %q || { echo 'proxy state removed too early' >&2; exit 1; }
  if test -f %q; then /bin/rm %q; exit 0; fi
  echo 'Error response from daemon: network %s not found' >&2; exit 1
fi
if [ "$1" = "compose" ]; then exit 0; fi
echo "unexpected Docker command: $*" >&2
exit 1
`, proxy.Container, containerFile, proxy.Container, containerFile, proxy.DockerNetwork(), containerFile, networkFile, proxy.StateDir, networkFile, networkFile, proxy.DockerNetwork()))

				var out bytes.Buffer
				require.NoError(t, destroyTaxiwayRuntime(&out, state))
				require.NoFileExists(t, containerFile)
				require.NoFileExists(t, networkFile)
				require.FileExists(t, otherFile)
				require.NoDirExists(t, proxy.StateDir)
				// Repeating destroy also handles missing state and resources.
				require.NoError(t, destroyTaxiwayRuntime(&out, state))
			})
		}
	}
}

func TestDestroyProxyNetworkFailureRetainsState(t *testing.T) {
	for _, detail := range []string{"network has active endpoints", "Cannot connect to the Docker daemon"} {
		t.Run(detail, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TAXIWAY_CONTEXT", "e2e")
			t.Setenv("TAXIWAY_CONTEXT_ID", "cleanup-test")
			_, state, _, _ := buildObserveTestRoot(t, tmp)
			state.Flags.StateDir = filepath.Join(tmp, "labs")
			state.Proxy = proxyRuntime{Context: "e2e", ContextID: "cleanup-test", StateDir: filepath.Join(tmp, "proxy")}
			state.Observability.StateDir = filepath.Join(tmp, "observability")
			require.NoError(t, os.MkdirAll(state.Proxy.StateDir, 0o700))
			statePath := proxyRuntimeStatePath(state.Proxy.StateDir)
			require.NoError(t, os.WriteFile(statePath, []byte(`{"port":55124}`), 0o600))
			writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "container" ]; then exit 1; fi
if [ "$1" = "network" ] && [ "$2" = "rm" ]; then echo %q >&2; exit 1; fi
if [ "$1" = "compose" ]; then exit 0; fi
exit 1
`, detail))
			var out bytes.Buffer
			err := destroyTaxiwayRuntime(&out, state)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "taxiway-e2e-cleanup-test-proxy_default")
			assert.Contains(t, err.Error(), detail)
			require.FileExists(t, statePath)
			assert.NotContains(t, out.String(), "Proxy: removed")
			assert.NotContains(t, out.String(), "Observability: removed")
		})
	}
}

func TestStartProxyRetainsNetworkDuringRecreation(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	state := &RootState{Proxy: proxyRuntime{Context: "host", StateDir: tmp, Container: "taxiway-proxy", ComposeProject: "taxiway-proxy", Port: 4000}}
	writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then exit 0; fi
if [ "$1" = "rm" ] && [ "$2" = "-f" ]; then exit 0; fi
if [ "$1" = "run" ]; then exit 0; fi
exit 1
`, logPath))
	restarted, err := startProxy(state, tmp)
	require.NoError(t, err)
	require.True(t, restarted)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(data)
	assert.Contains(t, log, "--network taxiway-proxy_default")
	assert.NotContains(t, log, "network rm")
	assert.NotContains(t, log, "network create")
	assert.Less(t, strings.Index(log, "rm -f"), strings.Index(log, "run -d"))
}

func TestConnectProxyToRouteNetworksMissingNetwork(t *testing.T) {
	for _, detail := range []string{
		"Error response from daemon: network gone_default not found",
		"Error response from daemon: No such network: gone_default",
	} {
		t.Run(detail, func(t *testing.T) {
			proxyDir := t.TempDir()
			logPath := filepath.Join(t.TempDir(), "docker.log")
			state := &RootState{Proxy: proxyRuntime{Context: "host", Container: "taxiway-proxy"}}
			routes := []proxyRoute{
				{ID: "lab:gone", Kind: "lab", Network: "gone_default"},
				{ID: "lab:healthy", Kind: "lab", Network: "healthy_default"},
				{ID: "lab:healthy-duplicate", Kind: "lab", Network: "healthy_default"},
			}
			_, err := writeProxyRoutes(proxyDir, routes)
			require.NoError(t, err)
			before, err := os.ReadFile(proxyRoutesStatePath(proxyDir))
			require.NoError(t, err)
			writeFakeDocker(t, fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1 $2" = 'network connect' ] && [ "$5" = gone_default ]; then echo %q >&2; exit 1; fi
if [ "$1 $2" = 'network connect' ] && [ "$5" = healthy_default ]; then exit 0; fi
exit 2
`, logPath, detail))
			warnings := captureProxyNetworkWarnings(t, func() {
				require.NoError(t, connectProxyToRouteNetworks(state, proxyDir))
			})
			require.Contains(t, warnings, `Warning: proxy route "lab:gone"`)
			require.Contains(t, warnings, "taxiway status")
			require.NotContains(t, warnings, "Error response from daemon")
			log, err := os.ReadFile(logPath)
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(string(log), "healthy_default"), "continue connecting remaining networks once")
			after, err := os.ReadFile(proxyRoutesStatePath(proxyDir))
			require.NoError(t, err)
			require.Equal(t, before, after, "do not prune another process's route")
			require.Error(t, connectProxyToNetwork(state, "gone_default"), "an explicitly requested gateway network must remain strict")
		})
	}
}

func TestConnectProxyToRouteNetworksKeepsRealFailures(t *testing.T) {
	for _, detail := range []string{
		"Error response from daemon: network another_default not found",
		"Error response from daemon: No such container: taxiway-proxy",
		"permission denied while connecting to Docker",
		"Cannot connect to the Docker daemon",
		"network missing_default not found in plugin configuration",
	} {
		t.Run(detail, func(t *testing.T) {
			proxyDir := t.TempDir()
			_, err := writeProxyRoutes(proxyDir, []proxyRoute{{ID: "lab:missing", Kind: "lab", Network: "missing_default"}})
			require.NoError(t, err)
			writeFakeDocker(t, fmt.Sprintf("#!/bin/sh\necho %q >&2\nexit 1\n", detail))
			warnings := captureProxyNetworkWarnings(t, func() {
				err := connectProxyToRouteNetworks(&RootState{}, proxyDir)
				require.ErrorContains(t, err, detail)
			})
			require.Empty(t, warnings)
		})
	}
}

func captureProxyNetworkWarnings(t *testing.T, f func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "warnings")
	require.NoError(t, err)
	original := os.Stderr
	os.Stderr = file
	defer func() { os.Stderr = original; _ = file.Close() }()
	f()
	data, err := os.ReadFile(file.Name())
	require.NoError(t, err)
	return string(data)
}
