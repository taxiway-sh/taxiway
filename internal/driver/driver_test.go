package driver

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/taxiway-sh/taxiway/internal/event"
)

// ---- MockDriver tests ----

func newMock(t *testing.T) *MockDriver {
	t.Helper()
	return NewMockDriver(t.TempDir())
}

func TestMock_LifecycleRoundTrip(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-gastown"

	// Initially absent
	exists, err := m.Exists(ctx, id)
	require.NoError(t, err)
	require.False(t, exists)

	// Create
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	exists, err = m.Exists(ctx, id)
	require.NoError(t, err)
	require.True(t, exists)

	running, err := m.Running(ctx, id)
	require.NoError(t, err)
	require.True(t, running)

	st, err := m.Status(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "running", st.State)

	// Stop
	require.NoError(t, m.Stop(ctx, id))

	st, err = m.Status(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "stopped", st.State)

	// Start again
	require.NoError(t, m.Start(ctx, id))

	running, err = m.Running(ctx, id)
	require.NoError(t, err)
	require.True(t, running)

	// Delete
	require.NoError(t, m.Delete(ctx, id))

	exists, err = m.Exists(ctx, id)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestMock_Idempotent_Create(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-test"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))
	require.NoError(t, m.Create(ctx, id, CreateOptions{})) // second create: overwrites state to running
	st, err := m.Status(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "running", st.State)
}

func TestMock_List(t *testing.T) {
	t.Setenv("TAXIWAY_CONTEXT", "")
	t.Setenv("TAXIWAY_CONTEXT_ID", "")

	ctx := context.Background()
	m := newMock(t)
	require.NoError(t, m.Create(ctx, "taxiway-gastown", CreateOptions{}))
	require.NoError(t, m.Create(ctx, "taxiway-codex", CreateOptions{}))

	list, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	names := make(map[string]string)
	for _, s := range list {
		names[s.Name] = s.State
	}
	require.Equal(t, "running", names["taxiway-gastown"])
	require.Equal(t, "running", names["taxiway-codex"])
}

func TestMock_Exec_CapturesOutput(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-test"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	var buf bytes.Buffer
	result, err := m.Exec(ctx, id, ExecRequest{
		Argv:   []string{"bash", "-c", "echo hello"},
		Stdout: &buf,
	})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)
	require.Equal(t, "hello\n", buf.String())
}

func TestMock_Exec_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-test"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	result, err := m.Exec(ctx, id, ExecRequest{
		Argv: []string{"bash", "-c", "exit 42"},
	})
	require.NoError(t, err) // non-zero exit is not a Go error
	require.Equal(t, 42, result.ExitCode)
}

func TestMock_Exec_EventSink(t *testing.T) {
	ctx := context.Background()
	m := newMock(t)
	id := "taxiway-test"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	sink := &event.CollectorSink{}
	script := `echo 'LAB_AGENT_EVENT {"type":"phase","name":"test","status":"ok"}'`

	result, err := m.Exec(ctx, id, ExecRequest{
		Argv:   []string{"bash", "-c", script},
		Events: sink,
	})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)
	require.Len(t, sink.Events, 1)
	require.Equal(t, event.TypePhase, sink.Events[0].Type)
}

func TestMock_Exec_EventsJSONL(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	m := NewMockDriver(tmp)
	id := "taxiway-test"
	require.NoError(t, m.Create(ctx, id, CreateOptions{}))

	jsonlPath := EventsJSONLPath(tmp, id)
	sink, err := event.NewJSONLSink(jsonlPath)
	require.NoError(t, err)
	defer sink.Close()

	script := `echo 'LAB_AGENT_EVENT {"type":"phase","name":"install","status":"done"}'`
	_, err = m.Exec(ctx, id, ExecRequest{
		Argv:   []string{"bash", "-c", script},
		Events: sink,
	})
	require.NoError(t, err)
	sink.Close()

	data, err := os.ReadFile(jsonlPath)
	require.NoError(t, err)
	require.Contains(t, string(data), `"type":"phase"`)
}

// ---- DryRunDriver tests ----

func TestDryRun_WritesDoNotModifyState(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	inner := NewMockDriver(tmp)
	dr := NewDryRun(inner)
	id := "taxiway-test"

	// dry-run create — should not write state
	require.NoError(t, dr.Create(ctx, id, CreateOptions{}))
	exists, err := inner.Exists(ctx, id)
	require.NoError(t, err)
	require.False(t, exists, "dry-run must not create lab")

	// dry-run stop on non-existent — should not error
	require.NoError(t, dr.Stop(ctx, id))
	require.NoError(t, dr.Delete(ctx, id))
}

// TestDryRun_Shell_and_Exec verifies the dryRun decorator handles
// both shell and exec without panicking or modifying state.
func TestDryRun_Shell_and_Exec(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	inner := NewMockDriver(tmp)
	dr := NewDryRun(inner)
	id := "taxiway-test"

	// Shell dry-run: must not error
	require.NoError(t, dr.Shell(ctx, id, "/workdir"))

	// Exec dry-run: must not error and return empty result
	res, err := dr.Exec(ctx, id, ExecRequest{
		Argv: []string{"bash", "-c", "echo hi"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)

	// State still absent
	exists, err := inner.Exists(ctx, id)
	require.NoError(t, err)
	require.False(t, exists)
}
