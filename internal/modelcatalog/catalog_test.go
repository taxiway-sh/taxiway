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
