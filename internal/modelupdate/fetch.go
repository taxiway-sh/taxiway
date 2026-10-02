package modelupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const maxSourceBytes = 8 << 20

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "taxiway-model-catalog-update")
	req.Header.Set("Accept", "application/json, text/markdown, text/plain")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxSourceBytes {
		return nil, fmt.Errorf("source size outside allowed bounds")
	}
	return body, nil
}

// Load performs unauthenticated public GETs, or reads a complete local fixture set.
// Fixture data uses the same parsers as online data, with no validation bypass.
func Load(ctx context.Context, client *http.Client, fixtureDir string) (Sources, error) {
	var s Sources
	for _, name := range []string{"codex", "chatgpt", "anthropic", "deprecations", "litellm", "releases"} {
		var data []byte
		var err error
		if fixtureDir != "" {
			data, err = os.ReadFile(filepath.Join(fixtureDir, name+".txt"))
			if len(data) == 0 || len(data) > maxSourceBytes {
				return Sources{}, fmt.Errorf("%s: fixture size outside allowed bounds", name)
			}
		} else {
			data, err = fetch(ctx, client, SourceURLs[name])
		}
		if err != nil {
			return Sources{}, fmt.Errorf("%s: %w", name, err)
		}
		if err = s.Set(name, data); err != nil {
			return Sources{}, err
		}
	}
	return s, nil
}
