//go:build !e2e

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var taxiwayTestRoot string

var taxiwayCallerContextEnvKeys = []string{
	"TAXIWAY_CONTEXT",
	"TAXIWAY_CONTEXT_ID",
	"TAXIWAY_AUTH_DIR",
	"TAXIWAY_OBSERVABILITY_DIR",
	"TAXIWAY_PROXY_DIR",
	"TAXIWAY_RUNTIME_DIR",
	"TAXIWAY_LAB_STATE_DIR",
	"TAXIWAY_DRIVER",
}

func TestMain(m *testing.M) {
	clearTaxiwayCallerContextEnv()
	testRoot, err := os.MkdirTemp("", "taxiway-cli-tests-*")
	if err != nil {
		panic(err)
	}
	taxiwayTestRoot = testRoot
	home := filepath.Join(testRoot, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	binDir := filepath.Join(testRoot, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		panic(err)
	}
	for _, name := range []string{"docker", "limactl"} {
		if err := writeUnexpectedHostCommandDouble(filepath.Join(binDir, name), name); err != nil {
			panic(err)
		}
	}
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(testRoot)
	os.Exit(code)
}

func writeUnexpectedHostCommandDouble(path, name string) error {
	content := "#!/bin/sh\n" +
		"printf '%s\\n' 'unexpected " + name + " invocation in non-e2e tests' >&2\n" +
		"printf '%s\\n' \"$*\" >&2\n" +
		"exit 127\n"
	return os.WriteFile(path, []byte(content), 0o755)
}

func TestNonE2ETestsUseCommandDoublesByDefault(t *testing.T) {
	binDir := filepath.Join(taxiwayTestRoot, "bin")
	for _, name := range []string{"docker", "limactl"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("expected %s test double on PATH: %v", name, err)
		}
		if !strings.HasPrefix(path, binDir+string(os.PathSeparator)) {
			t.Fatalf("expected %s to resolve under %s, got %s", name, binDir, path)
		}
	}
}

func clearTaxiwayCallerContextEnv() {
	for _, key := range taxiwayCallerContextEnvKeys {
		_ = os.Unsetenv(key)
	}
}

func isolateTaxiwayCallerContextEnvForTest(t *testing.T) {
	t.Helper()
	for _, key := range taxiwayCallerContextEnvKeys {
		t.Setenv(key, "")
	}
}
