package recording

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPlayerManifestPathsRemainLocal(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("player JavaScript evaluation requires Node.js")
	}
	// Execute the actual player helpers, without DOM setup or network requests.
	cmd := exec.Command(node, "-e", `
const assert = require('node:assert/strict');
const fs = require('node:fs'), vm = require('node:vm');
const html = fs.readFileSync(0, 'utf8');
const scope = vm.createContext({});
vm.runInContext(html.slice(html.indexOf('function basename(path)'), html.indexOf('function labelForSession')), scope);
for (const session of [{cast_path_host:'https:example.invalid.cast',id:'safe'}, {id:'https://example.invalid'}, {id:'../outside'}, {cast_path_host:'%2e%2e.cast'}]) {
  assert.equal(scope.castFileForSession(session), '');
}
for (const session of [{cast_path_host:'/state/demo/recordings/safe_recording.cast'}, {id:'safe_recording'}]) {
  const file = scope.castFileForSession(session);
  assert.equal(file, 'safe_recording.cast');
  assert.equal(new URL(file, 'http://localhost:18735/').origin, 'http://localhost:18735');
}`)
	cmd.Stdin = bytes.NewBufferString(playerHTML)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestPlayerFilesystemRejectsOutsideSymlinks(t *testing.T) {
	store := NewStore(t.TempDir(), "demo")
	_, err := EnsurePlayer(store)
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "outside.cast")
	require.NoError(t, os.WriteFile(outside, []byte("harmless outside sentinel"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(store.Dir(), "unsafe.cast")))
	root, err := store.OpenRoot(false)
	require.NoError(t, err)
	defer root.Close()
	server := httptest.NewServer(http.FileServer(http.FS(PlayerFiles(root))))
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/unsafe.cast")
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NotEqual(t, http.StatusOK, response.StatusCode)
	require.NotContains(t, string(data), "harmless outside sentinel")
	require.NoError(t, os.Remove(filepath.Join(store.Dir(), "index.html")))
	require.NoError(t, os.Symlink(outside, filepath.Join(store.Dir(), "index.html")))
	_, err = EnsurePlayer(store)
	require.Error(t, err)
	data, err = os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "harmless outside sentinel", string(data))
}

func TestPlayerRejectsFIFOArtifactsWithoutBlocking(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo unavailable on this host")
	}
	store := NewStore(t.TempDir(), "demo")
	_, err = EnsurePlayer(store)
	require.NoError(t, err)
	fifo := filepath.Join(store.Dir(), "index.html")
	require.NoError(t, os.Remove(fifo))
	require.NoError(t, exec.Command(mkfifo, fifo).Run())
	finished := make(chan error, 1)
	go func() { _, err := EnsurePlayer(store); finished <- err }()
	select {
	case err := <-finished:
		require.ErrorContains(t, err, "regular file")
	case <-time.After(3 * time.Second):
		t.Fatal("FIFO blocked player generation")
	}
	root, err := store.OpenRoot(false)
	require.NoError(t, err)
	defer root.Close()
	go func() {
		f, err := PlayerFiles(root).Open("index.html")
		if f != nil {
			f.Close()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		require.ErrorContains(t, err, "regular file")
	case <-time.After(3 * time.Second):
		t.Fatal("FIFO blocked player serving")
	}
}

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
	require.NotContains(t, html, "https://")
	for _, asset := range []string{"asciinema-player.min.js", "asciinema-player.css", "LICENSE"} {
		data, err := os.ReadFile(filepath.Join(store.Dir(), "player", asset))
		require.NoError(t, err)
		require.NotEmpty(t, data)
	}
}
