package recording

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreRoundTripAndLatestActive(t *testing.T) {
	store := NewStore(t.TempDir(), "demo")
	first := Session{ID: "one", Name: "one", State: StateRecording, StartedAt: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)}
	second := Session{ID: "two", Name: "two", State: StateRecording, StartedAt: time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)}

	require.NoError(t, store.Save(Index{Sessions: []Session{second, first}}))
	got, err := store.Load()
	require.NoError(t, err)
	require.Len(t, got.Sessions, 2)
	require.Equal(t, "one", got.Sessions[0].ID)
	latest, ok := got.LatestActive()
	require.True(t, ok)
	require.Equal(t, "two", latest.ID)
}

func TestNewIDValidatesName(t *testing.T) {
	_, err := NewID(time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), "../bad")
	require.Error(t, err)

	id, err := NewID(time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), "demo")
	require.NoError(t, err)
	require.Equal(t, "20260520-100000-demo", id)
}

func TestStoreDir(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root, "demo")
	require.Equal(t, "demo", store.Lab())
	require.Equal(t, filepath.Join(root, "demo", "recordings"), store.Dir())
}

func TestIndexRemoveByName(t *testing.T) {
	idx := Index{Sessions: []Session{
		{ID: "one", Name: "one", State: StateStopped},
		{ID: "two", Name: "two", State: StateStopped},
	}}

	session, ok := idx.RemoveByName("one")
	require.True(t, ok)
	require.Equal(t, "one", session.ID)
	require.Len(t, idx.Sessions, 1)
	require.Equal(t, "two", idx.Sessions[0].ID)
}

func TestIndexRemoveByNameRemovesFirstMatchingName(t *testing.T) {
	idx := Index{Sessions: []Session{
		{ID: "first", Name: "demo", State: StateStopped},
		{ID: "second", Name: "demo", State: StateStopped},
	}}

	session, ok := idx.RemoveByName("demo")
	require.True(t, ok)
	require.Equal(t, "first", session.ID)
	require.Len(t, idx.Sessions, 1)
	require.Equal(t, "second", idx.Sessions[0].ID)
}
