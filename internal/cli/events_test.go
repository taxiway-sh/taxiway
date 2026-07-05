package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/taxiway-sh/taxiway/internal/event"
)

func TestMakeExecSinkCreatesJSONLSink(t *testing.T) {
	sink, closeSink, err := makeExecSink(t.TempDir() + "/events.jsonl")

	require.NoError(t, err)
	require.NotNil(t, closeSink)
	require.IsType(t, &event.JSONLSink{}, sink)
	closeSink()
}

func TestMakeExecSinkReturnsDiscardSinkOnInvalidPath(t *testing.T) {
	sink, closeSink, err := makeExecSink(t.TempDir())

	require.Error(t, err)
	require.NotNil(t, closeSink)
	require.IsType(t, event.DiscardSink{}, sink)
	closeSink()
}
