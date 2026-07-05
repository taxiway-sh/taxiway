package recording

import (
	_ "embed"
	"html"
	"os"
	"path/filepath"
	"strings"
)

const playerFilename = "index.html"

// EnsurePlayer writes the browser-based recording player into the store dir.
func EnsurePlayer(store Store) (string, error) {
	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(store.Dir(), playerFilename)
	if err := os.WriteFile(path, []byte(playerHTMLForLab(store.Lab())), 0o644); err != nil {
		return "", err
	}
	return path, nil
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
