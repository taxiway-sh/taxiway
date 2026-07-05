package recording

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const (
	StateRecording = "recording"
	StateStopped   = "stopped"
)

var recordingNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Session struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Lab             string     `json:"lab"`
	Driver          string     `json:"driver"`
	DriverID        string     `json:"driver_id"`
	State           string     `json:"state"`
	ShellCommand    string     `json:"shell_command"`
	RecorderSession string     `json:"recorder_session"`
	CastPath        string     `json:"cast_path"`
	CastPathHost    string     `json:"cast_path_host"`
	StartedAt       time.Time  `json:"started_at"`
	StoppedAt       *time.Time `json:"stopped_at,omitempty"`
}

type Index struct {
	Sessions []Session `json:"sessions"`
}

type Store struct {
	lab  string
	dir  string
	path string
}

func NewStore(stateDir, lab string) Store {
	dir := filepath.Join(stateDir, lab, "recordings")
	return Store{lab: lab, dir: dir, path: filepath.Join(dir, "recordings.json")}
}

func (s Store) Lab() string {
	return s.lab
}

func (s Store) Dir() string {
	return s.dir
}

func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("recording name is required")
	}
	if !recordingNameRe.MatchString(name) {
		return fmt.Errorf("invalid recording name %q — must match ^[A-Za-z0-9_-]+$", name)
	}
	return nil
}

func DefaultName(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}

func NewID(t time.Time, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	return t.UTC().Format("20060102-150405") + "-" + name, nil
}

func (s Store) Load() (Index, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Index{}, nil
	}
	if err != nil {
		return Index{}, fmt.Errorf("recording: read index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("recording: parse index: %w", err)
	}
	sortSessions(idx.Sessions)
	return idx, nil
}

func (s Store) Save(idx Index) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("recording: mkdir index dir: %w", err)
	}
	sortSessions(idx.Sessions)
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("recording: marshal index: %w", err)
	}
	data = append(data, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("recording: write tmp index: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("recording: replace index: %w", err)
	}
	return nil
}

func (idx Index) HasActiveName(name string) bool {
	for _, session := range idx.Sessions {
		if session.Name == name && session.State == StateRecording {
			return true
		}
	}
	return false
}

func (idx Index) LatestActive() (Session, bool) {
	var latest Session
	found := false
	for _, session := range idx.Sessions {
		if session.State != StateRecording {
			continue
		}
		if !found || session.StartedAt.After(latest.StartedAt) {
			latest = session
			found = true
		}
	}
	return latest, found
}

func (idx Index) ActiveByName(name string) (Session, bool) {
	for _, session := range idx.Sessions {
		if session.Name == name && session.State == StateRecording {
			return session, true
		}
	}
	return Session{}, false
}

func (idx *Index) Upsert(session Session) {
	for i := range idx.Sessions {
		if idx.Sessions[i].ID == session.ID {
			idx.Sessions[i] = session
			sortSessions(idx.Sessions)
			return
		}
	}
	idx.Sessions = append(idx.Sessions, session)
	sortSessions(idx.Sessions)
}

func (idx *Index) RemoveByName(name string) (Session, bool) {
	for i, session := range idx.Sessions {
		if session.Name != name {
			continue
		}
		idx.Sessions = append(idx.Sessions[:i], idx.Sessions[i+1:]...)
		return session, true
	}
	return Session{}, false
}

func sortSessions(sessions []Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.Before(sessions[j].StartedAt)
	})
}
