// Package driver defines the Driver interface and supporting types.
package driver

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/event"
)

// ErrExecTimeout is returned by Exec when the context deadline is exceeded
// before the command completes.
var ErrExecTimeout = errors.New("exec timeout")

// Driver is the central abstraction for lab lifecycle operations.
// All CLI commands call only methods on Driver, never tool-specific CLIs directly.
type Driver interface {
	Name() string

	// Lifecycle
	Exists(ctx context.Context, id string) (bool, error)
	Running(ctx context.Context, id string) (bool, error)
	Create(ctx context.Context, id string, opts CreateOptions) error
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	Status(ctx context.Context, id string) (Status, error)
	List(ctx context.Context) ([]Status, error)

	// LabRef sidecar — written on Create, read by commands that need lab metadata.
	WriteLabRef(ctx context.Context, id string, ref config.LabRef) error
	ReadLabRef(ctx context.Context, id string) (config.LabRef, bool, error)

	// File transfer
	// Copy copies a local file into the lab at the given destination path.
	Copy(ctx context.Context, id, srcHost, dstLab string) error

	// Execution
	Shell(ctx context.Context, id, workdir string) error
	// ShellExec opens an interactive session in the lab and executes cmd
	// instead of the default shell (e.g. "tmux attach-session -t <name>").
	ShellExec(ctx context.Context, id, workdir, cmd string) error
	InteractiveExec(ctx context.Context, id string, req InteractiveExecRequest) error
	Exec(ctx context.Context, id string, req ExecRequest) (ExecResult, error)
}

// CreateOptions carries lab creation parameters shared by drivers.
type CreateOptions struct {
	Lab  string // lab name (e.g. "mon-lab")
	Orch string // orchestrator name (e.g. "claude-code")

	RepoDir       string // host repo root used to populate or mount /lab
	GitDir        string // host bare git remotes dir mounted at /lab/git when the driver supports it
	RecordingsDir string // host recordings dir mounted at /lab/recordings when the driver supports it

	TemplatePath string // driver template path when the driver needs one
}

// ExecRequest describes a command to run inside a lab.
type ExecRequest struct {
	Workdir string
	Argv    []string
	Env     map[string]string
	Stdout  io.Writer  // if nil, discarded
	Stderr  io.Writer  // if nil, discarded
	Events  event.Sink // optional; receives parsed LAB_AGENT_EVENT lines
}

// InteractiveExecRequest describes a command to run in the lab with
// stdin/stdout/stderr attached to the user's terminal.
type InteractiveExecRequest struct {
	Workdir string
	Argv    []string
	Env     map[string]string
}

// ExecResult is returned by Exec.
type ExecResult struct {
	ExitCode int
	Duration time.Duration
}

// Status describes the current state of a lab runtime.
type Status struct {
	Name    string
	State   string // "running" | "stopped" | "absent"
	Driver  string
	Created time.Time
	Extra   map[string]string // driver-specific metadata (e.g. session_id, ip)
}
