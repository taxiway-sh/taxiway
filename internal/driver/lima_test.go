package driver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLimaYAML(t *testing.T) {
	tmp := t.TempDir()

	// Write a minimal template that exercises the mounted runtime fields.
	tmplContent := `mounts:
  - location: "{{.RepoDir}}/infra"
    mountPoint: "/lab/infra"
    writable: false
  - location: "{{.RepoDir}}/orchestrators/{{.Orch}}"
    mountPoint: "/lab/orchestrators/{{.Orch}}"
    writable: false
  - location: "{{.GitDir}}"
    mountPoint: "/lab/git"
    writable: true
  - location: "{{.RecordingsDir}}"
    mountPoint: "/lab/recordings"
    writable: true
`
	tmplPath := filepath.Join(tmp, "agent-lab.yaml.tmpl")
	require.NoError(t, os.WriteFile(tmplPath, []byte(tmplContent), 0644))

	outPath := filepath.Join(tmp, "rendered.yaml")
	data := LimaTemplateData{
		RepoDir:       "/home/lab/repo",
		Orch:          "gastown",
		GitDir:        "/home/lab/.lab-state/gastown/git",
		RecordingsDir: "/home/lab/.lab-state/gastown/recordings",
	}

	require.NoError(t, renderLimaYAML(tmplPath, data, outPath))

	rendered, err := os.ReadFile(outPath)
	require.NoError(t, err)
	out := string(rendered)

	// RepoDir expanded correctly
	require.Contains(t, out, `location: "/home/lab/repo/infra"`)
	require.Contains(t, out, `mountPoint: "/lab/infra"`)

	// Orch-specific mount
	require.Contains(t, out, `location: "/home/lab/repo/orchestrators/gastown"`)
	require.Contains(t, out, `mountPoint: "/lab/orchestrators/gastown"`)

	// Git dir mount
	require.Contains(t, out, `/home/lab/.lab-state/gastown/git`)
	require.Contains(t, out, `mountPoint: "/lab/git"`)

	// Recordings dir mount
	require.Contains(t, out, `/home/lab/.lab-state/gastown/recordings`)
	require.Contains(t, out, `mountPoint: "/lab/recordings"`)

	// No leftover template markers
	require.False(t, strings.Contains(out, "{{"), "rendered YAML must not contain unresolved template markers")
}

func TestRenderLimaYAML_MissingTemplate(t *testing.T) {
	tmp := t.TempDir()
	err := renderLimaYAML(filepath.Join(tmp, "nonexistent.tmpl"), LimaTemplateData{}, filepath.Join(tmp, "out.yaml"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "read template")
}

func TestRenderLimaYAML_CreatesOutDir(t *testing.T) {
	tmp := t.TempDir()
	tmplPath := filepath.Join(tmp, "t.tmpl")
	require.NoError(t, os.WriteFile(tmplPath, []byte("orch: {{.Orch}}\n"), 0644))

	// outPath is under a subdirectory that doesn't exist yet.
	outPath := filepath.Join(tmp, "subdir", "nested", "out.yaml")
	require.NoError(t, renderLimaYAML(tmplPath, LimaTemplateData{Orch: "gastown"}, outPath))

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.Equal(t, "orch: gastown\n", string(content))
}

// TestRenderLimaYAML_NoUsersOverride ensures the Lima template does not
// introduce a top-level `users:` block. Lima creates a lab user matching the
// host user by default, and TAXIWAY_CREW_NAME depends on that equivalence.
func TestRenderLimaYAML_NoUsersOverride(t *testing.T) {
	// Locate the real template from the repo root.
	repoRoot := findRepoRoot(t)
	tmplPath := filepath.Join(repoRoot, "infra", "lima", "agent-lab.yaml.tmpl")

	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		t.Skipf("Lima template not found at %s — skipping", tmplPath)
	}

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "rendered.yaml")
	data := LimaTemplateData{
		RepoDir:       "/home/lab/repo",
		Orch:          "gastown",
		GitDir:        "/home/lab/.lab-state/gastown/git",
		RecordingsDir: "/home/lab/.lab-state/gastown/recordings",
	}

	require.NoError(t, renderLimaYAML(tmplPath, data, outPath))

	renderedBytes, err := os.ReadFile(outPath)
	require.NoError(t, err)
	rendered := string(renderedBytes)

	// Assert no top-level `users:` key. A `users:` block would override Lima's
	// default host-user equivalence and break TAXIWAY_CREW_NAME resolution.
	for _, line := range strings.Split(rendered, "\n") {
		// Top-level YAML keys are not indented.
		if strings.HasPrefix(line, "users:") {
			t.Fatalf("Lima template must not contain a top-level `users:` block (found: %q); "+
				"this would break host-user-to-lab-user equivalence", line)
		}
	}
}

func TestRealLimaTemplateMountsAgents(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tmplPath := filepath.Join(repoRoot, "infra", "lima", "agent-lab.yaml.tmpl")

	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		t.Skipf("Lima template not found at %s — skipping", tmplPath)
	}

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "rendered.yaml")
	data := LimaTemplateData{
		RepoDir:       "/home/lab/repo",
		Orch:          "gastown",
		GitDir:        "/home/lab/.lab-state/gastown/git",
		RecordingsDir: "/home/lab/.lab-state/gastown/recordings",
	}

	require.NoError(t, renderLimaYAML(tmplPath, data, outPath))

	renderedBytes, err := os.ReadFile(outPath)
	require.NoError(t, err)
	rendered := string(renderedBytes)

	require.Contains(t, rendered, `location: "/home/lab/repo/agents"`)
	require.Contains(t, rendered, `mountPoint: "/lab/agents"`)
	require.Contains(t, rendered, `location: "/home/lab/.lab-state/gastown/git"`)
	require.Contains(t, rendered, `mountPoint: "/lab/git"`)
	require.Contains(t, rendered, `location: "/home/lab/.lab-state/gastown/recordings"`)
	require.Contains(t, rendered, `mountPoint: "/lab/recordings"`)
}

func TestRealLimaTemplateUsesMinimalRuntimeMounts(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tmplPath := filepath.Join(repoRoot, "infra", "lima", "agent-lab.yaml.tmpl")

	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		t.Skipf("Lima template not found at %s — skipping", tmplPath)
	}

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "rendered.yaml")
	data := LimaTemplateData{
		RepoDir:       "/home/lab/runtime",
		Orch:          "gastown",
		GitDir:        "/home/lab/state/gastown/git",
		RecordingsDir: "/home/lab/state/gastown/recordings",
		LabHost:       "gastown.litellm.localhost",
	}

	require.NoError(t, renderLimaYAML(tmplPath, data, outPath))

	renderedBytes, err := os.ReadFile(outPath)
	require.NoError(t, err)
	rendered := string(renderedBytes)

	require.Contains(t, rendered, `mountPoint: "/lab/infra"`)
	require.Contains(t, rendered, `mountPoint: "/lab/agents"`)
	require.Contains(t, rendered, `mountPoint: "/lab/orchestrators/gastown"`)
	require.Contains(t, rendered, `mountPoint: "/lab/git"`)
	require.Contains(t, rendered, `mountPoint: "/lab/recordings"`)
	require.NotContains(t, rendered, `mountPoint: "/lab/work"`)
	require.NotContains(t, rendered, `mountPoint: "/lab/agent-lab"`)
	require.NotContains(t, rendered, `mountPoint: "/lab/skills"`)
	require.NotContains(t, rendered, `mountPoint: "/lab/assets"`)
	require.NotContains(t, rendered, `mountPoint: "/lab/observability"`)
}

func TestRealLimaTemplateMapsObservabilityHost(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tmplPath := filepath.Join(repoRoot, "infra", "lima", "agent-lab.yaml.tmpl")

	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		t.Skipf("Lima template not found at %s — skipping", tmplPath)
	}

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "rendered.yaml")
	data := LimaTemplateData{
		RepoDir:       "/home/lab/runtime",
		Orch:          "gastown",
		GitDir:        "/home/lab/state/gastown/git",
		RecordingsDir: "/home/lab/state/gastown/recordings",
		LabHost:       "gastown.litellm.localhost",
	}

	require.NoError(t, renderLimaYAML(tmplPath, data, outPath))

	renderedBytes, err := os.ReadFile(outPath)
	require.NoError(t, err)
	rendered := string(renderedBytes)

	require.Contains(t, rendered, "host.lima.internal")
	require.Contains(t, rendered, "gastown.litellm.localhost")
	require.NotContains(t, rendered, "observability.taxiway.internal")
}

func TestLimaCreateRendersTemplateAndStartsNamedLab(t *testing.T) {
	logPath := installFakeLimactl(t, `#!/bin/sh
printf '%s\n' "$*" >> "$TAXIWAY_FAKE_LIMACTL_LOG"
exit 0
`)
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, "state")
	templatePath := filepath.Join(tmp, "agent-lab.yaml.tmpl")
	require.NoError(t, os.WriteFile(templatePath, []byte("repo: {{.RepoDir}}\norch: {{.Orch}}\ngit: {{.GitDir}}\nrecordings: {{.RecordingsDir}}\n"), 0o644))
	d := NewLimaDriver(stateDir)

	err := d.Create(context.Background(), "taxiway-demo", CreateOptions{
		Lab:           "demo",
		Orch:          "gastown",
		RepoDir:       "/repo",
		GitDir:        "/state/demo/git",
		RecordingsDir: "/state/demo/recordings",
		TemplatePath:  templatePath,
	})

	require.NoError(t, err)
	renderedPath := filepath.Join(stateDir, "demo", "agent-lab.yaml")
	rendered, err := os.ReadFile(renderedPath)
	require.NoError(t, err)
	require.Equal(t, "repo: /repo\norch: gastown\ngit: /state/demo/git\nrecordings: /state/demo/recordings\n", string(rendered))
	require.Equal(t, []string{
		"start --name=taxiway-demo " + renderedPath,
		`shell --workdir=/ taxiway-demo -- sh -c sudo mkdir -p /lab/work && sudo chown "$(id -u):$(id -g)" /lab/work`,
	}, readCommandLog(t, logPath))
	require.FileExists(t, filepath.Join(stateDir, "demo", "created_at"))
}

func TestLimaListParsesManagedLabsOnly(t *testing.T) {
	logPath := installFakeLimactl(t, `#!/bin/sh
printf '%s\n' "$*" >> "$TAXIWAY_FAKE_LIMACTL_LOG"
if [ "$1" = "list" ]; then
  printf 'taxiway-alpha Running\n'
  printf 'unmanaged Running\n'
  printf 'taxiway-beta Stopped\n'
fi
exit 0
`)
	stateDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "alpha"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "alpha", "created_at"), []byte("2026-05-12T10:00:00Z"), 0o644))
	d := NewLimaDriver(stateDir)

	got, err := d.List(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, Status{Name: "taxiway-alpha", State: "running", Driver: "lima", Created: got[0].Created}, got[0])
	require.Equal(t, "taxiway-beta", got[1].Name)
	require.Equal(t, "stopped", got[1].State)
	require.Equal(t, "lima", got[1].Driver)
	require.False(t, got[0].Created.IsZero())
	require.True(t, got[1].Created.IsZero())
	require.Equal(t, []string{"list --format={{.Name}} {{.Status}}"}, readCommandLog(t, logPath))
}

func TestLimaExecBuildsShellCommandWithEnv(t *testing.T) {
	logPath := installFakeLimactl(t, `#!/bin/sh
printf '%s|TAXIWAY_AGENT=%s\n' "$*" "$TAXIWAY_AGENT" >> "$TAXIWAY_FAKE_LIMACTL_LOG"
if [ "$1" = "shell" ] && [ "$2" = "--workdir=/" ]; then
  exit 0
fi
exit 9
`)
	d := NewLimaDriver(t.TempDir())

	result, err := d.Exec(context.Background(), "taxiway-demo", ExecRequest{
		Workdir: "/lab/work",
		Argv:    []string{"sh", "-c", "echo hi"},
		Env:     map[string]string{"TAXIWAY_AGENT": "codex"},
	})

	require.NoError(t, err)
	require.Equal(t, 9, result.ExitCode)
	require.Equal(t, []string{
		`shell --workdir=/ taxiway-demo -- sh -c sudo mkdir -p /lab/work && sudo chown "$(id -u):$(id -g)" /lab/work|TAXIWAY_AGENT=`,
		"shell --workdir=/lab/work taxiway-demo -- sh -c echo hi|TAXIWAY_AGENT=codex",
	}, readCommandLog(t, logPath))
}

func installFakeLimactl(t *testing.T, script string) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "limactl.log")
	path := filepath.Join(binDir, "limactl")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	t.Setenv("PATH", binDir)
	t.Setenv("TAXIWAY_FAKE_LIMACTL_LOG", logPath)
	return logPath
}

// findRepoRoot walks up from the current working directory until it finds
// a directory containing a go.mod file (the repo root).
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
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
