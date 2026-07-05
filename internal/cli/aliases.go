package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

var (
	stopLabLiteLLMSidecarForRm    = stopLabLiteLLMSidecar
	removeLabLiteLLMSidecarForRm  = removeLabLiteLLMSidecar
	removeLabLangfuseProjectForRm = removeLabLangfuseProject
)

// newCreateCmd creates and starts the lab for a lab.
func newCreateCmd(state *RootState) *cobra.Command {
	var orchType string
	cmd := &cobra.Command{
		Use:               "create <lab> [--type <orch>]",
		Short:             "Create the lab",
		Long:              "Create the lab.\n\n" + labNameHelpText() + "\n\nPass --type <orch> to choose the orchestrator type. If omitted, --type defaults to claude-code.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ref, err := makeLabRef(a[0], orchType)
			if err != nil {
				return err
			}
			ctx := context.Background()
			if err := labUp(ctx, state, ref, os.Stderr); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return phases.Mark(stateDir, idName(ref.Lab), phases.PhaseCreate)
		},
	}
	addTypeFlag(cmd, state, &orchType)
	return cmd
}

// newListCmd: taxiway list [lab]
func newListCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:               "list [lab]",
		Aliases:           []string{"ls"},
		Short:             "List labs (or show one lab if <lab> is provided)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if len(args) == 0 {
				return printList(ctx, state, cmd.OutOrStdout())
			}
			if err := validateLabArg(args[0]); err != nil {
				return err
			}
			return printList(ctx, state, cmd.OutOrStdout(), args[0])
		},
	}
}

// newRmCmd: taxiway rm <lab> — delete lab runtime state + clear phase markers
func newRmCmd(state *RootState) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:               "rm <lab>",
		Short:             "Delete a lab",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			lab := args[0]
			id := idName(lab)
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			hasFork := ref.Workspace != nil && ref.Workspace.Fork != "" && isGitHubURL(ref.Workspace.Fork)
			if hasFork {
				forkName := repoBasename(ref.Workspace.Fork)
				fmt.Fprintf(cmd.OutOrStdout(), "Warning: lab %q has a workspace fork that must be deleted manually:\n  Repo: %s\n  URL:  %s\n\n", lab, forkName, ref.Workspace.Fork)
			}
			if !yes {
				fmt.Fprintf(cmd.OutOrStdout(), "Delete lab %q? [y/N] ", lab)
				scanner := bufio.NewScanner(os.Stdin)
				scanner.Scan()
				if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Deleting lab %q\n", lab)
			d, err := driverForRef(state, ref)
			if err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			if labGatewayStateExists(stateDir, ref) {
				fmt.Fprintf(cmd.ErrOrStderr(), "\nStopping LiteLLM sidecar for lab %q\n", lab)
				if err := stopLabLiteLLMSidecarForRm(ctx, state, ref); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "\nRemoving Langfuse project for lab %q\n", lab)
				if err := removeLabLangfuseProjectForRm(state, stateDir, ref); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "\nRemoving LiteLLM sidecar state for lab %q\n", lab)
				if err := removeLabLiteLLMSidecarForRm(ctx, state, ref); err != nil {
					return err
				}
				fmt.Fprintln(cmd.ErrOrStderr())
			}
			if err := d.Delete(ctx, id); err != nil {
				return err
			}
			remindManualWorkspaceForkCleanup(cmd.ErrOrStderr(), ref)
			if err := phases.ClearAll(stateDir, id); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func labGatewayStateExists(stateDir string, ref config.LabRef) bool {
	if phases.Done(stateDir, idName(ref.Lab), phases.PhaseGateway) {
		return true
	}
	_, err := os.Stat(labGatewayDir(stateDir, ref))
	return err == nil
}

// newStartCmd: taxiway start <lab>
func newStartCmd(state *RootState) *cobra.Command {
	var force bool
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "start <lab>",
		Short:             "Start lab runtime sessions",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			if ref.Orch == "" {
				return fmt.Errorf("lab %q has no orchestrator type; re-create with: taxiway up %s --type <orch>", a[0], a[0])
			}
			script, err := config.StartScript(state.RepoDir, ref.Orch)
			if err != nil {
				return err
			}
			env, err := buildBaseEnv(ref)
			if err != nil {
				return err
			}
			if force {
				env["TAXIWAY_FORCE"] = "true"
			}
			if err := preparePortForward(ctx, state, ref, env); err != nil {
				return err
			}
			if err := execScriptWithRef(ctx, state, ref, script, env); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return phases.Mark(stateDir, id, phases.PhaseStart)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "reinitialize even if already initialized")
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

func newGatewayCmd(state *RootState) *cobra.Command {
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "gateway <lab>",
		Short:             "Reconcile lab gateway access",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			if err := reconcileGateway(ctx, state, ref); err != nil {
				return err
			}
			return phases.Mark(stateDir, id, phases.PhaseGateway)
		},
	}
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

// newWorkspaceCmd: taxiway workspace <lab>
func newWorkspaceCmd(state *RootState) *cobra.Command {
	var repo, repoRef, repoPath string
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "workspace <lab>",
		Short:             "Prepare the lab workspace",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			if err := applyWorkspaceFlags(ctx, state, id, &ref, repo, repoRef, repoPath); err != nil {
				return err
			}
			if !workspaceConfigured(ref) {
				fmt.Fprintln(cmd.OutOrStdout(), "No repo configured for this lab — skipping workspace phase")
				return nil
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			if err := runPhase(ctx, state, ref, phases.PhaseWorkspace); err != nil {
				return err
			}
			return phases.Mark(stateDir, id, phases.PhaseWorkspace)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "git URL of the workspace repository")
	cmd.Flags().StringVar(&repoRef, "repo-ref", "", "branch, tag, or SHA to check out")
	cmd.Flags().StringVar(&repoPath, "repo-path", "", "subdirectory inside the workspace repository to use as cwd")
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

func applyWorkspaceFlags(ctx context.Context, state *RootState, id string, ref *config.LabRef, repo, repoRef, repoPath string) error {
	if repo == "" {
		return nil
	}
	if err := validateRepoURL(repo); err != nil {
		return err
	}
	if ref.Workspace != nil && ref.Workspace.Repo != "" && ref.Workspace.Repo != repo {
		return fmt.Errorf(
			"refusing to switch workspace repo for lab %q: existing=%q requested=%q\n"+
				"  To change repos, run: taxiway rm %s && taxiway workspace %s --repo %s",
			ref.Lab, ref.Workspace.Repo, repo, ref.Lab, ref.Lab, repo,
		)
	}
	ref.Workspace = &config.Workspace{
		Repo: repo,
		Ref:  repoRef,
		Path: repoPath,
	}
	if err := state.Driver.WriteLabRef(ctx, id, *ref); err != nil {
		return fmt.Errorf("persisting workspace config: %w", err)
	}
	return nil
}

// newInstallCmd: taxiway install <lab>
func newInstallCmd(state *RootState) *cobra.Command {
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "install <lab>",
		Short:             "Install the lab orchestrator and agents",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			if ref.Orch == "" {
				return fmt.Errorf("lab %q has no orchestrator type; re-create with: taxiway up %s --type <orch>", a[0], a[0])
			}
			if err := runPhase(ctx, state, ref, phases.PhaseInstall); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return phases.Mark(stateDir, id, phases.PhaseInstall)
		},
	}
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

// newVerifyCmd: taxiway verify <lab>
func newVerifyCmd(state *RootState) *cobra.Command {
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "verify <lab>",
		Short:             "Verify the lab orchestrator and agents",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			if ref.Orch == "" {
				return fmt.Errorf("lab %q has no orchestrator type", a[0])
			}
			env, err := buildBaseEnv(ref)
			if err != nil {
				return err
			}
			script, err := config.VerifyScript(state.RepoDir, ref.Orch)
			if err != nil {
				return err
			}
			if err := execScriptWithRef(ctx, state, ref, script, env); err != nil {
				return err
			}
			if err := runAgentScripts(ctx, state, ref, "verify.sh", env); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return phases.Mark(stateDir, id, phases.PhaseVerify)
		},
	}
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

// newBootstrapCmd: taxiway bootstrap <lab>
func newBootstrapCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:               "bootstrap <lab>",
		Short:             "Install system dependencies in the lab",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if err := execScriptWithRef(ctx, state, ref, config.BootstrapScript(state.RepoDir), nil); err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return phases.Mark(stateDir, id, phases.PhaseBootstrap)
		},
	}
}

// newDoctorCmd: taxiway doctor <lab>
func newDoctorCmd(state *RootState) *cobra.Command {
	var fix bool
	var setValues, clearSet []string
	cmd := &cobra.Command{
		Use:               "doctor <lab>",
		Short:             "Diagnose the lab environment",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if _, err := applySettingsFromFlags(ctx, state, id, &ref, setValues, clearSet); err != nil {
				return err
			}
			return runDoctor(ctx, state, ref, fix)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "attempt doctor fixes when supported")
	addSetFlags(cmd, &setValues, &clearSet)
	return cmd
}

// newResetCmd: taxiway reset <lab>
func newResetCmd(state *RootState) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:               "reset <lab>",
		Short:             "Reset the lab and clear phase markers",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, a []string) error {
			if a[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(a[0])
		},
		RunE: func(_ *cobra.Command, a []string) error {
			ctx := context.Background()
			id := idName(a[0])
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			if err := phases.ClearAll(stateDir, id); err != nil {
				return err
			}
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			var env map[string]string
			if yes {
				env = map[string]string{"LAB_RESET_YES": "1"}
			}
			return execScriptWithRef(ctx, state, ref, config.ResetScript(state.RepoDir), env)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}
