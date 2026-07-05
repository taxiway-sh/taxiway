package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Workspace holds the optional workspace configuration for a lab.
// It is stored in ref.json under the "workspace" key.
type Workspace struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref,omitempty"`
	Path string `json:"path,omitempty"`
	Fork string `json:"fork,omitempty"` // URL of the isolated per-lab Git remote
}

// LabRefFile is the JSON structure written to <stateDir>/<lab>/ref.json.
// It is the canonical source of truth for reconstructing a LabRef from a lab name.
type LabRefFile struct {
	Version             int                  `json:"version"`
	Lab                 string               `json:"lab"`
	Orch                string               `json:"orch"`
	Driver              string               `json:"driver,omitempty"`
	Workspace           *Workspace           `json:"workspace,omitempty"`
	OrchestratorProfile *OrchestratorProfile `json:"orchestrator_profile,omitempty"`
	Settings            map[string]string    `json:"settings,omitempty"`
}

func labRefPath(stateDir, id string) string {
	return filepath.Join(stateDir, LabDirOf(id), "ref.json")
}

// WriteLabRef writes ref.json for id into stateDir using an atomic tmp+rename.
// Creates parent directories as needed. Always writes version 5.
// id may be the full driver id ("taxiway-<lab>") or just the lab name ("<lab>").
func WriteLabRef(stateDir, id string, ref LabRef) error {
	if ref.Driver == "" {
		return fmt.Errorf("labref: driver is required for %s", id)
	}
	dir := filepath.Join(stateDir, LabDirOf(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("labref: mkdir %s: %w", dir, err)
	}
	f := LabRefFile{
		Version:             5,
		Lab:                 ref.Lab,
		Orch:                ref.Orch,
		Driver:              ref.Driver,
		Workspace:           ref.Workspace,
		OrchestratorProfile: ref.OrchestratorProfile,
		Settings:            ref.Settings,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("labref: marshal: %w", err)
	}
	data = append(data, '\n')
	// Atomic write: write to tmp then rename.
	tmp := labRefPath(stateDir, id) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("labref: write tmp: %w", err)
	}
	if err := os.Rename(tmp, labRefPath(stateDir, id)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("labref: rename: %w", err)
	}
	return nil
}

// ReadLabRef reads ref.json for id from stateDir.
// Returns (ref, true, nil) when found, (LabRef{}, false, nil) when absent,
// and (LabRef{}, false, err) when the file exists but cannot be parsed.
//
// Missing optional fields are decoded as zero values.
func ReadLabRef(stateDir, id string) (LabRef, bool, error) {
	data, err := os.ReadFile(labRefPath(stateDir, id))
	if errors.Is(err, os.ErrNotExist) {
		return LabRef{}, false, nil
	}
	if err != nil {
		return LabRef{}, false, fmt.Errorf("labref: read %s: %w", id, err)
	}
	var f LabRefFile
	if err := json.Unmarshal(data, &f); err != nil {
		return LabRef{}, false, fmt.Errorf("labref: parse %s: %w", id, err)
	}
	return LabRef{
		Lab:                 f.Lab,
		Orch:                f.Orch,
		Driver:              f.Driver,
		Workspace:           f.Workspace,
		OrchestratorProfile: f.OrchestratorProfile,
		Settings:            f.Settings,
	}, true, nil
}
