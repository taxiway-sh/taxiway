package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
)

type dryRunPlan struct {
	out      io.Writer
	finished bool
}

func newDryRunPlan(out io.Writer, kind, name, lab string) *dryRunPlan {
	fmt.Fprintf(out, "Dry-run for %s %q on lab %q\n", kind, name, lab)
	return &dryRunPlan{out: out}
}

func (p *dryRunPlan) Step(component, label string) {
	fmt.Fprintf(p.out, "\n[%s] %s\n", component, label)
}

func (p *dryRunPlan) Detail(detail string) {
	fmt.Fprintf(p.out, "  %s\n", detail)
}

func (p *dryRunPlan) Finish() {
	if p.finished {
		return
	}
	p.finished = true
	fmt.Fprintln(p.out, "\nNo changes were made.")
}

func execOfflinePlanScript(ctx context.Context, scriptPath string, stdout, stderr io.Writer, env map[string]string) error {
	execEnv := append([]string(nil), os.Environ()...)
	execEnv = append(execEnv,
		"TAXIWAY_EXECUTION_MODE=plan",
		"TAXIWAY_PLAN_INSPECTION=unavailable",
		"HOME=/nonexistent",
	)

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		execEnv = append(execEnv, key+"="+env[key])
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath) //nolint:gosec // scriptPath is resolved from the trusted repository tree.
	cmd.Env = execEnv
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
