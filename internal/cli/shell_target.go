package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

type shellTarget struct {
	Workdir       string
	Command       string
	SessionName   string
	RequiresTmux  bool
	AttachCommand string
}

func resolveShellTarget(ctx context.Context, state *RootState, ref config.LabRef) (shellTarget, error) {
	orchName := ref.Orch
	if orchName == "" {
		orchName = ref.Lab
	}

	manifest, err := config.LoadOrchManifest(state.RepoDir, orchName)
	if err != nil {
		return shellTarget{}, fmt.Errorf("shell: loading manifest for %s: %w", orchName, err)
	}

	if manifest != nil && manifest.Shell != nil && len(manifest.Shell.Command) > 0 {
		quoted := make([]string, len(manifest.Shell.Command))
		for i, tok := range manifest.Shell.Command {
			quoted[i] = shellQuote(tok)
		}
		cmd := strings.Join(quoted, " ")
		return shellTarget{
			Workdir:       LabRepoRoot,
			Command:       "exec " + cmd,
			AttachCommand: strings.Join(manifest.Shell.Command, " "),
		}, nil
	}

	if err := ensureTmuxSession(ctx, state, ref, orchName); err != nil {
		return shellTarget{}, err
	}

	attach := "tmux attach-session -t " + orchName
	resize := fmt.Sprintf(
		"if size=$(stty size 2>/dev/null); then set -- $size; if [ $# -eq 2 ]; then tmux resize-window -t %s -y \"$1\" -x \"$2\" 2>/dev/null || true; tmux list-sessions -F '#{session_name}' 2>/dev/null | while IFS= read -r record_session; do case \"$record_session\" in taxiway-record-*) tmux resize-window -t \"$record_session\" -y \"$1\" -x \"$2\" 2>/dev/null || true ;; esac; done; fi; fi",
		orchName,
	)
	drain := fmt.Sprintf(
		"dd if=/dev/tty of=/dev/null bs=64 count=1 iflag=nonblock 2>/dev/null; %s; exec %s",
		resize, attach,
	)
	return shellTarget{
		Workdir:       LabRepoRoot,
		Command:       drain,
		SessionName:   orchName,
		RequiresTmux:  true,
		AttachCommand: attach,
	}, nil
}

func ensureTmuxSession(ctx context.Context, state *RootState, ref config.LabRef, session string) error {
	id := idName(ref.Lab)
	d, err := driverForRef(state, ref)
	if err != nil {
		return err
	}
	res, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"tmux", "has-session", "-t", session},
	})
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf(
			"lab %q shell session %q is not running\n\nThe tmux session may have been closed, for example after exiting the agent or pressing Ctrl-C.\n\nRestart it with:\ttaxiway up %s --from start --force\nDiagnose with:\ttaxiway doctor %s",
			ref.Lab, session, ref.Lab, ref.Lab,
		)
	}
	return nil
}
