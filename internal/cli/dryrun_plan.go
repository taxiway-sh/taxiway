package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

type phasePlanOptions struct {
	InspectionAvailable bool
	ClearProfile        bool
}

type dryRunPlan struct {
	out      io.Writer
	finished bool
}

func newDryRunPlan(out io.Writer, kind, name, lab string) *dryRunPlan {
	fmt.Fprintf(out, "Dry-run for %s %q on lab %q\n", kind, name, lab)
	return &dryRunPlan{out: out}
}

func (p *dryRunPlan) Step(component, label string) {
	fmt.Fprintf(p.out, "\n[%s] %s\n", component, label)
}

func (p *dryRunPlan) Detail(detail string) {
	fmt.Fprintf(p.out, "  %s\n", detail)
}

func (p *dryRunPlan) Finish() {
	if p.finished {
		return
	}
	p.finished = true
	fmt.Fprintln(p.out, "\nNo changes were made.")
}

func execOfflinePlanScript(ctx context.Context, scriptPath string, stdout, stderr io.Writer, env map[string]string) error {
	execEnv := append([]string(nil), os.Environ()...)
	execEnv = append(execEnv,
		"TAXIWAY_EXECUTION_MODE=plan",
		"TAXIWAY_PLAN_INSPECTION=unavailable",
		"HOME=/nonexistent",
	)

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		execEnv = append(execEnv, key+"="+env[key])
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath) //nolint:gosec // scriptPath is resolved from the trusted repository tree.
	cmd.Env = execEnv
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func planSinglePhase(ctx context.Context, cmd *cobra.Command, state *RootState, ref config.LabRef, phase phases.Phase) error {
	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	exists, err := d.Exists(ctx, idName(ref.Lab))
	if err != nil {
		return err
	}

	plan := newDryRunPlan(cmd.OutOrStdout(), "phase", string(phase), ref.Lab)
	if err := planPhase(ctx, state, ref, phase, cmd.OutOrStdout(), cmd.ErrOrStderr(), phasePlanOptions{
		InspectionAvailable: exists,
	}); err != nil {
		return err
	}
	plan.Finish()
	return nil
}

func planPhase(ctx context.Context, state *RootState, ref config.LabRef, phase phases.Phase, stdout, stderr io.Writer, opts phasePlanOptions) error {
	baseEnv, err := buildBaseEnv(ref)
	if err != nil {
		return err
	}
	if opts.ClearProfile && (phase == phases.PhaseWorkspace || phase == phases.PhaseStart) {
		(&dryRunPlan{out: stdout}).Step("profile", "Clearing the configured orchestrator profile")
	}

	switch phase {
	case phases.PhaseCreate:
		return planCreate(state, ref, &dryRunPlan{out: stdout})
	case phases.PhaseBootstrap:
		return planScript(ctx, state, ref, config.BootstrapScript(state.RepoDir), stdout, stderr, baseEnv, opts)
	case phases.PhaseInstall:
		script, err := config.InstallScript(state.RepoDir, ref.Orch)
		if err != nil {
			return err
		}
		if err := planScript(ctx, state, ref, script, stdout, stderr, baseEnv, opts); err != nil {
			return err
		}
		return planAgentScripts(ctx, state, ref, "install.sh", stdout, stderr, baseEnv, opts)
	case phases.PhaseVerify:
		script, err := config.VerifyScript(state.RepoDir, ref.Orch)
		if err != nil {
			return err
		}
		if err := planScript(ctx, state, ref, script, stdout, stderr, baseEnv, opts); err != nil {
			return err
		}
		return planAgentScripts(ctx, state, ref, "verify.sh", stdout, stderr, baseEnv, opts)
	case phases.PhaseWorkspace:
		if !workspaceConfigured(ref) {
			(&dryRunPlan{out: stdout}).Step("workspace", "No repository configured; workspace phase would be skipped")
			return nil
		}
		script, err := workspaceScript(state.RepoDir, ref.Orch)
		if err != nil || script == "" {
			return err
		}
		return planScript(ctx, state, ref, script, stdout, stderr, baseEnv, opts)
	case phases.PhaseAuth:
		return planAuth(ctx, state, ref, stdout, stderr, nil, opts)
	case phases.PhaseStart:
		script, err := config.StartScript(state.RepoDir, ref.Orch)
		if err != nil {
			return err
		}
		return planScript(ctx, state, ref, script, stdout, stderr, baseEnv, opts)
	case phases.PhaseGateway:
		return planGateway(state, ref, &dryRunPlan{out: stdout})
	default:
		return fmt.Errorf("phase %q does not have a semantic dry-run plan", phase)
	}
}

func planCreate(state *RootState, ref config.LabRef, plan *dryRunPlan) error {
	driverName := ref.Driver
	if driverName == "" {
		driverName = state.Driver.Name()
	}
	plan.Step("create", fmt.Sprintf("Creating %s lab runtime", driverDisplayName(driverName)))
	plan.Step("create", "Preparing Taxiway lab state")
	plan.Detail("runtime reference, workspace directories, and lifecycle metadata")
	return nil
}

func planGateway(state *RootState, ref config.LabRef, plan *dryRunPlan) error {
	plan.Step("gateway", "Configuring lab gateway environment")
	plan.Detail(fmt.Sprintf("lab: %s", ref.Lab))
	plan.Step("gateway", "Reconciling LiteLLM sidecar")
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	if labGatewayStateExists(stateDir, ref) {
		plan.Detail("existing sidecar state will be inspected and updated")
	} else {
		plan.Detail("sidecar state will be created")
	}
	return nil
}

func planDown(state *RootState, ref config.LabRef, plan *dryRunPlan) error {
	plan.Step("down", "Stopping lab runtime")
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	plan.Step("down", "Stopping LiteLLM sidecar")
	if !labGatewayStateExists(stateDir, ref) {
		plan.Detail("no persisted gateway state is currently present")
	}
	return nil
}

func planRemove(state *RootState, ref config.LabRef, plan *dryRunPlan) error {
	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	if labGatewayStateExists(stateDir, ref) {
		plan.Step("rm", "Removing LiteLLM sidecar")
		plan.Step("rm", "Removing Langfuse project")
	}
	plan.Step("rm", "Deleting lab runtime and storage")
	plan.Step("rm", "Clearing lifecycle phase markers")
	return nil
}

func driverDisplayName(name string) string {
	switch name {
	case "docker":
		return "Docker"
	case "lima":
		return "Lima"
	case "mock":
		return "Mock"
	default:
		return name
	}
}

func planAuth(ctx context.Context, state *RootState, ref config.LabRef, stdout, stderr io.Writer, requestedAgents []string, opts phasePlanOptions) error {
	agents := requestedAgents
	if len(agents) == 0 {
		manifest, err := config.LoadOrchManifest(state.RepoDir, ref.Orch)
		if err != nil {
			return err
		}
		agents = manifestAgents(manifest)
	}
	baseEnv, err := buildBaseEnv(ref)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		script, err := agentScript(state.RepoDir, agent, "auth.sh")
		if err != nil {
			return err
		}
		if script == "" {
			return fmt.Errorf("auth agent %q has no agents/%s/auth.sh", agent, agent)
		}
		env := agentEnv(baseEnv, agent)
		if _, err := injectAgentAuthEnv(state.RepoDir, agent, env); err != nil {
			return err
		}
		if err := planScript(ctx, state, ref, script, stdout, stderr, env, opts); err != nil {
			return fmt.Errorf("auth agent %q plan: %w", agent, err)
		}
	}
	return nil
}

func planScript(ctx context.Context, state *RootState, ref config.LabRef, script string, stdout, stderr io.Writer, env map[string]string, opts phasePlanOptions) error {
	if opts.InspectionAvailable {
		return execPlannableScriptToWithRef(ctx, state, ref, script, stdout, stderr, env)
	}
	return execOfflinePlanScript(ctx, script, stdout, stderr, env)
}

func planAgentScripts(ctx context.Context, state *RootState, ref config.LabRef, scriptName string, stdout, stderr io.Writer, baseEnv map[string]string, opts phasePlanOptions) error {
	manifest, err := config.LoadOrchManifest(state.RepoDir, ref.Orch)
	if err != nil {
		return err
	}
	for _, agent := range manifestAgents(manifest) {
		script, err := agentScript(state.RepoDir, agent, scriptName)
		if err != nil {
			return err
		}
		if script == "" {
			return fmt.Errorf("agent %q has no agents/%s/%s", agent, agent, scriptName)
		}
		if err := planScript(ctx, state, ref, script, stdout, stderr, agentEnv(baseEnv, agent), opts); err != nil {
			return fmt.Errorf("agent %q %s plan: %w", agent, scriptName, err)
		}
	}
	return nil
}
