package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// dryRunDriver wraps any Driver and prints write operations instead of executing them.
// Read operations (Exists, Running, Status, List) pass through.
type dryRunDriver struct {
	inner Driver
}

// NewDryRun wraps d so that all write operations print a description and return nil.
func NewDryRun(d Driver) Driver {
	return &dryRunDriver{inner: d}
}

func (dr *dryRunDriver) Name() string { return dr.inner.Name() + "+dryrun" }

func (dr *dryRunDriver) Exists(ctx context.Context, id string) (bool, error) {
	return dr.inner.Exists(ctx, id)
}

func (dr *dryRunDriver) Running(ctx context.Context, id string) (bool, error) {
	return dr.inner.Running(ctx, id)
}

func (dr *dryRunDriver) Create(_ context.Context, id string, opts CreateOptions) error {
	fmt.Printf("[dry-run] %s: Create id=%s orch=%s\n", dr.inner.Name(), id, opts.Orch)
	return nil
}

func (dr *dryRunDriver) Start(_ context.Context, id string) error {
	fmt.Printf("[dry-run] %s: Start id=%s\n", dr.inner.Name(), id)
	return nil
}

func (dr *dryRunDriver) Stop(_ context.Context, id string) error {
	fmt.Printf("[dry-run] %s: Stop id=%s\n", dr.inner.Name(), id)
	return nil
}

func (dr *dryRunDriver) Delete(_ context.Context, id string) error {
	fmt.Printf("[dry-run] %s: Delete id=%s\n", dr.inner.Name(), id)
	return nil
}

func (dr *dryRunDriver) Status(ctx context.Context, id string) (Status, error) {
	return dr.inner.Status(ctx, id)
}

func (dr *dryRunDriver) List(ctx context.Context) ([]Status, error) {
	return dr.inner.List(ctx)
}

func (dr *dryRunDriver) Copy(_ context.Context, id, srcHost, dstlab string) error {
	fmt.Printf("[dry-run] %s: Copy id=%s src=%s dst=%s\n", dr.inner.Name(), id, srcHost, dstlab)
	return nil
}

func (dr *dryRunDriver) WriteLabRef(_ context.Context, id string, ref config.LabRef) error {
	if ref.Workspace != nil {
		fmt.Printf("[dry-run] %s: WriteLabRef id=%s lab=%s orch=%s repo=%s ref=%s path=%s\n",
			dr.inner.Name(), id, ref.Lab, ref.Orch,
			ref.Workspace.Repo, ref.Workspace.Ref, ref.Workspace.Path)
	} else {
		fmt.Printf("[dry-run] %s: WriteLabRef id=%s lab=%s orch=%s\n", dr.inner.Name(), id, ref.Lab, ref.Orch)
	}
	return nil
}

func (dr *dryRunDriver) ReadLabRef(ctx context.Context, id string) (config.LabRef, bool, error) {
	return dr.inner.ReadLabRef(ctx, id)
}

func (dr *dryRunDriver) Shell(_ context.Context, id, workdir string) error {
	fmt.Printf("[dry-run] %s: Shell id=%s workdir=%s\n", dr.inner.Name(), id, workdir)
	return nil
}

func (dr *dryRunDriver) ShellExec(_ context.Context, id, workdir, shellCmd string) error {
	fmt.Printf("[dry-run] %s: ShellExec id=%s workdir=%s cmd=%s\n", dr.inner.Name(), id, workdir, shellCmd)
	return nil
}

func (dr *dryRunDriver) InteractiveExec(_ context.Context, id string, req InteractiveExecRequest) error {
	fmt.Printf("[dry-run] %s: InteractiveExec id=%s workdir=%s cmd=%s\n",
		dr.inner.Name(), id, req.Workdir, strings.Join(req.Argv, " "))
	return nil
}

func (dr *dryRunDriver) Exec(_ context.Context, id string, req ExecRequest) (ExecResult, error) {
	fmt.Printf("[dry-run] %s: Exec id=%s cmd=%s\n", dr.inner.Name(), id, strings.Join(req.Argv, " "))
	return ExecResult{}, nil
}
