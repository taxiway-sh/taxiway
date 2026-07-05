//go:build e2e

package cli

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

var (
	e2eRunMu                   sync.Mutex
	e2eTestRoot                string
	e2eObservabilityStartMutex sync.Mutex
)

var taxiwayCallerContextEnvKeys = []string{
	"TAXIWAY_AUTH_DIR",
	"TAXIWAY_CONTEXT",
	"TAXIWAY_CONTEXT_ID",
	"TAXIWAY_OBSERVABILITY_DIR",
	"TAXIWAY_PROXY_DIR",
	"TAXIWAY_RUNTIME_DIR",
	"TAXIWAY_LAB_STATE_DIR",
	"TAXIWAY_DRIVER",
}

func TestClearTaxiwayCallerContextEnvClearsMutableRuntimeDirs(t *testing.T) {
	t.Setenv("TAXIWAY_AUTH_DIR", "/tmp/global-auth")
	t.Setenv("TAXIWAY_OBSERVABILITY_DIR", "/tmp/global-observability")
	t.Setenv("TAXIWAY_PROXY_DIR", "/tmp/global-proxy")

	clearTaxiwayCallerContextEnv()

	require.Empty(t, os.Getenv("TAXIWAY_AUTH_DIR"))
	require.Empty(t, os.Getenv("TAXIWAY_OBSERVABILITY_DIR"))
	require.Empty(t, os.Getenv("TAXIWAY_PROXY_DIR"))
}

func TestMain(m *testing.M) {
	hostHome := os.Getenv("HOME")
	clearTaxiwayCallerContextEnv()
	if os.Getenv("DOCKER_CONFIG") == "" && hostHome != "" {
		if err := os.Setenv("DOCKER_CONFIG", filepath.Join(hostHome, ".docker")); err != nil {
			panic(err)
		}
	}

	testRoot, err := os.MkdirTemp("", "taxiway-cli-e2e-tests-*")
	if err != nil {
		panic(err)
	}
	home := filepath.Join(testRoot, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		panic(err)
	}
	e2eRunMu.Lock()
	e2eTestRoot = testRoot
	e2eRunMu.Unlock()
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(testRoot)
	os.Exit(code)
}

func clearTaxiwayCallerContextEnv() {
	for _, key := range taxiwayCallerContextEnvKeys {
		_ = os.Unsetenv(key)
	}
}

func configureE2EScenarioEnvironment(t *testing.T, scenario string) observabilityRuntime {
	t.Helper()

	root := e2eScenarioRoot(t, scenario)
	require.NoError(t, os.MkdirAll(root, 0o700))
	contextID := e2eShortSHA1(root)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".context-id"), []byte(contextID+"\n"), 0o600))

	values := map[string]string{
		"TAXIWAY_AUTH_DIR":          filepath.Join(root, ".auth"),
		"TAXIWAY_CONTEXT":           "e2e",
		"TAXIWAY_CONTEXT_ID":        contextID,
		"TAXIWAY_OBSERVABILITY_DIR": filepath.Join(root, ".observability"),
		"TAXIWAY_PROXY_DIR":         filepath.Join(root, ".proxy"),
		"TAXIWAY_RUNTIME_DIR":       root,
		"TAXIWAY_LAB_STATE_DIR":     filepath.Join(root, ".lab-state"),
	}
	for key, value := range values {
		t.Setenv(key, value)
	}

	runtime := e2eObservabilityRuntime(t)
	proxy := e2eProxyRuntime(t)
	cleanupE2EObservabilityRuntime(t, runtime, proxy)
	return runtime
}

func e2eScenarioRoot(t *testing.T, scenario string) string {
	t.Helper()
	e2eRunMu.Lock()
	root := e2eTestRoot
	e2eRunMu.Unlock()
	require.NotEmpty(t, root)
	return filepath.Join(root, "scenarios", labLiteLLMSlug(t.Name()), labLiteLLMSlug(scenario))
}

func e2eShortSHA1(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])[:8]
}

func e2eLabStateDir() string {
	if stateDir := os.Getenv("TAXIWAY_LAB_STATE_DIR"); stateDir != "" {
		return stateDir
	}
	return filepath.Join(os.TempDir(), "taxiway-e2e-lab-state")
}

func requireDockerOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("LAB_NO_DOCKER") != "" {
		t.Skip("LAB_NO_DOCKER is set - skipping end-to-end test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker binary not on PATH - skipping end-to-end test")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable - skipping end-to-end test")
	}
}

func uniqueDockerID(t *testing.T, orch, action string) string {
	t.Helper()
	runtime := e2eObservabilityRuntime(t)
	name := fmt.Sprintf("taxiway-e2e-%s-%s-%s", runtime.ContextID, labLiteLLMSlug(orch), labLiteLLMSlug(action))
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
		_ = exec.Command("docker", "volume", "rm", "taxiway-"+name+"-lab").Run()
	})
	return name
}

func e2eObservabilityRuntime(t *testing.T) observabilityRuntime {
	t.Helper()
	state := &RootState{}
	runtime, err := state.resolveObservabilityRuntime()
	require.NoError(t, err)
	return runtime
}

func e2eProxyRuntime(t *testing.T) proxyRuntime {
	t.Helper()
	state := &RootState{}
	runtime, err := state.resolveProxyRuntime()
	require.NoError(t, err)
	return runtime
}

func cleanupE2EObservabilityRuntime(t *testing.T, runtime observabilityRuntime, proxy proxyRuntime) {
	t.Helper()
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", proxy.Container).Run()
		for _, container := range e2eDockerNames("container", runtime.ComposeProject+"-") {
			_ = exec.Command("docker", "rm", "-f", container).Run()
		}
		for _, volume := range e2eDockerNames("volume", runtime.ComposeProject+"_") {
			_ = exec.Command("docker", "volume", "rm", volume).Run()
		}
		_ = exec.Command("docker", "network", "rm", runtime.DockerNetwork()).Run()
		_ = exec.Command("docker", "network", "rm", proxy.DockerNetwork()).Run()
	})
}

func e2eDockerNames(kind, prefix string) []string {
	args := []string{kind, "ls", "--format", "{{.Name}}"}
	if kind == "container" {
		args = []string{"ps", "-a", "--format", "{{.Names}}"}
	}
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, name := range strings.Fields(string(out)) {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names
}

func requireE2ERuntimeIsolated(t *testing.T, state *RootState) {
	t.Helper()
	e2eRunMu.Lock()
	root := e2eTestRoot
	e2eRunMu.Unlock()
	require.NotEmpty(t, root)
	requirePathUnder(t, authStateDir(state), root)
	requirePathUnder(t, state.Flags.StateDir, root)
	requirePathUnder(t, observabilityStateDir(state), root)
	requirePathUnder(t, state.proxyRuntime().StateDir, root)
	runtime := state.observabilityRuntime()
	proxy := state.proxyRuntime()
	require.Equal(t, "e2e", runtime.Context)
	require.Equal(t, os.Getenv("TAXIWAY_CONTEXT_ID"), runtime.ContextID)
	require.Equal(t, "taxiway-e2e-"+runtime.ContextID+"-observability", runtime.ComposeProject)
	require.Equal(t, "taxiway-e2e-"+runtime.ContextID+"-proxy", proxy.Container)
}

func requirePathUnder(t *testing.T, path, parent string) {
	t.Helper()
	rel, err := filepath.Rel(parent, path)
	require.NoError(t, err)
	require.False(t, rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel),
		"path %q must stay under %q", path, parent)
}

type dockerTestBuf struct {
	out bytes.Buffer
	err bytes.Buffer
}

func copyRuntimeTree(t *testing.T, src, dst string) {
	t.Helper()

	src = filepath.Join(findRepoRoot(t), src)
	require.NoError(t, filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		rel, err := filepath.Rel(src, path)
		require.NoError(t, err)
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			info, err := entry.Info()
			require.NoError(t, err)
			return os.MkdirAll(target, info.Mode().Perm())
		}
		copyRuntimeFile(t, path, target)
		return nil
	}))
}

func copyRuntimeFile(t *testing.T, src, dst string) {
	t.Helper()

	if !filepath.IsAbs(src) && !strings.HasPrefix(src, ".."+string(os.PathSeparator)) {
		src = filepath.Join(findRepoRoot(t), src)
	}
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	info, err := os.Stat(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.WriteFile(dst, data, info.Mode().Perm()))
}

func execDockerRoot(t *testing.T, root *cobra.Command, tb *dockerTestBuf, args ...string) (string, string, error) {
	t.Helper()
	tb.out.Reset()
	tb.err.Reset()
	resetCobraFlags(root)
	root.SetArgs(args)
	err := root.Execute()
	return tb.out.String(), tb.err.String(), err
}

func resetCobraFlags(cmd *cobra.Command) {
	resetFlagSet(cmd.Flags())
	resetFlagSet(cmd.PersistentFlags())
	for _, child := range cmd.Commands() {
		resetCobraFlags(child)
	}
}

func resetFlagSet(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(nil)
			flag.Changed = false
			return
		}
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	})
}

func labNameFromID(id string) string {
	return strings.TrimPrefix(id, "taxiway-")
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve current test file")
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod not found)")
		}
		dir = parent
	}
}
