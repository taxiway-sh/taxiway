package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

func TestFlatVerbs_CredentialsCodexPreparesGatewayAuth(t *testing.T) {
	root, _, _, stdout, stderr := buildAliasTestRoot(t)
	home := t.TempDir()
	authDir := filepath.Join(t.TempDir(), ".auth")
	t.Setenv("HOME", home)
	t.Setenv("TAXIWAY_AUTH_DIR", authDir)
	sourcePath := filepath.Join(home, ".codex", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o700))
	require.NoError(t, os.WriteFile(sourcePath, []byte(`{"tokens":{"access_token":"access","refresh_token":"refresh","id_token":"id"}}`), 0o600))

	_, _, err := execAlias(t, root, stdout, stderr, "credentials", "codex")

	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(authDir, "providers", "codex", "chatgpt_token", "auth.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"access_token":"access"`)
	require.Contains(t, stdout.String(), "LiteLLM will use Codex auth")
	require.Empty(t, stderr.String())
}

func writeCodexGlobalAuth(t *testing.T) {
	t.Helper()
	authDir := filepath.Join(t.TempDir(), ".auth")
	t.Setenv("TAXIWAY_AUTH_DIR", authDir)
	path := filepath.Join(authDir, "providers", "codex", "chatgpt_token", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"access_token":"access","refresh_token":"refresh"}`), 0o600))
}

func TestFlatVerbs_AuthRunsInteractiveScript(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	setAgents(t, state, "gastown", "claude-code")
	addAuthScript(t, state, "claude-code")

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "gastown")
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 1)
	authArgv := strings.Join(mock.InteractiveExecLog[0].Argv, " ")
	require.Contains(t, authArgv, "auth.sh")
	require.Contains(t, authArgv, "TAXIWAY_ORCH=gastown")
	require.Contains(t, authArgv, "TAXIWAY_AGENT=claude-code")
	require.Equal(t, LabWorkRoot, mock.InteractiveExecLog[0].Workdir)
}

func TestFlatVerbs_AuthCanRunExplicitAgent(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	addAuthScript(t, state, "codex")

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "gastown", "codex")
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 1)
	require.Contains(t, strings.Join(mock.InteractiveExecLog[0].Argv, " "), "TAXIWAY_AGENT=codex")
}

func TestFlatVerbs_AuthInjectsAgentManifestAuthMode(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "codex-lab")
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), idName("codex-lab"), config.LabRef{Lab: "codex-lab", Orch: "codex", Driver: "mock"}))
	setAgents(t, state, "codex", "codex")
	addAuthScript(t, state, "codex")
	writeAgentManifest(t, state, "codex", `name: codex
command: codex
auth:
  default_mode: subscription
  modes:
    subscription:
      scope: litellm
`)
	writeCodexGlobalAuth(t)

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "codex-lab")
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 1)
	authArgv := strings.Join(mock.InteractiveExecLog[0].Argv, " ")
	require.Contains(t, authArgv, "TAXIWAY_AUTH_MODE=subscription")
	require.Contains(t, authArgv, "TAXIWAY_AUTH_SCOPE=litellm")
}

func TestFlatVerbs_AuthFailsWhenCodexSubscriptionAuthMissing(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "codex-lab")
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), idName("codex-lab"), config.LabRef{Lab: "codex-lab", Orch: "codex", Driver: "mock"}))
	setAgents(t, state, "codex", "codex")
	addAuthScript(t, state, "codex")
	writeAgentManifest(t, state, "codex", `name: codex
command: codex
litellm:
  providers:
    - chatgpt
auth:
  default_mode: subscription
  modes:
    subscription:
      scope: litellm
    api-key:
      scope: litellm
`)
	t.Setenv("TAXIWAY_AUTH_DIR", filepath.Join(t.TempDir(), ".auth"))

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "codex-lab")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Codex credentials are not configured")
	require.Contains(t, err.Error(), "taxiway credentials codex")
	require.Empty(t, mock.InteractiveExecLog)
}

func TestFlatVerbs_AuthInjectsConfiguredAuthMode(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "codex-lab")
	require.NoError(t, state.Driver.WriteLabRef(context.Background(), idName("codex-lab"), config.LabRef{Lab: "codex-lab", Orch: "codex", Driver: "mock"}))
	setAgents(t, state, "codex", "codex")
	addAuthScript(t, state, "codex")
	writeAgentManifest(t, state, "codex", `name: codex
command: codex
auth:
  default_mode: subscription
  modes:
    subscription:
      scope: litellm
    api-key:
      scope: litellm
`)

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "codex-lab", "--set", "auth_mode=api-key")
	require.NoError(t, err)
	require.Len(t, mock.InteractiveExecLog, 1)
	authArgv := strings.Join(mock.InteractiveExecLog[0].Argv, " ")
	require.Contains(t, authArgv, "TAXIWAY_AUTH_MODE=api-key")
	require.Contains(t, authArgv, "TAXIWAY_AUTH_SCOPE=litellm")
}

func TestFlatVerbs_AuthCopiesLabScopedCredentialFromHost(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	setAgents(t, state, "gastown", "claude-code")
	addAuthScript(t, state, "claude-code")
	writeAgentManifest(t, state, "claude-code", `name: claude-code
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
`)

	home := t.TempDir()
	t.Setenv("HOME", home)
	hostCredential := filepath.Join(home, ".claude", ".credentials.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(hostCredential), 0755))
	require.NoError(t, os.WriteFile(hostCredential, []byte(`{"ok":true}`), 0600))
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		if strings.Contains(strings.Join(req.Argv, " "), "test -s") {
			return driver.MockExecResponse{ExitCode: 1}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "gastown")
	require.NoError(t, err)
	require.Len(t, mock.CopyLog, 1)
	require.Equal(t, hostCredential, mock.CopyLog[0].Src)
	require.Contains(t, mock.CopyLog[0].Dst, "/tmp/taxiway-auth-cred-claude-code-")
	require.Len(t, mock.InteractiveExecLog, 1)
	require.Contains(t, strings.Join(mock.InteractiveExecLog[0].Argv, " "), "auth.sh")
}

func TestFlatVerbs_AuthDoesNotCopyCredentialWhenLabAlreadyHasIt(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	setAgents(t, state, "gastown", "claude-code")
	addAuthScript(t, state, "claude-code")
	writeAgentManifest(t, state, "claude-code", `name: claude-code
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
`)

	home := t.TempDir()
	t.Setenv("HOME", home)
	hostCredential := filepath.Join(home, ".claude", ".credentials.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(hostCredential), 0755))
	require.NoError(t, os.WriteFile(hostCredential, []byte(`{"ok":true}`), 0600))
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		if strings.Contains(strings.Join(req.Argv, " "), "test -s") {
			return driver.MockExecResponse{ExitCode: 0}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "gastown")
	require.NoError(t, err)
	require.Empty(t, mock.CopyLog)
	require.Len(t, mock.InteractiveExecLog, 1)
}

func TestFlatVerbs_AuthFallsBackToInteractiveWhenHostCredentialMissing(t *testing.T) {
	root, state, mock, stdout, stderr := buildAliasTestRoot(t)
	createAliasLab(t, state, "gastown")
	setAgents(t, state, "gastown", "claude-code")
	addAuthScript(t, state, "claude-code")
	writeAgentManifest(t, state, "claude-code", `name: claude-code
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
`)

	t.Setenv("HOME", t.TempDir())
	mock.ExecResponder = func(_ string, req driver.ExecRequest) driver.MockExecResponse {
		if strings.Contains(strings.Join(req.Argv, " "), "test -s") {
			return driver.MockExecResponse{ExitCode: 1}
		}
		return driver.MockExecResponse{ExitCode: 0}
	}

	_, _, err := execAlias(t, root, stdout, stderr, "auth", "gastown")
	require.NoError(t, err)
	require.Empty(t, mock.CopyLog)
	require.Len(t, mock.InteractiveExecLog, 1)
}
