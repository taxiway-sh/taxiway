package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/event"
)

// LimaDriver wraps limactl for production use.
type LimaDriver struct {
	stateDir string
}

func NewLimaDriver(stateDir string) *LimaDriver { return &LimaDriver{stateDir: stateDir} }

func (l *LimaDriver) Name() string { return "lima" }

func (l *LimaDriver) Exists(ctx context.Context, id string) (bool, error) {
	out, err := exec.CommandContext(ctx, "limactl", "list", "--format={{.Name}}").Output()
	if err != nil {
		return false, inspectionError("limactl list", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == id {
			return true, nil
		}
	}
	return false, nil
}

func (l *LimaDriver) Running(ctx context.Context, id string) (bool, error) {
	out, err := exec.CommandContext(ctx, "limactl", "list", "--format={{.Name}} {{.Status}}").Output()
	if err != nil {
		return false, inspectionError("limactl list", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 && parts[0] == id {
			return parts[1] == "Running", nil
		}
	}
	return false, nil
}

// LimaTemplateData holds the values injected into agent-lab.yaml.tmpl.
type LimaTemplateData struct {
	RepoDir       string // host repo root
	Orch          string // orchestrator name
	GitDir        string // host bare git remotes dir (mounted read-only at /lab/git-source)
	RecordingsDir string // host recordings dir (mounted rw at /lab/recordings)
	LabHost       string // lab-specific host observability hostname
}

// renderLimaYAML reads tmplPath, renders it with data, and writes the result
// to outPath.
func renderLimaYAML(tmplPath string, data LimaTemplateData, outPath string) error {
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("lima: read template %s: %w", tmplPath, err)
	}
	tmpl, err := template.New("lima").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("lima: parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("lima: render template: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("lima: mkdir for yaml: %w", err)
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}

func (l *LimaDriver) Create(ctx context.Context, id string, opts CreateOptions) error {
	if opts.Lab == "" || opts.Orch == "" || opts.RepoDir == "" || opts.GitDir == "" || opts.RecordingsDir == "" || opts.TemplatePath == "" {
		return fmt.Errorf("lima driver: Lab, Orch, RepoDir, GitDir, RecordingsDir, TemplatePath are all required")
	}

	// Render the template and persist for audit/debug.
	yamlPath := filepath.Join(l.stateDir, config.LabDirOf(id), "agent-lab.yaml")
	data := LimaTemplateData{
		RepoDir:       opts.RepoDir,
		Orch:          opts.Orch,
		GitDir:        opts.GitDir,
		RecordingsDir: opts.RecordingsDir,
		LabHost:       config.LabLiteLLMInternalHost(opts.Lab),
	}
	if err := renderLimaYAML(opts.TemplatePath, data, yamlPath); err != nil {
		return err
	}

	if err := l.startInstance(ctx, id, "--name="+id, yamlPath); err != nil {
		return err
	}
	_ = config.EnsureCreatedAt(l.stateDir, id)
	return nil
}

func (l *LimaDriver) Start(ctx context.Context, id string) error {
	return l.startInstance(ctx, id, id)
}

// startInstance applies the same startup policy on every host, for both new
// and existing VMs. Bound the process as well as Lima's own readiness wait so
// downloads and a stuck limactl cannot wait indefinitely.
func (l *LimaDriver) startInstance(ctx context.Context, id string, args ...string) error {
	timeout := 15 * time.Minute
	if value := os.Getenv("TAXIWAY_LIMA_START_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("lima: TAXIWAY_LIMA_START_TIMEOUT must be a positive duration (for example 15m), got %q", value)
		}
		timeout = parsed
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "limactl", append([]string{"start", "--timeout=" + timeout.String()}, args...)...)
	// Do not wait forever for output pipes inherited by Lima subprocesses.
	cmd.WaitDelay = time.Second
	output := limaStartupOutput{stage: "not reported"}
	stream := io.MultiWriter(os.Stderr, &output)
	cmd.Stdout, cmd.Stderr = stream, stream
	err := cmd.Run()
	if err == nil {
		return l.prepareInternalDirs(ctx, id)
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return fmt.Errorf("lima: start %s (timeout %s, last readiness stage: %s): %w\n%s\nInspect the instance with: limactl list %s", id, timeout, output.stage, err, strings.TrimSpace(string(output.tail)), id)
}

// Retain bounded failure context while forwarding native progress immediately.
type limaStartupOutput struct {
	tail  []byte
	stage string
	line  []byte
}

func (o *limaStartupOutput) Write(p []byte) (int, error) {
	const limit = 16 * 1024
	if len(p) >= limit {
		o.tail = append(o.tail[:0], p[len(p)-limit:]...)
	} else {
		o.tail = append(o.tail, p...)
		if len(o.tail) > limit {
			o.tail = o.tail[len(o.tail)-limit:]
		}
	}
	for _, b := range p {
		if b == '\n' {
			if strings.Contains(string(o.line), "Waiting for") {
				o.stage = string(o.line)
			}
			o.line = o.line[:0]
		} else if len(o.line) < 512 {
			o.line = append(o.line, b)
		}
	}
	return len(p), nil
}

func (l *LimaDriver) Stop(ctx context.Context, id string) error {
	return limaOperation(ctx, "stop", id)
}

// Delete stops, deletes the lima lab, and removes all on-disk state for the lab
// (rendered YAML, work dirs, phase markers, events.jsonl).
func (l *LimaDriver) Delete(ctx context.Context, id string) error {
	if err := limaOperation(ctx, "delete", "--force", id); err != nil {
		return err
	}
	// Remove the entire lab state directory so work/, phases/, agent-lab.yaml, etc.
	// do not linger.
	idDir := filepath.Join(l.stateDir, config.LabDirOf(id))
	if err := os.RemoveAll(idDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("lima: remove id state dir %s: %w", idDir, err)
	}
	return nil
}

func (l *LimaDriver) Status(ctx context.Context, id string) (Status, error) {
	exists, err := l.Exists(ctx, id)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{Name: id, State: "absent", Driver: "lima"}, nil
	}
	running, err := l.Running(ctx, id)
	if err != nil {
		return Status{}, err
	}
	state := "stopped"
	if running {
		state = "running"
	}
	st := Status{Name: id, State: state, Driver: "lima"}
	if raw, err := os.ReadFile(filepath.Join(l.stateDir, config.LabDirOf(id), "created_at")); err == nil {
		st.Created, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	}
	return st, nil
}

func (l *LimaDriver) List(ctx context.Context) ([]Status, error) {
	out, err := exec.CommandContext(ctx, "limactl", "list", "--format={{.Name}} {{.Status}}").Output()
	if err != nil {
		return nil, inspectionError("limactl list", err)
	}
	var result []Status
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) < 1 {
			continue
		}
		name := parts[0]
		if !strings.HasPrefix(name, config.DefaultPrefix) {
			continue
		}
		state := "stopped"
		if len(parts) >= 2 && parts[1] == "Running" {
			state = "running"
		}
		st := Status{Name: name, State: state, Driver: "lima"}
		if raw, err := os.ReadFile(filepath.Join(l.stateDir, config.LabDirOf(name), "created_at")); err == nil {
			st.Created, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
		}
		result = append(result, st)
	}
	return result, nil
}

// Copy copies a local file to the lab at the given destination path.
// It uses `limactl copy <srcHost> <id>:<dstLab>`.
func (l *LimaDriver) Copy(ctx context.Context, id, srcHost, dstlab string) error {
	return limaOperation(ctx, "copy", srcHost, id+":"+dstlab)
}

func limaOperation(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, "limactl", args...).CombinedOutput()
	if err != nil {
		return toolOutputError("limactl "+args[0], err, out)
	}
	return nil
}

// Shell opens an interactive shell in the lab at /lab (the lab repo root).
func (l *LimaDriver) Shell(_ context.Context, id, workdir string) error {
	if err := l.prepareInternalDirs(context.Background(), id); err != nil {
		return err
	}
	cmd := exec.Command("limactl", "shell", "--workdir="+workdir, id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ShellExec opens an interactive session in the lab and runs cmd instead of
// the default shell. cmd is passed via limactl's "--" separator so the
// process replaces the shell (useful for e.g. "tmux attach-session -t <name>").
func (l *LimaDriver) ShellExec(_ context.Context, id, workdir, shellCmd string) error {
	if err := l.prepareInternalDirs(context.Background(), id); err != nil {
		return err
	}
	args := []string{"shell", "--workdir=" + workdir, id, "--"}
	args = append(args, "sh", "-c", shellCmd)
	cmd := exec.Command("limactl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (l *LimaDriver) InteractiveExec(ctx context.Context, id string, req InteractiveExecRequest) error {
	if err := l.prepareInternalDirs(ctx, id); err != nil {
		return err
	}
	workdir := req.Workdir
	if workdir == "" {
		workdir = "/lab"
	}
	if len(req.Argv) == 0 {
		return fmt.Errorf("lima: InteractiveExec called with empty argv")
	}
	args := []string{"shell", "--workdir=" + workdir, id, "--"}
	args = append(args, req.Argv...)
	//nolint:gosec
	cmd := exec.CommandContext(ctx, "limactl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd.Run()
}

func (l *LimaDriver) Exec(ctx context.Context, id string, req ExecRequest) (ExecResult, error) {
	if err := l.prepareInternalDirs(ctx, id); err != nil {
		return ExecResult{}, err
	}
	args := append([]string{"shell", "--workdir=" + req.Workdir, id, "--"}, req.Argv...)
	//nolint:gosec
	cmd := exec.CommandContext(ctx, "limactl", args...)

	stdout := req.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	cmd.Env = os.Environ()
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	start := time.Now()

	if req.Events != nil {
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

func (l *LimaDriver) prepareInternalDirs(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "limactl", "shell", "--workdir=/", id, "--", "sh", "-c",
		`test -s /run/lima-boot-done || { printf 'Lima boot scripts have not finished\n' >&2; exit 1; }; sudo mkdir -p /lab/work /lab/git && sudo chown "$(id -u):$(id -g)" /lab/work /lab/git`)
	cmd.WaitDelay = time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return fmt.Errorf("lima: prepare internal dirs for %s: %w\n%s", id, err, out)
	}
	return nil
}

// EventsJSONLPath returns the path for the lab's events.jsonl file.
func EventsJSONLPath(stateDir, id string) string {
	return filepath.Join(stateDir, config.LabDirOf(id), "events.jsonl")
}

// WriteLabRef writes the lab ref sidecar via the config package.
func (l *LimaDriver) WriteLabRef(_ context.Context, id string, ref config.LabRef) error {
	return config.WriteLabRef(l.stateDir, id, ref)
}

// ReadLabRef reads the lab ref sidecar via the config package.
func (l *LimaDriver) ReadLabRef(_ context.Context, id string) (config.LabRef, bool, error) {
	return config.ReadLabRef(l.stateDir, id)
}
