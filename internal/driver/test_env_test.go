package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var driverTestRoot string

func TestMain(m *testing.M) {
	testRoot, err := os.MkdirTemp("", "taxiway-driver-tests-*")
	if err != nil {
		panic(err)
	}
	driverTestRoot = testRoot
	binDir := filepath.Join(testRoot, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		panic(err)
	}
	for _, name := range []string{"docker", "limactl"} {
		if err := writeUnexpectedCommandDouble(filepath.Join(binDir, name), name); err != nil {
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

func writeUnexpectedCommandDouble(path, name string) error {
	content := "#!/bin/sh\n" +
		"printf '%s\\n' 'unexpected " + name + " invocation in non-e2e tests' >&2\n" +
		"printf '%s\\n' \"$*\" >&2\n" +
		"exit 127\n"
	return os.WriteFile(path, []byte(content), 0o755)
}

func TestNonE2ETestsUseCommandDoublesByDefault(t *testing.T) {
	binDir := filepath.Join(driverTestRoot, "bin")
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
