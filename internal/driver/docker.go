// Package driver implements the DockerDriver.
//
// Each lab maps to a Docker container:
//   - Create  → docker volume create + docker run -d with runtime/state mounts
//   - Exec    → docker exec  (synchronous, native exit code)
//   - Shell   → docker exec -it bash -l
//   - Copy    → docker cp
//   - Stop    → docker stop
//   - Start   → docker start
//   - Delete  → docker rm -f + docker volume rm + os.RemoveAll(idDir)
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

// DefaultDockerImage is the ubuntu:24.04 image used by Docker-backed labs.
// It intentionally follows the tag so local labs and CI receive upstream patch
// updates when the image is pulled.
const DefaultDockerImage = "ubuntu:24.04"

const (
	dockerLabUser  = "taxiway"
	dockerLabHome  = "/home/" + dockerLabUser
	dockerLabShell = "/bin/bash"
)

// DockerDriver implements Driver using Docker containers as labs.
// Each lab maps to a single long-running container (sleep infinity).
// State is tracked both via Docker and via a filesystem sidecar at stateDir.
type DockerDriver struct {
	stateDir string
	image    string // overridable for tests; defaults to DefaultDockerImage
}

// NewDockerDriver creates a DockerDriver backed by stateDir.
func NewDockerDriver(stateDir string) *DockerDriver {
	return &DockerDriver{
		stateDir: stateDir,
		image:    DefaultDockerImage,
	}
}

func (d *DockerDriver) Name() string { return "docker" }

// idDir returns the sidecar directory for a given lab/lab name.
func (d *DockerDriver) idDir(id string) string {
	return filepath.Join(d.stateDir, config.LabDirOf(id))
}

// dockerCmd runs a docker command and returns the combined output.
// It is a helper that sets no-TTY mode suitable for non-interactive calls.
func dockerCmd(ctx context.Context, args ...string) ([]byte, error) {
	//nolint:gosec
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.Output()
}

// Exists reports whether a container named id exists (regardless of state).
func (d *DockerDriver) Exists(_ context.Context, id string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "--format={{.Name}}", id) //nolint:gosec
	if err := cmd.Run(); err != nil {
		return false, nil // non-zero exit = container not found
	}
	return true, nil
}

// Running reports whether the container is in the running state.
func (d *DockerDriver) Running(ctx context.Context, id string) (bool, error) {
	out, err := dockerCmd(ctx, "inspect", "--format={{.State.Running}}", id)
	if err != nil {
		return false, nil // container absent
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// labVolumeName returns the canonical Docker named-volume name for a lab's /lab.
func labVolumeName(id string) string {
	return "taxiway-" + id + "-lab"
}

// Create creates a named Docker volume for /lab and starts a container with
// runtime assets and mutable lab state mounted under it.
//
// The sidecar `created_at` is written BEFORE docker run so that any
// mid-flight failure (volume create, docker run, bootstrap setup, …) still leaves
// the lab visible in `taxiway list` with state `stopped`/`created`. The user
// can then run `taxiway rm <lab>` to clean up without manual `docker rm`.
//
// Layout:
//
//	/lab                          → named volume taxiway-<id>-lab
//	/lab/infra                    → opts.RepoDir/infra bind mount (read-only)
//	/lab/agents                   → opts.RepoDir/agents bind mount (read-only)
//	/lab/orchestrators/<orch>     → opts.RepoDir/orchestrators/<orch> bind mount (read-only)
//	/lab/work                     → internal writable lab state
//	/lab/git                      → opts.GitDir bind mount (writable)
//	/lab/recordings               → opts.RecordingsDir bind mount (writable)
func (d *DockerDriver) Create(_ context.Context, id string, opts CreateOptions) error {
	if opts.Lab == "" || opts.Orch == "" || opts.RepoDir == "" || opts.GitDir == "" || opts.RecordingsDir == "" {
		return fmt.Errorf("docker driver: Lab, Orch, RepoDir, GitDir, RecordingsDir are all required")
	}

	idDir := d.idDir(id)
	if err := os.MkdirAll(idDir, 0o755); err != nil {
		return fmt.Errorf("docker: mkdir %s: %w", idDir, err)
	}
	if err := os.MkdirAll(opts.GitDir, 0o755); err != nil {
		return fmt.Errorf("docker: mkdir git dir %s: %w", opts.GitDir, err)
	}
	if err := os.MkdirAll(opts.RecordingsDir, 0o755); err != nil {
		return fmt.Errorf("docker: mkdir recordings dir %s: %w", opts.RecordingsDir, err)
	}

	// Keep created_at for direct driver callers, without overwriting the
	// timestamp initialized by the CLI before Create.
	if err := config.EnsureCreatedAt(d.stateDir, id); err != nil {
		return fmt.Errorf("docker: write created_at: %w", err)
	}

	image := d.image
	if image == "" {
		image = DefaultDockerImage
	}

	volName := labVolumeName(id)

	// 1. Create the named volume for /lab.
	if out, err := exec.Command("docker", "volume", "create", volName).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker volume create %s: %w\n%s", volName, err, out)
	}

	// 2. Start the container with the named volume mounted at /lab and the
	// same read-only/writable runtime mounts used by Lima.
	runArgs := dockerRunArgs(id, volName, opts, image)
	if out, err := exec.Command("docker", runArgs...).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker run: %w\n%s", err, out)
	}

	// 3. Install sudo — the lab scripts are written for Lima labs where sudo is
	// present. The ubuntu:24.04 base image ships without it, so we install it
	// here to keep all code paths identical between Docker and Lima.
	// On failure we do NOT clean up the container or volume: created_at was
	// written before docker run, so the lab is intentionally visible in
	// `taxiway list` as stopped/created. The user can run `taxiway rm` to clean up —
	// exactly as for any other partial-create failure.
	if out, err := exec.Command("docker", "exec", id, //nolint:gosec
		"apt-get", "update", "-qq").CombinedOutput(); err != nil {
		return fmt.Errorf("docker: apt-get update in %s: %w\n%s", id, err, out)
	}
	if out, err := exec.Command("docker", "exec", id, //nolint:gosec
		"apt-get", "install", "-y", "-qq", "sudo").CombinedOutput(); err != nil {
		return fmt.Errorf("docker: apt-get install sudo in %s: %w\n%s", id, err, out)
	}

	// 4. Create a Taxiway-owned non-root user and run lab commands as that
	// user. This keeps Docker behavior aligned with Lima and avoids tools such
	// as Claude Code refusing privileged execution.
	setupUserCmd := strings.Join([]string{
		"id -u " + dockerShellQuote(dockerLabUser) + " >/dev/null 2>&1 || useradd -m -s " + dockerShellQuote(dockerLabShell) + " " + dockerShellQuote(dockerLabUser),
		"usermod -aG sudo " + dockerShellQuote(dockerLabUser),
		"printf '%s\\n' '" + dockerLabUser + " ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/taxiway-" + dockerLabUser,
		"chmod 0440 /etc/sudoers.d/taxiway-" + dockerLabUser,
		"mkdir -p /lab/work /lab/git /lab/recordings",
		"chown -R " + dockerShellQuote(dockerLabUser+":"+dockerLabUser) + " /lab/work /lab/git /lab/recordings",
	}, " && ")
	if out, err := exec.Command("docker", "exec", id, "sh", "-c", setupUserCmd).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker: setup lab user in %s: %w\n%s", id, err, out)
	}

	// 5. Install container-environment shims so that bootstrap.sh and other
	// lab scripts run identically inside Docker containers and Lima labs.
	//
	// Shim A — systemctl no-op stub
	// ─────────────────────────────
	// `curl -fsSL https://get.docker.com | sh` (called by bootstrap.sh when
	// Docker is absent) invokes `systemctl enable --now docker.service` to
	// enable the daemon.  In a container there is no systemd init, so systemctl
	// exits non-zero and the install script fails.  We place a no-op stub at
	// /usr/local/bin/systemctl (higher priority than /bin/systemctl) that exits
	// 0 for every invocation.  The stub is intentionally installed by the
	// driver, not by bootstrap.sh, so the script itself is unchanged.
	//
	// The stub prints a notice to stderr so runs are observable:
	//   [docker-shim] systemctl <args> — no-op (no systemd in container)
	// The stub is written by piping its content to `tee` inside the container
	// so that no shell quoting or escaping of newlines is needed on the host.
	// Double-quoted echo expands $@ so the actual arguments appear in the log.
	systemctlStub := "#!/bin/sh\n" +
		"echo \"[docker-shim] systemctl $@ — no-op (no systemd in container)\" >&2\n" +
		"exit 0\n"
	teeCmd := exec.Command("docker", "exec", "-i", id, "tee", "/usr/local/bin/systemctl") //nolint:gosec
	teeCmd.Stdin = strings.NewReader(systemctlStub)
	if out, err := teeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker: write systemctl stub in %s: %w\n%s", id, err, out)
	}
	if out, err := exec.Command("docker", "exec", id, "chmod", "+x", "/usr/local/bin/systemctl").CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker: chmod systemctl stub in %s: %w\n%s", id, err, out)
	}

	return nil
}

func dockerRunArgs(id, volName string, opts CreateOptions, image string) []string {
	args := []string{
		"run", "-d",
		"--name", id,
		"--add-host", config.LabLiteLLMHost(opts.Lab) + ":host-gateway",
		"-e", "USER=" + dockerLabUser,
		"-e", "HOME=" + dockerLabHome,
		"-e", "SHELL=" + dockerLabShell,
		"-e", "LANG=C.UTF-8",
		"-e", "LC_ALL=C.UTF-8",
		"-e", "LC_CTYPE=C.UTF-8",
		"-e", "TERM=xterm-256color",
		"-v", volName + ":/lab",
		"-v", filepath.Join(opts.RepoDir, "infra") + ":/lab/infra:ro",
		"-v", filepath.Join(opts.RepoDir, "agents") + ":/lab/agents:ro",
		"-v", filepath.Join(opts.RepoDir, "orchestrators", opts.Orch) + ":/lab/orchestrators/" + opts.Orch + ":ro",
		"-v", opts.GitDir + ":/lab/git",
	}
	if opts.RecordingsDir != "" {
		args = append(args, "-v", opts.RecordingsDir+":/lab/recordings")
	}
	args = append(args, image, "sleep", "infinity")
	return args
}

// Start starts an existing (stopped) container.
func (d *DockerDriver) Start(_ context.Context, id string) error {
	if out, err := exec.Command("docker", "start", id).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker start %s: %w\n%s", id, err, out)
	}
	return nil
}

// Stop stops a running container (does not remove it).
func (d *DockerDriver) Stop(_ context.Context, id string) error {
	if out, err := exec.Command("docker", "stop", id).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker stop %s: %w\n%s", id, err, out)
	}
	return nil
}

// Delete force-removes the container, its named /lab volume, and the sidecar
// directory. Both the container and the volume may be absent (e.g. Create
// failed partway through); those cases are silently tolerated so that
// `taxiway rm` always succeeds for partially-created labs.
func (d *DockerDriver) Delete(_ context.Context, id string) error {
	// docker rm -f: tolerate "no such container" (partial create or already removed).
	if out, err := exec.Command("docker", "rm", "-f", id).CombinedOutput(); err != nil { //nolint:gosec
		if !strings.Contains(strings.ToLower(string(out)), "no such container") {
			return fmt.Errorf("docker rm -f %s: %w\n%s", id, err, out)
		}
	}
	// docker volume rm: tolerate "no such volume" (volume may not have been created).
	if out, err := exec.Command("docker", "volume", "rm", labVolumeName(id)).CombinedOutput(); err != nil { //nolint:gosec
		if !strings.Contains(strings.ToLower(string(out)), "no such volume") {
			return fmt.Errorf("docker volume rm %s: %w\n%s", labVolumeName(id), err, out)
		}
	}
	return os.RemoveAll(d.idDir(id))
}

// normaliseDockerState maps Docker container state strings to the Driver
// contract states (running | stopped | absent).
//
//	"running"  → "running"
//	"exited"   → "stopped"  (container ran and exited)
//	"created"  → "stopped"  (container was created but never started;
//	                         semantically equivalent to stopped for callers)
//	"paused"   → "paused"   (passed through; rare in practice)
//	anything else → passed through unchanged
func normaliseDockerState(raw string) string {
	switch raw {
	case "exited", "created":
		return "stopped"
	default:
		return raw
	}
}

// Status returns the current status of a container.
func (d *DockerDriver) Status(_ context.Context, id string) (Status, error) {
	out, err := exec.Command("docker", "inspect", //nolint:gosec
		"--format", "{{.State.Status}}",
		id,
	).Output()
	if err != nil {
		return Status{Name: id, State: "absent", Driver: "docker"}, nil
	}

	state := normaliseDockerState(strings.TrimSpace(string(out)))

	st := Status{Name: id, State: state, Driver: "docker"}
	if raw, err := os.ReadFile(filepath.Join(d.idDir(id), "created_at")); err == nil {
		st.Created, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	}
	return st, nil
}

// List returns all containers managed by this driver.
//
// Discovery strategy: scan stateDir/ for sidecar directories whose name
// starts with the "taxiway-" prefix (same guard as LimaDriver). The sidecar
// created_at file is the source-of-truth for lab existence: it is written at
// the very start of Create, so even a partially-created lab (docker run
// failed, docker cp failed, …) will appear in the list with state
// "stopped"/"absent". The user can then run `taxiway rm` to clean up — Delete
// tolerates absent containers and volumes.
func (d *DockerDriver) List(_ context.Context) ([]Status, error) {
	labsDir := d.stateDir
	entries, err := os.ReadDir(labsDir)
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
		// Require created_at sidecar: its presence is the signal that Create
		// completed successfully. Dirs without it are incomplete/orphaned creates.
		createdRaw, err := os.ReadFile(filepath.Join(labsDir, lab, "created_at"))
		if err != nil {
			continue // incomplete Create — skip silently
		}
		id := config.RuntimeIDOf(lab)                                                          // driver id used for docker inspect
		raw, _ := exec.Command("docker", "inspect", "--format={{.State.Status}}", id).Output() //nolint:gosec
		state := normaliseDockerState(strings.TrimSpace(string(raw)))
		if state == "" {
			state = "absent"
		}
		st := Status{Name: id, State: state, Driver: "docker"}
		st.Created, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(createdRaw)))
		out = append(out, st)
	}
	return out, nil
}

// Copy copies a file from the host into the container using docker cp.
func (d *DockerDriver) Copy(_ context.Context, id, srcHost, dstlab string) error {
	dest := id + ":" + dstlab
	if out, err := exec.Command("docker", "cp", srcHost, dest).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker cp %s %s: %w\n%s", srcHost, dest, err, out)
	}
	if out, err := exec.Command("docker", "exec", id, "chown", "-R", dockerLabUser+":"+dockerLabUser, dstlab).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker chown %s:%s %s:%s: %w\n%s", dockerLabUser, dockerLabUser, id, dstlab, err, out)
	}
	return nil
}

// Shell opens an interactive bash shell inside the container.
func (d *DockerDriver) Shell(_ context.Context, id, workdir string) error {
	if workdir == "" {
		workdir = "/lab"
	}
	args := append(dockerUserExecArgs(), "-it", "-w", workdir, id, "bash", "-l")
	cmd := exec.Command("docker", args...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ShellExec opens an interactive shell and executes shellCmd inside the container.
func (d *DockerDriver) ShellExec(_ context.Context, id, workdir, shellCmd string) error {
	if workdir == "" {
		workdir = "/lab"
	}
	args := append(dockerUserExecArgs(), "-it", "-w", workdir, id, "bash", "-c", shellCmd)
	cmd := exec.Command("docker", args...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *DockerDriver) InteractiveExec(ctx context.Context, id string, req InteractiveExecRequest) error {
	workdir := req.Workdir
	if workdir == "" {
		workdir = "/lab"
	}
	if len(req.Argv) == 0 {
		return fmt.Errorf("docker: InteractiveExec called with empty argv")
	}

	args := append(dockerUserExecArgs(), "-i")
	if isTerminal(os.Stdin) && isTerminal(os.Stdout) {
		args = append(args, "-t")
	}
	for k, v := range req.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, "-w", workdir, id)
	args = append(args, req.Argv...)

	//nolint:gosec
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

// Exec runs a command inside the container synchronously and returns the
// native exit code. stdout/stderr are written to req.Stdout/req.Stderr if set.
// If req.Events is set, stdout is parsed for LAB_AGENT_EVENT lines.
func (d *DockerDriver) Exec(ctx context.Context, id string, req ExecRequest) (ExecResult, error) {
	if len(req.Argv) == 0 {
		return ExecResult{}, fmt.Errorf("docker: Exec called with empty argv")
	}

	workdir := req.Workdir
	if workdir == "" {
		workdir = "/lab"
	}

	// docker exec -w <workdir> <id> <argv...>
	dockerArgs := append(dockerUserExecArgs(), "-w", workdir, id) //nolint:gocritic
	dockerArgs = append(dockerArgs, req.Argv...)

	//nolint:gosec
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)

	// Build environment: docker exec -e KEY=VALUE ...
	if len(req.Env) > 0 {
		envArgs := make([]string, 0, len(req.Env)*2+len(dockerArgs))
		envArgs = append(envArgs, dockerUserExecArgs()...)
		for k, v := range req.Env {
			envArgs = append(envArgs, "-e", k+"="+v)
		}
		envArgs = append(envArgs, "-w", workdir, id)
		envArgs = append(envArgs, req.Argv...)
		//nolint:gosec
		cmd = exec.CommandContext(ctx, "docker", envArgs...)
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
			} else if ctx.Err() != nil {
				return ExecResult{ExitCode: -1, Duration: dur}, fmt.Errorf("docker exec: %w", ErrExecTimeout)
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
		} else if ctx.Err() != nil {
			return ExecResult{ExitCode: -1, Duration: dur}, fmt.Errorf("docker exec: %w", ErrExecTimeout)
		} else {
			return ExecResult{}, runErr
		}
	}
	return ExecResult{ExitCode: code, Duration: dur}, nil
}

func dockerUserExecArgs() []string {
	return []string{
		"exec",
		"-u", dockerLabUser,
		"-e", "USER=" + dockerLabUser,
		"-e", "HOME=" + dockerLabHome,
		"-e", "SHELL=" + dockerLabShell,
	}
}

func dockerShellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// WriteLabRef writes the lab reference sidecar using the config package.
// The sidecar layout is identical to the LimaDriver and MockDriver.
func (d *DockerDriver) WriteLabRef(_ context.Context, id string, ref config.LabRef) error {
	return config.WriteLabRef(d.stateDir, id, ref)
}

// ReadLabRef reads the lab reference sidecar.
func (d *DockerDriver) ReadLabRef(_ context.Context, id string) (config.LabRef, bool, error) {
	return config.ReadLabRef(d.stateDir, id)
}
