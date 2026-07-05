package recording

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsurePlayerWritesIndexHTML(t *testing.T) {
	store := NewStore(t.TempDir(), "demo")

	path, err := EnsurePlayer(store)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(store.Dir(), "index.html"), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	html := string(data)
	require.Contains(t, html, "<title>taxiway Recording Player - demo</title>")
	require.Contains(t, html, "<h1>taxiway Recording Player - demo</h1>")
	require.Contains(t, html, `fetch("recordings.json"`)
	require.Contains(t, html, "AsciinemaPlayer.create")
	require.Contains(t, html, "cast_path_host")
	require.Contains(t, html, "split(\"/\")")
	require.Contains(t, html, `fit: "width"`)
}
