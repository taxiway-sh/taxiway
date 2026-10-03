package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/event"
)

type agentVersion struct {
	Actual     string `json:"actual"`
	Executable string `json:"executable"`
}

// versionSink keeps only bounded executable reports, alongside ordinary events.
type versionSink struct{ versions map[string]agentVersion }

func (s *versionSink) Handle(_ context.Context, ev event.Event) error {
	if ev.Type != "agent-version" {
		return nil
	}
	agent, _ := ev.Fields["agent"].(string)
	if agent != "claude-code" && agent != "codex" {
		return nil
	}
	requested, _ := ev.Fields["requested"].(string)
	actual, _ := ev.Fields["actual"].(string)
	executable, _ := ev.Fields["executable"].(string)
	if requested != "" && actual != "" && executable != "" {
		if s.versions == nil {
			s.versions = map[string]agentVersion{}
		}
		s.versions[agent] = agentVersion{actual, executable}
	}
	return nil
}
func (*versionSink) Close() error { return nil }
func agentVersionsPath(stateDir, id string) string {
	return filepath.Join(stateDir, config.LabDirOf(id), "agent-versions.json")
}
func readAgentVersions(stateDir, id string) (map[string]agentVersion, error) {
	raw, err := os.ReadFile(agentVersionsPath(stateDir, id))
	if os.IsNotExist(err) {
		return map[string]agentVersion{}, nil
	}
	if err != nil {
		return nil, err
	}
	var versions map[string]agentVersion
	if err := json.Unmarshal(raw, &versions); err != nil {
		return nil, fmt.Errorf("read agent versions: %w", err)
	}
	if versions == nil {
		versions = map[string]agentVersion{}
	}
	return versions, nil
}
func (s *versionSink) persist(stateDir, id string) error {
	if len(s.versions) == 0 {
		return nil
	}
	versions, err := readAgentVersions(stateDir, id)
	if err != nil {
		return err
	}
	for agent, version := range s.versions {
		versions[agent] = version
	}
	raw, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return err
	}
	path := agentVersionsPath(stateDir, id)
	defer os.Remove(path + ".tmp")
	if err := os.WriteFile(path+".tmp", append(raw, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
func printAgentVersions(w io.Writer, stateDir, id string) error {
	versions, err := readAgentVersions(stateDir, id)
	if err != nil {
		return err
	}
	ref, _, err := config.ReadLabRef(stateDir, id)
	if err != nil {
		return err
	}
	agents := make([]string, 0, len(versions))
	for agent := range versions {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	for _, agent := range agents {
		version := versions[agent]
		requested := ref.Settings[agent+"-version"]
		if requested == "" {
			requested = "latest"
		}
		fmt.Fprintf(w, "  %s: requested %s, installed %s (%s)\n", agent, requested, version.Actual, version.Executable)
	}
	return nil
}
