package modelupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLoadFetchesGatewayPinInsteadOfDevelopmentMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "cli", "lab_gateway.go"), []byte("image: litellm/litellm:1.79.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	s := fixtureSources()
	responses := map[string][]byte{CodexURL: s.Codex, ChatGPTURL: s.ChatGPT, AnthropicURL: s.Anthropic, DeprecationsURL: s.Deprecations, ReleasesURL: s.Releases, fmt.Sprintf(LiteLLMURL, "v1.79.0"): s.LiteLLM}
	client := &http.Client{Transport: sourceTransport(func(req *http.Request) (*http.Response, error) {
		data, ok := responses[req.URL.String()]
		if !ok {
			t.Fatalf("unexpected source %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}}, nil
	})}
	loaded, err := Load(context.Background(), client, "")
	if err != nil || loaded.PinnedVersion != "1.79.0" {
		t.Fatalf("%#v %v", loaded, err)
	}
}

func TestLoadFixturesRequiresAllSixFiles(t *testing.T) {
	dir := t.TempDir()
	sources := fixtureSources()
	for name, data := range map[string][]byte{"codex": sources.Codex, "chatgpt": sources.ChatGPT, "anthropic": sources.Anthropic, "deprecations": sources.Deprecations, "litellm": sources.LiteLLM, "releases": sources.Releases, "gateway": []byte("image: litellm/litellm:1.79.0\n")} {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Load(context.Background(), http.DefaultClient, dir)
	if err != nil || string(s.Codex) != string(sources.Codex) {
		t.Fatalf("%#v %v", s, err)
	}
	if err = os.Remove(filepath.Join(dir, "releases.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(context.Background(), http.DefaultClient, dir); err == nil {
		t.Fatal("missing fixture accepted")
	}
}

func TestLoadUsesPinnedMetadataAndFinalReleasesOnly(t *testing.T) {
	s := fixtureSources()
	s.LiteLLM = []byte(`{"gpt-6-sol":{"mode":"chat","litellm_provider":"openai"}}`)
	var releases []map[string]any
	for _, tag := range []string{"v1.82.0", "v1.80.0-dev", "v1.80.0-rc.1", "v1.80.0", "v1.81.0", "v1.79.0"} {
		releases = append(releases, map[string]any{"tag_name": tag, "html_url": "https://github.com/BerriAI/litellm/releases/tag/" + tag, "published_at": "2026-10-01T00:00:00Z"})
	}
	for _, flag := range []string{"draft", "prerelease"} {
		releases = append(releases, map[string]any{"tag_name": "v1.79.1", "html_url": "https://github.com/BerriAI/litellm/releases/tag/v1.79.1", "published_at": "2026-10-01T00:00:00Z", flag: true})
	}
	s.Releases, _ = json.Marshal(releases)
	dir := t.TempDir()
	for name, data := range map[string][]byte{"codex": s.Codex, "chatgpt": s.ChatGPT, "anthropic": s.Anthropic, "deprecations": s.Deprecations, "litellm": s.LiteLLM, "releases": s.Releases, "gateway": []byte("image: litellm/litellm:1.79.0\n"), "litellm-v1.80.0": s.LiteLLM, "litellm-v1.81.0": fixtureSources().LiteLLM} {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := Load(context.Background(), http.DefaultClient, dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Discover(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Candidates) != 1 || len(d.Waiting) != 1 || !strings.Contains(strings.Join(d.Notes, "\n"), "Earliest final release carrying matching metadata: v1.81.0") {
		t.Fatalf("%#v", d)
	}
	if len(loaded.ReleaseMetadata) != 2 {
		t.Fatal("unexpected release metadata sources")
	}
}

func TestLoadValidatesFutureReleaseMetadata(t *testing.T) {
	for _, test := range []struct {
		name      string
		metadata  string
		wantError bool
	}{
		{"invalid entry type", `{"bad":42,"claude-opus-5-5":{"mode":"chat","litellm_provider":"anthropic"}}`, true},
		{"invalid mode type", `{"claude-opus-5-5":{"mode":42,"litellm_provider":"anthropic"}}`, true},
		{"sample specification", `{"sample_spec":{},"claude-opus-5-5":{"mode":"chat","litellm_provider":"anthropic"}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.LiteLLM = []byte(`{"gpt-6-sol":{"mode":"chat","litellm_provider":"openai"}}`)
			dir := t.TempDir()
			for name, data := range map[string][]byte{"codex": s.Codex, "chatgpt": s.ChatGPT, "anthropic": s.Anthropic, "deprecations": s.Deprecations, "litellm": s.LiteLLM, "releases": s.Releases, "gateway": []byte("image: litellm/litellm:1.79.0\n"), "litellm-v1.80.0": []byte(test.metadata)} {
				if err := os.WriteFile(filepath.Join(dir, name+".txt"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := Load(context.Background(), http.DefaultClient, dir)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "litellm v1.80.0: invalid pricing schema") || loaded.PinnedVersion != "" {
					t.Fatalf("invalid source accepted or partial sources returned: %#v %v", loaded, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			d, err := Discover(loaded)
			if err != nil || !strings.Contains(strings.Join(d.Notes, "\n"), "Earliest final release carrying matching metadata: v1.80.0") {
				t.Fatalf("%#v %v", d, err)
			}
		})
	}
}

func TestFetchRejectsHTTPFailureAndOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	if _, err := fetch(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("503 accepted")
	}
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, maxSourceBytes+1)) }))
	defer server2.Close()
	if _, err := fetch(context.Background(), server2.Client(), server2.URL); err == nil {
		t.Fatal("oversized source accepted")
	}
}

func TestGitHubAuthorizationIsRestrictedToAPIHost(t *testing.T) {
	for _, url := range []string{"https://api.github.com/repos/BerriAI/litellm/releases", "https://raw.githubusercontent.com/file", "https://api.github.com.example.org/file", "http://api.github.com/file"} {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		setGitHubAuthorization(req, "test-placeholder")
		expected := url == "https://api.github.com/repos/BerriAI/litellm/releases"
		if (req.Header.Get("Authorization") != "") != expected {
			t.Fatal("authorization host scope violated")
		}
	}
}
