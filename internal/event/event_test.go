package event

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse_Valid(t *testing.T) {
	line := `LAB_AGENT_EVENT {"type":"phase","name":"install-go","status":"started"}`
	ev, ok := Parse(line)
	require.True(t, ok)
	require.Equal(t, TypePhase, ev.Type)
	require.Equal(t, "install-go", ev.Fields["name"])
	require.Equal(t, "started", ev.Fields["status"])
}

func TestParse_UnknownType(t *testing.T) {
	line := `LAB_AGENT_EVENT {"type":"custom_future","foo":"bar"}`
	ev, ok := Parse(line)
	require.True(t, ok, "unknown type must not fail")
	require.Equal(t, Type("custom_future"), ev.Type)
	require.Equal(t, "bar", ev.Fields["foo"])
}

func TestParse_NotAnEvent(t *testing.T) {
	lines := []string{
		"",
		"regular output line",
		"LAB_AGENT_EVENTnospace",
		"LAB_AGENT_EVENT not-json",
		"LAB_AGENT_EVENT ",
	}
	for _, l := range lines {
		_, ok := Parse(l)
		require.False(t, ok, "expected not-event for: %q", l)
	}
}

func TestParse_ExitEvent(t *testing.T) {
	line := `LAB_AGENT_EVENT {"type":"exit","code":0,"duration_ms":1234}`
	ev, ok := Parse(line)
	require.True(t, ok)
	require.Equal(t, TypeExit, ev.Type)
}

func TestJSONLSink(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "events.jsonl")

	sink, err := NewJSONLSink(path)
	require.NoError(t, err)

	ctx := context.Background()
	ev1, _ := Parse(`LAB_AGENT_EVENT {"type":"phase","name":"test","status":"ok"}`)
	ev2, _ := Parse(`LAB_AGENT_EVENT {"type":"exit","code":0}`)

	require.NoError(t, sink.Handle(ctx, ev1))
	require.NoError(t, sink.Handle(ctx, ev2))
	require.NoError(t, sink.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	require.Contains(t, content, `"type":"phase"`)
	require.Contains(t, content, `"type":"exit"`)
}

func TestCollectorSink(t *testing.T) {
	sink := &CollectorSink{}
	ctx := context.Background()
	ev, _ := Parse(`LAB_AGENT_EVENT {"type":"log","msg":"hello"}`)
	require.NoError(t, sink.Handle(ctx, ev))
	require.Len(t, sink.Events, 1)
	require.Equal(t, TypeLog, sink.Events[0].Type)
}

func TestMultiSink(t *testing.T) {
	c1, c2 := &CollectorSink{}, &CollectorSink{}
	multi := NewMultiSink(c1, c2)
	ctx := context.Background()
	ev, _ := Parse(`LAB_AGENT_EVENT {"type":"metric","name":"dur","value":42}`)
	require.NoError(t, multi.Handle(ctx, ev))
	require.Len(t, c1.Events, 1)
	require.Len(t, c2.Events, 1)
}
