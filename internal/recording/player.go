package recording

import (
	"embed"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const playerFilename = "index.html"

// EnsurePlayer writes the browser-based recording player into the store dir.
func EnsurePlayer(store Store) (string, error) {
	root, err := store.OpenRoot(true)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := root.MkdirAll("player", 0o755); err != nil {
		return "", err
	}
	for _, name := range []string{"asciinema-player.min.js", "asciinema-player.css", "LICENSE"} {
		data, err := playerAssets.ReadFile("player/" + name)
		if err != nil {
			return "", err
		}
		if err := writePlayerFile(root, "player/"+name, data); err != nil {
			return "", err
		}
	}
	path := filepath.Join(store.Dir(), playerFilename)
	if err := writePlayerFile(root, playerFilename, []byte(playerHTMLForLab(store.Lab()))); err != nil {
		return "", err
	}
	return path, nil
}

func writePlayerFile(root *os.Root, name string, data []byte) error {
	info, err := root.Lstat(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("recording: player artifact must be a regular file")
	}
	// Rename replaces the directory entry without opening an existing guest file.
	return writeAtomicFile(root, name, data)
}

type playerFiles struct{ root *os.Root }

// PlayerFiles exposes only contained directories and regular files, so guest
// symlinks cannot escape and guest FIFOs cannot block the local HTTP server.
func PlayerFiles(root *os.Root) fs.FS { return playerFiles{root: root} }

func (p playerFiles) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return openArtifact(p.root, name, true)
}

func playerHTMLForLab(lab string) string {
	title := "taxiway Recording Player"
	if lab != "" {
		title += " - " + lab
	}
	return strings.ReplaceAll(playerHTML, "{{TITLE}}", html.EscapeString(title))
}

//go:embed player.html
var playerHTML string

//go:embed player/asciinema-player.min.js player/asciinema-player.css player/LICENSE
var playerAssets embed.FS
