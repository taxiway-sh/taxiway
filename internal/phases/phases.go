package phases

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// Phase is a named step in the taxiway up pipeline.
type Phase string

const (
	PhaseCreate    Phase = "create"
	PhaseBootstrap Phase = "bootstrap"
	PhaseInstall   Phase = "install"
	// PhaseVerify runs orchestrators/<orch>/verify.sh after install to confirm
	// the binary works before start. Optional: absent verify.sh = silent skip.
	PhaseVerify Phase = "verify"
	// PhaseGateway reconciles generated runtime gateway access before workspace/auth/start.
	// Skippable via --skip-gateway.
	PhaseGateway Phase = "gateway"
	// PhaseWorkspace clones the configured git repository into the lab before auth/start.
	// Skippable via --skip-workspace.
	// Silent no-op when no --repo is configured for the lab.
	PhaseWorkspace Phase = "workspace"
	// PhaseAuth represents declared agent authentication before start.
	// It executes by default and is only skipped with --skip-auth-check.
	PhaseAuth  Phase = "auth"
	PhaseStart Phase = "start"
)

// Order is the canonical execution order for taxiway up.
// taxiway doctor is a standalone diagnostic command, not part of the pipeline.
var Order = []Phase{PhaseCreate, PhaseBootstrap, PhaseInstall, PhaseVerify, PhaseGateway, PhaseWorkspace, PhaseAuth, PhaseStart}

// IsKnown reports whether p is a defined phase.
func IsKnown(p Phase) bool {
	for _, q := range Order {
		if q == p {
			return true
		}
	}
	return false
}

// Dir returns the directory that holds phase marker files for a given lab.
// id may be the full driver id ("taxiway-<lab>") or just the lab name ("<lab>").
func Dir(stateDir, id string) string {
	return filepath.Join(stateDir, config.LabDirOf(id), "phases")
}

func markerPath(stateDir, id string, p Phase) string {
	return filepath.Join(Dir(stateDir, id), string(p)+".done")
}

// Done reports whether the phase marker file exists.
func Done(stateDir, id string, p Phase) bool {
	_, err := os.Stat(markerPath(stateDir, id, p))
	return err == nil
}

// Mark writes the phase marker file, creating directories as needed.
func Mark(stateDir, id string, p Phase) error {
	dir := Dir(stateDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("phases: mkdir %s: %w", dir, err)
	}
	return os.WriteFile(markerPath(stateDir, id, p),
		[]byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Clear removes a single phase marker (no-op if already absent).
func Clear(stateDir, id string, p Phase) error {
	err := os.Remove(markerPath(stateDir, id, p))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ClearAll removes the entire phases directory for a lab.
func ClearAll(stateDir, id string) error {
	err := os.RemoveAll(Dir(stateDir, id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ParsePhase converts a string to a Phase, returning an error for unknown values.
func ParsePhase(s string) (Phase, error) {
	p := Phase(s)
	if !IsKnown(p) {
		return "", fmt.Errorf("unknown phase %q (valid: %v)", s, Order)
	}
	return p, nil
}
