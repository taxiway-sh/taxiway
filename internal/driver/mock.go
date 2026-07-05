package driver

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/event"
)

// MockCopyCall records a Copy invocation.
type MockCopyCall struct {
	ID  string
	Src string
	Dst string
}

// ShellExecCall records a ShellExec invocation.
type ShellExecCall struct {
	ID      string
	Workdir string
	Cmd     string
}

// InteractiveExecCall records a InteractiveExec invocation.
type InteractiveExecCall struct {
	ID      string
	Workdir string
	Argv    []string
	Env     map[string]string
}

// MockExecResponse is the response injected by ExecResponder.
type MockExecResponse struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// MockDriver implements Driver using local filesystem state.
// State lives at <stateDir>/<lab>/:
//
//	state       — "running" | "stopped"
//	created_at  — RFC3339
type MockDriver struct {
	stateDir string
	// RepoDir is set by tests so Exec can translate /lab/ lab paths back to
	// real host paths for local script execution.
	RepoDir string
	// FailExec maps basename(script) → error to inject for that script execution.
	// Example: FailExec["install.sh"] = errors.New("injected failure")
	FailExec map[string]error
	// ExecLog records the basename of each script passed to Exec (in order).
	ExecLog []string
	// ExecEnvLog records the env map passed to each Exec call (in order, parallel to ExecLog).
	ExecEnvLog []map[string]string
	// CopyLog records all Copy calls (in order).
	CopyLog []MockCopyCall
	// LastCreateOptions records the opts passed to the most recent Create call.
	LastCreateOptions CreateOptions
	// ShellExecLog records all ShellExec calls (id, workdir, cmd triples).
	ShellExecLog []ShellExecCall
	// InteractiveExecLog records all InteractiveExec calls.
	InteractiveExecLog []InteractiveExecCall
	// CallLog records driver operation order for tests.
	CallLog []string
	// ExecResponder, when non-nil, is called for every Exec instead of
	// executing a real process.  It receives the lab name and request and
	// returns a controlled response. Useful for testing commands that issue
	// inline shell one-liners.
	ExecResponder func(id string, req ExecRequest) MockExecResponse
	// CopyResponder, when non-nil, is called for every Copy after the call is
	// recorded. Useful for inspecting temp files before callers remove them.
	CopyResponder func(id, srcHost, dstLab string) error
	// FailWriteLabRef, when true, makes WriteLabRef always return an error.
	// Used to test rollback behaviour in labUp.
	FailWriteLabRef bool
}

// NewMockDriver creates a MockDriver backed by stateDir.
func NewMockDriver(stateDir string) *MockDriver {
	return &MockDriver{stateDir: stateDir, FailExec: make(map[string]error)}
}

func (m *MockDriver) Name() string { return "mock" }

func (m *MockDriver) idDir(id string) string {
	return filepath.Join(m.stateDir, config.LabDirOf(id))
}

func (m *MockDriver) stateFile(id string) string {
	return filepath.Join(m.idDir(id), "state")
}

func (m *MockDriver) Exists(_ context.Context, id string) (bool, error) {
	_, err := os.Stat(m.idDir(id))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (m *MockDriver) Running(ctx context.Context, id string) (bool, error) {
	exists, err := m.Exists(ctx, id)
	if !exists || err != nil {
		return false, err
	}
	data, err := os.ReadFile(m.stateFile(id))
	if err != nil {
		return false, err
	}
	return string(data) == "running", nil
}

func (m *MockDriver) Create(_ context.Context, id string, opts CreateOptions) error {
	m.LastCreateOptions = opts
	dir := m.idDir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(m.stateFile(id), []byte("running"), 0644); err != nil {
		return err
	}
	_ = config.EnsureCreatedAt(m.stateDir, id)
	return nil
}

func (m *MockDriver) Start(ctx context.Context, id string) error {
	exists, err := m.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("lab %q does not exist", id)
	}
	return os.WriteFile(m.stateFile(id), []byte("running"), 0644)
}

func (m *MockDriver) Stop(ctx context.Context, id string) error {
	exists, err := m.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("lab %q does not exist", id)
	}
	return os.WriteFile(m.stateFile(id), []byte("stopped"), 0644)
}

func (m *MockDriver) Delete(ctx context.Context, id string) error {
	exists, err := m.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("lab %q does not exist", id)
	}
	return os.RemoveAll(m.idDir(id))
}

func (m *MockDriver) Status(ctx context.Context, id string) (Status, error) {
	exists, err := m.Exists(ctx, id)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{Name: id, State: "absent", Driver: "mock"}, nil
	}
	data, err := os.ReadFile(m.stateFile(id))
	if err != nil {
		return Status{}, err
	}
	st := Status{Name: id, State: string(data), Driver: "mock"}
	if raw, err := os.ReadFile(filepath.Join(m.idDir(id), "created_at")); err == nil {
		st.Created, _ = time.Parse(time.RFC3339, string(raw))
	}
	return st, nil
}

func (m *MockDriver) List(_ context.Context) ([]Status, error) {
	idsDir := m.stateDir
	entries, err := os.ReadDir(idsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Status
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		lab := e.Name()
		data, _ := os.ReadFile(filepath.Join(idsDir, lab, "state"))
		state := string(data)
		if state == "" {
			state = "unknown"
		}
		st := Status{Name: config.RuntimeIDOf(lab), State: state, Driver: "mock"}
		if raw, err := os.ReadFile(filepath.Join(idsDir, lab, "created_at")); err == nil {
			st.Created, _ = time.Parse(time.RFC3339, string(raw))
		}
		out = append(out, st)
	}
	return out, nil
}

func (m *MockDriver) Shell(ctx context.Context, id, workdir string) error {
	exists, err := m.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("lab %q does not exist — run: taxiway up", id)
	}
	cmd := exec.Command("bash", "-l")
	cmd.Dir = workdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "LAB_MOCK=1")
	return cmd.Run()
}

// ShellExec records the call and is a no-op in tests (no real TTY to attach).
func (m *MockDriver) ShellExec(_ context.Context, id, workdir, shellCmd string) error {
	m.ShellExecLog = append(m.ShellExecLog, ShellExecCall{
		ID:      id,
		Workdir: workdir,
		Cmd:     shellCmd,
	})
	return nil
}

func (m *MockDriver) InteractiveExec(_ context.Context, id string, req InteractiveExecRequest) error {
	env := make(map[string]string, len(req.Env))
	for k, v := range req.Env {
		env[k] = v
	}
	m.InteractiveExecLog = append(m.InteractiveExecLog, InteractiveExecCall{
		ID:      id,
		Workdir: req.Workdir,
		Argv:    append([]string(nil), req.Argv...),
		Env:     env,
	})
	name := "interactive"
	if len(req.Argv) > 0 {
		name = "interactive:" + filepath.Base(req.Argv[len(req.Argv)-1])
	}
	m.CallLog = append(m.CallLog, name)
	return nil
}

// Copy records the copy operation in the CopyLog.
func (m *MockDriver) Copy(_ context.Context, id, srcHost, dstlab string) error {
	m.CopyLog = append(m.CopyLog, MockCopyCall{ID: id, Src: srcHost, Dst: dstlab})
	if m.CopyResponder != nil {
		return m.CopyResponder(id, srcHost, dstlab)
	}
	return nil
}

// WriteLabRef writes the lab ref sidecar via the config package.
// Returns an error if FailWriteLabRef is set (for testing rollback).
func (m *MockDriver) WriteLabRef(_ context.Context, id string, ref config.LabRef) error {
	if m.FailWriteLabRef {
		return fmt.Errorf("mock: WriteLabRef injected failure for %s", id)
	}
	return config.WriteLabRef(m.stateDir, id, ref)
}

// ReadLabRef reads the lab ref sidecar via the config package.
func (m *MockDriver) ReadLabRef(_ context.Context, id string) (config.LabRef, bool, error) {
	return config.ReadLabRef(m.stateDir, id)
}

func (m *MockDriver) Exec(ctx context.Context, id string, req ExecRequest) (ExecResult, error) {
	exists, err := m.Exists(ctx, id)
	if err != nil {
		return ExecResult{}, err
	}
	if !exists {
		return ExecResult{}, fmt.Errorf("lab %q does not exist — run: taxiway up", id)
	}

	running, err := m.Running(ctx, id)
	if err != nil {
		return ExecResult{}, err
	}
	if !running {
		return ExecResult{}, fmt.Errorf("lab %q is not running — run: taxiway up", id)
	}

	// If a responder is registered, delegate entirely to it.
	if m.ExecResponder != nil {
		resp := m.ExecResponder(id, req)
		if resp.Stdout != "" && req.Stdout != nil {
			_, _ = io.WriteString(req.Stdout, resp.Stdout)
		}
		if resp.Stderr != "" && req.Stderr != nil {
			_, _ = io.WriteString(req.Stderr, resp.Stderr)
		}
		return ExecResult{ExitCode: resp.ExitCode}, resp.Err
	}

	// Inject failure by script basename if configured.
	if len(req.Argv) > 0 {
		base := filepath.Base(req.Argv[len(req.Argv)-1])
		m.CallLog = append(m.CallLog, "exec:"+base)
		m.ExecLog = append(m.ExecLog, base)
		m.ExecEnvLog = append(m.ExecEnvLog, req.Env)
		if injErr, ok := m.FailExec[base]; ok && injErr != nil {
			return ExecResult{ExitCode: 1}, injErr
		}
	}

	stdout := req.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	start := time.Now()

	// Resolve the argv: if RepoDir is set, translate /lab/ lab paths to real paths.
	argv := make([]string, len(req.Argv))
	copy(argv, req.Argv)
	if m.RepoDir != "" {
		const labRoot = "/lab"
		for i, arg := range argv {
			if strings.HasPrefix(arg, labRoot+"/") || arg == labRoot {
				rel := strings.TrimPrefix(arg, labRoot)
				if rel == "" {
					rel = "."
				}
				argv[i] = filepath.Join(m.RepoDir, rel)
			}
		}
	}

	// Use real workdir for execution (not /lab which doesn't exist locally).
	workdir := req.Workdir
	if workdir == "/lab" && m.RepoDir != "" {
		workdir = m.RepoDir
	}

	// Build command with translated argv.
	//nolint:gosec
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Env = append(os.Environ(), "LAB_MOCK=1")
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if req.Events != nil {
		// Pipe stdout through event splitter
		pr, pw, err := os.Pipe()
		if err != nil {
			return ExecResult{}, err
		}
		cmd.Stdout = pw
		cmd.Stderr = stderr

		errCh := make(chan error, 1)
		go func() {
			event.SplitOutput(ctx, pr, stdout, req.Events, "stdout")
			errCh <- pr.Close()
		}()

		runErr := cmd.Run()
		pw.Close()
		<-errCh

		dur := time.Since(start)
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				return ExecResult{}, runErr
			}
		}
		return ExecResult{ExitCode: code, Duration: dur}, nil
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	dur := time.Since(start)

	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			return ExecResult{}, runErr
		}
	}
	return ExecResult{ExitCode: code, Duration: dur}, nil
}
