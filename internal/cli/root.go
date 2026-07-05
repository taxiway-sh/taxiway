// Package cli wires cobra commands and global flags.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

var (
	Version   = "0.1.0"
	Commit    = "dev"
	BuildDate = "unknown"
)

const (
	commandGroupLifecycle       = "lifecycle"
	commandGroupLifecyclePhases = "lifecycle-phases"
	commandGroupLabs            = "labs"
	commandGroupRuntime         = "runtime"
	commandGroupUtility         = "utility"
)

// GlobalFlags holds parsed values of PersistentFlags.
type GlobalFlags struct {
	DriverName string
	DryRun     bool
	StateDir   string
	Verbose    bool
}

// RootState is threaded through cobra commands via closures.
type RootState struct {
	Flags         GlobalFlags
	RepoDir       string // Runtime asset root; kept named RepoDir until call sites are renamed.
	Driver        driver.Driver
	Observability observabilityRuntime
	Proxy         proxyRuntime
}

// Execute builds the root command and runs it.
func Execute() {
	state := &RootState{}

	root := &cobra.Command{
		Use:           "taxiway",
		Short:         "Taxiway lab operations CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip driver init for built-in cobra commands and no-driver commands.
			skip := map[string]bool{"version": true, "completion": true, "help": true}
			if skip[cmd.Name()] {
				return nil
			}
			// Commands annotated with skipDriver="true" don't need a driver.
			if cmd.Annotations["skipDriver"] == "true" {
				return nil
			}
			return initDriver(state)
		},
	}

	root.PersistentFlags().StringVar(&state.Flags.DriverName, "driver", "", "driver: lima|docker (default: auto)")
	root.PersistentFlags().BoolVar(&state.Flags.DryRun, "dry-run", false, "print driver calls without executing")
	root.PersistentFlags().StringVar(&state.Flags.StateDir, "state-dir", "", "override TAXIWAY_LAB_STATE_DIR")
	root.PersistentFlags().BoolVarP(&state.Flags.Verbose, "verbose", "v", false, "verbose logging")

	// RepoDir is currently the runtime asset root. It no longer follows the
	// caller's working directory; release builds install these assets under
	// ~/.taxiway/runtime by default.
	state.RepoDir = config.RuntimeDir("", Version)

	root.AddCommand(
		newUpCmd(state),
		newPrepareCmd(state),
		newRunCmd(state),
		newCreateCmd(state),
		newBootstrapCmd(state),
		newInstallCmd(state),
		newVerifyCmd(state),
		newGatewayCmd(state),
		newWorkspaceCmd(state),
		newLabAuthCmd(state),
		newStartCmd(state),
		newListCmd(state),
		newRecordCmd(state),
		newShellCmd(state),
		newDoctorCmd(state),
		newDownCmd(state),
		newRmCmd(state),
		newResetCmd(state),
		newInitCmd(state),
		newStatusCmd(state),
		newAccessCmd(state),
		newRepairCmd(state),
		newDestroyCmd(state),
		newCredentialsCmd(state),
		newObserveCmd(state),
		newDescribeCmd(state),
		newVersionCmd(state),
	)
	configureCommandGroups(root)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func topLevelCommandName(cmd *cobra.Command) string {
	for cmd.HasParent() && cmd.Parent().Name() != "taxiway" {
		cmd = cmd.Parent()
	}
	return cmd.Name()
}

func configureCommandGroups(root *cobra.Command) {
	cobra.EnableCommandSorting = false
	root.AddGroup(
		&cobra.Group{ID: commandGroupLifecycle, Title: "Lifecycle:"},
		&cobra.Group{ID: commandGroupLifecyclePhases, Title: "Lifecycle Phases:"},
		&cobra.Group{ID: commandGroupLabs, Title: "Labs:"},
		&cobra.Group{ID: commandGroupRuntime, Title: "Runtime:"},
		&cobra.Group{ID: commandGroupUtility, Title: "Utility:"},
	)
	root.SetHelpCommandGroupID(commandGroupUtility)
	root.SetCompletionCommandGroupID(commandGroupUtility)

	groupByName := map[string]string{
		"up":          commandGroupLifecycle,
		"prepare":     commandGroupLifecycle,
		"run":         commandGroupLifecycle,
		"create":      commandGroupLifecyclePhases,
		"bootstrap":   commandGroupLifecyclePhases,
		"install":     commandGroupLifecyclePhases,
		"verify":      commandGroupLifecyclePhases,
		"gateway":     commandGroupLifecyclePhases,
		"workspace":   commandGroupLifecyclePhases,
		"auth":        commandGroupLifecyclePhases,
		"start":       commandGroupLifecyclePhases,
		"list":        commandGroupLabs,
		"record":      commandGroupLabs,
		"shell":       commandGroupLabs,
		"doctor":      commandGroupLabs,
		"down":        commandGroupLabs,
		"rm":          commandGroupLabs,
		"reset":       commandGroupLabs,
		"init":        commandGroupRuntime,
		"status":      commandGroupRuntime,
		"access":      commandGroupRuntime,
		"repair":      commandGroupRuntime,
		"destroy":     commandGroupRuntime,
		"observe":     commandGroupRuntime,
		"credentials": commandGroupRuntime,
		"describe":    commandGroupUtility,
		"version":     commandGroupUtility,
	}
	for _, cmd := range root.Commands() {
		if groupID := groupByName[cmd.Name()]; groupID != "" {
			cmd.GroupID = groupID
		}
	}
}

// initDriver selects and configures the driver, applying dry-run wrapping.
func initDriver(state *RootState) error {
	name, err := selectDriverName(state.Flags.DriverName)
	if err != nil {
		return err
	}

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)

	var d driver.Driver
	switch name {
	case "lima":
		if _, err := exec.LookPath("limactl"); err != nil {
			return fmt.Errorf("--driver=lima requested but limactl is not on PATH")
		}
		d = driver.NewLimaDriver(stateDir)
	case "docker":
		if _, err := exec.LookPath("docker"); err != nil {
			return fmt.Errorf("--driver=docker requested but docker is not on PATH")
		}
		d = driver.NewDockerDriver(stateDir)
	default:
		return fmt.Errorf("unknown driver %q — valid values: lima, docker", name)
	}

	if state.Flags.DryRun {
		d = driver.NewDryRun(d)
	}

	state.Driver = d
	return nil
}

// selectDriverName resolves the requested driver. An empty value means auto:
// prefer Lima when available, then Docker.
func selectDriverName(requested string) (string, error) {
	name := requested
	if name == "" {
		name = os.Getenv("LAB_DRIVER")
	}
	if name != "" {
		return name, nil
	}
	if _, err := exec.LookPath("limactl"); err == nil {
		return "lima", nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", nil
	}
	return "", fmt.Errorf("no lab driver found — install Lima or Docker, or pass --driver")
}

// newDriverByName instantiates a driver by name without auto-detection.
// Used by printList to query runtime state for labs regardless of the
// driver currently selected for the session.
func newDriverByName(name, stateDir string) (driver.Driver, error) {
	switch name {
	case "lima":
		return driver.NewLimaDriver(stateDir), nil
	case "docker":
		return driver.NewDockerDriver(stateDir), nil
	default:
		return nil, fmt.Errorf("unknown driver %q — valid values: lima, docker", name)
	}
}

// idName returns the runtime-scoped driver identifier for a lab.
func idName(lab string) string {
	return config.RuntimeIDOf(lab)
}

// limaYAMLTemplate returns the standard lima yaml template path.
func limaYAMLTemplate(repoDir string) string {
	return filepath.Join(repoDir, "infra", "lima", "agent-lab.yaml.tmpl")
}
