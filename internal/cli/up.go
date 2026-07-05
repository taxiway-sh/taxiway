package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
	"github.com/taxiway-sh/taxiway/internal/phases"
)

// repoURLRe accepts https://, git@host:path, and ssh://host/path.
// Explicitly rejects file:// and bare paths.
var repoURLRe = regexp.MustCompile(`^(https?://|git@[^:/]+:|ssh://[^/]+)`)

// validateRepoURL returns an error if url is not an acceptable git remote URL.
func validateRepoURL(url string) error {
	if url == "" {
		return fmt.Errorf("--repo URL must not be empty")
	}
	if strings.HasPrefix(url, "file://") {
		return fmt.Errorf("--repo must not use a file:// URL (security policy)")
	}
	if !repoURLRe.MatchString(url) {
		return fmt.Errorf("--repo %q is not a valid git URL (must start with https://, git@host:, or ssh://)", url)
	}
	return nil
}

// sanitizeIdentifier replaces every byte outside [A-Za-z0-9_] with '_'.
// Gas Town forbids hyphens, dots, spaces and path separators in rig/crew names.
// The substitution is byte-wise and idempotent; consecutive underscores are NOT collapsed.
// Multi-byte UTF-8 sequences: each individual byte of a multi-byte character falls outside
// [A-Za-z0-9_] and is replaced by '_' independently (e.g. "caf\xc3\xa9-app" → "caf___app").
func sanitizeIdentifier(s string) string {
	b := []byte(s)
	for i, c := range b {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_') {
			b[i] = '_'
		}
	}
	return string(b)
}

// repoBasename extracts the repository name from a git URL.
// Examples:
//
//	"https://github.com/foo/bar.git" → "bar"
//	"https://github.com/foo/bar"     → "bar"
//	"git@github.com:foo/bar.git"     → "bar"
//	"ssh://git@github.com/foo/bar"   → "bar"
func repoBasename(url string) string {
	// Strip trailing .git
	url = strings.TrimSuffix(url, ".git")
	// For git@host:path/to/repo, strip up to and including the colon.
	if idx := strings.LastIndex(url, ":"); idx != -1 && !strings.HasPrefix(url, "https:") && !strings.HasPrefix(url, "http:") && !strings.HasPrefix(url, "ssh:") {
		url = url[idx+1:]
	}
	// Take the last path segment.
	return filepath.Base(url)
}

// ── taxiway up ──────────────────────────────────────────────────────────────────

func newUpCmd(state *RootState) *cobra.Command {
	var (
		orchType      string
		force         bool
		fromPhase     string
		prepareOnly   bool
		skipGateway   bool
		skipWorkspace bool
		dryRunUp      bool
		skipAuthCheck bool
		repo          string
		repoRef       string
		repoPath      string
		profileName   string
		noProfile     bool
		setValues     []string
		clearSet      []string
	)

	cmd := &cobra.Command{
		Use:   "up <lab> [--type <orch>] [flags]",
		Short: "Run prepare then runtime phases",
		Long: `Lifecycle:
  prepare: create -> bootstrap -> install -> verify
  run:     gateway -> workspace -> auth -> start

By default, taxiway up runs prepare then run.

` + labNameHelpText() + `

--type <orch>   required when creating a new lab (defaults to claude-code)
                ignored when resuming — type is read from ref.json instead

auth            checks declared agent authentication during auth and may
                prompt interactively. Use --skip-auth-check only for tests or
                intentionally unmanaged labs.

--profile <name>
                selects an orchestrator profile from
                orchestrators/<orch>/profiles/<name>; omitted means re-use the
                persisted profile, or use orchestrator defaults when none is
                configured

--no-profile    clears the persisted orchestrator profile and returns to
                orchestrator defaults

Each phase is idempotent and can be run individually:
  taxiway bootstrap <lab>
  taxiway install <lab>
  taxiway verify <lab>
  taxiway gateway <lab>
  taxiway workspace <lab>
  taxiway auth <lab>
  taxiway start <lab>

Use --from <phase> to resume from a specific phase.`,
		ValidArgsFunction: completeActiveLabs(state),
		Args:              cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --repo URL if provided.
			if repo != "" {
				if err := validateRepoURL(repo); err != nil {
					return err
				}
			}
			profileSel, err := profileSelectionFromFlags(cmd, profileName, noProfile)
			if err != nil {
				return err
			}
			settingsSel, err := settingsSelectionFromFlags(setValues, clearSet)
			if err != nil {
				return err
			}

			ref, err := makeLabRef(args[0], orchType)
			if err != nil {
				return err
			}

			typeExplicit := cmd.Flags().Changed("type")
			ctx := context.Background()
			id := idName(ref.Lab)
			if existing, ok, rErr := state.Driver.ReadLabRef(ctx, id); ok && rErr == nil {
				if existing.Driver == "" {
					return fmt.Errorf("lab %q is missing driver in ref.json", ref.Lab)
				}
				ref.Driver = existing.Driver
				if existing.Orch != "" && !typeExplicit {
					ref.Orch = existing.Orch
				} else if existing.Orch != "" && existing.Orch != ref.Orch {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"⚠  Lab %q already exists with --type=%s\n   Omit --type to re-use the existing lab, or run: taxiway rm %s first.\n",
						ref.Lab, existing.Orch, ref.Lab,
					)
					return fmt.Errorf(
						"lab %q already exists with orchestrator type %q (requested %q)",
						ref.Lab, existing.Orch, ref.Orch,
					)
				}

				// Repo-switch guard: refuse if existing sidecar has a different repo URL.
				if existing.Workspace != nil && repo != "" && existing.Workspace.Repo != repo {
					return fmt.Errorf(
						"refusing to switch workspace repo for lab %q: existing=%q requested=%q\n"+
							"  To change repos, run: taxiway rm %s && taxiway up %s --repo %s",
						ref.Lab, existing.Workspace.Repo, repo, ref.Lab, ref.Lab, repo,
					)
				}

				// Inherit existing workspace if none requested on this invocation.
				if repo == "" && existing.Workspace != nil {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"Re-using existing workspace config: %s\n", existing.Workspace.Repo,
					)
					ref.Workspace = existing.Workspace
				}
				if !profileSel.set && !profileSel.clear && existing.OrchestratorProfile != nil {
					ref.OrchestratorProfile = existing.OrchestratorProfile
				}
				ref.Settings = existing.Settings
			}
			if ref.Driver == "" {
				ref.Driver = state.Driver.Name()
			}
			// Validate that the resolved orch type has install.sh.
			if _, err := config.InstallScript(state.RepoDir, ref.Orch); err != nil {
				return err
			}

			// Attach workspace to the ref if --repo was given.
			if repo != "" {
				ref.Workspace = &config.Workspace{
					Repo: repo,
					Ref:  repoRef,
					Path: repoPath,
				}
			}

			// B3/I1: Persist workspace config in the sidecar immediately after it
			// is configured, regardless of which phase we start from (--from).
			// This ensures ref.json always reflects the current workspace so that
			// a subsequent `taxiway up` without --repo inherits the correct state.
			//
			// We only write early when the lab already exists (i.e. PhaseCreate has
			// already run). If the lab does not exist yet, PhaseCreate → labUp will
			// call WriteLabRef itself after Create, passing the full ref including
			// the Workspace field. Writing before Create would create the lab
			// directory prematurely, causing Exists to return true and labUp to
			// skip Create and fail trying to start a non-existent lab.
			if ref.Workspace != nil {
				if idExists, _ := state.Driver.Exists(ctx, id); idExists {
					if wErr := state.Driver.WriteLabRef(ctx, id, ref); wErr != nil {
						return fmt.Errorf("persisting workspace config: %w", wErr)
					}
				}
			}
			settingsChanged, err := applySettingsSelection(ctx, state, id, &ref, settingsSel)
			if err != nil {
				return err
			}
			profileChanged, err := applyProfileSelection(ctx, state, id, &ref, profileSel)
			if err != nil {
				return err
			}

			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

			var fromP phases.Phase
			if fromPhase != "" {
				fromP, err = phases.ParsePhase(fromPhase)
				if err != nil {
					return err
				}
			}

			isDryRun := state.Flags.DryRun || dryRunUp

			return runUp(context.Background(), state, ref, id, stateDir, runUpOpts{
				force:                force,
				from:                 fromP,
				prepareOnly:          prepareOnly,
				skipGateway:          skipGateway,
				skipWorkspace:        skipWorkspace,
				dryRun:               isDryRun,
				out:                  cmd.OutOrStdout(),
				skipAuthCheck:        skipAuthCheck,
				showPrepareOnlySkips: true,
				profileChanged:       profileChanged,
				settingsChanged:      settingsChanged,
				profileClear:         profileSel.clear,
			})
		},
	}

	addTypeFlag(cmd, state, &orchType)
	cmd.Flags().BoolVar(&force, "force", false, "re-run all phases even if already done")
	cmd.Flags().StringVar(&fromPhase, "from", "", "resume from phase: create, bootstrap, install, verify, gateway, workspace, auth, start")
	cmd.Flags().BoolVar(&prepareOnly, "prepare-only", false, "run only prepare phases: create, bootstrap, install, verify")
	cmd.Flags().BoolVar(&skipGateway, "skip-gateway", false, "skip the gateway phase")
	cmd.Flags().BoolVar(&skipWorkspace, "skip-workspace", false, "skip the workspace phase")
	cmd.Flags().BoolVar(&skipAuthCheck, "skip-auth-check", false, "skip declared agent authentication checks before start")
	cmd.Flags().BoolVar(&dryRunUp, "dry-run", false, "print phases without executing")
	cmd.Flags().StringVar(&repo, "repo", "", "git URL of the workspace repository")
	cmd.Flags().StringVar(&repoRef, "repo-ref", "", "branch, tag, or SHA to check out")
	cmd.Flags().StringVar(&repoPath, "repo-path", "", "subdirectory inside the workspace repository to use as cwd")
	addProfileFlags(cmd, &profileName, &noProfile)
	addSetFlags(cmd, &setValues, &clearSet)

	return cmd
}

func newPrepareCmd(state *RootState) *cobra.Command {
	var (
		orchType    string
		force       bool
		dryRun      bool
		profileName string
		noProfile   bool
		setValues   []string
		clearSet    []string
	)

	cmd := &cobra.Command{
		Use:               "prepare <lab> [--type <orch>] [flags]",
		Short:             "Run prepare phases: create, bootstrap, install, verify",
		Long:              "Run prepare phases: create -> bootstrap -> install -> verify.\n\n" + labNameHelpText() + "\n\nBecause prepare may create the lab, pass --type <orch>. If omitted, --type defaults to claude-code.\n\nUse --profile <name> to select an orchestrator profile for later runtime phases. Use --no-profile to clear the persisted profile.",
		ValidArgsFunction: completeActiveLabs(state),
		Args:              cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := makeLabRef(args[0], orchType)
			if err != nil {
				return err
			}
			profileSel, err := profileSelectionFromFlags(cmd, profileName, noProfile)
			if err != nil {
				return err
			}
			settingsSel, err := settingsSelectionFromFlags(setValues, clearSet)
			if err != nil {
				return err
			}
			if _, err := config.InstallScript(state.RepoDir, ref.Orch); err != nil {
				return err
			}
			ctx := context.Background()
			id := idName(ref.Lab)
			if existing, ok, rErr := state.Driver.ReadLabRef(ctx, id); ok && rErr == nil {
				if existing.Driver == "" {
					return fmt.Errorf("lab %q is missing driver in ref.json", ref.Lab)
				}
				ref.Driver = existing.Driver
				if existing.Workspace != nil {
					ref.Workspace = existing.Workspace
				}
				if !profileSel.set && !profileSel.clear && existing.OrchestratorProfile != nil {
					ref.OrchestratorProfile = existing.OrchestratorProfile
				}
				ref.Settings = existing.Settings
			}
			if ref.Driver == "" {
				ref.Driver = state.Driver.Name()
			}
			settingsChanged, err := applySettingsSelection(ctx, state, id, &ref, settingsSel)
			if err != nil {
				return err
			}
			profileChanged, err := applyProfileSelection(ctx, state, id, &ref, profileSel)
			if err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return runUp(ctx, state, ref, id, stateDir, runUpOpts{
				force:                force,
				prepareOnly:          true,
				dryRun:               state.Flags.DryRun || dryRun,
				out:                  cmd.OutOrStdout(),
				showPrepareOnlySkips: false,
				profileChanged:       profileChanged,
				settingsChanged:      settingsChanged,
				profileClear:         profileSel.clear,
			})
		},
	}

	addTypeFlag(cmd, state, &orchType)
	cmd.Flags().BoolVar(&force, "force", false, "re-run prepare phases even if already done")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print phases without executing")
	addProfileFlags(cmd, &profileName, &noProfile)
	addSetFlags(cmd, &setValues, &clearSet)

	return cmd
}

func newRunCmd(state *RootState) *cobra.Command {
	var (
		force         bool
		skipGateway   bool
		skipWorkspace bool
		skipAuthCheck bool
		dryRun        bool
		profileName   string
		noProfile     bool
		setValues     []string
		clearSet      []string
	)

	cmd := &cobra.Command{
		Use:               "run <lab> [flags]",
		Short:             "Run runtime phases: gateway, workspace, auth, start",
		Long:              "Run runtime phases: gateway -> workspace -> auth -> start.\n\nUse --profile <name> to update the orchestrator profile before runtime phases. Use --no-profile to clear the persisted profile and return to orchestrator defaults.",
		ValidArgsFunction: completeActiveLabs(state),
		Args:              cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			id := idName(args[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			if ref.Orch == "" {
				return fmt.Errorf("lab %q has no orchestrator type; re-create with: taxiway up %s --type <orch>", args[0], args[0])
			}
			profileSel, err := profileSelectionFromFlags(cmd, profileName, noProfile)
			if err != nil {
				return err
			}
			settingsSel, err := settingsSelectionFromFlags(setValues, clearSet)
			if err != nil {
				return err
			}
			settingsChanged, err := applySettingsSelection(ctx, state, id, &ref, settingsSel)
			if err != nil {
				return err
			}
			profileChanged, err := applyProfileSelection(ctx, state, id, &ref, profileSel)
			if err != nil {
				return err
			}
			stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
			return runUp(ctx, state, ref, id, stateDir, runUpOpts{
				force:                   force,
				from:                    phases.PhaseGateway,
				requirePrepareCompleted: true,
				skipGateway:             skipGateway,
				skipWorkspace:           skipWorkspace,
				dryRun:                  state.Flags.DryRun || dryRun,
				out:                     cmd.OutOrStdout(),
				skipAuthCheck:           skipAuthCheck,
				profileChanged:          profileChanged,
				settingsChanged:         settingsChanged,
				profileClear:            profileSel.clear,
			})
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "re-run run phases even if already done")
	cmd.Flags().BoolVar(&skipGateway, "skip-gateway", false, "skip the gateway phase")
	cmd.Flags().BoolVar(&skipWorkspace, "skip-workspace", false, "skip the workspace phase")
	cmd.Flags().BoolVar(&skipAuthCheck, "skip-auth-check", false, "skip declared agent authentication checks before start")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print phases without executing")
	addProfileFlags(cmd, &profileName, &noProfile)
	addSetFlags(cmd, &setValues, &clearSet)

	return cmd
}

type runUpOpts struct {
	force                   bool
	from                    phases.Phase
	requirePrepareCompleted bool
	prepareOnly             bool
	skipGateway             bool
	skipWorkspace           bool
	dryRun                  bool
	out                     io.Writer
	skipAuthCheck           bool
	showPrepareOnlySkips    bool
	profileChanged          bool
	settingsChanged         bool
	profileClear            bool
}

func runUp(ctx context.Context, state *RootState, ref config.LabRef, id, stateDir string, opts runUpOpts) error {
	if opts.out == nil {
		opts.out = os.Stdout
	}

	startIdx := 0
	if opts.from != "" {
		for i, p := range phases.Order {
			if p == opts.from {
				startIdx = i
				break
			}
		}
	}
	if opts.from != "" && !opts.dryRun {
		if err := preflightResumePrerequisites(ctx, state, id, stateDir, ref, opts, startIdx); err != nil {
			return err
		}
	}

	resumedStoppedLab := false
	for i, phase := range phases.Order {
		if i < startIdx {
			continue
		}
		if !opts.dryRun && phase != phases.PhaseCreate {
			gitDir := filepath.Join(stateDir, ref.Lab, "git")
			if err := os.MkdirAll(gitDir, 0o755); err != nil {
				return fmt.Errorf("phase %s: create git dir for %s: %w", phase, id, err)
			}
			recordingsDir := filepath.Join(stateDir, ref.Lab, "recordings")
			if err := os.MkdirAll(recordingsDir, 0o755); err != nil {
				return fmt.Errorf("phase %s: create recordings dir for %s: %w", phase, id, err)
			}
		}

		if opts.prepareOnly && isRunPhase(phase) {
			if opts.showPrepareOnlySkips {
				fmt.Fprintf(opts.out, "  ⏭  %-20s (--prepare-only)\n", phase)
			}
			continue
		}

		if phase == phases.PhaseGateway && opts.skipGateway {
			fmt.Fprintf(opts.out, "  ⏭  %-20s (--skip-gateway)\n", phase)
			continue
		}

		if phase == phases.PhaseWorkspace && opts.skipWorkspace {
			fmt.Fprintf(opts.out, "  ⏭  %-20s (--skip-workspace)\n", phase)
			continue
		}

		if phase == phases.PhaseWorkspace && !workspaceConfigured(ref) {
			fmt.Fprintln(opts.out, "No repo configured for this lab — skipping workspace phase")
			fmt.Fprintf(opts.out, "  ⏭  %-20s (no repo configured)\n", phase)
			continue
		}

		if phase == phases.PhaseAuth && opts.skipAuthCheck {
			fmt.Fprintf(opts.out, "  ⏭  %-20s (--skip-auth-check)\n", phase)
			continue
		}

		profileMustRun := opts.profileChanged && (phase == phases.PhaseWorkspace || phase == phases.PhaseStart)
		settingsMustRun := opts.settingsChanged && phase != phases.PhaseCreate
		phaseDone := phases.Done(stateDir, id, phase)
		if phase == phases.PhaseCreate && phaseDone && !opts.force && !opts.dryRun {
			d, err := driverForRef(state, ref)
			if err != nil {
				return fmt.Errorf("phase %s: resolve driver: %w", phase, err)
			}
			running, err := d.Running(ctx, id)
			if err != nil {
				return fmt.Errorf("phase %s: check lab state: %w", phase, err)
			}
			if !running {
				phaseDone = false
				resumedStoppedLab = true
			}
		}
		startMustRunAfterResume := resumedStoppedLab && phase == phases.PhaseStart
		authMustRun := phase == phases.PhaseAuth
		if !opts.force && !profileMustRun && !settingsMustRun && !authMustRun && !startMustRunAfterResume && phaseDone {
			if phase == phases.PhaseGateway {
				fmt.Fprintf(opts.out, "  ⏵  %-20s …\n", phase)
				if err := ensureLabLiteLLMSidecarForUp(ctx, state, ref); err != nil {
					return fmt.Errorf("phase %s failed: %w", phase, err)
				}
				fmt.Fprintf(opts.out, "  ✓  %-20s (ready)\n", phase)
				continue
			}
			fmt.Fprintf(opts.out, "  ✓  %-20s (cached)\n", phase)
			continue
		}

		if opts.dryRun {
			fmt.Fprintf(opts.out, "  ⏵  %-20s (dry-run)\n", phase)
			continue
		}

		fmt.Fprintf(opts.out, "  ⏵  %-20s …\n", phase)

		var err error
		if phase == phases.PhaseAuth {
			err = runAuth(ctx, state, ref, false)
		} else {
			err = runPhaseWithProfile(ctx, state, ref, phase, opts.profileClear)
		}
		if err != nil {
			return fmt.Errorf("phase %s failed: %w", phase, err)
		}

		if err := phases.Mark(stateDir, id, phase); err != nil {
			return fmt.Errorf("phase %s: failed to write marker: %w", phase, err)
		}

		fmt.Fprintf(opts.out, "  ✓  %-20s\n", phase)
	}

	return nil
}

func preflightResumePrerequisites(ctx context.Context, state *RootState, id, stateDir string, ref config.LabRef, opts runUpOpts, startIdx int) error {
	if startIdx == 0 {
		return nil
	}
	d, err := driverForRef(state, ref)
	if err != nil {
		return fmt.Errorf("cannot resume from phase %q for lab %q: resolve driver: %w", opts.from, ref.Lab, err)
	}
	exists, err := d.Exists(ctx, id)
	if err != nil {
		return fmt.Errorf("cannot resume from phase %q for lab %q: check lab existence: %w", opts.from, ref.Lab, err)
	}
	if !exists {
		return fmt.Errorf(
			"cannot resume from phase %q for lab %q: missing required earlier phase %q\n"+
				"  Create or repair the lab from the beginning with: %s",
			opts.from, ref.Lab, phases.PhaseCreate, resumeFromBeginningCommand(ref),
		)
	}
	if opts.requirePrepareCompleted && !phases.Done(stateDir, id, phases.PhaseVerify) {
		return fmt.Errorf(
			"cannot run lab %q: missing required prepare phase %q\n"+
				"  Finish prepare first with: %s",
			ref.Lab, phases.PhaseVerify, prepareCommand(ref),
		)
	}
	return nil
}

func prepareCommand(ref config.LabRef) string {
	args := []string{"taxiway", "prepare", ref.Lab}
	if ref.Driver != "" && ref.Driver != "mock" {
		args = append(args, "--driver", ref.Driver)
	}
	if ref.Orch != "" {
		args = append(args, "--type", ref.Orch)
	}
	return strings.Join(args, " ")
}

func resumeFromBeginningCommand(ref config.LabRef) string {
	args := []string{"taxiway", "up", ref.Lab}
	if ref.Driver != "" && ref.Driver != "mock" {
		args = append(args, "--driver", ref.Driver)
	}
	if ref.Orch != "" {
		args = append(args, "--type", ref.Orch)
	}
	args = append(args, "--force")
	return strings.Join(args, " ")
}

func isRunPhase(phase phases.Phase) bool {
	switch phase {
	case phases.PhaseGateway, phases.PhaseWorkspace, phases.PhaseAuth, phases.PhaseStart:
		return true
	default:
		return false
	}
}

// userLookup resolves the host OS username for TAXIWAY_CREW_NAME.
// Tests that modify this variable MUST NOT call t.Parallel().
var userLookup = func() (string, error) {
	u, err := user.Current()
	if err == nil && u.Username != "" {
		return u.Username, nil
	}
	if v := os.Getenv("USER"); v != "" {
		return v, nil
	}
	if v := os.Getenv("LOGNAME"); v != "" {
		return v, nil
	}
	return "", fmt.Errorf(
		"cannot resolve host username for TAXIWAY_CREW_NAME: " +
			"tried os/user.Current(), $USER, $LOGNAME — set USER explicitly",
	)
}

// buildBaseEnv constructs the base environment variables for phase scripts.
// It includes workspace variables when ref.Workspace is non-nil.
// Returns an error if the host username cannot be resolved (gastown only).
func buildBaseEnv(ref config.LabRef) (map[string]string, error) {
	orch := ref.Orch
	env := map[string]string{
		"TAXIWAY_ORCH": orch,
		"TAXIWAY_LAB":  ref.Lab,
		"TAXIWAY_ID":   idName(ref.Lab),
	}
	if hqDir := os.Getenv("TAXIWAY_HQ_DIR"); hqDir != "" {
		env["TAXIWAY_HQ_DIR"] = hqDir
	}
	injectSettingsEnv(env, ref.Settings)

	if ref.Workspace != nil {
		name := repoBasename(ref.Workspace.Repo)
		env["TAXIWAY_REPO_URL"] = ref.Workspace.Repo
		env["TAXIWAY_REPO_REF"] = ref.Workspace.Ref
		env["TAXIWAY_REPO_PATH"] = ref.Workspace.Path
		if ref.Workspace.Fork != "" {
			env["TAXIWAY_REPO_FORK_URL"] = ref.Workspace.Fork
		}

		switch orch {
		case "gastown":
			// Resolve host username for TAXIWAY_CREW_NAME.
			// Lima creates lab users matching the host user by default.
			rawCrew, err := userLookup()
			if err != nil {
				return nil, err
			}
			rigName := sanitizeIdentifier(name)
			crewName := sanitizeIdentifier(rawCrew)
			env["TAXIWAY_RIG_NAME"] = rigName
			env["TAXIWAY_CREW_NAME"] = crewName
			env["TAXIWAY_RIG_SOURCE_URL"] = workspaceBareRepoURL(ref)
			// TAXIWAY_WORKSPACE_DIR is intentionally NOT set here: gastown owns
			// its HQ layout and workspace.sh exports the crew directory.
		default:
			wd := filepath.Join("/lab/work", name)
			if ref.Workspace.Path != "" {
				wd = filepath.Join(wd, ref.Workspace.Path)
			}
			env["TAXIWAY_WORKSPACE_DIR"] = wd
		}
	}

	return env, nil
}

// workspaceScript returns the path to orchestrators/<orch>/workspace.sh.
// Returns ("", nil) when the file is absent (silent skip).
func workspaceScript(repoDir, orch string) (string, error) {
	p := filepath.Join(repoDir, "orchestrators", orch, "workspace.sh")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return p, nil
}

func orchestratorOptionalScript(repoDir, orch, script string) (string, error) {
	p := filepath.Join(repoDir, "orchestrators", orch, script)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return p, nil
}

func manifestAgents(manifest *config.OrchManifest) []string {
	if manifest == nil || len(manifest.Agents) == 0 {
		return nil
	}
	return append([]string(nil), manifest.Agents...)
}

func agentScript(repoDir, agent, script string) (string, error) {
	p := filepath.Join(repoDir, "agents", agent, script)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return p, nil
}

func agentEnv(base map[string]string, agent string) map[string]string {
	env := make(map[string]string, len(base)+1)
	for k, v := range base {
		env[k] = v
	}
	env["TAXIWAY_AGENT"] = agent
	return env
}

func runAgentScripts(ctx context.Context, state *RootState, ref config.LabRef, scriptName string, baseEnv map[string]string) error {
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
		if err := execScriptWithRef(ctx, state, ref, script, agentEnv(baseEnv, agent)); err != nil {
			return fmt.Errorf("agent %q %s: %w", agent, scriptName, err)
		}
	}
	return nil
}

func runDoctor(ctx context.Context, state *RootState, ref config.LabRef, fix bool) error {
	baseEnv, err := buildBaseEnv(ref)
	if err != nil {
		return err
	}
	if fix {
		baseEnv["TAXIWAY_DOCTOR_FIX"] = "true"
	}
	if err := execScriptWithRef(ctx, state, ref, config.DoctorScript(state.RepoDir), baseEnv); err != nil {
		return err
	}

	script, err := orchestratorOptionalScript(state.RepoDir, ref.Orch, "doctor.sh")
	if err != nil {
		return err
	}
	if script != "" {
		if err := execScriptWithRef(ctx, state, ref, script, baseEnv); err != nil {
			return err
		}
	}

	return runAgentScripts(ctx, state, ref, "doctor.sh", baseEnv)
}

// runPhase dispatches to the appropriate driver/exec call for each phase.
func runPhase(ctx context.Context, state *RootState, ref config.LabRef, phase phases.Phase) error {
	return runPhaseWithProfile(ctx, state, ref, phase, false)
}

func runPhaseWithProfile(ctx context.Context, state *RootState, ref config.LabRef, phase phases.Phase, clearProfile bool) error {
	repoDir := state.RepoDir
	orch := ref.Orch

	// PRD FR7: all phase scripts receive the base TAXIWAY_* vars plus workspace vars.
	baseEnv, err := buildBaseEnv(ref)
	if err != nil {
		return err
	}

	switch phase {
	case phases.PhaseCreate:
		return labUp(ctx, state, ref, os.Stderr)

	case phases.PhaseBootstrap:
		return execScriptWithRef(ctx, state, ref, config.BootstrapScript(repoDir), baseEnv)

	case phases.PhaseVerify:
		script, err := config.VerifyScript(repoDir, orch)
		if err == nil {
			if err := execScriptWithRef(ctx, state, ref, script, baseEnv); err != nil {
				return err
			}
		}
		return runAgentScripts(ctx, state, ref, "verify.sh", baseEnv)

	case phases.PhaseInstall:
		script, err := config.InstallScript(repoDir, orch)
		if err != nil {
			return err
		}
		if err := execScriptWithRef(ctx, state, ref, script, baseEnv); err != nil {
			return err
		}
		return runAgentScripts(ctx, state, ref, "install.sh", baseEnv)

	case phases.PhaseGateway:
		return reconcileGateway(ctx, state, ref)

	case phases.PhaseWorkspace:
		if !workspaceConfigured(ref) {
			return nil
		}
		if err := prepareWorkspaceRepository(ctx, state, &ref); err != nil {
			return err
		}
		baseEnv, err = buildBaseEnv(ref)
		if err != nil {
			return err
		}
		script, err := workspaceScript(repoDir, orch)
		if err != nil {
			return err
		}
		if script == "" {
			return nil // no workspace.sh = skip silently
		}
		if err := prepareOrchestratorProfileRuntime(ctx, state, ref, baseEnv, clearProfile); err != nil {
			return err
		}
		return execScriptWithRef(ctx, state, ref, script, baseEnv)

	case phases.PhaseAuth:
		return nil

	case phases.PhaseStart:
		script, err := config.StartScript(repoDir, orch)
		if err != nil {
			return nil // no start.sh = skip silently
		}
		if err := preparePortForward(ctx, state, ref, baseEnv); err != nil {
			return err
		}
		if err := prepareOrchestratorProfileRuntime(ctx, state, ref, baseEnv, clearProfile); err != nil {
			return err
		}
		return execScriptWithRef(ctx, state, ref, script, baseEnv)

	default:
		return fmt.Errorf("unknown phase %q", phase)
	}
}

// ── taxiway down ─────────────────────────────────────────────────────────────────

func newDownCmd(state *RootState) *cobra.Command {
	return &cobra.Command{
		Use:               "down <lab>",
		Short:             "Stop a lab (preserves its state)",
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
			id := idName(args[0])
			fmt.Fprintf(cmd.ErrOrStderr(), "Stopping lab %q\n", args[0])
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			d, err := driverForRef(state, ref)
			if err != nil {
				return err
			}
			if err := d.Stop(ctx, id); err != nil {
				return err
			}
			return stopLabLiteLLMSidecarForDown(ctx, state, ref)
		},
	}
}

// ── taxiway shell ────────────────────────────────────────────────────────────────

func newShellCmd(state *RootState) *cobra.Command {
	var check bool
	var input string
	cmd := &cobra.Command{
		Use:               "shell <lab>",
		Short:             "Open a shell in the lab",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeActiveLabs(state),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "help" {
				return cmd.Help()
			}
			return validateLabArg(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			lab := args[0]
			id := idName(lab)
			ctx := context.Background()

			// Load the ref to get the orch name and the driver that owns this lab.
			ref, err := loadLabRef(ctx, state, id)
			if err != nil {
				return err
			}
			target, err := resolveShellTarget(ctx, state, ref)
			if err != nil {
				return err
			}
			if check {
				if input != "" {
					return fmt.Errorf("use either --check or --input, not both")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Shell target ready: %s\n", target.AttachCommand)
				return nil
			}
			d, err := driverForRef(state, ref)
			if err != nil {
				return err
			}
			if input != "" {
				if err := sendShellInput(ctx, d, id, target, input); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Sent to shell target: %s\n", target.AttachCommand)
				return nil
			}
			return d.ShellExec(ctx, id, target.Workdir, target.Command)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Check that the lab shell target is ready without opening it")
	cmd.Flags().StringVar(&input, "input", "", "Send literal input followed by Enter to the lab shell target")
	return cmd
}

func sendShellInput(ctx context.Context, d driver.Driver, id string, target shellTarget, input string) error {
	if target.SessionName == "" || !target.RequiresTmux {
		return fmt.Errorf("shell --input requires a tmux-backed shell target")
	}
	for _, argv := range [][]string{
		{"tmux", "send-keys", "-l", "-t", target.SessionName, input},
		{"tmux", "send-keys", "-t", target.SessionName, "Enter"},
	} {
		var stdout, stderr strings.Builder
		res, err := d.Exec(ctx, id, driver.ExecRequest{
			Argv:   argv,
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			return fmt.Errorf("shell --input failed: %w", err)
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("shell --input failed: %s", strings.TrimSpace(stderr.String()))
		}
	}
	return nil
}
