package recording

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreRejectsUntrustedArtifactPaths(t *testing.T) {
	for _, mode := range []string{"outside", "traversal", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root, "demo")
			require.NoError(t, os.MkdirAll(store.Dir(), 0700))
			outside := filepath.Join(root, "outside.cast")
			require.NoError(t, os.WriteFile(outside, []byte("harmless sentinel"), 0600))
			path := outside
			if mode == "traversal" {
				path = store.Dir() + "/../..//outside.cast"
			}
			if mode == "symlink" {
				path = filepath.Join(store.Dir(), "safe.cast")
				require.NoError(t, os.Symlink(outside, path))
			}
			data, err := json.Marshal(Index{Sessions: []Session{{ID: "safe", Name: "safe", State: StateStopped, CastPathHost: path}}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(store.path, data, 0600))
			_, err = store.Load()
			require.Error(t, err)
			require.FileExists(t, outside)
		})
	}
}

func TestStoreCastOperationsRemainContainedAfterIndexLoad(t *testing.T) {
	store := NewStore(t.TempDir(), "demo")
	cast := filepath.Join(store.Dir(), "safe.cast")
	session := Session{ID: "safe", Name: "safe", State: StateStopped, CastPathHost: cast}
	require.NoError(t, store.Save(Index{Sessions: []Session{session}}))
	require.NoError(t, os.WriteFile(cast, []byte("inside evidence"), 0600))
	idx, err := store.Load()
	require.NoError(t, err)
	data, err := store.ReadCast(idx.Sessions[0])
	require.NoError(t, err)
	require.Equal(t, "inside evidence", string(data))
	outside := filepath.Join(t.TempDir(), "sentinel.cast")
	require.NoError(t, os.WriteFile(outside, []byte("outside evidence"), 0600))
	require.NoError(t, os.Remove(cast))
	require.NoError(t, os.Symlink(outside, cast))
	_, err = store.ReadCast(idx.Sessions[0])
	require.Error(t, err)
	require.NoError(t, store.RemoveCast(idx.Sessions[0]))
	data, err = os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "outside evidence", string(data))
}

func TestStoreIndexAndDirectorySymlinks(t *testing.T) {
	store := NewStore(t.TempDir(), "demo")
	require.NoError(t, store.Save(Index{}))
	outside := filepath.Join(t.TempDir(), "sentinel.json")
	require.NoError(t, os.WriteFile(outside, []byte("outside evidence"), 0600))
	require.NoError(t, os.Remove(store.path))
	require.NoError(t, os.Symlink(outside, store.path))
	_, err := store.Load()
	require.Error(t, err)
	// Atomic index replacement replaces the link itself, never its target.
	require.NoError(t, store.Save(Index{}))
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "outside evidence", string(data))
	require.NoError(t, os.RemoveAll(store.Dir()))
	require.NoError(t, os.Symlink(filepath.Dir(outside), store.Dir()))
	_, err = store.Load()
	require.Error(t, err)
	require.Error(t, store.Save(Index{}))
}

func TestStoreRejectsFIFOArtifactsWithoutBlocking(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo unavailable on this host")
	}
	store := NewStore(t.TempDir(), "demo")
	cast := filepath.Join(store.Dir(), "safe.cast")
	session := Session{ID: "safe", Name: "safe", State: StateStopped, CastPathHost: cast}
	require.NoError(t, store.Save(Index{Sessions: []Session{session}}))
	for _, path := range []string{cast, store.path} {
		if path == store.path {
			require.NoError(t, os.Remove(path))
		}
		require.NoError(t, exec.Command(mkfifo, path).Run())
		finished := make(chan error, 1)
		go func() {
			if path == cast {
				_, err := store.ReadCast(session)
				finished <- err
			} else {
				_, err := store.Load()
				finished <- err
			}
		}()
		select {
		case err := <-finished:
			require.ErrorContains(t, err, "regular file")
		case <-time.After(time.Second):
			t.Fatal("FIFO artifact blocked the host")
		}
	}
}

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
	require.True(t, strings.HasPrefix(id, "20260520-100000-demo-"))
	require.NoError(t, ValidateName(id))
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

func TestNewIDDistinctStartsWithinOneSecond(t *testing.T) {
	now := time.Date(2026, 10, 4, 11, 0, 0, 1, time.UTC)
	first, err := NewID(now, "demo")
	require.NoError(t, err)
	second, err := NewID(now, "demo")
	require.NoError(t, err)
	third, err := NewID(now.Add(time.Millisecond), "demo")
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.NotEqual(t, first, third)
	require.NoError(t, ValidateName(first))
}
