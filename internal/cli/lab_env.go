package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/taxiway-sh/taxiway/internal/config"
	"github.com/taxiway-sh/taxiway/internal/driver"
)

// randomSuffix returns a short random hex string suitable for temp file names.
func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func readExistingLabEnvFile(ctx context.Context, d driver.Driver, ref config.LabRef) (string, error) {
	var stdout bytes.Buffer
	_, err := d.Exec(ctx, idName(ref.Lab), driver.ExecRequest{
		Argv:   []string{"/bin/sh", "-c", `cat "$HOME/.config/taxiway/env" 2>/dev/null || true`},
		Stdout: &stdout,
	})
	if err != nil {
		return "", fmt.Errorf("read existing env file failed: %w", err)
	}
	return stdout.String(), nil
}

func renderManagedEnvBlock(owner, scope string, env map[string]string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# >>> taxiway %s scope=%s\n", owner, scope)
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&out, "%s=%s\n", k, shellQuote(env[k]))
	}
	fmt.Fprintf(&out, "# <<< taxiway %s scope=%s\n", owner, scope)
	return out.String()
}

func upsertManagedEnvBlock(existing, owner, scope, block string) string {
	startMarker := fmt.Sprintf("# >>> taxiway %s scope=%s", owner, scope)
	endMarker := fmt.Sprintf("# <<< taxiway %s scope=%s", owner, scope)

	start := strings.Index(existing, startMarker)
	if start >= 0 {
		endRel := strings.Index(existing[start:], endMarker)
		if endRel >= 0 {
			end := start + endRel + len(endMarker)
			if end < len(existing) && existing[end] == '\n' {
				end++
			}
			return strings.TrimRight(existing[:start], "\n") + "\n\n" + strings.TrimRight(block, "\n") + "\n" + strings.TrimLeft(existing[end:], "\n")
		}
	}

	if strings.TrimSpace(existing) == "" {
		return strings.TrimRight(block, "\n") + "\n"
	}
	return strings.TrimRight(existing, "\n") + "\n\n" + strings.TrimRight(block, "\n") + "\n"
}

func writeAndCopyEnvContentWithLabel(ctx context.Context, d driver.Driver, ref config.LabRef, content string, valueCount int, label string) error {
	// Write tempfile with mode 0600.
	tmp, err := os.CreateTemp("", "taxiway-env-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}

	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	id := idName(ref.Lab)

	// Copy to a temp path in the lab (limactl copy does not expand ~).
	remoteTmp := "/tmp/taxiway-env-" + randomSuffix()
	if err := d.Copy(ctx, id, tmp.Name(), remoteTmp); err != nil {
		return fmt.Errorf("%s: copy to lab failed: %w", label, err)
	}

	// Move into place using the shell so $HOME is resolved server-side.
	// This also creates the parent directory and sets correct permissions.
	moveCmd := fmt.Sprintf(
		`mkdir -p "$HOME/.config/taxiway" && chmod 700 "$HOME/.config/taxiway" && mv %s "$HOME/.config/taxiway/env" && chmod 600 "$HOME/.config/taxiway/env"`,
		shellQuotePath(remoteTmp),
	)
	if _, err := d.Exec(ctx, id, driver.ExecRequest{
		Argv: []string{"/bin/sh", "-c", moveCmd},
	}); err != nil {
		return fmt.Errorf("%s: install env file failed: %w", label, err)
	}

	fmt.Fprintf(os.Stderr, "[%s] wrote %d value(s) into %s\n", label, valueCount, id)
	return nil
}

// shellQuote wraps v in single quotes, escaping embedded single quotes as '\”.
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// shellQuotePath wraps a path in single quotes, escaping embedded single quotes.
// Use this for lab paths in shell commands to safely handle spaces and metacharacters.
func shellQuotePath(p string) string {
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

func labPathShellExpr(p string) string {
	if strings.HasPrefix(p, "~/") {
		return `"$HOME/` + strings.TrimPrefix(p, "~/") + `"`
	}
	return shellQuotePath(p)
}
