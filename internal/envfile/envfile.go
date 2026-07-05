package envfile

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Load reads a KEY=value env file and returns a KEY->VALUE map. A missing file
// is treated as an empty env file.
func Load(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := line[idx+1:]

		if i := strings.Index(val, " #"); i >= 0 {
			val = val[:i]
		}
		if i := strings.Index(val, "\t#"); i >= 0 {
			val = val[:i]
		}

		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '\'' && val[len(val)-1] == '\'') ||
				(val[0] == '"' && val[len(val)-1] == '"') {
				val = val[1 : len(val)-1]
			}
		}
		if val == "" {
			continue
		}
		result[key] = val
	}
	return result, scanner.Err()
}

// ExpandHostPath expands a leading ~ or $HOME prefix using os.UserHomeDir.
// Absolute paths are returned unchanged.
func ExpandHostPath(p string) (string, error) {
	if p == "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p, fmt.Errorf("envfile: cannot determine home directory: %w", err)
	}
	if p == "~" {
		return home, nil
	}
	if strings.HasPrefix(p, "~/") {
		return home + p[1:], nil
	}
	if p == "$HOME" {
		return home, nil
	}
	if strings.HasPrefix(p, "$HOME/") {
		return home + p[5:], nil
	}
	return p, nil
}

// SortedKeys returns the map keys sorted alphabetically.
func SortedKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
