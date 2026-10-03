package modelcatalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetirementBoundary(t *testing.T) {
	model := Model{Name: "old", Status: "deprecated", RetirementDate: "2026-10-14", Replacement: "new"}
	boundary := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	require.Equal(t, "deprecated", model.StatusAt(boundary.Add(-time.Nanosecond)))
	require.NoError(t, model.SelectionError(boundary.Add(-time.Nanosecond)))
	require.Equal(t, "retired", model.StatusAt(boundary))
	require.ErrorContains(t, model.SelectionError(boundary), "new")
}

func TestParseDefaultsChecksStructureWithoutCurrentTime(t *testing.T) {
	data := []byte("defaults: {anthropic: {sonnet: old}}\nmodels: [{name: old, provider: anthropic, upstream: old, status: deprecated, retirement_date: '2000-01-01', replacement: new}]")
	catalog, err := Parse(data)
	require.NoError(t, err)
	require.ErrorContains(t, catalog.Models[0].SelectionError(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), "new")
}
