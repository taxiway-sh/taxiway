package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempEnvFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadMissingFile(t *testing.T) {
	result, err := Load(filepath.Join(t.TempDir(), "missing"))

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestLoadParsesShellLikeKeyValues(t *testing.T) {
	path := writeTempEnvFile(t, `
# comment
export TOKEN=secret123
QUOTED="value with spaces"
SINGLE='single quoted'
HASH=value#not-comment
COMMENTED=value   # comment
EMPTY=
INVALID_LINE
`)

	result, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"TOKEN":     "secret123",
		"QUOTED":    "value with spaces",
		"SINGLE":    "single quoted",
		"HASH":      "value#not-comment",
		"COMMENTED": "value",
	}, result)
}

func TestSortedKeys(t *testing.T) {
	assert.Equal(t, []string{"A", "B", "C"}, SortedKeys(map[string]string{
		"B": "2",
		"C": "3",
		"A": "1",
	}))
}

func TestExpandHostPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ExpandHostPath("~/.config/tool/credentials.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config/tool/credentials.json"), got)

	got, err = ExpandHostPath("$HOME/.config/tool/credentials.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config/tool/credentials.json"), got)

	got, err = ExpandHostPath("/absolute/path")
	require.NoError(t, err)
	assert.Equal(t, "/absolute/path", got)
}
