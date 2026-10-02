package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandFailsWithoutModifyingCatalogOnIncompleteSources(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "models.yaml")
	before := []byte("models: []\n")
	if err := os.WriteFile(catalog, before, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"-catalog", catalog, "-fixtures", dir, "-write"}, &out)
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("unexpected error %v", err)
	}
	after, err := os.ReadFile(catalog)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("catalog modified after failure")
	}
}

func TestCommandRejectsInvalidDateBeforeFetching(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-as-of", "bad-date"}, &out); err == nil {
		t.Fatal("invalid date accepted")
	}
}
