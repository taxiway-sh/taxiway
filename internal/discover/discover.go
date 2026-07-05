// Package discover scans the orchestrators/ directory to find available orchestrators.
package discover

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// Orchestrators returns a sorted list of orchestrator names found under
// <repoDir>/orchestrators/*/install.sh.
func Orchestrators(repoDir string) ([]string, error) {
	orchDir := filepath.Join(repoDir, "orchestrators")
	entries, err := os.ReadDir(orchDir)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		installSh := filepath.Join(orchDir, e.Name(), "install.sh")
		st, err := os.Stat(installSh)
		if err == nil && !st.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// ActiveLabs scans <stateDir>/*/ and returns LabRef for all labs.
// Discovery signal: presence of a created_at file (written by all drivers at Create time).
// A ref.json sidecar is required so completions only expose runnable labs.
func ActiveLabs(stateDir string) ([]config.LabRef, error) {
	pattern := filepath.Join(stateDir, "*")
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var out []config.LabRef
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "created_at")); err != nil {
			continue // not a real lab directory
		}
		lab := filepath.Base(d)
		ref, ok, _ := config.ReadLabRef(stateDir, lab)
		if !ok {
			continue
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lab < out[j].Lab })
	return out, nil
}
