package modelupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFixturesRequiresAllSixFiles(t *testing.T) {
	dir := t.TempDir()
	for name := range SourceURLs {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Load(context.Background(), http.DefaultClient, dir)
	if err != nil || string(s.Codex) != "codex" {
		t.Fatalf("%#v %v", s, err)
	}
	if err = os.Remove(filepath.Join(dir, "releases.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(context.Background(), http.DefaultClient, dir); err == nil {
		t.Fatal("missing fixture accepted")
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
