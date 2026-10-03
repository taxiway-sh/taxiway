package driver

import (
	"context"

	"github.com/taxiway-sh/taxiway/internal/config"
)

// dryRunDriver wraps any Driver and suppresses write operations.
// The CLI renders the semantic operation plans.
// Read operations (Exists, Running, Status, List) pass through.
type dryRunDriver struct {
	inner Driver
}

// NewDryRun wraps d so that all write operations return nil without executing.
func NewDryRun(d Driver) Driver {
	if _, ok := d.(*dryRunDriver); ok {
		return d
	}
	return &dryRunDriver{inner: d}
}

func (dr *dryRunDriver) Name() string { return dr.inner.Name() }

func (dr *dryRunDriver) Exists(ctx context.Context, id string) (bool, error) {
	return dr.inner.Exists(ctx, id)
}

func (dr *dryRunDriver) Running(ctx context.Context, id string) (bool, error) {
	return dr.inner.Running(ctx, id)
}

func (dr *dryRunDriver) Create(_ context.Context, _ string, _ CreateOptions) error {
	return nil
}

func (dr *dryRunDriver) Start(_ context.Context, _ string) error {
	return nil
}

func (dr *dryRunDriver) Stop(_ context.Context, _ string) error {
	return nil
}

func (dr *dryRunDriver) Delete(_ context.Context, _ string) error {
	return nil
}

func (dr *dryRunDriver) Status(ctx context.Context, id string) (Status, error) {
	return dr.inner.Status(ctx, id)
}

func (dr *dryRunDriver) List(ctx context.Context) ([]Status, error) {
	return dr.inner.List(ctx)
}

func (dr *dryRunDriver) Copy(_ context.Context, _, _, _ string) error {
	return nil
}

func (dr *dryRunDriver) WriteLabRef(_ context.Context, _ string, _ config.LabRef) error {
	return nil
}

func (dr *dryRunDriver) ReadLabRef(ctx context.Context, id string) (config.LabRef, bool, error) {
	return dr.inner.ReadLabRef(ctx, id)
}

func (dr *dryRunDriver) Shell(_ context.Context, _, _ string) error {
	return nil
}

func (dr *dryRunDriver) ShellExec(_ context.Context, _, _, _ string) error {
	return nil
}

func (dr *dryRunDriver) InteractiveExec(_ context.Context, _ string, _ InteractiveExecRequest) error {
	return nil
}

func (dr *dryRunDriver) Exec(ctx context.Context, id string, req ExecRequest) (ExecResult, error) {
	if req.Inspect {
		return dr.inner.Exec(ctx, id, req)
	}
	return ExecResult{}, nil
}
