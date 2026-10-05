package recording

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
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
	return t.UTC().Format("20060102-150405") + "-" + name + "-" + rand.Text(), nil
}

func (s Store) Load() (Index, error) {
	root, err := s.OpenRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return Index{}, nil
	}
	if err != nil {
		return Index{}, err
	}
	defer root.Close()
	data, err := readRegularFile(root, "recordings.json")
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
	for _, session := range idx.Sessions {
		if err := s.validateSession(root, session); err != nil {
			return Index{}, err
		}
	}
	sortSessions(idx.Sessions)
	return idx, nil
}

func (s Store) Save(idx Index) error {
	root, err := s.OpenRoot(true)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, session := range idx.Sessions {
		if err := s.validateSession(root, session); err != nil {
			return err
		}
	}
	sortSessions(idx.Sessions)
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("recording: marshal index: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomicFile(root, "recordings.json", data); err != nil {
		return fmt.Errorf("recording: write index: %w", err)
	}
	return nil
}

func writeAtomicFile(root *os.Root, name string, data []byte) error {
	tmp := ".recordings-" + rand.Text() + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("recording: create tmp index: %w", err)
	}
	defer root.Remove(tmp)
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("recording: write tmp index: %w", err)
	}
	if err := root.Rename(tmp, name); err != nil {
		return fmt.Errorf("recording: replace artifact: %w", err)
	}
	return nil
}

// OpenRoot anchors all host filesystem operations to the lab's recording mount.
// The guest can change its contents, so checking a path before an unrooted open
// or remove would leave a symlink race. The mount directory itself must be real.
func (s Store) OpenRoot(create bool) (*os.Root, error) {
	if err := ValidateName(s.lab); err != nil {
		return nil, err
	}
	info, err := os.Lstat(s.dir)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.MkdirAll(s.dir, 0755); err != nil {
			return nil, err
		}
		info, err = os.Lstat(s.dir)
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("recording: recording directory must be a real directory")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, fmt.Errorf("recording: recording directory changed while opening")
	}
	return root, nil
}

func (s Store) castName(session Session) (string, error) {
	if err := ValidateName(session.ID); err != nil {
		return "", err
	}
	if session.Lab != "" && session.Lab != s.lab {
		return "", fmt.Errorf("recording: session belongs to another lab")
	}
	if session.CastPathHost == "" {
		return "", nil
	}
	dir, err := filepath.Abs(s.dir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(session.CastPathHost)
	if err != nil {
		return "", err
	}
	name, err := filepath.Rel(dir, path)
	if err != nil || name != filepath.Base(name) || name == "." || !strings.HasSuffix(name, ".cast") {
		return "", fmt.Errorf("recording: cast path is outside the lab recordings directory")
	}
	if session.CastPath != "" && session.CastPath != "/lab/recordings/"+name {
		return "", fmt.Errorf("recording: inconsistent lab cast path")
	}
	return name, nil
}

func (s Store) validateSession(root *os.Root, session Session) error {
	name, err := s.castName(session)
	if err != nil || name == "" {
		return err
	}
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} // Interrupted launches remain recoverable.
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("recording: cast must not be a symbolic link")
	}
	return nil
}

func (s Store) ReadCast(session Session) ([]byte, error) {
	root, err := s.OpenRoot(false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := s.validateSession(root, session); err != nil {
		return nil, err
	}
	name, err := s.castName(session)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("recording: cast path is missing")
	}
	return readRegularFile(root, name)
}

func readRegularFile(root *os.Root, name string) ([]byte, error) {
	f, err := openArtifact(root, name, false)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func openArtifact(root *os.Root, name string, allowDirectory bool) (*os.File, error) {
	// A guest can replace an artifact with a FIFO between validation and open.
	// Nonblocking open followed by a check on the handle rejects special files
	// without hanging the host or relying on a racy path-based type check.
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() && !(allowDirectory && info.IsDir()) {
		f.Close()
		return nil, fmt.Errorf("recording: artifact must be a regular file")
	}
	return f, nil
}

func (s Store) RemoveCast(session Session) error {
	root, err := s.OpenRoot(false)
	if err != nil {
		return err
	}
	defer root.Close()
	name, err := s.castName(session)
	if err != nil || name == "" {
		return err
	}
	return root.Remove(name)
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
