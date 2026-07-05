package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// ── sanitizeIdentifier ──────────────────────────────────────────────────────

func TestSanitizeIdentifier(t *testing.T) {
	cases := []struct{ in, want string }{
		{"agentic-clm-demo", "agentic_clm_demo"},
		{"my.repo", "my_repo"},
		{"already_fine", "already_fine"},
		{"foo--bar", "foo__bar"}, // no collapse
		{"", ""},
		{"abc123", "abc123"},
		{"with space", "with_space"},
		// UTF-8: é is 2 bytes (0xc3 0xa9), each replaced by '_'; hyphen also replaced
		{"café-app", "caf___app"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := sanitizeIdentifier(c.in); got != c.want {
				t.Errorf("sanitizeIdentifier(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ── repoBasename ─────────────────────────────────────────────────────────────

func TestRepoBasename(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/foo/bar.git", "bar"},
		{"https://github.com/foo/bar", "bar"},
		{"git@github.com:foo/bar.git", "bar"},
		{"git@github.com:foo/bar", "bar"},
		{"ssh://git@github.com/foo/bar.git", "bar"},
		{"ssh://git@github.com/foo/bar", "bar"},
		{"https://github.com/org/my-project.git", "my-project"},
		{"https://gitlab.com/team/sub/repo.git", "repo"},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			got := repoBasename(tc.url)
			require.Equal(t, tc.want, got)
		})
	}
}

// ── validateRepoURL ──────────────────────────────────────────────────────────

func TestValidateRepoURL_Valid(t *testing.T) {
	valid := []string{
		"https://github.com/foo/bar.git",
		"https://github.com/foo/bar",
		"http://github.example.com/foo/bar",
		"git@github.com:foo/bar.git",
		"git@gitlab.com:group/project",
		"ssh://git@github.com/foo/bar",
	}
	for _, u := range valid {
		t.Run(u, func(t *testing.T) {
			require.NoError(t, validateRepoURL(u))
		})
	}
}

func TestValidateRepoURL_Invalid(t *testing.T) {
	invalid := []struct {
		url string
		msg string
	}{
		{"", "empty"},
		{"file:///home/user/repo", "file://"},
		{"file://localhost/repo", "file://"},
		{"./relative/path", "relative"},
		{"/absolute/local/path", "local path"},
		{"not-a-url", "bare name"},
		{"ftp://example.com/repo", "unknown scheme"},
	}
	for _, tc := range invalid {
		t.Run(tc.msg, func(t *testing.T) {
			err := validateRepoURL(tc.url)
			require.Error(t, err, "expected error for %q", tc.url)
		})
	}
}

func TestValidateRepoURL_FileRejected(t *testing.T) {
	err := validateRepoURL("file:///etc/passwd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "file://")
}

func TestBuildBaseEnv_DoesNotInjectHostHQDirByDefault(t *testing.T) {
	t.Setenv("TAXIWAY_HQ_DIR", "")

	ref := config.LabRef{Lab: "mylab", Orch: "gastown"}
	env, err := buildBaseEnv(ref)
	require.NoError(t, err)
	require.NotContains(t, env, "TAXIWAY_HQ_DIR",
		"TAXIWAY_HQ_DIR defaults must be resolved inside the lab, not on the host")
}

func TestBuildBaseEnv_HQDirOverride(t *testing.T) {
	t.Setenv("TAXIWAY_HQ_DIR", "/custom/hq")

	ref := config.LabRef{Lab: "mylab", Orch: "gastown"}
	env, err := buildBaseEnv(ref)
	require.NoError(t, err)
	require.Equal(t, "/custom/hq", env["TAXIWAY_HQ_DIR"])
}

func TestGastownRuntimeScriptsIncludeLocalBinOnPATH(t *testing.T) {
	for _, script := range []string{"workspace.sh", "start.sh", "doctor.sh", "verify.sh"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", script))
		require.NoError(t, err)

		body := string(content)
		require.Contains(t, body, `export PATH="$HOME/.local/bin:$PATH"`,
			"%s must find gt installed by install.sh when run through non-login lab exec", script)
		require.NotContains(t, body, `/usr/local/go/bin`, "%s must not depend on Go being installed", script)
		require.NotContains(t, body, `$HOME/go/bin`, "%s must not depend on Go being installed", script)
	}
}

func TestBootstrapInstallsBashCompletion(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "infra", "commands", "bootstrap.sh"))
	require.NoError(t, err)

	require.Contains(t, string(content), "bash-completion",
		"base labs must load completions installed under /etc/bash_completion.d")
}

func TestGastownInstallUsesSetVersionWithLatestDefault(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "install.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `GASTOWN_VERSION="${TAXIWAY_SET_VERSION:-latest}"`)
	require.Contains(t, script, `GASTOWN_INSTALL_REF="$GASTOWN_VERSION"`)
	require.Contains(t, script, `GASTOWN_INSTALL_REF="v${GASTOWN_VERSION}"`)
	require.Contains(t, script, `install_github_release_binary gastownhall gastown gastown "$GASTOWN_INSTALL_REF" gt`)
	require.NotContains(t, script, `go install`)
	require.NotContains(t, script, `GO_VERSION`)
	require.NotContains(t, script, `go.dev/dl`)
}

func TestGastownInstallUsesReleaseArchivesOnly(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "install.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `resolve_latest_release_ref()`)
	require.Contains(t, script, `install_github_release_binary()`)
	require.Contains(t, script, `https://github.com/${owner}/${repo}/releases/latest`)
	require.NotContains(t, script, `https://api.github.com/repos/${owner}/${repo}/releases/latest`)
	require.Contains(t, script, `download "$url" "$archive"`)
	require.Contains(t, script, `tar -C "$tmpdir" -xzf "$archive"`)
	require.Contains(t, script, `install -m 0755 "$bin" "$target"`)
	require.Contains(t, script, `install_github_release_binary gastownhall gastown gastown "$GASTOWN_INSTALL_REF" gt`)
	require.Contains(t, script, `install_github_release_binary gastownhall beads beads "$BEADS_INSTALL_REF" bd`)
	require.Contains(t, script, `Release archive unavailable for ${owner}/${repo} ${ref}`)
	require.NotContains(t, script, `falling back to go install`)
	require.NotContains(t, script, `$HOME/go/bin`)
	require.NotContains(t, script, `/usr/local/go`)
}

func TestGastownInstallOutputUsesUserFacingToolNames(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "install.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `log "Installing Gas Town"`)
	require.Contains(t, script, `log "Installing Beads"`)
	require.Contains(t, script, `log "Configuring Dolt identity"`)
	require.Contains(t, script, `log "Installed tools"`)
	require.NotContains(t, script, `log "Installed ${owner}/${repo} ${release_ref} from release archive"`)
	require.NotContains(t, script, `log "Configuring Dolt user.name for lab use"`)
	require.NotContains(t, script, `log "Configuring Dolt user.email for lab use"`)
	require.NotContains(t, script, `log "Installed binaries"`)
	require.NotContains(t, script, `log "Done. If 'gt' or 'bd' is not on PATH`)
}

func TestGastownInstallRetriesNetworkDownloads(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "install.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `download()`)
	require.Contains(t, script, `--retry 5 --retry-all-errors --retry-delay 3 --connect-timeout 20`)
	require.Contains(t, script, `download "https://github.com/dolthub/dolt/releases/latest/download/install.sh" /tmp/dolt-install.sh`)
}

func TestGastownInstallSkipsAlreadyInstalledReleaseBinaries(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "install.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `installed_tool_version()`)
	require.Contains(t, script, `if [ "$release_ref" = "latest" ]; then`)
	require.Contains(t, script, `if command -v "$binary" >/dev/null 2>&1; then`)
	require.Contains(t, script, `log "$binary already installed (version: ${current:-unknown}) - skipping"`)
	require.Contains(t, script, `local version="${release_ref#v}"`)
	require.Contains(t, script, `if [ "$current" = "$version" ]; then`)
	require.Contains(t, script, `return 0`)
}

func TestGastownInstallPinsBeadsWithCompatibilityMatrix(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "install.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `BEADS_VERSION="${TAXIWAY_SET_BEADS_VERSION:-}"`)
	require.Contains(t, script, `"1.0.0"|"v1.0.0") BEADS_INSTALL_REF="v1.0.0" ;;`)
	require.Contains(t, script, `"1.0.1"|"v1.0.1") BEADS_INSTALL_REF="v1.0.0" ;;`)
	require.Contains(t, script, `"1.1.0"|"v1.1.0") BEADS_INSTALL_REF="v1.0.4" ;;`)
	require.Contains(t, script, `"1.2.1"|"v1.2.1"|latest) BEADS_INSTALL_REF="v1.0.4" ;;`)
	require.Contains(t, script, `install_github_release_binary gastownhall beads beads "$BEADS_INSTALL_REF" bd`)
	require.NotContains(t, script, `go get "github.com/steveyegge/gastown@${GASTOWN_INSTALL_REF}"`)
	require.NotContains(t, script, `go install`)
}

func TestGastownManifestDocumentsBeadsVersionSetting(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "manifest.yaml"))
	require.NoError(t, err)

	manifest := string(content)
	require.Contains(t, manifest, `Gastown version/tag to install from release archive`)
	require.Contains(t, manifest, `name: beads-version`)
	require.Contains(t, manifest, `Beads version/tag override`)
	require.Contains(t, manifest, "phases:\n      - install")
}

func TestGastownManifestDocumentsModelSetting(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "manifest.yaml"))
	require.NoError(t, err)

	manifest := string(content)
	require.Contains(t, manifest, `name: model`)
	require.Contains(t, manifest, `Full Claude Code model name passed with --model and resolved by LiteLLM.`)
	require.Contains(t, manifest, `default: claude-opus-4-8`)
	require.Contains(t, manifest, "examples:\n      - claude-opus-4-8\n      - claude-sonnet-4-6\n      - claude-haiku-4-5-20251001")
	require.Contains(t, manifest, "phases:\n      - start")
}

func TestGastownBudgetProfileIsValidTownSettings(t *testing.T) {
	settingsPath := filepath.Join("..", "..", "orchestrators", "gastown", "profiles", "budget", "settings", "config.json")
	raw, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(raw, &settings))
	require.Equal(t, "town-settings", settings["type"])
	require.Equal(t, "claude-sonnet", settings["default_agent"])
}

func TestGastownTownSettingsInstalledOnlyFromProfile(t *testing.T) {
	for _, scriptName := range []string{"workspace.sh", "start.sh"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", scriptName))
		require.NoError(t, err)
		script := string(content)
		require.Contains(t, script, `TAXIWAY_ORCH_PROFILE_DIR`)
		require.Contains(t, script, `TAXIWAY_ORCH_PROFILE_CLEAR`)
		require.NotContains(t, script, `SCRIPT_DIR`)
		require.NotContains(t, script, `orchestrators/gastown/settings/config.json`)
	}

	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "workspace.sh"))
	require.NoError(t, err)
	workspaceScript := string(content)
	require.Contains(t, workspaceScript, `$TAXIWAY_ORCH_PROFILE_DIR/settings/config.json`)
	require.Contains(t, workspaceScript, `$HQ_DIR/settings/config.json`)

	content, err = os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "start.sh"))
	require.NoError(t, err)
	startScript := string(content)
	require.Contains(t, startScript, `for file in config.json agents.json`)
	require.Contains(t, startScript, `${TAXIWAY_ORCH_PROFILE_DIR:-}/settings/$file`)
	require.Contains(t, startScript, `$HQ_DIR/settings/$file`)
}

func TestGastownStartConfiguresLiteLLMClaudeCodeAgent(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `. "${HOME}/.config/taxiway/env"`)
	require.Contains(t, script, `GASTOWN_MODEL="${TAXIWAY_SET_MODEL:-claude-opus-4-8}"`)
	require.Contains(t, script, `TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.localhost:4000}"`)
	require.Contains(t, script, `LiteLLM is required for Gas Town`)
	require.Contains(t, script, `settings/agents.json`)
	require.Contains(t, script, `claude-code-litellm`)
	require.Contains(t, script, `default_agent`)
	require.Contains(t, script, `role_agents`)
	require.Contains(t, script, `ANTHROPIC_BASE_URL`)
	require.Contains(t, script, `ANTHROPIC_CUSTOM_HEADERS`)
	require.Contains(t, script, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`)
	require.Contains(t, script, `--dangerously-skip-permissions`)
	require.Contains(t, script, `json.dump`)
	require.NotContains(t, script, `orchestrators/gastown/settings/config.json`)
}

func TestGastownStartPreservesProfileSettingsWhenAddingLiteLLMOverlay(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `town_settings = load_json(config_path)`)
	require.Contains(t, script, `town_settings.setdefault("type", "town-settings")`)
	require.Contains(t, script, `town_settings.setdefault("version", 1)`)
	require.Contains(t, script, `town_settings["default_agent"] = agent_name`)
	require.Contains(t, script, `role_agents = town_settings.get("role_agents")`)
	require.Contains(t, script, `agent_registry = load_json(agents_path)`)
	require.Contains(t, script, `agents = agent_registry.get("agents")`)
	require.Contains(t, script, `agents[agent_name] = {`)
	require.NotContains(t, script, `town_settings = {`)
	require.NotContains(t, script, `agent_registry = {`)
}

func TestGastownStartPreflightsLiteLLMBeforeHQInitialization(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	preflightIdx := requireSubstringIndex(t, script, "require_litellm_env")
	mkdirIdx := requireSubstringIndex(t, script, `mkdir -p "$HQ_DIR"`)
	installIdx := requireSubstringIndex(t, script, `gt install "$HQ_DIR" --git`)
	markerIdx := requireSubstringIndex(t, script, `touch "$MARKER"`)

	require.Less(t, preflightIdx, mkdirIdx)
	require.Less(t, preflightIdx, installIdx)
	require.Less(t, preflightIdx, markerIdx)
}

func TestGastownStartRunsTownLifecycleCommands(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	claudeSettingsRemoveIdx := requireSubstringIndex(t, script, `rm -f "$HQ_DIR/.claude/settings.json"`)
	upIdx := requireSubstringIndexAfter(t, script, "start_gastown", claudeSettingsRemoveIdx)
	crewStartIdx := requireSubstringIndex(t, script, `gt crew start --rig "$TAXIWAY_RIG_NAME"`)
	enableIdx := requireSubstringIndex(t, script, "gt enable")
	shellInstallIdx := requireSubstringIndex(t, script, "gt shell install")
	doctorFixIdx := requireSubstringIndexAfter(t, script, "run_startup_doctor_fix", shellInstallIdx)
	statusIdx := requireSubstringIndex(t, script, "gt status")
	feedIdx := requireSubstringIndexAfter(t, script, "start_feed", statusIdx)
	dashboardIdx := requireSubstringIndexAfter(t, script, "start_dashboard", feedIdx)
	tmuxIdx := requireSubstringIndexAfter(t, script, `tmux new-session -d -s "$SESSION"`, dashboardIdx)

	require.Less(t, claudeSettingsRemoveIdx, upIdx)
	require.Less(t, claudeSettingsRemoveIdx, crewStartIdx)
	require.Less(t, upIdx, crewStartIdx)
	require.Less(t, crewStartIdx, enableIdx)
	require.Less(t, enableIdx, shellInstallIdx)
	require.Less(t, shellInstallIdx, doctorFixIdx)
	require.Less(t, doctorFixIdx, statusIdx)
	require.Less(t, statusIdx, feedIdx)
	require.Less(t, feedIdx, dashboardIdx)
	require.Less(t, statusIdx, dashboardIdx)
	require.Less(t, dashboardIdx, tmuxIdx)
	require.Contains(t, script, `FEED_SESSION="feed"`)
	require.Contains(t, script, `tmux has-session -t "$FEED_SESSION"`)
	require.Contains(t, script, `gt feed`)
	require.Contains(t, script, `DASHBOARD_SESSION="dashboard"`)
	require.Contains(t, script, `dashboard_host_port="${TAXIWAY_DASHBOARD_HOST_PORT:-}"`)
	require.Contains(t, script, `dashboard_port_file="$HQ_DIR/.runtime/dashboard.port"`)
	require.Contains(t, script, `dashboard_host_port="$(<"$dashboard_port_file")"`)
	require.NotContains(t, script, `port_in_use`)
	require.Contains(t, script, `printf '%s\n' "$dashboard_host_port" > "$dashboard_port_file"`)
	require.Contains(t, script, `gt dashboard --bind 127.0.0.1`)
	require.NotContains(t, script, `gt dashboard --bind 127.0.0.1 --port`)
	require.Contains(t, script, `dashboard_url="http://127.0.0.1:$dashboard_host_port"`)
	require.Contains(t, script, `echo "Dashboard: $dashboard_url"`)
	require.Contains(t, script, `gt doctor --fix --no-start`)
	require.NotContains(t, script, `run_gt_doctor`)
	require.Contains(t, script, `start_dir="$HQ_DIR"`)
	require.Contains(t, script, `crew_dir="$HQ_DIR/$TAXIWAY_RIG_NAME/crew/$TAXIWAY_CREW_NAME"`)
	require.Contains(t, script, `start_dir="$crew_dir"`)
	require.NotContains(t, script, `TAXIWAY_WORKSPACE_DIR`)
}

func TestGastownStartWaitsForDaemonHeartbeatBeforeGtUp(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `start_gastown`)
	require.Contains(t, script, `ensure_gastown_daemon_ready`)
	require.Contains(t, script, `wait_for_gastown_daemon_heartbeat`)
	require.Contains(t, script, `gt up`)
	require.Contains(t, script, `gt daemon logs`)
	require.Contains(t, script, `gt daemon start`)
	require.Contains(t, script, `gt daemon status`)
	require.Contains(t, script, `Last heartbeat:`)
	require.Contains(t, script, `Heartbeat complete`)
	require.NotContains(t, script, `TAXIWAY_GASTOWN_START_ATTEMPTS`)
	require.NotContains(t, script, `TAXIWAY_GASTOWN_START_RETRY_DELAY`)
	require.NotContains(t, script, `Gastown startup failed on attempt`)
	require.NotContains(t, script, "\ngt up\n")

	startIdx := requireSubstringIndex(t, script, `start_gastown`)
	ensureIdx := requireSubstringIndexAfter(t, script, `ensure_gastown_daemon_ready`, startIdx)
	upIdx := requireSubstringIndexAfter(t, script, `gt up`, ensureIdx)
	require.Less(t, ensureIdx, upIdx)
}

func TestGastownVerifyDoesNotCreateThrowawayHQ(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "gastown", "verify.sh"))
	require.NoError(t, err)

	script := string(content)
	require.NotContains(t, script, "gt install on a throwaway HQ")
	require.NotContains(t, script, "choose_dolt_port")
	require.NotContains(t, script, "mktemp -d -t gastown-verify")
	require.NotContains(t, script, `"$GT" install "$tmp_hq"`)
}

func TestSingleAgentStartDefaultsToLabWorkWithoutRepo(t *testing.T) {
	for _, orch := range []string{"claude-code", "codex"} {
		t.Run(orch, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", orch, "start.sh"))
			require.NoError(t, err)

			script := string(content)
			require.Contains(t, script, `start_dir="/lab/work"`)
			require.Contains(t, script, `mkdir -p "$start_dir"`)
			require.NotContains(t, script, `start_dir="${HOME}"`)
		})
	}
}

func TestCodexStartConfiguresLiteLLMSubscriptionProvider(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "codex", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `CODEX_MODEL="${TAXIWAY_SET_MODEL:-gpt-5.5}"`)
	require.Contains(t, script, `model_provider = "taxiway-litellm"`)
	require.Contains(t, script, `printf 'model_provider = "taxiway-litellm"\n'`)
	require.Contains(t, script, `printf 'model = "%s"\n' "$CODEX_MODEL"`)
	require.Contains(t, script, `/^model_provider = / { next }`)
	require.Contains(t, script, `/^model = / { next }`)
	require.Contains(t, script, `trusted_project_dirs=("/lab/work")`)
	require.Contains(t, script, `trusted_project_dirs+=("$start_dir")`)
	require.Contains(t, script, `project_headers+=("[projects.\"${toml_trusted_project_dir}\"]")`)
	require.Contains(t, script, `$0 in trusted_headers { skip=1; next }`)
	require.Contains(t, script, `trust_level = "trusted"`)
	require.Contains(t, script, `requires_openai_auth = false`)
	require.Contains(t, script, `TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.localhost:4000}"`)
	require.Contains(t, script, `TAXIWAY_LITELLM_OPENAI_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}/v1"`)
	require.Contains(t, script, `base_url = "${TAXIWAY_LITELLM_OPENAI_BASE_URL}"`)
	require.Contains(t, script, `"x-litellm-api-key" = "TAXIWAY_LITELLM_API_KEY"`)
	require.Contains(t, script, `"x-litellm-agent-id" = "TAXIWAY_LITELLM_AGENT_ID"`)
	require.NotContains(t, script, `"x-litellm-session-id"`)
	require.NotContains(t, script, "langfuse_session_id")
	require.NotContains(t, script, `TAXIWAY_LITELLM_SESSION_ID`)
	require.Contains(t, script, `TAXIWAY_LITELLM_AGENT_ID`)
	require.Contains(t, script, `agent_cmd="codex resume --last || codex"`)
	require.Contains(t, script, `"$agent_cmd"`)
	require.Contains(t, script, `TAXIWAY_SET_MODEL`)
}

func TestClaudeCodeStartPropagatesLiteLLMEnvironment(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", "claude-code", "start.sh"))
	require.NoError(t, err)

	script := string(content)
	require.Contains(t, script, `. "${HOME}/.config/taxiway/env"`)
	require.Contains(t, script, `CLAUDE_CODE_MODEL="${TAXIWAY_SET_MODEL:-claude-opus-4-8}"`)
	require.Contains(t, script, `TAXIWAY_LITELLM_BASE_URL="${TAXIWAY_LITELLM_BASE_URL:-http://${TAXIWAY_LAB:-lab}.litellm.localhost:4000}"`)
	require.Contains(t, script, `export ANTHROPIC_BASE_URL="${TAXIWAY_LITELLM_BASE_URL%/}"`)
	require.Contains(t, script, `export ANTHROPIC_CUSTOM_HEADERS="x-litellm-api-key: Bearer ${TAXIWAY_LITELLM_API_KEY}"`)
	require.NotContains(t, script, `ANTHROPIC_MODEL`)
	require.Contains(t, script, `TAXIWAY_LITELLM_API_KEY`)
	require.Contains(t, script, `export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="${CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:-1}"`)
	require.Contains(t, script, `agent_cmd="claude --model \"$CLAUDE_CODE_MODEL\""`)
	require.Contains(t, script, `"$agent_cmd"`)
	require.Contains(t, script, `tmux_env_args+=(-e "${name}=${!name}")`)
}

func TestAgentAuthScriptsLoadProvisionedTaxiwayEnv(t *testing.T) {
	for _, script := range []string{
		filepath.Join("..", "..", "agents", "claude-code", "auth.sh"),
		filepath.Join("..", "..", "agents", "codex", "auth.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			content, err := os.ReadFile(script)
			require.NoError(t, err)

			text := string(content)
			require.Contains(t, text, `[ -f "${HOME}/.config/taxiway/env" ]`)
			require.Contains(t, text, `. "${HOME}/.config/taxiway/env"`)
			require.Contains(t, text, "TAXIWAY_LITELLM_API_KEY")
		})
	}
}

func TestAgentStartScriptsHaveValidBashSyntax(t *testing.T) {
	for _, script := range []string{
		filepath.Join("..", "..", "orchestrators", "claude-code", "start.sh"),
		filepath.Join("..", "..", "orchestrators", "codex", "start.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			cmd := exec.Command("bash", "-n", script)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		})
	}
}

func TestOrchestratorDoctorsCheckTmuxSession(t *testing.T) {
	for _, tt := range []struct {
		orch    string
		session string
	}{
		{orch: "claude-code", session: "claude-code"},
		{orch: "codex", session: "codex"},
	} {
		t.Run(tt.orch, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "orchestrators", tt.orch, "doctor.sh"))
			require.NoError(t, err)

			script := string(content)
			require.Contains(t, script, "tmux has-session -t "+tt.session)
			require.Contains(t, script, "session "+tt.session+" is running")
			require.Contains(t, script, "session "+tt.session+" is not running")
			require.Contains(t, script, `taxiway up %s --from start --force`)
		})
	}
}

func requireSubstringIndex(t *testing.T, s, substr string) int {
	t.Helper()
	idx := strings.Index(s, substr)
	require.NotEqual(t, -1, idx, "expected %q in script", substr)
	return idx
}

func requireSubstringIndexAfter(t *testing.T, s, substr string, after int) int {
	t.Helper()
	idx := strings.Index(s[after:], substr)
	require.NotEqual(t, -1, idx, "expected %q in script after index %d", substr, after)
	return after + idx
}
