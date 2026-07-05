package cli

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/envfile"
	"github.com/taxiway-sh/taxiway/internal/event"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

func newCredentialsCmd(state *RootState) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "credentials",
		Short:       "Manage global Taxiway credentials",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCredentialsCodexCmd(state))
	return cmd
}

func newLabAuthCmd(state *RootState) *cobra.Command {
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "auth <lab> [agent...]",
		Short:             "Run agent authentication interactively",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			if err := validateLabArg(args[0]); err != nil {
				return err
			}
			for _, agent := range args[1:] {
				if err := config.ValidateOrchName(agent); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			id := idName(args[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			if err := runAuth(ctx, state, ref, true, args[1:]...); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return phases.Mark(stateDir, id, phases.PhaseAuth)
		},
	}
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

func newCredentialsCodexCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:         "codex",
		Short:       "Prepare host Codex auth for Taxiway LiteLLM",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipDriver": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCodexAuth(cmd, state)
		},
	}
}

func runCodexAuth(cmd *cobra.Command, state *RootState) error {
	if _, err := ensureLiteLLMChatGPTAuth(authStateDir(state), true); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "LiteLLM will use Codex auth from %s\n", defaultCodexAuthFile())
	return nil
}

func runAuth(ctx context.Context, state *RootState, ref config.LabRef, requireAuth bool, requestedAgents ...string) error {
	agents := requestedAgents
	if len(agents) == 0 {
		manifest, err := config.LoadOrchManifest(state.RepoDir, ref.Orch)
		if err != nil {
			return err
		}
		agents = manifestAgents(manifest)
	}
	if len(agents) == 0 {
		if requireAuth {
			return fmt.Errorf("orchestrator %q declares no agents", ref.Orch)
		}
		return nil
	}

	env, err := buildBaseEnv(ref)
	if err != nil {
		return err
	}

	id := idName(ref.Lab)
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	jsonlPath := driver.EventsJSONLPath(stateDir, id)

	for _, agent := range agents {
		script, err := agentScript(state.RepoDir, agent, "auth.sh")
		if err != nil {
			return err
		}
		if script == "" {
			return fmt.Errorf("auth agent %q has no agents/%s/auth.sh", agent, agent)
		}

		agentEnv := make(map[string]string, len(env)+1)
		for k, v := range env {
			agentEnv[k] = v
		}
		agentEnv["TAXIWAY_AGENT"] = agent
		authMode, err := injectAgentAuthEnv(state.RepoDir, agent, agentEnv)
		if err != nil {
			return err
		}
		if err := validateAgentAuthReadiness(state, agent, agentEnv["TAXIWAY_AUTH_MODE"], authMode); err != nil {
			return err
		}

		labScript, err := hostScriptToLab(state.RepoDir, script)
		if err != nil {
			return fmt.Errorf("auth agent %q: %w", agent, err)
		}

		argv := buildEnvScriptArgv(agentEnv, labScript)

		sink, closeSink, _ := makeExecSink(jsonlPath)
		_ = sink.Handle(ctx, event.Event{
			Type:      event.TypePhase,
			LabName:   id,
			Source:    agent,
			Timestamp: time.Now().UTC(),
			Fields:    map[string]any{"phase": "start"},
		})

		d, err := driverForRef(state, ref)
		if err != nil {
			return err
		}
		if authMode != nil && authMode.CredentialFile != nil {
			if err := bootstrapAgentAuthCredential(ctx, d, ref, agent, *authMode.CredentialFile); err != nil {
				return fmt.Errorf("auth agent %q: %w", agent, err)
			}
		}
		authErr := d.InteractiveExec(ctx, id, driver.InteractiveExecRequest{
			Workdir: LabWorkRoot,
			Argv:    argv,
		})

		_ = sink.Handle(ctx, event.Event{
			Type:      event.TypePhase,
			LabName:   id,
			Source:    agent,
			Timestamp: time.Now().UTC(),
			Fields:    map[string]any{"phase": "done"},
		})
		closeSink()

		if authErr != nil {
			return fmt.Errorf("auth agent %q: %w", agent, authErr)
		}
	}
	return nil
}

func injectAgentAuthEnv(repoDir, agent string, env map[string]string) (*config.AgentAuthMode, error) {
	manifest, err := config.LoadAgentManifest(repoDir, agent)
	if err != nil {
		return nil, err
	}
	if manifest == nil || manifest.Auth == nil {
		return nil, nil
	}
	mode := manifest.Auth.DefaultMode
	if configured := env["TAXIWAY_SET_AUTH_MODE"]; configured != "" {
		mode = configured
	}
	cfg, ok := manifest.Auth.Modes[mode]
	if !ok {
		return nil, fmt.Errorf("agent %q auth mode %q is not declared in agents/%s/manifest.yaml", agent, mode, agent)
	}
	env["TAXIWAY_AUTH_MODE"] = mode
	if cfg.Scope != "" {
		env["TAXIWAY_AUTH_SCOPE"] = cfg.Scope
	}
	return &cfg, nil
}

func validateAgentAuthReadiness(state *RootState, agent, mode string, cfg *config.AgentAuthMode) error {
	if cfg == nil || cfg.Scope != "litellm" || mode != "subscription" {
		return nil
	}
	manifest, err := config.LoadAgentManifest(state.RepoDir, agent)
	if err != nil {
		return err
	}
	if manifest == nil || manifest.LiteLLM == nil {
		return nil
	}
	requiresCodexAuth := false
	for _, provider := range manifest.LiteLLM.Providers {
		if provider == "chatgpt" {
			requiresCodexAuth = true
			break
		}
	}
	if !requiresCodexAuth {
		return nil
	}
	if _, err := os.Stat(liteLLMChatGPTAuthStatePath(authStateDir(state))); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check Codex credentials: %w", err)
	}
	return fmt.Errorf("Codex credentials are not configured\n  Run: taxiway credentials codex")
}

func bootstrapAgentAuthCredential(ctx context.Context, d driver.Driver, ref config.LabRef, agent string, cred config.AgentAuthCredentialFile) error {
	present, err := labAuthCredentialFilePresent(ctx, d, ref, cred.LabPath)
	if err != nil {
		return err
	}
	if present {
		return nil
	}

	src, err := envfile.ExpandHostPath(cred.HostPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read host credential %s: %w", cred.HostPath, err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil
	}

	return copyAgentAuthCredentialFile(ctx, d, ref, agent, src, cred)
}

func labAuthCredentialFilePresent(ctx context.Context, d driver.Driver, ref config.LabRef, labPath string) (bool, error) {
	cmd := fmt.Sprintf("test -s %s", labPathShellExpr(labPath))
	result, err := d.Exec(ctx, idName(ref.Lab), driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", cmd},
	})
	if err != nil {
		return false, fmt.Errorf("check lab credential file %s: %w", labPath, err)
	}
	return result.ExitCode == 0, nil
}

func copyAgentAuthCredentialFile(ctx context.Context, d driver.Driver, ref config.LabRef, agent, src string, cred config.AgentAuthCredentialFile) error {
	mode := cred.Mode
	if mode == "" {
		mode = "0600"
	}
	remoteTmp := "/tmp/taxiway-auth-cred-" + agent + "-" + randomSuffix()
	id := idName(ref.Lab)
	if err := d.Copy(ctx, id, src, remoteTmp); err != nil {
		return fmt.Errorf("copy host credential to lab failed: %w", err)
	}

	var installCmd string
	if strings.HasPrefix(cred.LabPath, "~/") {
		rel := strings.TrimPrefix(cred.LabPath, "~/")
		parentRel := path.Dir(rel)
		installCmd = fmt.Sprintf(
			`mkdir -p "$HOME/%s" && chmod 700 "$HOME/%s" && mv %s "$HOME/%s" && chmod %s "$HOME/%s"`,
			parentRel, parentRel,
			shellQuotePath(remoteTmp),
			rel,
			mode, rel,
		)
	} else {
		parent := path.Dir(cred.LabPath)
		installCmd = fmt.Sprintf(
			`mkdir -p %s && chmod 700 %s && mv %s %s && chmod %s %s`,
			shellQuotePath(parent), shellQuotePath(parent),
			shellQuotePath(remoteTmp), shellQuotePath(cred.LabPath),
			mode, shellQuotePath(cred.LabPath),
		)
	}
	if _, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", installCmd},
	}); err != nil {
		return fmt.Errorf("install lab credential failed: %w", err)
	}
	return nil
}

func buildEnvScriptArgv(env map[string]string, labScript string) []string {
	var argv []string
	if len(env) > 0 {
		argv = append(argv, "env")
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			argv = append(argv, k+"="+env[k])
		}
	}
	return append(argv, "bash", labScript)
}
