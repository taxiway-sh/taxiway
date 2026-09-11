package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShellAttachHintUsesLabName(t *testing.T) {
	got := shellAttachHint("example-lab")
	if !strings.Contains(got, "taxiway shell example-lab") {
		t.Fatalf("hint = %q, want lab name example-lab", got)
	}
	if strings.Contains(got, "claude-code") || strings.Contains(got, "codex") || strings.Contains(got, "gastown") {
		t.Fatalf("hint must not use orchestrator name: %q", got)
	}
}

func TestPrintShellAttachHint(t *testing.T) {
	var buf bytes.Buffer
	printShellAttachHint(&buf, "demo-lab")
	if got := buf.String(); got != "  Attach with: taxiway shell demo-lab\n" {
		t.Fatalf("got %q", got)
	}
}

func TestUp_PrintsShellAttachHintWithLabName(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "example-lab", "--type", "gastown")
	require.NoError(t, err)

	out := stdout.String()
	require.Contains(t, out, "Attach with: taxiway shell example-lab")
	require.NotContains(t, out, "taxiway shell gastown")
	require.NotContains(t, out, "taxiway shell claude-code")
	require.NotContains(t, out, "taxiway shell codex")
}

func TestUp_PrintsShellAttachHintOnCachedStart(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "cached-lab", "--type", "gastown")
	require.NoError(t, err)

	stdout.Reset()
	stderr.Reset()
	_, _, err = execUpRoot(t, root, stdout, stderr, "up", "cached-lab", "--type", "gastown")
	require.NoError(t, err)

	out := stdout.String()
	require.Contains(t, out, "(cached)")
	require.Contains(t, out, "Attach with: taxiway shell cached-lab")
}

func TestUp_PrepareOnlyOmitsShellAttachHint(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "prep-lab", "--type", "gastown", "--prepare-only")
	require.NoError(t, err)

	require.NotContains(t, stdout.String(), "Attach with:")
}

func TestUp_DryRunOmitsShellAttachHint(t *testing.T) {
	root, _, _, stdout, stderr := buildUpTestRoot(t)

	_, _, err := execUpRoot(t, root, stdout, stderr, "up", "dry-lab", "--type", "gastown", "--dry-run")
	require.NoError(t, err)

	require.NotContains(t, stdout.String(), "Attach with:")
}
