package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOrchName(t *testing.T) {
	valid := []string{"codex", "gastown", "g_a-1", "MY-ORCH", "a1b2c3"}
	for _, v := range valid {
		require.NoError(t, ValidateOrchName(v), "expected valid: %s", v)
	}

	invalid := []string{"", "../evil", "bad name", "bad/slash", "bad;cmd", "../../etc"}
	for _, v := range invalid {
		require.Error(t, ValidateOrchName(v), "expected invalid: %s", v)
	}
}

func TestIDOf(t *testing.T) {
	require.Equal(t, "taxiway-mon-lab", IDOf("mon-lab"))
	require.Equal(t, "taxiway-gastown", IDOf("gastown"))
}

func TestRuntimeIDOfUsesCurrentRuntimeContext(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")

	require.Equal(t, "taxiway-mon-lab", RuntimeIDOf("mon-lab"))

	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")

	require.Equal(t, "taxiway-dev-a1b2c3d4-mon-lab", RuntimeIDOf("mon-lab"))
	require.Equal(t, "taxiway-dev-a1b2c3d4-mon-lab", RuntimeIDOf("dev-a1b2c3d4-mon-lab"))
	require.Equal(t, "taxiway-dev-a1b2c3d4-mon-lab", RuntimeIDOf("taxiway-dev-a1b2c3d4-mon-lab"))
}

func TestLabDirOfStripsCurrentRuntimeContext(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "dev")
	t.Setenv("TAXIWAY_CONTEXT_ID", "a1b2c3d4")

	require.Equal(t, "mon-lab", LabDirOf("taxiway-dev-a1b2c3d4-mon-lab"))
	require.Equal(t, "mon-lab", LabDirOf("taxiway-mon-lab"))
	require.Equal(t, "mon-lab", LabDirOf("mon-lab"))
}

func TestStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// override takes precedence
	require.Equal(t, "/tmp/custom", StateDir("/tmp/custom", "/repo"))

	// env var
	t.Setenv("TAXIWAY_LAB_STATE_DIR", "/tmp/env-state")
	require.Equal(t, "/tmp/env-state", StateDir("", "/repo"))
	os.Unsetenv("TAXIWAY_LAB_STATE_DIR")

	// default
	require.Equal(t, filepath.Join(home, ".taxiway", "lab-state"), StateDir("", "/repo"))
}

func TestRuntimeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.Equal(t, "/tmp/runtime", RuntimeDir("/tmp/runtime", "1.2.3"))

	t.Setenv("TAXIWAY_RUNTIME_DIR", "/tmp/env-runtime")
	require.Equal(t, "/tmp/env-runtime", RuntimeDir("", "1.2.3"))
	os.Unsetenv("TAXIWAY_RUNTIME_DIR")

	require.Equal(t, filepath.Join(home, ".taxiway", "runtime"), RuntimeDir("", "1.2.3"))
}

func TestObservabilityDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.Equal(t, filepath.Join(home, ".taxiway", "observability"), ObservabilityDir())
}

func TestInstallScript(t *testing.T) {
	tmp := t.TempDir()
	orchDir := filepath.Join(tmp, "orchestrators", "testorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))

	// not found
	_, err := InstallScript(tmp, "testorch")
	require.Error(t, err)

	// found
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "install.sh"), []byte("#!/bin/bash\n"), 0755))
	p, err := InstallScript(tmp, "testorch")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmp, "orchestrators", "testorch", "install.sh"), p)
}

func TestInfraCommandScripts(t *testing.T) {
	tmp := t.TempDir()

	require.Equal(t, filepath.Join(tmp, "infra", "commands", "bootstrap.sh"), BootstrapScript(tmp))
	require.Equal(t, filepath.Join(tmp, "infra", "commands", "doctor.sh"), DoctorScript(tmp))
	require.Equal(t, filepath.Join(tmp, "infra", "commands", "reset.sh"), ResetScript(tmp))
}

func TestLoadOrchManifest_Absent(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "myorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))

	m, err := LoadOrchManifest(dir, "myorch")
	require.NoError(t, err)
	assert.Nil(t, m, "absent manifest.yaml should return nil without error")
}

func TestLoadOrchManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "myorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := "name: myorch\ndescription: A test orchestrator\ndocs_url: https://example.com/docs\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadOrchManifest(dir, "myorch")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "myorch", m.Name)
	assert.Equal(t, "A test orchestrator", m.Description)
	assert.Equal(t, "https://example.com/docs", m.DocsURL)
}

func TestLoadOrchManifest_WithAgents(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "gastown")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := "name: gastown\nagents:\n  - claude-code\n  - codex\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadOrchManifest(dir, "gastown")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, []string{"claude-code", "codex"}, m.Agents)
}

func TestLoadAgentManifest_WithAuthModes(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents", "codex")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	content := `name: codex
command: codex
auth:
  default_mode: subscription
  modes:
    subscription:
      scope: litellm
    api-key:
      scope: litellm
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadAgentManifest(dir, "codex")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "codex", m.Name)
	assert.Equal(t, "codex", m.Command)
	require.NotNil(t, m.Auth)
	assert.Equal(t, "subscription", m.Auth.DefaultMode)
	assert.Equal(t, "litellm", m.Auth.Modes["subscription"].Scope)
	assert.Equal(t, "litellm", m.Auth.Modes["api-key"].Scope)
}

func TestLoadAgentManifest_WithAuthCredentialFile(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents", "claude-code")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	content := `name: claude-code
command: claude
auth:
  default_mode: subscription
  modes:
    subscription:
      scope: lab
      credential_file:
        host_path: ~/.claude/.credentials.json
        lab_path: ~/.claude/.credentials.json
        mode: "0600"
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadAgentManifest(dir, "claude-code")
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Auth.Modes["subscription"].CredentialFile)
	assert.Equal(t, "~/.claude/.credentials.json", m.Auth.Modes["subscription"].CredentialFile.HostPath)
	assert.Equal(t, "~/.claude/.credentials.json", m.Auth.Modes["subscription"].CredentialFile.LabPath)
	assert.Equal(t, "0600", m.Auth.Modes["subscription"].CredentialFile.Mode)
}

func TestLoadOrchManifest_WithSettings(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "gastown")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := `name: gastown
description: Gas Town
settings:
  - name: version
    description: Gastown version/tag to install from release archive
    default: latest
    examples: ["1.1.0", "latest"]
    phases: [install]
`
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadOrchManifest(dir, "gastown")
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Len(t, m.Settings, 1)
	assert.Equal(t, "version", m.Settings[0].Name)
	assert.Equal(t, "latest", m.Settings[0].Default)
	assert.Equal(t, []string{"1.1.0", "latest"}, m.Settings[0].Examples)
	assert.Equal(t, []string{"install"}, m.Settings[0].Phases)
}

func TestLoadOrchManifest_InvalidAgentRejected(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "gastown")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := "name: gastown\nagents:\n  - ../codex\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	_, err := LoadOrchManifest(dir, "gastown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid agent")
}

func TestLoadOrchManifest_SecretsFieldRejected(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "myorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := "name: myorch\nsecrets:\n  - name: MY_KEY\n    required: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	_, err := LoadOrchManifest(dir, "myorch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secrets:")
	assert.Contains(t, err.Error(), "not supported")
}

func TestLoadOrchManifest_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "myorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	// tab where YAML expects spaces triggers a parse error
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte("name:\n\t- bad"), 0644))

	_, err := LoadOrchManifest(dir, "myorch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot parse")
}

func TestLoadOrchManifest_WithShellCommand(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "gastown")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := "name: gastown\ndescription: Gas Town\nshell:\n  command: [\"gt\", \"mayor\", \"attach\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadOrchManifest(dir, "gastown")
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Shell)
	assert.Equal(t, []string{"gt", "mayor", "attach"}, m.Shell.Command)
}

func TestLoadOrchManifest_ShellEmptyCommandRejected(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "badorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	// shell: present but command is empty list
	content := "name: badorch\nshell:\n  command: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	_, err := LoadOrchManifest(dir, "badorch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shell.command")
	assert.Contains(t, err.Error(), "non-empty")
}

func TestLoadOrchManifest_WithoutShell(t *testing.T) {
	dir := t.TempDir()
	orchDir := filepath.Join(dir, "orchestrators", "myorch")
	require.NoError(t, os.MkdirAll(orchDir, 0755))
	content := "name: myorch\ndescription: plain\n"
	require.NoError(t, os.WriteFile(filepath.Join(orchDir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadOrchManifest(dir, "myorch")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Nil(t, m.Shell, "shell should be nil when not specified")
}

// ---- LabRef tests ----

func TestLabRef_ID(t *testing.T) {
	require.Equal(t, "taxiway-mon-lab", LabRef{Lab: "mon-lab", Orch: "claude-code"}.ID())
	require.Equal(t, "taxiway-gastown", LabRef{Lab: "gastown", Orch: "gastown"}.ID())
}

func TestLabNameFromID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "multi-word lab", id: "taxiway-mon-lab", want: "mon-lab"},
		{name: "single-word lab", id: "taxiway-gastown", want: "gastown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LabNameFromID(tt.id)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestLabNameFromIDRequiresPrefix(t *testing.T) {
	_, err := LabNameFromID("noprefix")
	require.EqualError(t, err, `invalid lab id "noprefix": expected prefix "taxiway-"`)
}

func TestValidateLabName(t *testing.T) {
	valid := []string{"mon-lab", "lab1", "my_lab", "Lab123", "a"}
	for _, n := range valid {
		require.NoError(t, ValidateLabName(n), "expected valid: %s", n)
	}
	invalid := []string{"", "bad name", "bad!", "a/b", "../../etc", "help", "version"}
	for _, n := range invalid {
		require.Error(t, ValidateLabName(n), "expected invalid: %s", n)
	}
}

func TestValidateLabName_MaxLength(t *testing.T) {
	require.NoError(t, ValidateLabName(strings.Repeat("a", 48)))
	require.EqualError(t, ValidateLabName(strings.Repeat("a", 49)), "invalid lab name: must be 48 characters or fewer")
}
