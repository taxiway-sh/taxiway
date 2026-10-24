package modelupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxSourceBytes = 8 << 20

func setGitHubAuthorization(req *http.Request, token string) {
	if token != "" && req.URL.Scheme == "https" && req.URL.Host == "api.github.com" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "taxiway-model-catalog-update")
	req.Header.Set("Accept", "application/json, text/markdown, text/plain")
	setGitHubAuthorization(req, os.Getenv("GITHUB_TOKEN"))
	// Never forward the optional GitHub credential to a redirected download host.
	scopedClient := *client
	scopedClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next.URL.Scheme != "https" || next.URL.Host != "api.github.com" {
			next.Header.Del("Authorization")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(next, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many source redirects")
		}
		return nil
	}
	res, err := scopedClient.Do(req)
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

// Load performs public GETs (optionally authenticated for GitHub rate limits), or reads a complete local fixture set.
// Fixture data uses the same parsers as online data, with no validation bypass.
func Load(ctx context.Context, client *http.Client, fixtureDir string) (Sources, error) {
	var s Sources
	read := func(name, url string) ([]byte, error) {
		if fixtureDir == "" {
			return fetch(ctx, client, url)
		}
		data, err := os.ReadFile(filepath.Join(fixtureDir, name+".txt"))
		if err != nil {
			return nil, err
		}
		if len(data) == 0 || len(data) > maxSourceBytes {
			return nil, fmt.Errorf("%s: fixture size outside allowed bounds", name)
		}
		return data, nil
	}
	for _, name := range []string{"codex", "chatgpt", "anthropic", "deprecations", "litellm", "releases"} {
		var data []byte
		var err error
		if name == "litellm" {
			var gateway []byte
			if fixtureDir != "" {
				gateway, err = read("gateway", "")
			} else {
				gateway, err = os.ReadFile("internal/cli/lab_gateway.go")
			}
			if err == nil {
				s.PinnedVersion, err = pinnedVersion(gateway)
			}
			if err != nil {
				return Sources{}, fmt.Errorf("litellm: %w", err)
			}
			data, err = read(name, fmt.Sprintf(LiteLLMURL, "v"+s.PinnedVersion))
		} else if fixtureDir != "" {
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
	// Paginate the release inventory before looking for the earliest final
	// release. GitHub orders by publication, not semantic version.
	if fixtureDir == "" {
		var all []json.RawMessage
		if err := json.Unmarshal(s.Releases, &all); err != nil {
			return Sources{}, err
		}
		for page := 2; ; page++ {
			if page > 100 {
				return Sources{}, fmt.Errorf("litellm release inventory exceeds page limit")
			}
			if len(all) < (page-1)*100 {
				break
			}
			data, err := fetch(ctx, client, ReleasesURL+"&page="+fmt.Sprint(page))
			if err != nil {
				return Sources{}, err
			}
			var next []json.RawMessage
			if err := json.Unmarshal(data, &next); err != nil {
				return Sources{}, err
			}
			all = append(all, next...)
		}
		s.Releases, _ = json.Marshal(all)
	}
	d, err := Discover(s)
	if err != nil {
		return Sources{}, err
	}
	if len(d.Waiting) == 0 {
		return s, nil
	}
	var releases []release
	if err := json.Unmarshal(s.Releases, &releases); err != nil {
		return Sources{}, err
	}
	s.ReleaseMetadata = map[string][]byte{}
	for _, r := range releases {
		if !r.Draft && !r.Prerelease && strings.HasPrefix(r.Tag, "v") && finalVersion.MatchString(strings.TrimPrefix(r.Tag, "v")) && versionLess(s.PinnedVersion, r.Tag) {
			s.ReleaseMetadata[r.Tag] = nil
		}
	}
	waiting := d.Waiting
	for _, tag := range sortedFinalReleases(s.ReleaseMetadata) {
		data, err := read("litellm-"+tag, fmt.Sprintf(LiteLLMURL, tag))
		if err != nil {
			return Sources{}, fmt.Errorf("litellm %s: %w", tag, err)
		}
		var prices map[string]json.RawMessage
		if err := json.Unmarshal(data, &prices); err != nil || len(prices) == 0 {
			return Sources{}, fmt.Errorf("litellm %s: invalid pricing schema", tag)
		}
		s.ReleaseMetadata[tag] = data
		remaining := []Candidate{}
		for _, candidate := range waiting {
			if !matchingMetadata(data, candidate.Name, candidate.Provider) {
				remaining = append(remaining, candidate)
			}
		}
		waiting = remaining
		if len(waiting) == 0 {
			break
		}
	}
	for tag, data := range s.ReleaseMetadata {
		if data == nil {
			delete(s.ReleaseMetadata, tag)
		}
	}
	return s, nil
}
