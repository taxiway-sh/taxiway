package cli

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/taxiway-sh/taxiway/internal/config"
)

const (
	portForwardHostPortBase  = 18080
	portForwardHostPortRange = 30000
)

type portForwardSpec struct {
	EnvKey    string
	StateFile string
	GuestPort int
}

var orchestratorPortForwards = map[string]portForwardSpec{
	"gastown": {
		EnvKey:    "TAXIWAY_DASHBOARD_HOST_PORT",
		StateFile: "dashboard.port",
		GuestPort: 8080,
	},
}

func preparePortForward(ctx context.Context, state *RootState, ref config.LabRef, env map[string]string) error {
	spec, ok := orchestratorPortForwards[ref.Orch]
	if !ok {
		return nil
	}

	stateDir := config.StateDir(state.Flags.StateDir, state.RepoDir)
	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	checkHostPort := !state.Flags.DryRun && d.Name() == "lima"
	port, err := portForwardHostPort(stateDir, ref, spec, state.Flags.DryRun, checkHostPort)
	if err != nil {
		return err
	}
	env[spec.EnvKey] = strconv.Itoa(port)

	if state.Flags.DryRun || d.Name() != "lima" {
		return nil
	}
	return ensureLimaPortForward(ctx, idName(ref.Lab), port, spec.GuestPort)
}

func portForwardHostPort(stateDir string, ref config.LabRef, spec portForwardSpec, dryRun, checkHostPort bool) (int, error) {
	portPath := portForwardPath(stateDir, ref, spec)
	if port, ok := readPortForwardHostPort(portPath, spec.GuestPort); ok {
		return port, nil
	}

	used := allocatedPortForwardHostPorts(stateDir, spec)
	for attempt := 0; attempt < portForwardHostPortRange; attempt++ {
		port := portForwardCandidate(ref.Lab, attempt)
		if port == spec.GuestPort || used[port] || (checkHostPort && !hostPortAvailable(port)) {
			continue
		}
		if !dryRun {
			if err := os.MkdirAll(filepath.Dir(portPath), 0o755); err != nil {
				return 0, fmt.Errorf("port forward: mkdir: %w", err)
			}
			if err := os.WriteFile(portPath, []byte(strconv.Itoa(port)+"\n"), 0o644); err != nil {
				return 0, fmt.Errorf("port forward: write: %w", err)
			}
		}
		return port, nil
	}
	return 0, fmt.Errorf("port forward: no available host port found")
}

func portForwardPath(stateDir string, ref config.LabRef, spec portForwardSpec) string {
	return filepath.Join(stateDir, config.LabDirOf(idName(ref.Lab)), spec.StateFile)
}

func readPortForwardHostPort(path string, guestPort int) (int, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || port <= 0 || port > 65535 || port == guestPort {
		return 0, false
	}
	return port, true
}

func allocatedPortForwardHostPorts(stateDir string, spec portForwardSpec) map[int]bool {
	used := map[int]bool{}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return used
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		port, ok := readPortForwardHostPort(filepath.Join(stateDir, entry.Name(), spec.StateFile), spec.GuestPort)
		if ok {
			used[port] = true
		}
	}
	return used
}

func portForwardCandidate(lab string, attempt int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(lab))
	return portForwardHostPortBase + int((h.Sum32()+uint32(attempt))%portForwardHostPortRange)
}

func hostPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func ensureLimaPortForward(ctx context.Context, id string, hostPort, guestPort int) error {
	if !hostPortAvailable(hostPort) {
		return nil
	}
	sshConfig := filepath.Join(os.Getenv("HOME"), ".lima", id, "ssh.config")
	target := "lima-" + id
	forward := fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", hostPort, guestPort)
	cmd := exec.CommandContext(ctx, "ssh", "-F", sshConfig, "-f", "-N", "-o", "ExitOnForwardFailure=yes", "-L", forward, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("port forward: start lima forward %s: %w\n%s", forward, err, strings.TrimSpace(string(out)))
	}
	return nil
}
